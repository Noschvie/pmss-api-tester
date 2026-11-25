// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package web

type PageState struct {
	AuthEnabled    bool
	Authenticated  bool
	Username       string
	ScopeLabel     string
	HasReadScope   bool
	CSRFToken      string
	ExpiresAt      string
	PMSSConfigured bool
	PMSSAPIBaseURL string
	Error          string
}
