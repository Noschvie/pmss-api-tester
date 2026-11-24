package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/noschvie/pmss-api-tester/internal/config"
	"golang.org/x/oauth2"
)

const (
	sessionCookieName = "pmss_session"
	stateCookieName   = "pmss_auth_state"
	sessionDuration   = 8 * time.Hour
	authFlowDuration  = 5 * time.Minute
)

type Session struct {
	Username     string
	CSRFToken    string
	Scopes       []string
	HasReadScope bool
	ExpiresAt    time.Time

	idToken        string
	exchangedToken string
}

type authFlow struct {
	nonce        string
	codeVerifier string
	expiresAt    time.Time
}

type Service struct {
	cfg config.OIDCConfig

	mu       sync.Mutex
	sessions map[string]*Session
	flows    map[string]authFlow

	providerMu sync.Mutex
	provider   *oidc.Provider
	oauth      *oauth2.Config
	verifier   *oidc.IDTokenVerifier
	endSession string
}

func New(cfg config.OIDCConfig) *Service {
	return &Service{
		cfg:      cfg,
		sessions: make(map[string]*Session),
		flows:    make(map[string]authFlow),
	}
}

func (s *Service) Enabled() bool {
	return s.cfg.IssuerURL != "" && s.cfg.ClientID != "" && s.cfg.ClientSecret != ""
}

func (s *Service) BeginLogin(ctx context.Context) (string, error) {
	if !s.Enabled() {
		return "", errors.New("OIDC login is not configured")
	}
	ctx = s.providerContext(ctx)
	if err := s.ensureProvider(ctx); err != nil {
		return "", err
	}

	state, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("create login state: %w", err)
	}
	nonce, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("create OIDC nonce: %w", err)
	}
	verifier, err := randomToken(32)
	if err != nil {
		return "", fmt.Errorf("create PKCE verifier: %w", err)
	}

	now := time.Now()
	s.mu.Lock()
	s.pruneLocked(now)
	s.flows[state] = authFlow{
		nonce:        nonce,
		codeVerifier: verifier,
		expiresAt:    now.Add(authFlowDuration),
	}
	s.mu.Unlock()

	return s.oauth.AuthCodeURL(
		state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.S256ChallengeOption(verifier),
	), nil
}

func (s *Service) CompleteLogin(ctx context.Context, state, code string) (string, *Session, error) {
	ctx = s.providerContext(ctx)
	if err := s.ensureProvider(ctx); err != nil {
		return "", nil, err
	}
	now := time.Now()
	s.mu.Lock()
	flow, ok := s.flows[state]
	delete(s.flows, state)
	s.pruneLocked(now)
	s.mu.Unlock()
	if !ok || state == "" || now.After(flow.expiresAt) {
		return "", nil, errors.New("login state is invalid or expired")
	}

	token, err := s.oauth.Exchange(
		ctx,
		code,
		oauth2.VerifierOption(flow.codeVerifier),
	)
	if err != nil {
		return "", nil, fmt.Errorf("authorization code exchange failed: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return "", nil, errors.New("identity provider did not return an ID token")
	}
	idToken, err := s.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return "", nil, fmt.Errorf("ID token validation failed: %w", err)
	}
	if idToken.Nonce != flow.nonce {
		return "", nil, errors.New("ID token nonce does not match the login request")
	}
	if token.AccessToken == "" {
		return "", nil, errors.New("identity provider did not return an access token")
	}

	var claims struct {
		Username string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return "", nil, fmt.Errorf("read user identity: %w", err)
	}
	if claims.Username == "" {
		claims.Username = idToken.Subject
	}

	exchangedToken, scopes, exchangedExpiry, err := s.exchangeForPMSS(ctx, token.AccessToken)
	if err != nil {
		return "", nil, err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", nil, fmt.Errorf("create CSRF token: %w", err)
	}

	expiresAt := now.Add(sessionDuration)
	if !token.Expiry.IsZero() && token.Expiry.Before(expiresAt) {
		expiresAt = token.Expiry
	}
	if idToken.Expiry.Before(expiresAt) {
		expiresAt = idToken.Expiry
	}
	if !exchangedExpiry.IsZero() && exchangedExpiry.Before(expiresAt) {
		expiresAt = exchangedExpiry
	}
	if expiresAt.Before(time.Now()) {
		return "", nil, errors.New("identity provider returned an expired access token")
	}
	sessionID, err := randomToken(32)
	if err != nil {
		return "", nil, fmt.Errorf("create session: %w", err)
	}

	session := &Session{
		Username:       claims.Username,
		CSRFToken:      csrfToken,
		Scopes:         scopes,
		HasReadScope:   contains(scopes, "read"),
		ExpiresAt:      expiresAt,
		idToken:        rawIDToken,
		exchangedToken: exchangedToken,
	}
	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()
	return sessionID, session, nil
}

