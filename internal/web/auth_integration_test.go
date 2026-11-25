// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noschvie/pmss-api-tester/internal/config"
)

const (
	testOIDCClientID     = "tester-client"
	testOIDCClientSecret = "tester-secret"
	testPMSSAccessToken  = "pmss-test-token"
)

type mockOIDCProvider struct {
	server *httptest.Server
	issuer string
	key    *rsa.PrivateKey

	mu            sync.Mutex
	nonce         string
	exchangeScope string
	rejectCode    bool
}

func newMockOIDCProvider(t *testing.T) *mockOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	provider := &mockOIDCProvider{key: key, exchangeScope: "read"}
	provider.server = httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	provider.issuer = provider.server.URL + "/realms/test"
	t.Cleanup(provider.server.Close)
	return provider
}

func (p *mockOIDCProvider) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/realms/test/.well-known/openid-configuration":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                p.issuer,
			"authorization_endpoint":                p.server.URL + "/authorize",
			"token_endpoint":                        p.server.URL + "/realms/test/token",
			"jwks_uri":                              p.server.URL + "/realms/test/jwks",
			"end_session_endpoint":                  p.server.URL + "/realms/test/logout",
			"response_types_supported":              []string{"code"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	case "/realms/test/jwks":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{
			"kty": "RSA", "use": "sig", "kid": "test-key", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(p.key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
		}}})
	case "/realms/test/token":
		p.serveToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (p *mockOIDCProvider) serveToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		p.mu.Lock()
		nonce, reject := p.nonce, p.rejectCode
		p.mu.Unlock()
		if reject || r.Form.Get("code") == "reject" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"test authorization code rejected"}`)
			return
		}
		idToken, err := p.signedIDToken(nonce)
		if err != nil {
			http.Error(w, "could not sign ID token", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "user-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"id_token":     idToken,
		})
	case "urn:ietf:params:oauth:grant-type:token-exchange":
		p.mu.Lock()
		scope := p.exchangeScope
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": testPMSSAccessToken,
			"token_type":   "Bearer",
			"expires_in":   1800,
			"scope":        scope,
		})
	default:
		http.Error(w, "unsupported grant", http.StatusBadRequest)
	}
}

func (p *mockOIDCProvider) signedIDToken(nonce string) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims, err := json.Marshal(map[string]any{
		"iss": p.issuer, "sub": "alice-id", "aud": testOIDCClientID,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
		"nonce": nonce, "preferred_username": "alice",
	})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (p *mockOIDCProvider) setExchangeScope(scope string) {
	p.mu.Lock()
	p.exchangeScope = scope
	p.mu.Unlock()
}

func (p *mockOIDCProvider) setRejectCode(reject bool) {
	p.mu.Lock()
	p.rejectCode = reject
	p.mu.Unlock()
}

type targetRequest struct {
	method        string
	path          string
	authorization string
}

func TestAuthenticatedHTTPWorkflowsEnforceScopeCSRFAndParameters(t *testing.T) {
	provider := newMockOIDCProvider(t)
	targetRequests := make(chan targetRequest, 32)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetRequests <- targetRequest{method: r.Method, path: r.URL.Path, authorization: r.Header.Get("Authorization")}
		if r.Method != http.MethodGet {
			http.Error(w, "write request reached target", http.StatusMethodNotAllowed)
			return
		}
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/api/v1/info":
			if r.Header.Get("Authorization") != "Bearer "+testPMSSAccessToken {
				writeProblem(w, http.StatusUnauthorized, "invalid_token")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = io.WriteString(w, `{"apiVersion":"1.0","implementation":{"name":"PMSS","version":"2.1"},"broker":{"product":"Mosquitto","version":"2"},"managed":{"rolePrefix":"pmss_","groupPrefix":"pmss_","clientMarker":"managed-by: pmss"},"passwordPolicy":{"length":24,"alphabet":"A-Za-z0-9"},"reservedNames":[],"limits":{"maxRequestBytes":1048576}}`)
		case "/api/v1/clients/alice":
			writeProblem(w, http.StatusNotFound, "not_found")
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	handler, err := NewHandler(config.Config{
		PMSSAPIBaseURL:        target.URL,
		PMSSAllowInsecureHTTP: true,
		OIDC: config.OIDCConfig{
			IssuerURL:    provider.issuer,
			ClientID:     testOIDCClientID,
			ClientSecret: testOIDCClientSecret,
			CallbackURL:  "http://tester.example/auth/callback",
			Audience:     "pmss-test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	readSession := loginTestSession(t, handler, provider)
	assertNoTargetRequests(t, targetRequests)

	invalidChecksCSRF := formRequest(http.MethodPost, "/checks/run", url.Values{
		"operation_id": {"getClient"}, "csrf_token": {"invalid"},
	}, readSession.cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, invalidChecksCSRF)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "CSRF token is missing or invalid") {
		t.Fatalf("checks with invalid CSRF: status %d, body %s", response.Code, response.Body.String())
	}
	assertNoTargetRequests(t, targetRequests)

	for _, tt := range []struct {
		name   string
		values url.Values
		status int
		text   string
	}{
		{"unknown operation", url.Values{"operation_id": {"not-an-operation"}, "method": {"GET"}}, http.StatusBadRequest, "Unknown OpenAPI operation"},
		{"invalid method", url.Values{"operation_id": {"getInfo"}, "method": {"POST"}, "csrf_token": {readSession.csrf}}, http.StatusForbidden, "Only GET requests"},
		{"missing path parameter", url.Values{"operation_id": {"getClient"}, "method": {"GET"}, "csrf_token": {readSession.csrf}}, http.StatusBadRequest, "Invalid parameters"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, formRequest(http.MethodPost, "/api-explorer/execute", tt.values, readSession.cookie))
			if response.Code != tt.status || !strings.Contains(response.Body.String(), tt.text) {
				t.Fatalf("status = %d, want %d containing %q; body: %s", response.Code, tt.status, tt.text, response.Body.String())
			}
		})
	}
	assertNoTargetRequests(t, targetRequests)

	explorerRequest := formRequest(http.MethodPost, "/api-explorer/execute", url.Values{
		"operation_id": {"getInfo"}, "method": {"GET"}, "csrf_token": {readSession.csrf},
	}, readSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, explorerRequest)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "HTTP 200") {
		t.Fatalf("API Explorer GET: status %d, body %s", response.Code, response.Body.String())
	}
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info", authorization: "Bearer " + testPMSSAccessToken})

	checksRequest := formRequest(http.MethodPost, "/checks/run", url.Values{
		"operation_id": {"getClient"}, "csrf_token": {readSession.csrf},
	}, readSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, checksRequest)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "NOT TESTED") || !strings.Contains(response.Body.String(), "required path or query parameter is missing") {
		t.Fatalf("checks with missing parameter: status %d, body %s", response.Code, response.Body.String())
	}
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/healthz"})
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info"})
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info", authorization: "Bearer ******"})
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info", authorization: "Bearer " + testPMSSAccessToken})
	assertNoTargetRequests(t, targetRequests)

	logoutInvalid := formRequest(http.MethodPost, "/auth/logout", url.Values{"csrf_token": {"invalid"}}, readSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, logoutInvalid)
	if response.Code != http.StatusForbidden {
		t.Fatalf("logout with invalid CSRF status = %d, want %d", response.Code, http.StatusForbidden)
	}
	logoutValid := formRequest(http.MethodPost, "/auth/logout", url.Values{"csrf_token": {readSession.csrf}}, readSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, logoutValid)
	if response.Code != http.StatusSeeOther || !hasClearedCookie(response.Result().Cookies(), "pmss_session") {
		t.Fatalf("logout response = %d, cookies %v; want redirect and cleared session cookie", response.Code, response.Result().Cookies())
	}
	assertEndSessionRedirect(t, response, provider, true)

	provider.setExchangeScope("write")
	noReadSession := loginTestSession(t, handler, provider)
	assertNoTargetRequests(t, targetRequests)

	protectedRequest := formRequest(http.MethodPost, "/api-explorer/execute", url.Values{
		"operation_id": {"getInfo"}, "method": {"GET"}, "csrf_token": {noReadSession.csrf},
	}, noReadSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, protectedRequest)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "required read scope") {
		t.Fatalf("explorer without read scope: status %d, body %s", response.Code, response.Body.String())
	}
	assertNoTargetRequests(t, targetRequests)

	noReadChecks := formRequest(http.MethodPost, "/checks/run", url.Values{
		"operation_id": {"getClient"}, "param_getClient_p0": {"alice"}, "csrf_token": {noReadSession.csrf},
	}, noReadSession.cookie)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, noReadChecks)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "Token exchange did not return the read scope") ||
		!strings.Contains(response.Body.String(), "No protected request is sent without the read scope") {
		t.Fatalf("checks without read scope: status %d, body %s", response.Code, response.Body.String())
	}
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/healthz"})
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info"})
	assertTargetRequest(t, targetRequests, targetRequest{method: http.MethodGet, path: "/api/v1/info", authorization: strings.Repeat("*", 6)})
	assertNoTargetRequests(t, targetRequests)
}

func TestOIDCCallbackRejectsInvalidStateAndFailedExchange(t *testing.T) {
	provider := newMockOIDCProvider(t)
	handler, err := NewHandler(config.Config{OIDC: config.OIDCConfig{
		IssuerURL: provider.issuer, ClientID: testOIDCClientID, ClientSecret: testOIDCClientSecret,
		CallbackURL: "http://tester.example/auth/callback", Audience: "pmss-test",
	}})
	if err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name     string
		query    string
		cookie   *http.Cookie
		contains string
	}{
		{"missing state cookie", "state=expected&code=ok", nil, "state is invalid or has expired"},
		{"provider error", "error=access_denied&state=expected", &http.Cookie{Name: "pmss_auth_state", Value: "expected"}, "Keycloak rejected the sign-in request"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/auth/callback?"+tt.query, nil)
			if tt.cookie != nil {
				request.AddCookie(tt.cookie)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			location, err := url.Parse(response.Header().Get("Location"))
			if err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusSeeOther || !strings.Contains(location.Query().Get("auth_error"), tt.contains) {
				t.Fatalf("callback response = %d, Location %q", response.Code, response.Header().Get("Location"))
			}
		})
	}

	provider.setRejectCode(true)
	login := beginTestLogin(t, handler)
	callback := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+url.QueryEscape(login.state)+"&code=reject", nil)
	callback.AddCookie(login.stateCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, callback)
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusSeeOther ||
		!strings.Contains(location.Query().Get("auth_error"), "Sign-in failed") ||
		hasSessionCookie(response.Result().Cookies()) {
		t.Fatalf("failed code exchange response = %d, Location %q, Cookies %v", response.Code, response.Header().Get("Location"), response.Result().Cookies())
	}
}

type testLoginSession struct {
	cookie *http.Cookie
	csrf   string
}

type testLoginFlow struct {
	state       string
	stateCookie *http.Cookie
	nonce       string
}

func loginTestSession(t *testing.T, handler http.Handler, provider *mockOIDCProvider) testLoginSession {
	t.Helper()
	login := beginTestLogin(t, handler)
	provider.mu.Lock()
	provider.nonce = login.nonce
	provider.mu.Unlock()

	callback := httptest.NewRequest(http.MethodGet, "/auth/callback?state="+url.QueryEscape(login.state)+"&code=ok", nil)
	callback.AddCookie(login.stateCookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, callback)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/" {
		t.Fatalf("login callback = %d, Location %q, body %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	var sessionCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "pmss_session" && cookie.Value != "" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatalf("login callback did not set a session cookie: %v", response.Result().Cookies())
	}
	homeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	homeRequest.AddCookie(sessionCookie)
	homeResponse := httptest.NewRecorder()
	handler.ServeHTTP(homeResponse, homeRequest)
	if homeResponse.Code != http.StatusOK || !strings.Contains(homeResponse.Body.String(), "alice") {
		t.Fatalf("authenticated home: status %d, body %s", homeResponse.Code, homeResponse.Body.String())
	}
	csrf := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(homeResponse.Body.String())
	if len(csrf) != 2 {
		t.Fatalf("authenticated page does not include a CSRF token: %s", homeResponse.Body.String())
	}
	return testLoginSession{cookie: sessionCookie, csrf: csrf[1]}
}

func beginTestLogin(t *testing.T, handler http.Handler) testLoginFlow {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("login start = %d, Location %q, body %s", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	stateCookie := response.Result().Cookies()[0]
	if stateCookie.Name != "pmss_auth_state" {
		t.Fatalf("login state cookie = %q, want pmss_auth_state", stateCookie.Name)
	}
	if location.Query().Get("state") == "" || location.Query().Get("nonce") == "" || location.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("login redirect is missing state, nonce, or PKCE: %s", location)
	}
	return testLoginFlow{
		state:       location.Query().Get("state"),
		stateCookie: stateCookie,
		nonce:       location.Query().Get("nonce"),
	}
}

func formRequest(method, path string, values url.Values, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func writeProblem(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"title":"%s","status":%d,"code":%q}`, http.StatusText(status), status, code)
}

