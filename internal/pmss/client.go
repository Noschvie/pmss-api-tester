package pmss

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/noschvie/pmss-api-tester/internal/openapi"
)

const maxResponseBody = 1 << 20

type Header struct {
	Name  string
	Value string
}

type Result struct {
	RequestURL    string
	StatusCode    int
	StatusText    string
	Headers       []Header
	Body          string
	BodyTruncated bool
	SchemaChecked bool
	SchemaValid   bool
	SchemaError   string
	Duration      time.Duration
	Error         string
}

type Client struct {
	baseURL *url.URL
	http    *http.Client
}

func NewClient(baseURL string, allowInsecureHTTP bool) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "https" && !(allowInsecureHTTP && parsed.Scheme == "http")) || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, errors.New("PMSS API base URL must be an absolute HTTPS URL (or HTTP when explicitly enabled) without user info, query, or fragment")
	}
	return &Client{
		baseURL: parsed,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) Execute(ctx context.Context, operation openapi.Operation, values url.Values, accessToken string) (Result, error) {
	if operation.Method != http.MethodGet {
		return Result{}, fmt.Errorf("BLOCKED: %s operations are disabled in read-only mode", operation.Method)
	}
	if accessToken == "" {
		return Result{}, errors.New("BLOCKED: a PMSS access token is required")
	}
	if err := operation.ValidateParameters(values); err != nil {
		return Result{}, err
	}
	target, err := c.operationURL(operation, values)
	if err != nil {
		return Result{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("prepare PMSS request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json, application/problem+json")
	for _, parameter := range operation.Parameters {
		if parameter.In != "header" {
			continue
		}
		value := values.Get(parameter.FormKey)
		if value == "" {
			continue
		}
		if err := setParameterHeader(request.Header, parameter.Name, value); err != nil {
			return Result{}, err
		}
	}

	started := time.Now()
	response, err := c.http.Do(request)
	if err != nil {
		return Result{RequestURL: target.String(), Duration: time.Since(started), Error: err.Error()}, nil
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBody+1))
	if err != nil {
		return Result{}, fmt.Errorf("read PMSS response: %w", err)
	}
	duration := time.Since(started)
	truncated := len(body) > maxResponseBody
	if truncated {
		body = body[:maxResponseBody]
	}
	schemaChecked := false
	schemaValid := false
	schemaError := ""
	if truncated {
		schemaError = "Response body exceeded 1 MiB; schema validation was skipped."
	} else {
		schemaChecked, err = operation.ValidateResponse(response.StatusCode, response.Header.Get("Content-Type"), body)
		if err != nil {
			schemaError = err.Error()
		} else {
			schemaValid = schemaChecked
		}
	}

	headers := make([]Header, 0, len(response.Header))
	for name, values := range response.Header {
		if strings.EqualFold(name, "Set-Cookie") {
			continue
		}
		for _, value := range values {
			headers = append(headers, Header{Name: name, Value: value})
		}
	}
	sort.Slice(headers, func(i, j int) bool {
		if headers[i].Name == headers[j].Name {
			return headers[i].Value < headers[j].Value
		}
		return headers[i].Name < headers[j].Name
	})
	return Result{
		RequestURL:    target.String(),
		StatusCode:    response.StatusCode,
		StatusText:    response.Status,
		Headers:       headers,
		Body:          string(body),
		BodyTruncated: truncated,
		SchemaChecked: schemaChecked,
		SchemaValid:   schemaValid,
		SchemaError:   schemaError,
		Duration:      duration,
	}, nil
}

func (c *Client) operationURL(operation openapi.Operation, values url.Values) (*url.URL, error) {
	if operation.Path == "" || !strings.HasPrefix(operation.Path, "/") {
		return nil, errors.New("invalid OpenAPI operation path")
	}
	allowedKeys := make(map[string]struct{}, len(operation.Parameters))
	for _, parameter := range operation.Parameters {
		allowedKeys[parameter.FormKey] = struct{}{}
	}
	for key := range values {
		if _, ok := allowedKeys[key]; !ok {
			return nil, fmt.Errorf("unknown API parameter %q", key)
		}
	}

	path := operation.Path
	for _, parameter := range operation.Parameters {
		value := values.Get(parameter.FormKey)
		if parameter.In == "path" {
			placeholder := "{" + parameter.Name + "}"
			if !strings.Contains(path, placeholder) {
				return nil, fmt.Errorf("OpenAPI path parameter %q is not present in the operation path", parameter.Name)
			}
			if value == "" {
				return nil, fmt.Errorf("required path parameter %q is missing", parameter.Name)
			}
			if value == "." || value == ".." {
				return nil, fmt.Errorf("path parameter %q cannot be a dot segment", parameter.Name)
			}
			if strings.ContainsAny(value, `/\`) {
				return nil, fmt.Errorf("path parameter %q must be a single path segment", parameter.Name)
			}
			path = strings.ReplaceAll(path, placeholder, url.PathEscape(value))
		}
		if parameter.Required && (parameter.In == "query" || parameter.In == "header") && value == "" {
			return nil, fmt.Errorf("required %s parameter %q is missing", parameter.In, parameter.Name)
		}
	}
	if strings.ContainsAny(path, "{}") {
		return nil, errors.New("operation path contains an unresolved parameter")
	}

	base := *c.baseURL
	basePath := strings.TrimRight(base.EscapedPath(), "/")
	rawPath := basePath + path
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return nil, fmt.Errorf("build PMSS request path: %w", err)
	}
	base.Path = decodedPath
	base.RawPath = rawPath

	query := base.Query()
	for _, parameter := range operation.Parameters {
		if parameter.In != "query" {
			continue
		}
		value := values.Get(parameter.FormKey)
		if value != "" {
			query.Set(parameter.Name, value)
		}
	}
	base.RawQuery = query.Encode()
	return &base, nil
}

func setParameterHeader(headers http.Header, name, value string) error {
	canonical := http.CanonicalHeaderKey(name)
	if canonical == "" {
		return fmt.Errorf("invalid OpenAPI header parameter name %q", name)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("header parameter %q contains a line break", name)
	}
	switch strings.ToLower(canonical) {
	case "authorization", "cookie", "host", "content-length", "connection", "transfer-encoding", "proxy-authorization":
		return fmt.Errorf("OpenAPI header parameter %q cannot be overridden", name)
	}
	headers.Set(canonical, value)
	return nil
}
