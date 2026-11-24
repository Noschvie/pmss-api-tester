package openapi

import (
	"testing"

	"github.com/noschvie/pmss-api-tester/api"
)

func TestEmbeddedSpecificationIsValid(t *testing.T) {
	if err := Validate(api.Definition()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsMalformedSpecification(t *testing.T) {
	if err := Validate([]byte("openapi: [")); err == nil {
		t.Fatal("Validate accepted malformed YAML")
	}
}

func TestValidateRejectsUnsupportedOpenAPIVersion(t *testing.T) {
	definition := []byte("openapi: 3.1.0\ninfo:\n  title: test\n  version: 1.0.0\npaths: {}\n")
	if err := Validate(definition); err == nil {
		t.Fatal("Validate accepted an unsupported OpenAPI version")
	}
}
