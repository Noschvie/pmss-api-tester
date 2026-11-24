package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/noschvie/pmss-api-tester/internal/config"
	"github.com/noschvie/pmss-api-tester/internal/openapi"
	"github.com/noschvie/pmss-api-tester/internal/pmss"
)

func TestRunHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/healthz" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected health request: %s %s, authorization %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	}))
	defer server.Close()

	client, err := pmss.NewClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	result := runHealthCheck(context.Background(), client)
	if result.Status != "PASS" {
		t.Fatalf("health result = %+v", result)
	}
}

func TestRunAuthenticationCheckValidatesProblemDetails(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		content    string
		body       string
		wantStatus string
	}{
		{
			name:       "expected missing token problem",
			status:     http.StatusUnauthorized,
			content:    "application/problem+json",
			body:       `{"type":"about:blank","title":"Unauthorized","status":401,"code":"invalid_token"}`,
			wantStatus: "PASS",
		},
		{
			name:       "unexpected status",
			status:     http.StatusOK,
			content:    "application/json",
			body:       `{"apiVersion":"1.0"}`,
			wantStatus: "FAIL",
		},
		{
			name:       "wrong problem code",
			status:     http.StatusUnauthorized,
			content:    "application/problem+json",
			body:       `{"title":"Unauthorized","status":401,"code":"other_error"}`,
			wantStatus: "FAIL",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("anonymous authentication check sent an authorization header")
				}
				w.Header().Set("Content-Type", tt.content)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer server.Close()

			client, err := pmss.NewClient(server.URL, true)
			if err != nil {
				t.Fatal(err)
			}
			result := runAuthenticationCheck(context.Background(), client, openapi.Operation{
				ID:     "getInfo",
				Method: http.MethodGet,
				Path:   "/api/v1/info",
			})
			if result.Status != tt.wantStatus {
				t.Fatalf("authentication result = %+v, want status %s", result, tt.wantStatus)
			}
		})
	}
}

func TestRunInvalidTokenCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer pmss-api-tester-invalid-token" {
			t.Errorf("Authorization = %q, want the fixed invalid test token", got)
		}
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`)
	}))
	defer server.Close()

	client, err := pmss.NewClient(server.URL, true)
	if err != nil {
		t.Fatal(err)
	}
	result := runInvalidTokenCheck(context.Background(), client, openapi.Operation{
		ID:     "getInfo",
		Method: http.MethodGet,
		Path:   "/api/v1/info",
	})
	if result.Status != "PASS" {
		t.Fatalf("invalid-token result = %+v", result)
	}
}

func TestEvaluateResponseChecksETagAndBrokerErrors(t *testing.T) {
	operation := openapi.Operation{ID: "getClient", Method: http.MethodGet, Path: "/api/v1/clients/{username}"}
	valid := pmss.Result{
		StatusCode:   http.StatusOK,
		SchemaChecked: true,
		SchemaValid:   true,
		Headers: []pmss.Header{
			{Name: "ETag", Value: `"c-9f3b2a71"`},
			{Name: "Content-Type", Value: "application/json"},
		},
	}
	if result := evaluateResponse("client", operation, valid, nil, false); result.Status != "PASS" {
		t.Fatalf("valid ETag result = %+v", result)
	}
	valid.Headers[0].Value = "weak"
	if result := evaluateResponse("client", operation, valid, nil, false); result.Status != "FAIL" {
		t.Fatalf("invalid ETag result = %+v", result)
	}
	notFound := pmss.Result{
		StatusCode:    http.StatusNotFound,
		SchemaChecked: true,
		SchemaValid:   true,
		Headers:       []pmss.Header{{Name: "Content-Type", Value: "application/problem+json"}},
		Body:          `{"title":"Not found","status":404,"code":"not_found"}`,
	}
	if result := evaluateResponse("client", operation, notFound, nil, false); result.Status != "PASS" {
		t.Fatalf("expected read-only 404 result = %+v", result)
	}

	for _, test := range []struct {
		status int
		text   string
	}{
		{http.StatusServiceUnavailable, "503 broker_unavailable"},
		{http.StatusGatewayTimeout, "504 broker_timeout"},
	} {
		result := pmss.Result{
			StatusCode: test.status,
			Headers: []pmss.Header{
				{Name: "Content-Type", Value: "application/problem+json"},
			},
		}
		if test.status == http.StatusServiceUnavailable {
			result.Body = `{"title":"Broker unavailable","status":503,"code":"broker_unavailable"}`
		} else {
			result.Body = `{"title":"Broker timeout","status":504,"code":"broker_timeout"}`
		}
		evaluated := evaluateResponse("export", openapi.Operation{ID: "getExport"}, result, nil, false)
		if evaluated.Status != "FAIL" || !strings.Contains(evaluated.Detail, test.text) {
			t.Fatalf("broker status %d result = %+v", test.status, evaluated)
		}
	}
}

func TestEvaluateAPIVersion(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus string
	}{
		{"supported version", `{"apiVersion":"1.0","implementation":{"name":"PMSS","version":"2.1"}}`, "PASS"},
		{"unsupported major", `{"apiVersion":"2.0","implementation":{"name":"PMSS","version":"2.1"}}`, "FAIL"},
		{"invalid minor", `{"apiVersion":"1.next","implementation":{"name":"PMSS","version":"2.1"}}`, "FAIL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateAPIVersion(tt.body)
			if result.Status != tt.wantStatus {
				t.Fatalf("version result = %+v, want status %s", result, tt.wantStatus)
			}
		})
	}
}

func TestChecksRunAllowsAnonymousReadOnlyChecks(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/api/v1/info":
			if got := r.Header.Get("Authorization"); got != "" && got != "Bearer pmss-api-tester-invalid-token" {
				t.Errorf("unexpected authentication-check header: %q", got)
			}
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	handler := mustNewHandler(t, config.Config{
		PMSSAPIBaseURL:        target.URL,
		PMSSAllowInsecureHTTP: true,
	})
	request := httptest.NewRequest(http.MethodPost, "/checks/run", strings.NewReader(url.Values{}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", response.Code, http.StatusOK, response.Body.String())
	}
	for _, expected := range []string{"Availability · GET /healthz", "Authentication · Missing token", "Authentication · Invalid token", "GET /api/v1/info", "PASS", "BLOCKED"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("checks response is missing %q", expected)
		}
	}
}
