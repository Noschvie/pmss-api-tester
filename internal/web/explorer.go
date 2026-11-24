package web

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noschvie/pmss-api-tester/internal/auth"
	"github.com/noschvie/pmss-api-tester/internal/config"
	"github.com/noschvie/pmss-api-tester/internal/openapi"
	"github.com/noschvie/pmss-api-tester/internal/pmss"
)

type explorerPageData struct {
	TargetURL         string
	InsecureTarget    bool
	Configured        bool
	AuthEnabled       bool
	Authenticated     bool
	HasReadScope      bool
	Username          string
	ScopeLabel        string
	CSRFToken         string
	CoreOperations    []openapi.Operation
	ExtraOperations   []openapi.Operation
	SelectedOperation string
	Values            map[string]string
	Result            *explorerResult
}

type explorerResult struct {
	StatusText    string
	StatusCode    int
	RequestURL    string
	Duration      string
	Headers       []pmss.Header
	Body          string
	BodyTruncated bool
	SchemaChecked bool
	SchemaValid   bool
	SchemaError   string
	Error         string
}

func explorerTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"dict": func(values ...any) (map[string]any, error) {
			if len(values)%2 != 0 {
				return nil, errors.New("dict requires key/value pairs")
			}
			result := make(map[string]any, len(values)/2)
			for index := 0; index < len(values); index += 2 {
				key, ok := values[index].(string)
				if !ok {
					return nil, errors.New("dict keys must be strings")
				}
				result[key] = values[index+1]
			}
			return result, nil
		},
	}
}

