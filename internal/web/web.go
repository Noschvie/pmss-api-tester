// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"embed"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/noschvie/pmss-api-tester/api"
	"github.com/noschvie/pmss-api-tester/internal/auth"
	"github.com/noschvie/pmss-api-tester/internal/config"
	"github.com/noschvie/pmss-api-tester/internal/openapi"
	"github.com/noschvie/pmss-api-tester/internal/pmss"
)

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1118 generate

//go:embed static/*
var staticFiles embed.FS

func NewHandler(cfg config.Config) (http.Handler, error) {
	mux := http.NewServeMux()
	authService := auth.New(cfg.OIDC)
	operations, err := openapi.Catalog(api.Definition())
	if err != nil {
		return nil, err
	}
	operationsByID := make(map[string]openapi.Operation, len(operations))
	for _, operation := range operations {
		operationsByID[operation.ID] = operation
	}
	var pmssClient *pmss.Client
	if cfg.PMSSAPIBaseURL != "" {
		pmssClient, err = pmss.NewClient(cfg.PMSSAPIBaseURL, cfg.PMSSAllowInsecureHTTP)
		if err != nil {
			return nil, err
		}
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(mustSub(staticFiles, "static")))))
	mux.Handle("GET /_ui/backend-status", templ.Handler(BackendStatus()))
	mux.HandleFunc("GET /api-explorer", func(w http.ResponseWriter, r *http.Request) {
		data := explorerData(cfg, authService, operations, r)
		renderExplorer(w, data, http.StatusOK)
	})
	mux.HandleFunc("GET /checks", func(w http.ResponseWriter, r *http.Request) {
		data := checksData(cfg, authService, operations, r)
		renderChecks(w, data, http.StatusOK)
	})
	mux.HandleFunc("POST /checks/run", func(w http.ResponseWriter, r *http.Request) {
		handleChecksRun(w, r, cfg, authService, pmssClient, operations, operationsByID)
	})
	mux.HandleFunc("POST /api-explorer/execute", func(w http.ResponseWriter, r *http.Request) {
		handleExplorerExecute(w, r, cfg, authService, pmssClient, operations, operationsByID)
	})
	mux.HandleFunc("GET /auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		location, err := authService.BeginLogin(r.Context())
		if err != nil {
			http.Redirect(w, r, "/?auth_error="+url.QueryEscape("Could not start sign-in: "+err.Error()), http.StatusSeeOther)
			return
		}
		stateURL, err := url.Parse(location)
		if err != nil {
			http.Error(w, "OIDC sign-in URL is invalid", http.StatusInternalServerError)
			return
		}
		http.SetCookie(w, authService.StateCookie(stateURL.Query().Get("state")))
		http.Redirect(w, r, location, http.StatusFound)
	})
	mux.HandleFunc("GET /auth/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		http.SetCookie(w, authService.ClearStateCookie())
		if r.URL.Query().Get("error") != "" {
			http.Redirect(w, r, "/?auth_error="+url.QueryEscape("Keycloak rejected the sign-in request."), http.StatusSeeOther)
			return
		}
		state := r.URL.Query().Get("state")
		if !authService.StateCookieMatches(r, state) {
			http.Redirect(w, r, "/?auth_error="+url.QueryEscape("Sign-in state is invalid or has expired."), http.StatusSeeOther)
			return
		}
		sessionID, session, err := authService.CompleteLogin(r.Context(), state, r.URL.Query().Get("code"))
		if err != nil {
			http.Redirect(w, r, "/?auth_error="+url.QueryEscape("Sign-in failed: "+err.Error()), http.StatusSeeOther)
			return
		}
		http.SetCookie(w, authService.Cookie(sessionID, session.ExpiresAt))
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	mux.HandleFunc("POST /auth/logout", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid logout request", http.StatusBadRequest)
			return
		}
		// The cookie value, not Session(): Session() hides an expired
		// session, whose ID token Logout can still use as id_token_hint.
		idToken, result := authService.Logout(authService.SessionID(r), r.PostForm.Get("csrf_token"))
		if result == auth.LogoutInvalidCSRF {
			http.Error(w, "Invalid CSRF validation", http.StatusForbidden)
			return
		}
		// LogoutDone or LogoutNoSession: clear the cookie and end the
		// provider session. Without an ID token, the provider asks the user
		// to confirm, so a forged request cannot end the SSO session.
		http.SetCookie(w, authService.ClearCookie())
		redirectURL, err := url.Parse(cfg.OIDC.CallbackURL)
		if err != nil {
			http.Error(w, "Logout URL is invalidly configured", http.StatusInternalServerError)
			return
		}
		redirectURL.Path = "/"
		redirectURL.RawPath = ""
		http.Redirect(w, r, authService.EndSessionURL(r.Context(), idToken, redirectURL.String()), http.StatusSeeOther)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		state := PageState{
			AuthEnabled:    authService.Enabled(),
			PMSSConfigured: cfg.PMSSAPIBaseURL != "",
			PMSSAPIBaseURL: cfg.PMSSAPIBaseURL,
			Error:          r.URL.Query().Get("auth_error"),
		}
		if session, _ := authService.Session(r); session != nil {
			w.Header().Set("Cache-Control", "no-store")
			state.Authenticated = true
			state.Username = session.Username
			state.ScopeLabel = strings.Join(session.Scopes, " ")
			state.HasReadScope = session.HasReadScope
			state.CSRFToken = session.CSRFToken
			state.ExpiresAt = session.ExpiresAt.Format(time.RFC3339)
		} else if _, err := r.Cookie("pmss_session"); err == nil {
			http.SetCookie(w, authService.ClearCookie())
		}
		if err := Page(state).Render(r.Context(), w); err != nil {
			http.Error(w, "page rendering failed", http.StatusInternalServerError)
		}
	})
	return mux, nil
}

func mustSub(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic(err)
	}
	return sub
}
