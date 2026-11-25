// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/noschvie/pmss-api-tester/internal/auth"
	"github.com/noschvie/pmss-api-tester/internal/config"
	"github.com/noschvie/pmss-api-tester/internal/openapi"
	"github.com/noschvie/pmss-api-tester/internal/pmss"
)

type checkPageData struct {
	Configured    bool
	AuthEnabled   bool
	Authenticated bool
	HasReadScope  bool
	Username      string
	CSRFToken     string
	Operations    []openapi.Operation
	Values        map[string]string
	Selected      map[string]bool
	Results       []checkResult
}

type checkResult struct {
	Name   string
	Status string
	Detail string
}

var checksTemplate = template.Must(template.New("checks").Funcs(template.FuncMap{
	"operationInputName": func(operationID, formKey string) string {
		return checkParameterKey(operationID, formKey)
	},
	"operationInputID": func(operationID, formKey string) string {
		return "input_" + operationID + "_" + formKey
	},
	"isPath": func(location string) bool { return location == "path" },
	"statusClass": func(status string) string { return strings.ReplaceAll(strings.ToLower(status), " ", "-") },
}).Parse(`<!doctype html>
<html lang="en" data-theme="light">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="color-scheme" content="light dark">
	<title>Checks · PMSS API Tester</title>
	<link rel="preload" href="/static/geist-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="preload" href="/static/geist-mono-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="stylesheet" href="/static/app.css">
	<script src="/static/app.js" defer></script>
</head>
<body>
<div class="app-shell">
	<aside class="sidebar" aria-label="Main navigation">
		<a class="brand" href="/" aria-label="PMSS API Tester home"><span class="brand-mark" aria-hidden="true">P</span><span class="brand-copy"><strong>PMSS</strong><small>API Tester</small></span></a>
		<div class="nav-caption">WORKSPACE</div>
		<nav>
			<a class="nav-item" href="/"><span class="nav-icon" aria-hidden="true">▦</span><span>Overview</span></a>
			<a class="nav-item" href="/api-explorer"><span class="nav-icon" aria-hidden="true">⌘</span><span>API Explorer</span></a>
			<a class="nav-item active" href="/checks" aria-current="page"><span class="nav-icon" aria-hidden="true">✓</span><span>Checks</span></a>
		</nav>
		<div class="sidebar-bottom"><div class="sidebar-rule"></div><div class="version-label">VERSION 1 · READ-ONLY</div></div>
	</aside>
	<main class="main-content">
		<header class="topbar">
			<div class="breadcrumbs"><span>PMSS API Tester</span><span class="crumb-separator">/</span><strong>Checks</strong></div>
			<div class="topbar-actions">
				{{if .Authenticated}}<span class="user-label">{{.Username}}</span><form method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><button class="logout-button" type="submit">Log out</button></form>{{else if .AuthEnabled}}<a class="login-button" href="/auth/login">Sign in with Keycloak</a>{{end}}
				<button class="theme-toggle" type="button" aria-label="Toggle color scheme" title="Toggle color scheme"><span class="theme-light" aria-hidden="true">☼</span><span class="theme-dark" aria-hidden="true">◐</span></button>
			</div>
		</header>
		<div class="page-body checks-body">
			<div class="page-heading"><div><div class="eyebrow">AUTOMATED CORE CHECKS</div><h1>Checks</h1><p class="subtitle">Repeatable, read-only checks against the configured target system.</p></div><div class="readonly-badge"><span class="lock-icon" aria-hidden="true">▣</span> Read-only</div></div>
			<section class="explorer-status">
				<div><div class="eyebrow">REQUIREMENTS</div><strong>{{if .Configured}}Target system configured{{else}}Target system not configured{{end}}</strong></div>
				<p>{{if not .Configured}}Set <code>PMSS_API_BASE_URL</code> to run checks.{{else if not .Authenticated}}Sign in. Protected checks remain BLOCKED without a valid session.{{else if not .HasReadScope}}The token does not have the <code>read</code> scope; protected checks are BLOCKED.{{else}}Signed in as <strong>{{.Username}}</strong> · Scope <code>read</code>{{end}}</p>
			</section>
			{{if .Results}}
				<section class="explorer-result" aria-live="polite">
					<div class="result-heading"><div><div class="eyebrow">CHECK RESULTS · NOT PERSISTED</div><h2>{{len .Results}} results</h2></div></div>
					<div class="check-results">
						{{range .Results}}<article class="check-result check-{{.Status | statusClass}}"><div><strong>{{.Name}}</strong><span class="check-status">{{.Status}}</span></div><p>{{.Detail}}</p></article>{{end}}
					</div>
				</section>
			{{end}}
			<form method="post" action="/checks/run" class="checks-form">
				<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
				<section class="operation-section">
					<div class="section-title"><div><div class="eyebrow">OPTIONAL · CORE API</div><h2>Select GET endpoints</h2></div></div>
					<p class="checks-hint">Availability, anonymous authentication, the <code>read</code> scope, and <code>/api/v1/info</code> are always checked. Path values remain in local memory; endpoints missing a required value are marked NOT TESTED. Write operations and PMSS Extra are excluded.</p>
					{{range .Operations}}
						{{$operation := .}}
						<article class="operation-card check-operation">
							<label class="check-operation-title"><input type="checkbox" name="operation_id" value="{{.ID}}" {{if index $.Selected .ID}}checked{{end}}><span><span class="method-badge method-GET">GET</span><code class="operation-path">{{.Path}}</code><strong>{{.Summary}}</strong></span></label>
							{{if .Parameters}}<div class="parameter-grid">{{range .Parameters}}{{$name := operationInputName $operation.ID .FormKey}}<label class="parameter-field" for="{{operationInputID $operation.ID .FormKey}}"><span>{{.Name}} <small>{{.In}}{{if .Required}} · required{{end}}</small></span><input id="{{operationInputID $operation.ID .FormKey}}" name="{{$name}}" type="text" value="{{index $.Values $name}}" placeholder="{{if .Required}}Enter a value{{else}}Optional{{end}}">{{if isPath .In}}<small>Special characters such as spaces test URL encoding.</small>{{end}}</label>{{end}}</div>{{end}}
						</article>
					{{else}}<section class="info-panel"><p>No executable Core GET endpoints available.</p></section>{{end}}
				</section>
				<button class="login-button execute-button" type="submit" {{if not .Configured}}disabled{{end}}>Run read-only checks</button>
			</form>
			<footer class="page-footer">PMSS API Tester <span>·</span> Version 1 <span>·</span> Results are not saved</footer>
		</div>
	</main>
</div>
</body>
</html>`))

