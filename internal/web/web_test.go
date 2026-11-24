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
	handler := mustNewHandler(t, config.Config{})
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

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
	handler := mustNewHandler(t, config.Config{})

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("home page status = %d, want %d", page.Code, http.StatusOK)
	}
	if !strings.Contains(page.Body.String(), "PMSS API Tester") || !strings.Contains(page.Body.String(), "htmx.min.js") || !strings.Contains(page.Body.String(), `href="/api-explorer"`) {
		t.Fatal("home page is missing its title, HTMX integration, or API Explorer navigation")
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

func TestAPIExplorerSeparatesOperationsAndShowsReadOnlyStatus(t *testing.T) {
	handler := mustNewHandler(t, config.Config{PMSSAPIBaseURL: "https://pmss.example.org"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api-explorer", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("API Explorer status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, expected := range []string{"Core API", "PMSS Extra", "getInfo", "Run GET", "BLOCKED", "Response schema"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("API Explorer is missing %q", expected)
		}
	}
}

func TestAPIExplorerBlocksNonGETOperations(t *testing.T) {
	handler := mustNewHandler(t, config.Config{PMSSAPIBaseURL: "https://pmss.example.org"})
	body := strings.NewReader("operation_id=createClient&method=POST")
	request := httptest.NewRequest(http.MethodPost, "/api-explorer/execute", body)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if !strings.Contains(response.Body.String(), "BLOCKED") || !strings.Contains(response.Body.String(), "Only GET requests") {
		t.Fatal("non-GET operation was not clearly blocked")
	}
}

func TestAPIExplorerRequiresConfiguredHTTPSBaseURL(t *testing.T) {
	handler := mustNewHandler(t, config.Config{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api-explorer", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("API Explorer status = %d, want %d", response.Code, http.StatusOK)
	}

	if !strings.Contains(response.Body.String(), "HTTPS-Basis-URL") {
		t.Fatal("unconfigured PMSS target was not reported")
	}
}

func TestAPIExplorerHTTPRequiresExceptionAndWarns(t *testing.T) {
	cfg := config.Config{PMSSAPIBaseURL: "http://pmss.example.org"}
	if _, err := NewHandler(cfg); err == nil {
		t.Fatal("handler accepted HTTP without an explicit exception")
	}
	cfg.PMSSAllowInsecureHTTP = true
	for _, target := range []string{"http://pmss.example.org", "HTTP://pmss.example.org"} {
		t.Run(target, func(t *testing.T) {
			cfg.PMSSAPIBaseURL = target
			handler := mustNewHandler(t, cfg)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api-explorer", nil))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "HTTP is unencrypted") || !strings.Contains(response.Body.String(), "user token will be sent in plaintext") {
				t.Fatalf("HTTP warning missing from explorer: status %d", response.Code)
			}
		})
	}
}

func TestUnknownRouteReturnsNotFound(t *testing.T) {
	response := httptest.NewRecorder()
	mustNewHandler(t, config.Config{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestLogoutRequiresSessionAndCSRFToken(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/logout", strings.NewReader("csrf_token=forged"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	mustNewHandler(t, config.Config{}).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func mustNewHandler(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()
	handler, err := NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}