func (s *Service) Session(r *http.Request) (*Session, string) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return nil, ""
	}
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[cookie.Value]
	if !ok {
		return nil, ""
	}
	if !now.Before(session.ExpiresAt) {
		delete(s.sessions, cookie.Value)
		return nil, ""
	}
	return session, cookie.Value
}

func (s *Service) PMSSAccessToken(r *http.Request) (string, bool) {
	session, _ := s.Session(r)
	if session == nil || !session.HasReadScope {
		return "", false
	}
	return session.exchangedToken, true
}

func (s *Service) ValidateCSRF(r *http.Request, token string) bool {
	session, _ := s.Session(r)
	return session != nil && token != "" &&
		subtle.ConstantTimeCompare([]byte(session.CSRFToken), []byte(token)) == 1
}

func (s *Service) Logout(sessionID, csrfToken string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok || subtle.ConstantTimeCompare([]byte(session.CSRFToken), []byte(csrfToken)) != 1 {
		return "", false
	}
	delete(s.sessions, sessionID)
	return session.idToken, true
}

func (s *Service) EndSessionURL(idToken, redirectURI string) string {
	if s.endSession == "" {
		return "/"
	}
	endpoint, err := url.Parse(s.endSession)
	if err != nil {
		return "/"
	}
	query := endpoint.Query()
	if idToken != "" {
		query.Set("id_token_hint", idToken)
	}
	query.Set("post_logout_redirect_uri", redirectURI)
	endpoint.RawQuery = query.Encode()
	return endpoint.String()
}

func (s *Service) Cookie(sessionID string, expiresAt time.Time) *http.Cookie {
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   maxAge,
	}
}

func (s *Service) ClearCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func (s *Service) StateCookie(state string) *http.Cookie {
	return &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(authFlowDuration.Seconds()),
	}
}

func (s *Service) ClearStateCookie() *http.Cookie {
	return &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
}

func (s *Service) StateCookieMatches(r *http.Request, state string) bool {
	cookie, err := r.Cookie(stateCookieName)
	return err == nil && state != "" && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(state)) == 1
}

func (s *Service) ensureProvider(ctx context.Context) error {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.provider != nil {
		return nil
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: issuerTransport{
			base:        http.DefaultTransport,
			issuerURL:   s.cfg.IssuerURL,
			internalURL: s.cfg.InternalURL,
		},
	}
	ctx = oidc.ClientContext(ctx, client)
	provider, err := oidc.NewProvider(ctx, s.cfg.IssuerURL)
	if err != nil {
		return fmt.Errorf("load OIDC provider metadata: %w", err)
	}
	metadataURL := strings.TrimRight(s.cfg.IssuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metadataURL, nil)
	if err != nil {
		return fmt.Errorf("prepare OIDC metadata request: %w", err)
	}
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("load OIDC logout endpoint: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("load OIDC logout endpoint: provider returned HTTP %d", response.StatusCode)
	}
	var metadata struct {
		Issuer             string `json:"issuer"`
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&metadata); err != nil {
		return fmt.Errorf("decode OIDC provider metadata: %w", err)
	}
	if metadata.Issuer != s.cfg.IssuerURL {
		return fmt.Errorf("OIDC issuer mismatch: provider advertises %q, configured %q", metadata.Issuer, s.cfg.IssuerURL)
	}

	s.provider = provider
	s.oauth = &oauth2.Config{
		ClientID:     s.cfg.ClientID,
		ClientSecret: s.cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  s.cfg.CallbackURL,
		Scopes:       []string{oidc.ScopeOpenID},
	}
	s.verifier = provider.Verifier(&oidc.Config{ClientID: s.cfg.ClientID})
	s.endSession = metadata.EndSessionEndpoint
	return nil
}

