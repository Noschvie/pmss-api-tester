// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/noschvie/pmss-api-tester/internal/config"
)

func TestAnonymousHTTPWorkflowIsReadOnly(t *testing.T) {
	requests := make(chan string, 10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		switch r.URL.Path {
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":"ok"}`)
		case "/api/v1/info":
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer target.Close()

	app := httptest.NewServer(mustNewHandler(t, config.Config{
		PMSSAPIBaseURL: target.URL, PMSSAllowInsecureHTTP: true,
	}))
	defer app.Close()

	for _, tt := range []struct {
		name   string
		values url.Values
		status int
	}{
		{"write operation", url.Values{"operation_id": {"createClient"}, "method": {"POST"}}, http.StatusForbidden},
		{"forged GET method", url.Values{"operation_id": {"createClient"}, "method": {"GET"}}, http.StatusForbidden},
		{"anonymous GET", url.Values{"operation_id": {"getInfo"}, "method": {"GET"}}, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := http.PostForm(app.URL+"/api-explorer/execute", tt.values)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != tt.status || !strings.Contains(string(body), "BLOCKED") {
				t.Fatalf("status = %d, want %d with BLOCKED", response.StatusCode, tt.status)
			}
		})
	}
	select {
	case request := <-requests:
		t.Fatalf("blocked explorer request reached target: %s", request)
	default:
	}

	response, err := http.PostForm(app.URL+"/checks/run", url.Values{
		"operation_id": {"getClient", "createClient", "healthz"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Availability · GET /healthz", "Authentication · Missing token", "Authentication · Invalid token", "GET /api/v1/clients/{username}", "PASS", "BLOCKED"} {
		if !strings.Contains(string(body), expected) {
			t.Errorf("checks response missing %q", expected)
		}
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("checks status = %d, Cache-Control = %q", response.StatusCode, response.Header.Get("Cache-Control"))
	}
	for _, expected := range []string{"GET /healthz", "GET /api/v1/info", "GET /api/v1/info"} {
		select {
		case got := <-requests:
			if got != expected {
				t.Fatalf("target request = %q, want %q", got, expected)
			}
		default:
			t.Fatalf("missing target request %q", expected)
		}
	}
	select {
	case request := <-requests:
		t.Fatalf("unexpected target request: %s", request)
	default:
	}
}