func checksData(cfg config.Config, authService *auth.Service, operations []openapi.Operation, request *http.Request) checkPageData {
	data := checkPageData{
		Configured: cfg.PMSSAPIBaseURL != "",
		AuthEnabled: authService.Enabled(),
		Values:     make(map[string]string),
		Selected:   make(map[string]bool),
	}
	for _, operation := range operations {
		if !operation.Extra && operation.Method == http.MethodGet && operation.ID != "healthz" && operation.ID != "getInfo" {
			data.Operations = append(data.Operations, operation)
		}
	}
	if session, _ := authService.Session(request); session != nil {
		data.Authenticated = true
		data.HasReadScope = session.HasReadScope
		data.Username = session.Username
		data.CSRFToken = session.CSRFToken
	}
	return data
}

func renderChecks(w http.ResponseWriter, data checkPageData, status int) {
	var body bytes.Buffer
	if err := checksTemplate.Execute(&body, data); err != nil {
		slog.Error("automated checks rendering failed", "error", err)
		http.Error(w, "Automated checks rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if status != 0 {
		w.WriteHeader(status)
	}
	if _, err := w.Write(body.Bytes()); err != nil {
		slog.Error("write automated checks response", "error", err)
	}
}

var strongETag = regexp.MustCompile(`^"[cgr]-[0-9a-f]+"$`)

func handleChecksRun(
	w http.ResponseWriter,
	r *http.Request,
	cfg config.Config,
	authService *auth.Service,
	client *pmss.Client,
	operations []openapi.Operation,
	operationsByID map[string]openapi.Operation,
) {
	data := checksData(cfg, authService, operations, r)
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	if err := r.ParseForm(); err != nil {
		data.Results = []checkResult{{Name: "Check request", Status: "FAIL", Detail: fmt.Sprintf("Invalid request: %v", err)}}
		renderChecks(w, data, http.StatusBadRequest)
		return
	}
	for _, operationID := range r.PostForm["operation_id"] {
		if operation, ok := operationsByID[operationID]; ok && !operation.Extra && operation.Method == http.MethodGet && operation.ID != "healthz" && operation.ID != "getInfo" {
			data.Selected[operationID] = true
			for _, parameter := range operation.Parameters {
				key := checkParameterKey(operationID, parameter.FormKey)
				data.Values[key] = r.PostForm.Get(key)
			}
		}
	}
	session, _ := authService.Session(r)
	if session != nil && !authService.ValidateCSRF(r, r.PostForm.Get("csrf_token")) {
		data.Results = []checkResult{{Name: "CSRF check", Status: "BLOCKED", Detail: "CSRF token is missing or invalid. Reload the page."}}
		renderChecks(w, data, http.StatusForbidden)
		return
	}
	if client == nil {
		data.Results = []checkResult{
			{Name: "Target system", Status: "BLOCKED", Detail: "The PMSS target system is not configured."},
			{Name: "Protected Core checks", Status: "BLOCKED", Detail: "No API requests were made because no target system is configured."},
		}
		renderChecks(w, data, http.StatusOK)
		return
	}

	ctx := r.Context()
	data.Results = append(data.Results, runHealthCheck(ctx, client))
	authResult := runAuthenticationCheck(ctx, client, operationsByID["getInfo"])
	data.Results = append(data.Results, authResult)
	data.Results = append(data.Results, runInvalidTokenCheck(ctx, client, operationsByID["getInfo"]))

	if session == nil {
		data.Results = append(data.Results,
			checkResult{Name: "read permission", Status: "BLOCKED", Detail: "Not signed in; no protected API requests were made."},
			checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "A signed-in session with the read scope is required."},
		)
		for _, operation := range data.Operations {
			if data.Selected[operation.ID] {
				data.Results = append(data.Results, checkResult{Name: operation.Method + " " + operation.Path, Status: "BLOCKED", Detail: "A signed-in session with the read scope is required."})
			}
		}
		renderChecks(w, data, http.StatusOK)
		return
	}
	if !session.HasReadScope {
		data.Results = append(data.Results,
			checkResult{Name: "read permission", Status: "BLOCKED", Detail: "Token exchange did not return the read scope."},
			checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "No protected request is sent without the read scope."},
		)
		for _, operation := range data.Operations {
			if data.Selected[operation.ID] {
				data.Results = append(data.Results, checkResult{Name: operation.Method + " " + operation.Path, Status: "BLOCKED", Detail: "No protected request is sent without the read scope."})
			}
		}
		renderChecks(w, data, http.StatusOK)
		return
	}
	data.Results = append(data.Results, checkResult{Name: "read permission", Status: "PASS", Detail: "The server-side token exchange includes the required read scope."})
	token, ok := authService.PMSSAccessToken(r)
	if !ok {
		data.Results = append(data.Results, checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "The PMSS access token is not available."})
	} else {
		info := operationsByID["getInfo"]
		result, err := client.Execute(ctx, info, nil, token)
		infoResult := evaluateResponse("GET /api/v1/info", info, result, err, true)
		data.Results = append(data.Results, infoResult)
		if infoResult.Status == "PASS" {
			data.Results = append(data.Results, evaluateAPIVersion(result.Body))
		} else {
			data.Results = append(data.Results, checkResult{Name: "API version", Status: "BLOCKED", Detail: "The API version could not be checked without a successful /api/v1/info response."})
		}
	}
	for _, operation := range data.Operations {
		operationID := operation.ID
		if !data.Selected[operationID] {
			continue
		}
		values := make(url.Values, len(operation.Parameters))
		missingRequired := false
		for _, parameter := range operation.Parameters {
			value := data.Values[checkParameterKey(operationID, parameter.FormKey)]
			if parameter.Required && strings.TrimSpace(value) == "" {
				missingRequired = true
			}
			if value != "" {
				values.Set(parameter.FormKey, value)
			}
		}
		if missingRequired {
			data.Results = append(data.Results, checkResult{Name: "GET " + operation.Path, Status: "NOT TESTED", Detail: "At least one required path or query parameter is missing."})
			continue
		}
		result, err := client.Execute(ctx, operation, values, token)
		data.Results = append(data.Results, evaluateResponse("GET "+operation.Path, operation, result, err, requiresNoStore(operation)))
		for _, parameter := range operation.Parameters {
			value := values.Get(parameter.FormKey)
			if parameter.In != "path" || value == "" {
				continue
			}
			escaped := url.PathEscape(value)
			if escaped == value {
				data.Results = append(data.Results, checkResult{Name: "URL encoding " + operation.Path, Status: "NOT TESTED", Detail: "The path value contains no characters that need encoding. For example, use a space for this check."})
			} else if err == nil && result.RequestURL != "" && strings.Contains(result.RequestURL, escaped) {
				data.Results = append(data.Results, checkResult{Name: "URL encoding " + operation.Path, Status: "PASS", Detail: "The path parameter was encoded as a single URL segment."})
			} else {
				data.Results = append(data.Results, checkResult{Name: "URL encoding " + operation.Path, Status: "FAIL", Detail: "The encoded path parameter is not present in the request URL."})
			}
		}
	}
	renderChecks(w, data, http.StatusOK)
}