func (s *Service) providerContext(ctx context.Context) context.Context {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: issuerTransport{
			base:        http.DefaultTransport,
			issuerURL:   s.cfg.IssuerURL,
			internalURL: s.cfg.InternalURL,
		},
	}
	return oidc.ClientContext(ctx, client)
}

func (s *Service) exchangeForPMSS(ctx context.Context, subjectToken string) (string, []string, time.Time, error) {
	client, err := s.httpClient()
	if err != nil {
		return "", nil, time.Time{}, err
	}
	provider, err := s.providerEndpoint(ctx)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	form := url.Values{
		"grant_type":           {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":        {subjectToken},
		"subject_token_type":   {"urn:ietf:params:oauth:token-type:access_token"},
		"requested_token_type": {"urn:ietf:params:oauth:token-type:access_token"},
		"audience":             {s.cfg.Audience},
		"scope":                {"read"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, provider.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", nil, time.Time{}, fmt.Errorf("prepare PMSS token exchange: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(s.cfg.ClientID, s.cfg.ClientSecret)
	response, err := client.Do(req)
	if err != nil {
		return "", nil, time.Time{}, fmt.Errorf("PMSS token exchange request failed: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", nil, time.Time{}, fmt.Errorf("read PMSS token exchange response: %w", err)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		Scope       string `json:"scope"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", nil, time.Time{}, fmt.Errorf("decode PMSS token exchange response (HTTP %d)", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := result.Description
		if message == "" {
			message = result.Error
		}
		if message == "" {
			message = fmt.Sprintf("HTTP %d", response.StatusCode)
		}
		if len(message) > 400 {
			message = message[:400]
		}
		return "", nil, time.Time{}, fmt.Errorf("PMSS token exchange failed: %s", message)
	}
	if result.AccessToken == "" {
		return "", nil, time.Time{}, errors.New("PMSS token exchange response did not contain an access token")
	}
	var expiresAt time.Time
	if result.ExpiresIn > 0 && result.ExpiresIn < int64(sessionDuration/time.Second) {
		expiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}
	return result.AccessToken, strings.Fields(result.Scope), expiresAt, nil
}

func (s *Service) httpClient() (*http.Client, error) {
	s.providerMu.Lock()
	defer s.providerMu.Unlock()
	if s.provider == nil {
		return nil, errors.New("OIDC provider is not initialized")
	}
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: issuerTransport{
			base:        http.DefaultTransport,
			issuerURL:   s.cfg.IssuerURL,
			internalURL: s.cfg.InternalURL,
		},
	}, nil
}

func (s *Service) providerEndpoint(ctx context.Context) (*oauth2.Endpoint, error) {
	if err := s.ensureProvider(ctx); err != nil {
		return nil, err
	}
	endpoint := s.provider.Endpoint()
	return &endpoint, nil
}

func (s *Service) pruneLocked(now time.Time) {
	for state, flow := range s.flows {
		if !now.Before(flow.expiresAt) {
			delete(s.flows, state)
		}
	}
	for id, session := range s.sessions {
		if !now.Before(session.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
}

type issuerTransport struct {
	base        http.RoundTripper
	issuerURL   string
	internalURL string
}

func (t issuerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.internalURL != "" {
		issuer, issuerErr := url.Parse(t.issuerURL)
		internal, internalErr := url.Parse(t.internalURL)
		if issuerErr == nil && internalErr == nil && req.URL.Scheme == issuer.Scheme && req.URL.Host == issuer.Host {
			issuerPath := strings.TrimRight(issuer.Path, "/")
			if req.URL.Path == issuerPath || strings.HasPrefix(req.URL.Path, issuerPath+"/") {
				rewritten := *req
				rewrittenURL := *req.URL
				rewrittenURL.Scheme = internal.Scheme
				rewrittenURL.Host = internal.Host
				rewrittenURL.Path = strings.TrimRight(internal.Path, "/") + strings.TrimPrefix(req.URL.Path, issuerPath)
				rewritten.URL = &rewrittenURL
				rewritten.Host = ""
				return t.base.RoundTrip(&rewritten)
			}
		}
	}
	return t.base.RoundTrip(req)
}

func randomToken(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func contains(values []string, value string) bool {
	for _, current := range values {
		if current == value {
			return true
		}
	}
	return false
}
