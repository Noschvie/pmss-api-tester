package openapi

import (
	"fmt"

	"github.com/pb33f/libopenapi"
)

const supportedVersion = "3.2.1"

func Validate(definition []byte) error {
	document, err := libopenapi.NewDocument(definition)
	if err != nil {
		return fmt.Errorf("load embedded OpenAPI document: %w", err)
	}

	model, err := document.BuildV3Model()
	if err != nil {
		return fmt.Errorf("validate embedded OpenAPI document: %w", err)
	}
	if model == nil {
		return fmt.Errorf("validate embedded OpenAPI document: document model is empty")
	}
	if model.Model.Version != supportedVersion {
		return fmt.Errorf("unsupported OpenAPI version %q: expected %q", model.Model.Version, supportedVersion)
	}
	return nil
}