func runHealthCheck(ctx context.Context, client *pmss.Client) checkResult {
	operation := openapi.Operation{ID: "healthz", Method: http.MethodGet, Path: "/healthz"}
	result, err := client.ExecuteWithoutToken(ctx, operation, nil)
	if err != nil {
		return checkResult{Name: "Availability · GET /healthz", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Availability · GET /healthz", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusOK {
		return checkResult{Name: "Availability · GET /healthz", Status: "FAIL", Detail: fmt.Sprintf("Expected HTTP 200, received HTTP %d.", result.StatusCode)}
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Body), &body); err != nil || body.Status != "ok" {
		return checkResult{Name: "Availability · GET /healthz", Status: "FAIL", Detail: "Received HTTP 200, but the JSON body does not contain status=ok."}
	}
	return checkResult{Name: "Availability · GET /healthz", Status: "PASS", Detail: "HTTP 200 and JSON status=ok."}
}

func runAuthenticationCheck(ctx context.Context, client *pmss.Client, info openapi.Operation) checkResult {
	result, err := client.ExecuteWithoutToken(ctx, info, nil)
	if err != nil {
		return checkResult{Name: "Authentication · Missing token", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Authentication · Missing token", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusUnauthorized {
		return checkResult{Name: "Authentication · Missing token", Status: "FAIL", Detail: fmt.Sprintf("Expected HTTP 401 without a token, received HTTP %d.", result.StatusCode)}
	}
	if err := validateProblemDetails(result, http.StatusUnauthorized, "invalid_token"); err != nil {
		return checkResult{Name: "Authentication · Missing token", Status: "FAIL", Detail: err.Error()}
	}
	return checkResult{Name: "Authentication · Missing token", Status: "PASS", Detail: "The anonymous request was rejected with HTTP 401 and RFC 9457 invalid_token."}
}

func runInvalidTokenCheck(ctx context.Context, client *pmss.Client, info openapi.Operation) checkResult {
	result, err := client.Execute(ctx, info, nil, "pmss-api-tester-invalid-token")
	if err != nil {
		return checkResult{Name: "Authentication · Invalid token", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Authentication · Invalid token", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusUnauthorized {
		return checkResult{Name: "Authentication · Invalid token", Status: "FAIL", Detail: fmt.Sprintf("Expected HTTP 401 with an invalid token, received HTTP %d.", result.StatusCode)}
	}
	if err := validateProblemDetails(result, http.StatusUnauthorized, "invalid_token"); err != nil {
		return checkResult{Name: "Authentication · Invalid token", Status: "FAIL", Detail: err.Error()}
	}
	return checkResult{Name: "Authentication · Invalid token", Status: "PASS", Detail: "The invalid bearer token was rejected with HTTP 401 and RFC 9457 invalid_token."}
}

func evaluateAPIVersion(body string) checkResult {
	var info struct {
		APIVersion    string `json:"apiVersion"`
		Implementation struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"implementation"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		return checkResult{Name: "API version", Status: "FAIL", Detail: fmt.Sprintf("Could not read the info response: %v", err)}
	}
	parts := strings.Split(info.APIVersion, ".")
	if len(parts) != 2 || parts[0] != "1" {
		return checkResult{Name: "API version", Status: "FAIL", Detail: fmt.Sprintf("Unsupported API version %q; expected major version 1.", info.APIVersion)}
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return checkResult{Name: "API version", Status: "FAIL", Detail: fmt.Sprintf("API version %q is not in MAJOR.MINOR format.", info.APIVersion)}
	}
	implementation := strings.TrimSpace(info.Implementation.Name + " " + info.Implementation.Version)
	if implementation == "" {
		implementation = "unknown implementation"
	}
	return checkResult{Name: "API version", Status: "PASS", Detail: fmt.Sprintf("API version %s · implementation %s.", info.APIVersion, implementation)}
}

func evaluateResponse(name string, operation openapi.Operation, result pmss.Result, err error, checkInfoHeaders bool) checkResult {
	if err != nil {
		return checkResult{Name: name, Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: name, Status: "FAIL", Detail: "Request failed: " + result.Error}
	}
	if result.StatusCode == http.StatusServiceUnavailable {
		if problemErr := validateProblemDetails(result, http.StatusServiceUnavailable, "broker_unavailable"); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: "503 broker_unavailable: " + problemErr.Error()}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: "503 broker_unavailable: The broker was unreachable; the check failed."}
	}
	if result.StatusCode == http.StatusGatewayTimeout {
		if problemErr := validateProblemDetails(result, http.StatusGatewayTimeout, "broker_timeout"); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: "504 broker_timeout: " + problemErr.Error()}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: "504 broker_timeout: Broker state is unknown. No automatic retry was performed."}
	}
	if result.StatusCode == http.StatusNotFound && expectsNotFound(operation) {
		if problemErr := validateProblemDetails(result, http.StatusNotFound, ""); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: problemErr.Error()}
		}
		return checkResult{Name: name, Status: "PASS", Detail: "Received the expected HTTP 404 with consistent RFC 9457 Problem Details."}
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		detail := fmt.Sprintf("Unexpected HTTP status %d.", result.StatusCode)
		if strings.HasPrefix(strings.ToLower(contentType(result.Headers)), "application/problem+json") {
			if err := validateProblemDetails(result, result.StatusCode, ""); err != nil {
				detail += " RFC 9457: " + err.Error()
			} else {
				detail += " RFC 9457 Problem Details are consistent."
			}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: detail}
	}
	if result.SchemaError != "" {
		return checkResult{Name: name, Status: "FAIL", Detail: "Invalid response schema: " + result.SchemaError}
	}
	if !result.SchemaChecked {
		return checkResult{Name: name, Status: "NOT TESTED", Detail: "HTTP status was successful, but no response schema is defined for this status."}
	}
	if !result.SchemaValid {
		return checkResult{Name: name, Status: "FAIL", Detail: "The response does not match the OpenAPI response schema."}
	}
	if checkInfoHeaders {
		if !hasHeader(result.Headers, "Cache-Control", "no-store") {
			return checkResult{Name: name, Status: "FAIL", Detail: "Response schema is valid, but the required Cache-Control: no-store header is missing."}
		}
	}
	if requiresETag(operation) {
		etag := headerValue(result.Headers, "ETag")
		if !strongETag.MatchString(etag) {
			return checkResult{Name: name, Status: "FAIL", Detail: "Response schema is valid, but the required strong ETag header is missing or has an invalid format."}
		}
	}
	return checkResult{Name: name, Status: "PASS", Detail: fmt.Sprintf("HTTP %d, JSON response schema is valid%s.", result.StatusCode, headerCheckDetail(operation, checkInfoHeaders))}
}

