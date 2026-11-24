package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/noschvie/pmss-api-tester/internal/config"
)

func TestHealthEndpoint(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(config.Config{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestHomePageAndEmbeddedAssets(t *testing.T) {
	handler := NewHandler(config.Config{})

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status = %d, want %d", page.Code, http.StatusOK)
	}
	if !strings.Contains(page.Body.String(), "PMSS API Tester") || !strings.Contains(page.Body.String(), "htmx.min.js") {
		t.Fatal("home page is missing its title or HTMX integration")
	}

	asset := httptest.NewRecorder()
	handler.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, "/static/app.css", nil))
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want %d", asset.Code, http.StatusOK)
	}
	if !strings.Contains(asset.Body.String(), ".sidebar") {
		t.Fatal("embedded stylesheet was not served")
	}

	cssURL := &url.URL{Path: "/static/app.css"}
	for _, font := range []string{"geist-latin.woff2", "geist-mono-latin.woff2"} {
		t.Run(font, func(t *testing.T) {
			if !strings.Contains(asset.Body.String(), `url("`+font+`")`) {
				t.Fatalf("stylesheet is missing relative font URL %q", font)
			}
			fontURL := cssURL.ResolveReference(&url.URL{Path: font})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, fontURL.String(), nil))
			if response.Code != http.StatusOK {
				t.Fatalf("font status = %d, want %d", response.Code, http.StatusOK)
			}
			if response.Body.Len() == 0 {
				t.Fatal("embedded font is empty")
			}
		})
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	response := httptest.NewRecorder()
	NewHandler(config.Config{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestLogoutRequiresSessionAndCSRFToken(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader("csrf_token=forged"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	NewHandler(config.Config{}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}
