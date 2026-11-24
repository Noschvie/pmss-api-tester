package api

import _ "embed"

//go:embed openapi.yaml
var definition []byte

func Definition() []byte {
	return append([]byte(nil), definition...)
}
