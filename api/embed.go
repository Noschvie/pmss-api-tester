// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package api

import _ "embed"

//go:embed openapi.yaml
var definition []byte

func Definition() []byte {
	return append([]byte(nil), definition...)
}