var explorerTemplate = template.Must(template.New("api-explorer").Funcs(explorerTemplateFuncs()).Parse(`<!doctype html>
<html lang="de" data-theme="light">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<meta name="color-scheme" content="light dark">
	<title>API Explorer · PMSS API Tester</title>
	<link rel="preload" href="/static/geist-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="preload" href="/static/geist-mono-latin.woff2" as="font" type="font/woff2" crossorigin>
	<link rel="stylesheet" href="/static/app.css">
	<script src="/static/htmx.min.js" defer></script>
	<script src="/static/app.js" defer></script>
</head>
<body>
<div class="app-shell">
	<aside class="sidebar" aria-label="Hauptnavigation">
		<a class="brand" href="/" aria-label="PMSS API Tester Startseite"><span class="brand-mark" aria-hidden="true">P</span><span class="brand-copy"><strong>PMSS</strong><small>API Tester</small></span></a>
		<div class="nav-caption">WORKSPACE</div>
		<nav>
			<a class="nav-item" href="/"><span class="nav-icon" aria-hidden="true">▦</span><span>Übersicht</span></a>
			<a class="nav-item active" href="/api-explorer" aria-current="page"><span class="nav-icon" aria-hidden="true">⌘</span><span>API Explorer</span></a>
			<div class="nav-item nav-muted" aria-disabled="true"><span class="nav-icon" aria-hidden="true">✓</span><span>Prüfungen</span><span class="nav-soon">Demnächst</span></div>
		</nav>
		<div class="sidebar-bottom"><div class="sidebar-rule"></div><div class="version-label">VERSION 1 · READ-ONLY</div></div>
	</aside>
	<main class="main-content">
		<header class="topbar">
			<div class="breadcrumbs"><span>PMSS API Tester</span><span class="crumb-separator">/</span><strong>API Explorer</strong></div>
			<div class="topbar-actions">
				{{if .Authenticated}}
					<span class="user-label">{{.Username}}</span>
					<form method="post" action="/auth/logout"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"><button class="logout-button" type="submit">Abmelden</button></form>
				{{else if .AuthEnabled}}
					<a class="login-button" href="/auth/login">Mit Keycloak anmelden</a>
				{{end}}
				<button class="theme-toggle" type="button" aria-label="Farbschema umschalten" title="Farbschema umschalten"><span class="theme-light" aria-hidden="true">☼</span><span class="theme-dark" aria-hidden="true">◐</span></button>
			</div>
		</header>
		<div class="page-body explorer-body">
			<div class="page-heading">
				<div><div class="eyebrow">OPENAPI 3.2.1 · SPEZIFIKATION 1.0</div><h1>API Explorer</h1><p class="subtitle">GET-Aufrufe an das Zielsystem – ausschließlich lesend und mit deinem Benutzer-Token.</p></div>
				<div class="readonly-badge"><span class="lock-icon" aria-hidden="true">▣</span> Read-only</div>
			</div>
			<section class="explorer-status">
				<div><div class="eyebrow">ZIELSYSTEM</div><strong>{{if .Configured}}{{.TargetURL}}{{else}}Nicht konfiguriert{{end}}</strong></div>
				{{if .InsecureTarget}}<p class="blocked-text" role="alert">Warnung: HTTP ist unverschlüsselt; das Benutzer-Token wird im Klartext übertragen. Nur in isolierten Testumgebungen verwenden.</p>{{end}}
				{{if not .Configured}}<p>Setze <code>PMSS_API_BASE_URL</code> auf eine HTTPS-Basis-URL, um API-Aufrufe zu aktivieren.</p>{{else if not .Authenticated}}
					<p>{{if .AuthEnabled}}Melde dich an, um den Explorer mit deinem Benutzer-Token zu verwenden.{{else}}OIDC ist nicht konfiguriert. API-Aufrufe sind <strong>BLOCKED</strong>.{{end}}</p>
				{{else if not .HasReadScope}}<p class="blocked-text"><strong>BLOCKED</strong> · Der angemeldete Benutzer besitzt keinen <code>read</code>-Scope.</p>
				{{else}}<p>Angemeldet als <strong>{{.Username}}</strong> · Scopes: <code>{{.ScopeLabel}}</code></p>{{end}}
			</section>
			{{if .Result}}
				<section class="explorer-result" aria-live="polite">
					<div class="result-heading"><div><div class="eyebrow">LETZTE ANFRAGE</div><h2>{{.Result.StatusText}}{{if .Result.StatusCode}} · HTTP {{.Result.StatusCode}}{{end}}</h2></div>{{if .Result.Duration}}<span class="duration">{{.Result.Duration}}</span>{{end}}</div>
					{{if .Result.RequestURL}}<div class="request-url"><span>GET</span><code>{{.Result.RequestURL}}</code></div>{{end}}
					{{if .Result.Error}}<p class="blocked-text" role="alert">{{.Result.Error}}</p>{{end}}
					{{if .Result.SchemaChecked}}<p class="schema-status {{if .Result.SchemaValid}}schema-valid{{else}}schema-invalid{{end}}">Response-Schema: {{if .Result.SchemaValid}}VALID{{else}}INVALID{{end}}</p>{{end}}
					{{if .Result.SchemaError}}<p class="blocked-text schema-error" role="alert">{{.Result.SchemaError}}</p>{{end}}
					{{if .Result.Headers}}<details class="schema-details" open><summary>Response-Header</summary><dl class="response-headers">{{range .Result.Headers}}<dt>{{.Name}}</dt><dd>{{.Value}}</dd>{{end}}</dl></details>{{end}}
					{{if .Result.Body}}<details class="schema-details" open><summary>Response-Body{{if .Result.BodyTruncated}} (auf 1 MiB begrenzt){{end}}</summary><pre>{{.Result.Body}}</pre></details>{{end}}
				</section>
			{{end}}
			{{if not .CoreOperations}}<section class="info-panel"><p>Die OpenAPI-Spezifikation enthält keine Operationen.</p></section>{{end}}
			<section class="operation-section"><div class="section-title"><div><div class="eyebrow">STANDARDISIERTE SCHNITTSTELLE</div><h2>Core API</h2></div><span class="operation-count">{{len .CoreOperations}} Operationen</span></div>
				{{range .CoreOperations}}{{template "operation" (dict "Page" $ "Operation" .)}}{{end}}
			</section>
			<section class="operation-section"><div class="section-title"><div><div class="eyebrow">IMPLEMENTIERUNGSSPEZIFISCH</div><h2>PMSS Extra</h2></div><span class="operation-count">{{len .ExtraOperations}} Operationen</span></div>
				{{range .ExtraOperations}}{{template "operation" (dict "Page" $ "Operation" .)}}{{end}}
			</section>
			<footer class="page-footer">PMSS API Tester <span>·</span> Version 1 <span>·</span> Alle schreibenden Operationen sind serverseitig gesperrt</footer>
		</div>
	</main>
</div>
{{define "operation"}}
	{{$page := .Page}}
	{{$operation := .Operation}}
	<article class="operation-card" id="operation-{{$operation.ID}}">
		<div class="operation-heading">
			<div><span class="method-badge method-{{$operation.Method}}">{{$operation.Method}}</span><code class="operation-path">{{$operation.Path}}</code><h3>{{$operation.Summary}}</h3></div>
			{{if eq $operation.Method "GET"}}<span class="operation-safe">Read-only</span>{{else}}<span class="operation-blocked">BLOCKED</span>{{end}}
		</div>
		{{if $operation.Description}}<p class="operation-description">{{$operation.Description}}</p>{{end}}
		{{if eq $operation.Method "GET"}}
			<form method="post" action="/api-explorer/execute" class="operation-form">
				<input type="hidden" name="csrf_token" value="{{$page.CSRFToken}}">
				<input type="hidden" name="operation_id" value="{{$operation.ID}}">
				<input type="hidden" name="method" value="{{$operation.Method}}">
				{{if $operation.Parameters}}
					<div class="parameter-grid">
						<p class="parameter-hint">Pfadwerte unkodiert eingeben; sie werden vor dem Request RFC-3986-kodiert.</p>
						{{range $operation.Parameters}}
							<label class="parameter-field" for="{{$operation.ID}}-{{.FormKey}}">
								<span>{{.Name}} <small>{{.In}}{{if .Required}} · erforderlich{{end}}</small></span>
								<input id="{{$operation.ID}}-{{.FormKey}}" name="{{.FormKey}}" type="text" value="{{if eq $page.SelectedOperation $operation.ID}}{{index $page.Values .FormKey}}{{end}}" placeholder="{{if .Required}}Wert eingeben{{else}}Optional{{end}}" {{if .Required}}required{{end}}>
								{{if .Description}}<small>{{.Description}}</small>{{end}}
								{{if .Schema}}<details class="parameter-schema"><summary>Parameterschema</summary><pre>{{.Schema}}</pre></details>{{end}}
							</label>
						{{end}}
					</div>
				{{end}}
				<button class="login-button execute-button" type="submit" {{if not (and $page.Configured $page.Authenticated $page.HasReadScope)}}disabled{{end}}>GET ausführen</button>
			</form>
		{{else}}
			<p class="blocked-text">Diese Operation ist im Read-only-Modus serverseitig gesperrt.</p>
		{{end}}
		<div class="schema-grid">
			{{if $operation.RequestSchema}}<details class="schema-details"><summary>Request-Schema</summary><pre>{{$operation.RequestSchema}}</pre></details>{{end}}
			{{if $operation.ResponseSchema}}<details class="schema-details"><summary>Response-Schema</summary><pre>{{$operation.ResponseSchema}}</pre></details>{{end}}
		</div>
	</article>
{{end}}
</body>
</html>`))

