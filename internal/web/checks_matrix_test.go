// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noschvie/pmss-api-tester/internal/auth"
	"github.com/noschvie/pmss-api-tester/internal/config"
	"github.com/noschvie/pmss-api-tester/internal/openapi"
	"github.com/noschvie/pmss-api-tester/internal/pmss"
)

func TestEvaluateResponseStatusAndHeaders(t *testing.T) {
	valid := pmss.Result{StatusCode: http.StatusOK, SchemaChecked: true, SchemaValid: true}
	tests := []struct {
		name       string
		operation  openapi.Operation
		result     pmss.Result
		err        error
		noStore    bool
		wantStatus string
		wantDetail string
	}{
		{"valid schema", openapi.Operation{}, valid, nil, false, "PASS", "schema is valid"},
		{"request error", openapi.Operation{}, valid, errors.New("request error"), false, "FAIL", "request error"},
		{"transport error", openapi.Operation{}, pmss.Result{Error: "connection failed"}, nil, false, "FAIL", "connection failed"},
		{"unexpected status", openapi.Operation{}, pmss.Result{StatusCode: http.StatusForbidden}, nil, false, "FAIL", "403"},
		{"missing schema", openapi.Operation{}, pmss.Result{StatusCode: http.StatusOK}, nil, false, "NOT TESTED", "no response schema"},
		{"schema error", openapi.Operation{}, pmss.Result{StatusCode: http.StatusOK, SchemaError: "invalid field"}, nil, false, "FAIL", "invalid field"},
		{"invalid schema", openapi.Operation{}, pmss.Result{StatusCode: http.StatusOK, SchemaChecked: true}, nil, false, "FAIL", "does not match"},
		{"missing no-store", openapi.Operation{ID: "getInfo"}, valid, nil, true, "FAIL", "no-store"},
		{"no-store among directives", openapi.Operation{ID: "getInfo"}, pmss.Result{
			StatusCode: http.StatusOK, SchemaChecked: true, SchemaValid: true,
			Headers: []pmss.Header{{Name: "cache-control", Value: "private, no-store"}},
		}, nil, true, "PASS", "no-store"},
		{"strong etag", openapi.Operation{ID: "getGroup"}, pmss.Result{
			StatusCode: http.StatusOK, SchemaChecked: true, SchemaValid: true,
			Headers: []pmss.Header{{Name: "etag", Value: `"g-a123"`}},
		}, nil, false, "PASS", "ETag"},
		{"weak etag", openapi.Operation{ID: "getRole"}, pmss.Result{
			StatusCode: http.StatusOK, SchemaChecked: true, SchemaValid: true,
			Headers: []pmss.Header{{Name: "ETag", Value: `W/"r-a123"`}},
		}, nil, false, "FAIL", "ETag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := evaluateResponse("test", tt.operation, tt.result, tt.err, tt.noStore)
			if got.Status != tt.wantStatus || !strings.Contains(got.Detail, tt.wantDetail) {
				t.Fatalf("evaluateResponse() = %+v, want %s containing %q", got, tt.wantStatus, tt.wantDetail)
			}
		})
	}
}

func TestValidateProblemDetailsCases(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		status      int
		body        string
		code        string
		wantError   bool
	}{
		{"valid", "application/problem+json; charset=utf-8", 401, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`, "invalid_token", false},
		{"wrong content type", "application/json", 401, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`, "invalid_token", true},
		{"invalid json", "application/problem+json", 401, `{`, "invalid_token", true},
		{"mismatched body status", "application/problem+json", 401, `{"title":"Unauthorized","status":403,"code":"invalid_token"}`, "invalid_token", true},
		{"mismatched http status", "application/problem+json", 403, `{"title":"Unauthorized","status":401,"code":"invalid_token"}`, "invalid_token", true},
		{"missing title", "application/problem+json", 401, `{"status":401,"code":"invalid_token"}`, "invalid_token", true},
		{"missing code", "application/problem+json", 401, `{"title":"Unauthorized","status":401}`, "", true},
		{"wrong code", "application/problem+json", 401, `{"title":"Unauthorized","status":401,"code":"other"}`, "invalid_token", true},
		{"any nonempty code", "application/problem+json", 401, `{"title":"Unauthorized","status":401,"code":"other"}`, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pmss.Result{
				StatusCode: tt.status,
				Headers:    []pmss.Header{{Name: "Content-Type", Value: tt.contentType}},
				Body:       tt.body,
			}
			err := validateProblemDetails(result, http.StatusUnauthorized, tt.code)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateProblemDetails() error = %v, wantError %t", err, tt.wantError)
			}
		})
	}
}

func TestChecksDataOnlyOffersOptionalCoreGETs(t *testing.T) {
	operations := []openapi.Operation{
		{ID: "getClient", Method: http.MethodGet},
		{ID: "extraGet", Method: http.MethodGet, Extra: true},
		{ID: "createClient", Method: http.MethodPost},
		{ID: "getInfo", Method: http.MethodGet},
		{ID: "healthz", Method: http.MethodGet, Extra: true},
	}
	data := checksData(config.Config{}, auth.New(config.OIDCConfig{}), operations, httptest.NewRequest(http.MethodGet, "/checks", nil))
	if len(data.Operations) != 1 || data.Operations[0].ID != "getClient" {
		t.Fatalf("optional operations = %+v, want only getClient", data.Operations)
	}
}
