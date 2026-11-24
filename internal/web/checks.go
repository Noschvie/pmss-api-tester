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
<html lang="de" data-theme="light">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="color-scheme" content="light dark">
	<title>Prüfungen · PMSS API Tester</title>
	<link rel="preload" href="/static/geist-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="preload" href="/static/geist-mono-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="stylesheet" href="/static/app.css">
	<script src="/static/app.js" defer></script>
</head>
<body>
<div class="app-shell">
	<aside class="sidebar" aria-label="Hauptnavigation">
		<a class="brand" href="/" aria-label="PMSS API Tester Startseite"><span class="brand-mark" aria-hidden="true">P</span><span class="brand-copy"><strong>PMSS</strong><small>API Tester</small></span></a>
		<div class="nav-caption">WORKSPACE</div>
		<nav>
			<a class="nav-item" href="/"><span class="nav-icon" aria-hidden="true">▦</span><span>Übersicht</span></a>
			<a class="nav-item" href="/api-explorer"><span class="nav-icon" aria-hidden="true">⌘</span><span>API Explorer</span></a>
			<a class="nav-item active" href="/checks" aria-current="page"><span class="nav-icon" aria-hidden="true">✓</span><span>Prüfungen</span></a>
		</nav>
		<div class="sidebar-bottom"><div class="sidebar-rule"></div><div class="version-label">VERSION 1 · READ-ONLY</div></div>
	</aside>
	<main class="main-content">
		<header class="topbar">
			<div class="breadcrumbs"><span>PMSS API Tester</span><span class="crumb-separator">/</span><strong>Prüfungen</strong></div>
			<div class="topbar-actions">
				{{if .Authenticated}}<span class="user-label">{{.Username}}</span><form method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><button class="logout-button" type="submit">Abmelden</button></form>{{else if .AuthEnabled}}<a class="login-button" href="/auth/login">Mit Keycloak anmelden</a>{{end}}
				<button class="theme-toggle" type="button" aria-label="Farbschema umschalten" title="Farbschema umschalten"><span class="theme-light" aria-hidden="true">☼</span><span class="theme-dark" aria-hidden="true">◐</span></button>
			</div>
		</header>
		<div class="page-body checks-body">
			<div class="page-heading"><div><div class="eyebrow">AUTOMATISIERTE CORE-CHECKS</div><h1>Prüfungen</h1><p class="subtitle">Wiederholbare, ausschließlich lesende Prüfungen gegen das konfigurierte Zielsystem.</p></div><div class="readonly-badge"><span class="lock-icon" aria-hidden="true">▣</span> Read-only</div></div>
			<section class="explorer-status">
				<div><div class="eyebrow">VORAUSSETZUNGEN</div><strong>{{if .Configured}}Zielsystem konfiguriert{{else}}Zielsystem nicht konfiguriert{{end}}</strong></div>
				<p>{{if not .Configured}}Setze <code>PMSS_API_BASE_URL</code>, um Prüfungen auszuführen.{{else if not .Authenticated}}Melde dich an. Ohne gültige Sitzung bleiben geschützte Checks BLOCKED.{{else if not .HasReadScope}}Der Token besitzt keinen <code>read</code>-Scope; geschützte Checks werden BLOCKED.{{else}}Angemeldet als <strong>{{.Username}}</strong> · Scope <code>read</code>{{end}}</p>
			</section>
			{{if .Results}}
				<section class="explorer-result" aria-live="polite">
					<div class="result-heading"><div><div class="eyebrow">PRÜFERGEBNISSE · NICHT PERSISTIERT</div><h2>{{len .Results}} Ergebnisse</h2></div></div>
					<div class="check-results">
						{{range .Results}}<article class="check-result check-{{.Status | statusClass}}"><div><strong>{{.Name}}</strong><span class="check-status">{{.Status}}</span></div><p>{{.Detail}}</p></article>{{end}}
					</div>
				</section>
			{{end}}
			<form method="post" action="/checks/run" class="checks-form">
				<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
				<section class="operation-section">
					<div class="section-title"><div><div class="eyebrow">OPTIONAL · CORE API</div><h2>GET-Endpunkte auswählen</h2></div></div>
					<p class="checks-hint">Erreichbarkeit, anonymer Authentifizierungstest, read-Scope und <code>/api/v1/info</code> werden immer geprüft. Pfadwerte bleiben lokal im Arbeitsspeicher; ohne erforderlichen Wert wird der Endpunkt als NOT TESTED markiert. Schreibende Operationen und PMSS Extra sind ausgeschlossen.</p>
					{{range .Operations}}
						{{$operation := .}}
						<article class="operation-card check-operation">
							<label class="check-operation-title"><input type="checkbox" name="operation_id" value="{{.ID}}" {{if index $.Selected .ID}}checked{{end}}><span><span class="method-badge method-GET">GET</span><code class="operation-path">{{.Path}}</code><strong>{{.Summary}}</strong></span></label>
							{{if .Parameters}}<div class="parameter-grid">{{range .Parameters}}{{$name := operationInputName $operation.ID .FormKey}}<label class="parameter-field" for="{{operationInputID $operation.ID .FormKey}}"><span>{{.Name}} <small>{{.In}}{{if .Required}} · erforderlich{{end}}</small></span><input id="{{operationInputID $operation.ID .FormKey}}" name="{{$name}}" type="text" value="{{index $.Values $name}}" placeholder="{{if .Required}}Wert eingeben{{else}}Optional{{end}}">{{if isPath .In}}<small>Sonderzeichen wie Leerzeichen testen die URL-Kodierung.</small>{{end}}</label>{{end}}</div>{{end}}
						</article>
					{{else}}<section class="info-panel"><p>Keine ausführbaren Core-GET-Endpunkte vorhanden.</p></section>{{end}}
				</section>
				<button class="login-button execute-button" type="submit" {{if not .Configured}}disabled{{end}}>Read-only-Prüfungen starten</button>
			</form>
			<footer class="page-footer">PMSS API Tester <span>·</span> Version 1 <span>·</span> Ergebnisse werden nicht gespeichert</footer>
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
		data.Results = []checkResult{{Name: "Prüfanfrage", Status: "FAIL", Detail: fmt.Sprintf("Ungültige Anfrage: %v", err)}}
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
		data.Results = []checkResult{{Name: "CSRF-Prüfung", Status: "BLOCKED", Detail: "CSRF-Token fehlt oder ist ungültig. Lade die Seite neu."}}
		renderChecks(w, data, http.StatusForbidden)
		return
	}
	if client == nil {
		data.Results = []checkResult{
			{Name: "Zielsystem", Status: "BLOCKED", Detail: "Das PMSS-Zielsystem ist nicht konfiguriert."},
			{Name: "Geschützte Core-Checks", Status: "BLOCKED", Detail: "Ohne Zielsystem wurden keine API-Aufrufe ausgeführt."},
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
			checkResult{Name: "read-Berechtigung", Status: "BLOCKED", Detail: "Nicht angemeldet; geschützte API-Aufrufe wurden nicht ausgeführt."},
			checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "Eine angemeldete Sitzung mit read-Scope ist erforderlich."},
		)
		for _, operation := range data.Operations {
			if data.Selected[operation.ID] {
				data.Results = append(data.Results, checkResult{Name: operation.Method + " " + operation.Path, Status: "BLOCKED", Detail: "Eine angemeldete Sitzung mit read-Scope ist erforderlich."})
			}
		}
		renderChecks(w, data, http.StatusOK)
		return
	}
	if !session.HasReadScope {
		data.Results = append(data.Results,
			checkResult{Name: "read-Berechtigung", Status: "BLOCKED", Detail: "Der Token-Austausch hat keinen read-Scope geliefert."},
			checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "Ohne read-Scope wird kein geschützter Request gesendet."},
		)
		for _, operation := range data.Operations {
			if data.Selected[operation.ID] {
				data.Results = append(data.Results, checkResult{Name: operation.Method + " " + operation.Path, Status: "BLOCKED", Detail: "Ohne read-Scope wird kein geschützter Request gesendet."})
			}
		}
		renderChecks(w, data, http.StatusOK)
		return
	}
	data.Results = append(data.Results, checkResult{Name: "read-Berechtigung", Status: "PASS", Detail: "Der serverseitige Token-Austausch enthält den erforderlichen read-Scope."})
	token, ok := authService.PMSSAccessToken(r)
	if !ok {
		data.Results = append(data.Results, checkResult{Name: "GET /api/v1/info", Status: "BLOCKED", Detail: "Der PMSS-Zugriffstoken ist nicht verfügbar."})
	} else {
		info := operationsByID["getInfo"]
		result, err := client.Execute(ctx, info, nil, token)
		infoResult := evaluateResponse("GET /api/v1/info", info, result, err, true)
		data.Results = append(data.Results, infoResult)
		if infoResult.Status == "PASS" {
			data.Results = append(data.Results, evaluateAPIVersion(result.Body))
		} else {
			data.Results = append(data.Results, checkResult{Name: "API-Version", Status: "BLOCKED", Detail: "Die API-Version konnte ohne erfolgreiche /api/v1/info-Antwort nicht geprüft werden."})
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
			data.Results = append(data.Results, checkResult{Name: "GET " + operation.Path, Status: "NOT TESTED", Detail: "Mindestens ein erforderlicher Pfad- oder Query-Parameter fehlt."})
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
				data.Results = append(data.Results, checkResult{Name: "URL-Kodierung " + operation.Path, Status: "NOT TESTED", Detail: "Der Pfadwert enthält keine Zeichen, die kodiert werden müssen. Verwende z. B. ein Leerzeichen für diesen Check."})
			} else if err == nil && result.RequestURL != "" && strings.Contains(result.RequestURL, escaped) {
				data.Results = append(data.Results, checkResult{Name: "URL-Kodierung " + operation.Path, Status: "PASS", Detail: "Der Pfadparameter wurde als einzelnes URL-Segment kodiert."})
			} else {
				data.Results = append(data.Results, checkResult{Name: "URL-Kodierung " + operation.Path, Status: "FAIL", Detail: "Der kodierte Pfadparameter ist in der gesendeten Request-URL nicht vorhanden."})
			}
		}
	}
	renderChecks(w, data, http.StatusOK)
}

