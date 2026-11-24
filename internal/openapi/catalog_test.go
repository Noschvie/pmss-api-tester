package openapi

import (
	"net/url"
	"testing"

	"github.com/noschvie/pmss-api-tester/api"
)

func TestCatalogSeparatesCoreAndPMSSExtra(t *testing.T) {
	operations, err := Catalog(api.Definition())
	if err != nil {
		t.Fatal(err)
	}

	var info, health, createClient *Operation
	for index := range operations {
		operation := &operations[index]
		switch operation.ID {
		case "getInfo":
			info = operation
		case "healthz":
			health = operation
		case "createClient":
			createClient = operation
		}
	}
	if info == nil || info.Method != "GET" || info.Extra {
		t.Fatalf("getInfo operation = %+v, want Core GET operation", info)
	}
	if info.ResponseSchema == "" {
		t.Fatal("getInfo response schema was not loaded")
	}
	if health == nil || !health.Extra {
		t.Fatalf("healthz operation = %+v, want PMSS Extra", health)
	}
	if createClient == nil || createClient.Method != "POST" || createClient.RequestSchema == "" {
		t.Fatalf("createClient operation = %+v, want blocked POST with request schema", createClient)
	}
}

func TestCatalogIncludesPathAndQueryParameters(t *testing.T) {
	operations, err := Catalog(api.Definition())
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		if operation.ID != "getClient" {
			continue
		}
		if len(operation.Parameters) == 0 || operation.Parameters[0].Name != "username" || operation.Parameters[0].In != "path" {
			t.Fatalf("getClient parameters = %+v", operation.Parameters)
		}
		parameter := operation.Parameters[0]
		if err := operation.ValidateParameters(url.Values{parameter.FormKey: {"alice"}}); err != nil {
			t.Fatalf("valid path parameter rejected: %v", err)
		}
		if err := operation.ValidateParameters(url.Values{parameter.FormKey: {""}}); err == nil {
			t.Fatal("missing required path parameter was accepted")
		}
		return
	}
	t.Fatal("getClient operation was not found")
}

func TestCatalogValidatesResponseAgainstOpenAPISchema(t *testing.T) {
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
	operations, err := Catalog([]byte(definition))
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 {
		t.Fatalf("operation count = %d, want 1", len(operations))
	}

	checked, err := operations[0].ValidateResponse(200, "application/json", []byte(`{"name":"PMSS"}`))
	if !checked || err != nil {
		t.Fatalf("valid response = checked %t, error %v", checked, err)
	}
	checked, err = operations[0].ValidateResponse(200, "application/json", []byte(`{"name":42}`))
	if !checked || err == nil {
		t.Fatalf("invalid response = checked %t, error %v; want schema error", checked, err)
	}
	checked, err = operations[0].ValidateResponse(200, "application/jsonp", []byte(`{"name":"PMSS"}`))
	if !checked || err == nil {
		t.Fatalf("non-JSON content type = checked %t, error %v; want content-type error", checked, err)
	}
}