func explorerData(cfg config.Config, authService *auth.Service, operations []openapi.Operation, request *http.Request) explorerPageData {
	data := explorerPageData{
		TargetURL:      cfg.PMSSAPIBaseURL,
		InsecureTarget: strings.HasPrefix(strings.ToLower(cfg.PMSSAPIBaseURL), "http://"),
		Configured:     cfg.PMSSAPIBaseURL != "",
		AuthEnabled:    authService.Enabled(),
		Values:         make(map[string]string),
	}
	for _, operation := range operations {
		if operation.Extra {
			data.ExtraOperations = append(data.ExtraOperations, operation)
		} else {
			data.CoreOperations = append(data.CoreOperations, operation)
		}
	}
	if session, _ := authService.Session(request); session != nil {
		data.Authenticated = true
		data.HasReadScope = session.HasReadScope
		data.Username = session.Username
		data.ScopeLabel = strings.Join(session.Scopes, " ")
		data.CSRFToken = session.CSRFToken
	}
	return data
}

func renderExplorer(w http.ResponseWriter, data explorerPageData, status int) {
	var body bytes.Buffer
	if err := explorerTemplate.Execute(&body, data); err != nil {
		slog.Error("API Explorer rendering failed", "error", err)
		http.Error(w, "API Explorer rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if status != 0 {
		w.WriteHeader(status)
	}
	if _, err := w.Write(body.Bytes()); err != nil {
		slog.Error("write API Explorer response", "error", err)
	}
}

func blockedResult(message string) *explorerResult {
	return &explorerResult{StatusText: "BLOCKED", Error: message}
}

func errorResult(err error) *explorerResult {
	return &explorerResult{StatusText: "ERROR", Error: err.Error()}
}

func resultFromPMSS(result pmss.Result) *explorerResult {
	status := http.StatusText(result.StatusCode)
	if status == "" {
		status = result.StatusText
	}
	if status == "" && result.Error != "" {
		status = "ERROR"
	}
	return &explorerResult{
		StatusText:    status,
		StatusCode:    result.StatusCode,
		RequestURL:    result.RequestURL,
		Duration:      fmt.Sprintf("%.2f ms", float64(result.Duration)/float64(time.Millisecond)),
		Headers:       result.Headers,
		Body:          result.Body,
		BodyTruncated: result.BodyTruncated,
		SchemaChecked: result.SchemaChecked,
		SchemaValid:   result.SchemaValid,
		SchemaError:   result.SchemaError,
		Error:         result.Error,
	}
}

func handleExplorerExecute(
	w http.ResponseWriter,
	r *http.Request,
	cfg config.Config,
	authService *auth.Service,
	client *pmss.Client,
	allOperations []openapi.Operation,
	operations map[string]openapi.Operation,
) {
	data := explorerData(cfg, authService, allOperations, r)
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		data.Result = errorResult(fmt.Errorf("Ungültige Anfrage: %w", err))
		renderExplorer(w, data, http.StatusBadRequest)
		return
	}

	operationID := r.PostForm.Get("operation_id")
	operation, exists := operations[operationID]
	data.SelectedOperation = operationID
	if !exists {
		data.Result = blockedResult("Unbekannte OpenAPI-Operation.")
		renderExplorer(w, data, http.StatusBadRequest)
		return
	}
	for _, parameter := range operation.Parameters {
		data.Values[parameter.FormKey] = r.PostForm.Get(parameter.FormKey)
	}
	submittedMethod := strings.ToUpper(strings.TrimSpace(r.PostForm.Get("method")))
	if operation.Method != http.MethodGet || submittedMethod != http.MethodGet {
		data.Result = blockedResult("Nur GET-Aufrufe sind in Version 1 erlaubt.")
		renderExplorer(w, data, http.StatusForbidden)
		return
	}
	if !authService.ValidateCSRF(r, r.PostForm.Get("csrf_token")) {
		data.Result = blockedResult("Die CSRF-Prüfung ist fehlgeschlagen. Lade die Seite neu und versuche es erneut.")
		renderExplorer(w, data, http.StatusForbidden)
		return
	}
	if !data.Configured || client == nil {
		data.Result = blockedResult("Das Zielsystem ist nicht konfiguriert.")
		renderExplorer(w, data, http.StatusServiceUnavailable)
		return
	}
	if !data.Authenticated {
		data.Result = blockedResult("Melde dich an, bevor du API-Aufrufe ausführst.")
		renderExplorer(w, data, http.StatusForbidden)
		return
	}
	if !data.HasReadScope {
		data.Result = blockedResult("Der Token-Austausch hat keinen erforderlichen read-Scope geliefert.")
		renderExplorer(w, data, http.StatusForbidden)
		return
	}

	values := make(url.Values, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		if value, ok := r.PostForm[parameter.FormKey]; ok {
			values[parameter.FormKey] = append([]string(nil), value...)
		}
	}
	if err := operation.ValidateParameters(values); err != nil {
		data.Result = errorResult(fmt.Errorf("Ungültige Parameter: %w", err))
		renderExplorer(w, data, http.StatusBadRequest)
		return
	}
	accessToken, ok := authService.PMSSAccessToken(r)
	if !ok {
		data.Result = blockedResult("Der PMSS-Zugriff ist nicht autorisiert.")
		renderExplorer(w, data, http.StatusForbidden)
		return
	}
	result, err := client.Execute(r.Context(), operation, values, accessToken)
	if err != nil {
		if strings.Contains(err.Error(), "BLOCKED") {
			data.Result = blockedResult(err.Error())
			renderExplorer(w, data, http.StatusForbidden)
			return
		}
		data.Result = errorResult(err)
		renderExplorer(w, data, http.StatusBadGateway)
		return
	}
	data.Result = resultFromPMSS(result)
	renderExplorer(w, data, http.StatusOK)
}