func runHealthCheck(ctx context.Context, client *pmss.Client) checkResult {
	operation := openapi.Operation{ID: "healthz", Method: http.MethodGet, Path: "/healthz"}
	result, err := client.ExecuteWithoutToken(ctx, operation, nil)
	if err != nil {
		return checkResult{Name: "Erreichbarkeit · GET /healthz", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Erreichbarkeit · GET /healthz", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusOK {
		return checkResult{Name: "Erreichbarkeit · GET /healthz", Status: "FAIL", Detail: fmt.Sprintf("Erwartet HTTP 200, erhalten HTTP %d.", result.StatusCode)}
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(result.Body), &body); err != nil || body.Status != "ok" {
		return checkResult{Name: "Erreichbarkeit · GET /healthz", Status: "FAIL", Detail: "HTTP 200 erhalten, aber der JSON-Body enthält nicht status=ok."}
	}
	return checkResult{Name: "Erreichbarkeit · GET /healthz", Status: "PASS", Detail: "HTTP 200 und JSON status=ok."}
}

func runAuthenticationCheck(ctx context.Context, client *pmss.Client, info openapi.Operation) checkResult {
	result, err := client.ExecuteWithoutToken(ctx, info, nil)
	if err != nil {
		return checkResult{Name: "Authentifizierung · Token fehlt", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Authentifizierung · Token fehlt", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusUnauthorized {
		return checkResult{Name: "Authentifizierung · Token fehlt", Status: "FAIL", Detail: fmt.Sprintf("Erwartet HTTP 401 ohne Token, erhalten HTTP %d.", result.StatusCode)}
	}
	if err := validateProblemDetails(result, http.StatusUnauthorized, "invalid_token"); err != nil {
		return checkResult{Name: "Authentifizierung · Token fehlt", Status: "FAIL", Detail: err.Error()}
	}
	return checkResult{Name: "Authentifizierung · Token fehlt", Status: "PASS", Detail: "Anonymer Request wurde mit HTTP 401 und RFC 9457 invalid_token abgewiesen."}
}

func runInvalidTokenCheck(ctx context.Context, client *pmss.Client, info openapi.Operation) checkResult {
	result, err := client.Execute(ctx, info, nil, "pmss-api-tester-invalid-token")
	if err != nil {
		return checkResult{Name: "Authentifizierung · Ungültiger Token", Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: "Authentifizierung · Ungültiger Token", Status: "FAIL", Detail: result.Error}
	}
	if result.StatusCode != http.StatusUnauthorized {
		return checkResult{Name: "Authentifizierung · Ungültiger Token", Status: "FAIL", Detail: fmt.Sprintf("Erwartet HTTP 401 mit ungültigem Token, erhalten HTTP %d.", result.StatusCode)}
	}
	if err := validateProblemDetails(result, http.StatusUnauthorized, "invalid_token"); err != nil {
		return checkResult{Name: "Authentifizierung · Ungültiger Token", Status: "FAIL", Detail: err.Error()}
	}
	return checkResult{Name: "Authentifizierung · Ungültiger Token", Status: "PASS", Detail: "Ungültiger Bearer-Token wurde mit HTTP 401 und RFC 9457 invalid_token abgewiesen."}
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
		return checkResult{Name: "API-Version", Status: "FAIL", Detail: fmt.Sprintf("Info-Antwort konnte nicht gelesen werden: %v", err)}
	}
	parts := strings.Split(info.APIVersion, ".")
	if len(parts) != 2 || parts[0] != "1" {
		return checkResult{Name: "API-Version", Status: "FAIL", Detail: fmt.Sprintf("Nicht unterstützte API-Version %q; erwartet wird Hauptversion 1.", info.APIVersion)}
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return checkResult{Name: "API-Version", Status: "FAIL", Detail: fmt.Sprintf("API-Version %q ist nicht MAJOR.MINOR.", info.APIVersion)}
	}
	implementation := strings.TrimSpace(info.Implementation.Name + " " + info.Implementation.Version)
	if implementation == "" {
		implementation = "unbekannte Implementierung"
	}
	return checkResult{Name: "API-Version", Status: "PASS", Detail: fmt.Sprintf("API-Version %s · Implementierung %s.", info.APIVersion, implementation)}
}

func evaluateResponse(name string, operation openapi.Operation, result pmss.Result, err error, checkInfoHeaders bool) checkResult {
	if err != nil {
		return checkResult{Name: name, Status: "FAIL", Detail: err.Error()}
	}
	if result.Error != "" {
		return checkResult{Name: name, Status: "FAIL", Detail: "Request fehlgeschlagen: " + result.Error}
	}
	if result.StatusCode == http.StatusServiceUnavailable {
		if problemErr := validateProblemDetails(result, http.StatusServiceUnavailable, "broker_unavailable"); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: "503 broker_unavailable: " + problemErr.Error()}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: "503 broker_unavailable: Der Broker war nicht erreichbar; der Check ist fehlgeschlagen."}
	}
	if result.StatusCode == http.StatusGatewayTimeout {
		if problemErr := validateProblemDetails(result, http.StatusGatewayTimeout, "broker_timeout"); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: "504 broker_timeout: " + problemErr.Error()}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: "504 broker_timeout: Broker-Zustand unbekannt. Es wurde kein automatischer Retry ausgeführt."}
	}
	if result.StatusCode == http.StatusNotFound && expectsNotFound(operation) {
		if problemErr := validateProblemDetails(result, http.StatusNotFound, ""); problemErr != nil {
			return checkResult{Name: name, Status: "FAIL", Detail: problemErr.Error()}
		}
		return checkResult{Name: name, Status: "PASS", Detail: "Erwartetes HTTP 404 mit konsistenten RFC 9457 Problem Details."}
	}
	if result.StatusCode < 200 || result.StatusCode >= 300 {
		detail := fmt.Sprintf("Unerwarteter HTTP-Status %d.", result.StatusCode)
		if strings.HasPrefix(strings.ToLower(contentType(result.Headers)), "application/problem+json") {
			if err := validateProblemDetails(result, result.StatusCode, ""); err != nil {
				detail += " RFC 9457: " + err.Error()
			} else {
				detail += " RFC 9457 Problem Details sind konsistent."
			}
		}
		return checkResult{Name: name, Status: "FAIL", Detail: detail}
	}
	if result.SchemaError != "" {
		return checkResult{Name: name, Status: "FAIL", Detail: "Antwortschema ungültig: " + result.SchemaError}
	}
	if !result.SchemaChecked {
		return checkResult{Name: name, Status: "NOT TESTED", Detail: "HTTP-Status erfolgreich, aber für diesen Status ist kein Response-Schema definiert."}
	}
	if !result.SchemaValid {
		return checkResult{Name: name, Status: "FAIL", Detail: "Die Antwort entspricht nicht dem OpenAPI-Response-Schema."}
	}
	if checkInfoHeaders {
		if !hasHeader(result.Headers, "Cache-Control", "no-store") {
			return checkResult{Name: name, Status: "FAIL", Detail: "Response-Schema gültig, aber der vorgeschriebene Cache-Control: no-store Header fehlt."}
		}
	}
	if requiresETag(operation) {
		etag := headerValue(result.Headers, "ETag")
		if !strongETag.MatchString(etag) {
			return checkResult{Name: name, Status: "FAIL", Detail: "Response-Schema gültig, aber der erforderliche starke ETag-Header fehlt oder hat ein ungültiges Format."}
		}
	}
	return checkResult{Name: name, Status: "PASS", Detail: fmt.Sprintf("HTTP %d, JSON-Response-Schema gültig%s.", result.StatusCode, headerCheckDetail(operation, checkInfoHeaders))}
}

func validateProblemDetails(result pmss.Result, expectedStatus int, expectedCode string) error {
	contentTypeValue := contentType(result.Headers)
	mediaType, _, err := mime.ParseMediaType(contentTypeValue)
	if err != nil || mediaType != "application/problem+json" {
		return fmt.Errorf("Content-Type ist %q statt application/problem+json", contentTypeValue)
	}
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(result.Body), &problem); err != nil {
		return fmt.Errorf("Problem-Body ist kein gültiges JSON: %w", err)
	}
	if problem.Status != expectedStatus || result.StatusCode != expectedStatus {
		return fmt.Errorf("Problem-Status %d stimmt nicht mit HTTP %d überein (erwartet %d)", problem.Status, result.StatusCode, expectedStatus)
	}
	if problem.Title == "" || problem.Code == "" {
		return fmt.Errorf("Problem Details enthalten kein title- oder code-Feld")
	}
	if expectedCode != "" && problem.Code != expectedCode {
		return fmt.Errorf("Problem-Code %q statt %q", problem.Code, expectedCode)
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
		return " und gültigem ETag"
	}
	if info {
		return " und Cache-Control: no-store"
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