func validateProblemDetails(result pmss.Result, expectedStatus int, expectedCode string) error {
	contentTypeValue := contentType(result.Headers)
	mediaType, _, err := mime.ParseMediaType(contentTypeValue)
	if err != nil || mediaType != "application/problem+json" {
		return fmt.Errorf("Content-Type is %q instead of application/problem+json", contentTypeValue)
	}
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(result.Body), &problem); err != nil {
		return fmt.Errorf("problem body is not valid JSON: %w", err)
	}
	if problem.Status != expectedStatus || result.StatusCode != expectedStatus {
		return fmt.Errorf("problem status %d does not match HTTP %d (expected %d)", problem.Status, result.StatusCode, expectedStatus)
	}
	if problem.Title == "" || problem.Code == "" {
		return fmt.Errorf("Problem Details do not contain a title or code field")
	}
	if expectedCode != "" && problem.Code != expectedCode {
		return fmt.Errorf("problem code %q instead of %q", problem.Code, expectedCode)
	}
	return nil
}

func requiresETag(operation openapi.Operation) bool {
	return operation.ID == "getClient" || operation.ID == "getGroup" || operation.ID == "getRole"
}

func requiresNoStore(operation openapi.Operation) bool {
	return operation.ID == "getInfo" || operation.ID == "getExport"
}

func expectsNotFound(operation openapi.Operation) bool {
	return requiresETag(operation)
}

func headerCheckDetail(operation openapi.Operation, info bool) string {
	if requiresETag(operation) {
		return " and a valid ETag"
	}
	if info {
		return " and Cache-Control: no-store"
	}
	return ""
}

func contentType(headers []pmss.Header) string {
	return headerValue(headers, "Content-Type")
}

func headerValue(headers []pmss.Header, name string) string {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func hasHeader(headers []pmss.Header, name, expected string) bool {
	for _, header := range headers {
		if strings.EqualFold(header.Name, name) && strings.Contains(strings.ToLower(header.Value), strings.ToLower(expected)) {
			return true
		}
	}
	return false
}

func checkParameterKey(operationID, formKey string) string {
	return "param_" + operationID + "_" + formKey
}