func assertTargetRequest(t *testing.T, requests <-chan targetRequest, want targetRequest) {
	t.Helper()
	select {
	case got := <-requests:
		if strings.Contains(want.authorization, "*") && got.authorization != "" {
			got.authorization = want.authorization
		}
		if got != want {
			t.Fatalf("target request = %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("target request %+v was not made", want)
	}
}

func assertNoTargetRequests(t *testing.T, requests <-chan targetRequest) {
	t.Helper()
	select {
	case got := <-requests:
		t.Fatalf("unexpected PMSS target request: %+v", got)
	default:
	}
}

func hasSessionCookie(cookies []*http.Cookie) bool {
	for _, cookie := range cookies {
		if cookie.Name == "pmss_session" && cookie.Value != "" && cookie.MaxAge >= 0 {
			return true
		}
	}
	return false
}

func hasClearedCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.MaxAge < 0 {
			return true
		}
	}
	return false
}

// A logout without an active session (expired with the access token, or
// lost by a restart of the tester) still clears the cookie and ends the
// provider session; without an ID token only client_id identifies the
// client, so the provider asks the user to confirm.
func TestLogoutWithoutActiveSessionEndsProviderSession(t *testing.T) {
	provider := newMockOIDCProvider(t)
	handler, err := NewHandler(config.Config{
		OIDC: config.OIDCConfig{
			IssuerURL:    provider.issuer,
			ClientID:     testOIDCClientID,
			ClientSecret: testOIDCClientSecret,
			CallbackURL:  "http://tester.example/auth/callback",
			Audience:     "pmss-test",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		cookie *http.Cookie
		csrf   string
	}{
		{name: "unknown session cookie", cookie: &http.Cookie{Name: "pmss_session", Value: "lost-by-restart"}, csrf: "stale-token"},
		{name: "no session cookie", csrf: "stale-token"},
		{name: "no session cookie and no token"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := url.Values{}
			if test.csrf != "" {
				values.Set("csrf_token", test.csrf)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, formRequest(http.MethodPost, "/auth/logout", values, test.cookie))
			if response.Code != http.StatusSeeOther || !hasClearedCookie(response.Result().Cookies(), "pmss_session") {
				t.Fatalf("logout = %d, cookies %v; want redirect and cleared session cookie", response.Code, response.Result().Cookies())
			}
			assertEndSessionRedirect(t, response, provider, false)
		})
	}

	// An active session with a wrong token is still rejected.
	session := loginTestSession(t, handler, provider)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, formRequest(http.MethodPost, "/auth/logout", url.Values{"csrf_token": {"forged"}}, session.cookie))
	if response.Code != http.StatusForbidden || hasClearedCookie(response.Result().Cookies(), "pmss_session") {
		t.Fatalf("logout of an active session with a forged token = %d, cookies %v; want 403 and no cleared cookie", response.Code, response.Result().Cookies())
	}
}

func assertEndSessionRedirect(t *testing.T, response *httptest.ResponseRecorder, provider *mockOIDCProvider, wantHint bool) {
	t.Helper()
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.Scheme+"://"+location.Host+location.Path != provider.server.URL+"/realms/test/logout" {
		t.Fatalf("logout redirect = %q, want the end-session endpoint", location)
	}
	query := location.Query()
	if query.Get("client_id") == "" || query.Get("post_logout_redirect_uri") == "" {
		t.Fatalf("logout redirect %q lacks client_id or post_logout_redirect_uri", location)
	}
	if hasHint := query.Get("id_token_hint") != ""; hasHint != wantHint {
		t.Fatalf("logout redirect %q: id_token_hint present = %t, want %t", location, hasHint, wantHint)
	}
}
