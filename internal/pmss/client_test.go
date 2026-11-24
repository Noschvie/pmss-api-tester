package pmss

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/noschvie/pmss-api-tester/internal/openapi"
)

func TestExecuteEncodesPathAndQueryAndReturnsResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		if got, want := r.URL.EscapedPath(), "/base/api/v1/clients/name%20with%20space"; got != want {
			t.Errorf("escaped path = %q, want %q", got, want)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q, want 5", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "session=secret")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"items":[]}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL + "/base")
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	result, err := client.Execute(context.Background(), openapi.Operation{
		ID:     "getClient",
		Method: http.MethodGet,
		Path:   "/api/v1/clients/{username}",
		Parameters: []openapi.Parameter{
			{Name: "username", In: "path", Required: true, FormKey: "p0"},
			{Name: "limit", In: "query", FormKey: "p1"},
		},
	}, url.Values{"p0": {"name with space"}, "p1": {"5"}}, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != http.StatusOK || result.Body != `{"items":[]}` || result.Duration <= 0 {
		t.Fatalf("result = %+v", result)
	}
	for _, header := range result.Headers {
		if strings.EqualFold(header.Name, "Set-Cookie") {
			t.Fatal("response exposed Set-Cookie")
		}
	}
}

func TestExecuteBlocksNonGetBeforeSending(t *testing.T) {
	client, err := NewClient("https://pmss.example.org")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Execute(context.Background(), openapi.Operation{
		ID:     "createClient",
		Method: http.MethodPost,
		Path:   "/api/v1/clients",
	}, nil, "test-token")
	if err == nil || !strings.Contains(err.Error(), "BLOCKED") {
		t.Fatalf("Execute error = %v, want BLOCKED", err)
	}
}

func TestExecuteValidatesResponseAgainstOpenAPISchema(t *testing.T) {
	const definition = `openapi: 3.2.1
info:
  title: Validation test
  version: "1"
paths:
  /example:
    get:
      operationId: getExample
      responses:
        "200":
          description: Example
          content:
            application/json:
              schema:
                type: object
                required: [name]
                properties:
                  name:
                    type: string
`
	operations, err := openapi.Catalog([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":42}`)
	}))
	defer server.Close()

	client, err := NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client.http = server.Client()
	result, err := client.Execute(context.Background(), operations[0], nil, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if !result.SchemaChecked || result.SchemaValid || result.SchemaError == "" {
		t.Fatalf("schema validation result = %+v, want invalid response schema", result)
	}
}

func TestOperationURLRejectsPathTraversal(t *testing.T) {
	client, err := NewClient("https://pmss.example.org")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.operationURL(openapi.Operation{
		Method: http.MethodGet,
		Path:   "/api/v1/clients/{username}",
		Parameters: []openapi.Parameter{
			{Name: "username", In: "path", Required: true, FormKey: "p0"},
		},
	}, url.Values{"p0": {"../admin"}})
	if err == nil {
		t.Fatal("operationURL accepted a dot-segment path parameter")
	}
}
