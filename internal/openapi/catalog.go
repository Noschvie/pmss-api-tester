package openapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/url"
	"strconv"
	"strings"

	"github.com/pb33f/go-yaml"
	"github.com/pb33f/jsonschema/v6"
	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
)

type Parameter struct {
	Name        string
	In          string
	Description string
	Required    bool
	FormKey     string
	Schema      string
	validator   *jsonschema.Schema
}

type Operation struct {
	ID              string
	Method          string
	Path            string
	Summary         string
	Description     string
	Extra           bool
	Parameters      []Parameter
	RequestSchema   string
	ResponseSchema  string
	responseSchemas map[string]*jsonschema.Schema
}

func (operation Operation) ValidateParameters(values url.Values) error {
	for _, parameter := range operation.Parameters {
		value := values.Get(parameter.FormKey)
		if value == "" {
			if parameter.Required {
				return fmt.Errorf("required %s parameter %q is missing", parameter.In, parameter.Name)
			}
			continue
		}
		if parameter.validator != nil {
			if err := parameter.validator.Validate(value); err != nil {
				return fmt.Errorf("%s parameter %q does not match the OpenAPI schema: %w", parameter.In, parameter.Name, err)
			}
		}
	}
	return nil
}

func (operation Operation) ValidateResponse(status int, contentType string, body []byte) (bool, error) {
	schema := operation.responseSchemas[strconv.Itoa(status)]
	if schema == nil {
		schema = operation.responseSchemas[fmt.Sprintf("%dXX", status/100)]
	}
	if schema == nil {
		schema = operation.responseSchemas["default"]
	}
	if schema == nil {
		return false, nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || (mediaType != "application/json" && !strings.HasSuffix(strings.ToLower(mediaType), "+json")) {
		return true, errors.New("response content type is not JSON")
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return true, fmt.Errorf("decode JSON response: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return true, fmt.Errorf("response does not match the OpenAPI schema: %w", err)
	}
	return true, nil
}

func Catalog(definition []byte) ([]Operation, error) {
	document, err := libopenapi.NewDocument(definition)
	if err != nil {
		return nil, fmt.Errorf("load embedded OpenAPI document: %w", err)
	}
	model, err := document.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("build embedded OpenAPI model: %w", err)
	}
	if model == nil || model.Model.Paths == nil {
		return nil, fmt.Errorf("embedded OpenAPI document has no paths")
	}

	operations := make([]Operation, 0)
	ids := make(map[string]struct{})
	for path, item := range model.Model.Paths.PathItems.FromOldest() {
		if item == nil {
			continue
		}
		for method, operation := range item.GetOperations().FromOldest() {
			if operation == nil {
				continue
			}
			id := operation.OperationId
			if id == "" {
				id = strings.ToLower(method) + " " + path
			}
			if _, exists := ids[id]; exists {
				return nil, fmt.Errorf("duplicate OpenAPI operation ID %q", id)
			}
			ids[id] = struct{}{}

			entry := Operation{
				ID:          id,
				Method:      strings.ToUpper(method),
				Path:        path,
				Summary:     operation.Summary,
				Description: operation.Description,
				Extra:       isExtra(operation),
			}
			if entry.Summary == "" {
				entry.Summary = path
			}

			parameters := make(map[string]int)
			for _, parameter := range item.Parameters {
				if err := addParameter(&entry, parameters, parameter); err != nil {
					return nil, fmt.Errorf("operation %q: %w", id, err)
				}
			}
			for _, parameter := range operation.Parameters {
				if err := addParameter(&entry, parameters, parameter); err != nil {
					return nil, fmt.Errorf("operation %q: %w", id, err)
				}
			}
			entry.RequestSchema, err = requestSchema(operation)
			if err != nil {
				return nil, fmt.Errorf("operation %q request schema: %w", id, err)
			}
			entry.ResponseSchema, entry.responseSchemas, err = responseSchemas(id, operation)
			if err != nil {
				return nil, fmt.Errorf("operation %q response schema: %w", id, err)
			}
			operations = append(operations, entry)
		}
	}
	return operations, nil
}

func addParameter(operation *Operation, indices map[string]int, parameter *v3.Parameter) error {
	if parameter == nil {
		return nil
	}
	key := parameter.In + "\x00" + parameter.Name
	required := parameter.Required != nil && *parameter.Required
	entry := Parameter{
		Name:        parameter.Name,
		In:          parameter.In,
		Description: parameter.Description,
		Required:    required,
		Schema:      "",
	}
	if parameter.Schema != nil {
		schema, err := renderSchema(parameter.Schema.Schema())
		if err != nil {
			return err
		}
		entry.Schema = schema
		if schema != "" {
			entry.validator, err = compileSchema(schema, "parameter-"+parameter.Name)
			if err != nil {
				return fmt.Errorf("compile parameter %q schema: %w", parameter.Name, err)
			}
		}
	}
	if index, exists := indices[key]; exists {
		entry.FormKey = operation.Parameters[index].FormKey
		operation.Parameters[index] = entry
		return nil
	}
	entry.FormKey = fmt.Sprintf("p%d", len(operation.Parameters))
	indices[key] = len(operation.Parameters)
	operation.Parameters = append(operation.Parameters, entry)
	return nil
}

func requestSchema(operation *v3.Operation) (string, error) {
	if operation.RequestBody == nil || operation.RequestBody.Content == nil {
		return "", nil
	}
	media := operation.RequestBody.Content.GetOrZero("application/json")
	if media == nil {
		for _, candidate := range operation.RequestBody.Content.FromOldest() {
			media = candidate
			break
		}
	}
	if media == nil || media.Schema == nil {
		return "", nil
	}
	return renderSchema(media.Schema.Schema())
}

func responseSchemas(id string, operation *v3.Operation) (string, map[string]*jsonschema.Schema, error) {
	if operation.Responses == nil || operation.Responses.Codes == nil {
		return "", nil, nil
	}
	var schemas []string
	compiledSchemas := make(map[string]*jsonschema.Schema)
	for code, response := range operation.Responses.Codes.FromOldest() {
		if response == nil || response.Content == nil {
			continue
		}
		media := response.Content.GetOrZero("application/json")
		if media == nil {
			media = response.Content.GetOrZero("application/problem+json")
		}
		if media == nil {
			for _, candidate := range response.Content.FromOldest() {
				media = candidate
				break
			}
		}
		if media == nil || media.Schema == nil {
			continue
		}
		schema, err := renderSchema(media.Schema.Schema())
		if err != nil {
			return "", nil, fmt.Errorf("response %s: %w", code, err)
		}
		if schema == "" {
			continue
		}
		compiled, err := compileSchema(schema, id+"-"+code)
		if err != nil {
			return "", nil, fmt.Errorf("compile response %s schema: %w", code, err)
		}
		compiledSchemas[code] = compiled
		if schema != "" {
			schemas = append(schemas, fmt.Sprintf("%s:\n  %s", code, strings.ReplaceAll(schema, "\n", "\n  ")))
		}
	}
	return strings.Join(schemas, "\n"), compiledSchemas, nil
}

func compileSchema(definition, name string) (*jsonschema.Schema, error) {
	var value any
	if err := json.Unmarshal([]byte(definition), &value); err != nil {
		return nil, fmt.Errorf("decode schema: %w", err)
	}
	resourceURL := "https://pmss-api-tester.invalid/" + url.PathEscape(name) + ".json"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(resourceURL, value); err != nil {
		return nil, fmt.Errorf("register schema: %w", err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		return nil, fmt.Errorf("compile schema: %w", err)
	}
	return compiled, nil
}

func renderSchema(schema *base.Schema) (string, error) {
	if schema == nil {
		return "", nil
	}
	rendered, err := schema.RenderInline()
	if err != nil {
		return "", fmt.Errorf("render schema: %w", err)
	}
	var definition any
	if err := yaml.Unmarshal(rendered, &definition); err != nil {
		return "", fmt.Errorf("decode rendered schema: %w", err)
	}
	encoded, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode rendered schema: %w", err)
	}
	return string(encoded), nil
}

func isExtra(operation *v3.Operation) bool {
	if operation.Extensions != nil {
		if extension := operation.Extensions.GetOrZero("x-pmss-extra"); extension != nil && extension.Value == "true" {
			return true
		}
	}
	for _, tag := range operation.Tags {
		if tag == "pmss-extra" {
			return true
		}
	}
	return false
}
