package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/noschvie/pmss-api-tester/internal/config"
)

func TestBeginLoginRequiresOIDCConfiguration(t *testing.T) {
	service := New(config.OIDCConfig{})
	if _, err := service.BeginLogin(context.Background()); err == nil {
		t.Fatal("BeginLogin succeeded without OIDC configuration")
	}
}

func TestBeginLoginUsesPKCEAndExchangesOnlyReadScope(t *testing.T) {
	tokenExchangeChecked := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/realms/pmss-test/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                "http://" + r.Host + "/realms/pmss-test",
				"authorization_endpoint":                "http://" + r.Host + "/authorize",
				"token_endpoint":                        "http://" + r.Host + "/realms/pmss-test/token",
				"jwks_uri":                              "http://" + r.Host + "/realms/pmss-test/jwks",
				"response_types_supported":              []string{"code"},
				"subject_types_supported":               []string{"public"},
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/realms/pmss-test/token":
			username, password, ok := r.BasicAuth()
			if !ok || username != "tester-client" || password != "client-secret" {
				t.Errorf("token endpoint client authentication = %q, %q, %t", username, password, ok)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token exchange form: %v", err)
			}
			for key, expected := range map[string]string{
				"grant_type":           "urn:ietf:params:oauth:grant-type:token-exchange",
				"subject_token":        "user-access-token",
				"subject_token_type":   "urn:ietf:params:oauth:token-type:access_token",
				"requested_token_type": "urn:ietf:params:oauth:token-type:access_token",
				"audience":             "test-audience",
				"scope":                "read",
			} {
				if got := r.Form.Get(key); got != expected {
					t.Errorf("form %s = %q, want %q", key, got, expected)
				}
			}
			tokenExchangeChecked <- struct{}{}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"exchanged-token","scope":"read","expires_in":300}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	issuerURL := server.URL + "/realms/pmss-test"
	service := New(config.OIDCConfig{
		IssuerURL:    issuerURL,
		ClientID:     "tester-client",
		ClientSecret: "client-secret",
		CallbackURL:  "http://localhost:8080/auth/callback",
		Audience:     "test-audience",
	})
	loginURL, err := service.BeginLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parsedLoginURL, err := url.Parse(loginURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsedLoginURL.Query()
	if query.Get("response_type") != "code" || query.Get("client_id") != "tester-client" {
		t.Fatalf("unexpected authorization request: %s", loginURL)
	}
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization request is missing S256 PKCE: %s", loginURL)
	}
	if query.Get("state") == "" || query.Get("nonce") == "" {
		t.Fatalf("authorization request is missing state or nonce: %s", loginURL)
	}

	token, scopes, expiresAt, err := service.exchangeForPMSS(context.Background(), "user-access-token")
	if err != nil {
		t.Fatal(err)
	}
	if token != "exchanged-token" || len(scopes) != 1 || scopes[0] != "read" || !expiresAt.After(time.Now()) {
		t.Fatalf("unexpected token exchange result: token=%q scopes=%v expires=%s", token, scopes, expiresAt)
	}
	select {
	case <-tokenExchangeChecked:
	default:
		t.Fatal("token exchange endpoint was not called")
	}
}

func TestStateCookieBinding(t *testing.T) {
	service := New(config.OIDCConfig{CookieSecure: true})
	cookie := service.StateCookie("expected-state")
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/auth" {
		t.Fatalf("state cookie is missing security flags: %+v", cookie)
	}

	request := httptest.NewRequest(http.MethodGet, "/auth/callback?state=expected-state", nil)
	request.AddCookie(cookie)
	if !service.StateCookieMatches(request, "expected-state") {
		t.Fatal("matching state cookie was rejected")
	}
	if service.StateCookieMatches(request, "attacker-state") {
		t.Fatal("mismatched state cookie was accepted")
	}
}

func TestSessionTokenAndCSRFProtectedLogout(t *testing.T) {
	service := New(config.OIDCConfig{CookieSecure: true})
	expiresAt := time.Now().Add(time.Hour)
	service.sessions["opaque-session"] = &Session{
		Username:       "alice",
		CSRFToken:      "random-csrf-token",
		Scopes:         []string{"read"},
		HasReadScope:   true,
		ExpiresAt:      expiresAt,
		idToken:        "id-token",
		exchangedToken: "pmss-access-token",
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "opaque-session"})

	session, sessionID := service.Session(request)
	if session == nil || sessionID != "opaque-session" || session.Username != "alice" {
		t.Fatalf("unexpected session lookup: session=%+v id=%q", session, sessionID)
	}
	if token, ok := service.PMSSAccessToken(request); !ok || token != "pmss-access-token" {
		t.Fatalf("PMSSAccessToken = %q, %t", token, ok)
	}

	cookie := service.Cookie(sessionID, expiresAt)
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" {
		t.Fatalf("session cookie is missing security flags: %+v", cookie)
	}
	if _, ok := service.Logout(sessionID, "incorrect"); ok {
		t.Fatal("logout accepted an invalid CSRF token")
	}
	if idToken, ok := service.Logout(sessionID, "random-csrf-token"); !ok || idToken != "id-token" {
		t.Fatalf("logout = %q, %t", idToken, ok)
	}
	if session, _ := service.Session(request); session != nil {
		t.Fatal("session remains after logout")
	}
}

func TestPMSSAccessTokenRequiresReadScope(t *testing.T) {
	service := New(config.OIDCConfig{})
	service.sessions["session"] = &Session{
		ExpiresAt:      time.Now().Add(time.Hour),
		exchangedToken: "no-read-token",
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})

	if token, ok := service.PMSSAccessToken(request); ok || token != "" {
		t.Fatalf("PMSSAccessToken = %q, %t, want blocked", token, ok)
	}
}

func TestIssuerTransportRewritesInternalRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/token" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/internal/token")
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	transport := issuerTransport{
		base:        http.DefaultTransport,
		issuerURL:   "http://127.0.0.1:8180/realms/pmss-test",
		internalURL: server.URL + "/internal",
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8180/realms/pmss-test/token", strings.NewReader("a=b"))
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestExpiredSessionIsRemoved(t *testing.T) {
	service := New(config.OIDCConfig{})
	service.sessions["expired"] = &Session{ExpiresAt: time.Now().Add(-time.Second)}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "expired"})

	if session, _ := service.Session(request); session != nil {
		t.Fatal("expired session was returned")
	}
	if _, ok := service.sessions["expired"]; ok {
		t.Fatal("expired session remains in the session store")
	}
}
