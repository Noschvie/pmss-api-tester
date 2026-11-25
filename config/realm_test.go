// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 Noschvie
// PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"slices"
	"testing"
)

// realmPath is the test realm of the Security Service, pinned by the
// submodule stack/pmss. It is the only copy; the tester keeps none.
const realmPath = "../stack/pmss/test/integration/keycloak/realm/pmss-test-realm.json"

func TestTesterRealmConfiguration(t *testing.T) {
	data, err := os.ReadFile(realmPath)
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("submodule stack/pmss not checked out; run git submodule update --init")
	}
	if err != nil {
		t.Fatal(err)
	}
	var realm struct {
		Realm   string `json:"realm"`
		Clients []struct {
			ID             string            `json:"clientId"`
			Enabled        bool              `json:"enabled"`
			Public         bool              `json:"publicClient"`
			Authenticator  string            `json:"clientAuthenticatorType"`
			StandardFlow   bool              `json:"standardFlowEnabled"`
			ImplicitFlow   bool              `json:"implicitFlowEnabled"`
			DirectGrants   bool              `json:"directAccessGrantsEnabled"`
			ServiceAccount bool              `json:"serviceAccountsEnabled"`
			Redirects      []string          `json:"redirectUris"`
			Attributes     map[string]string `json:"attributes"`
			DefaultScopes  []string          `json:"defaultClientScopes"`
			OptionalScopes []string          `json:"optionalClientScopes"`
		} `json:"clients"`
		ClientScopes []struct {
			Name    string `json:"name"`
			Mappers []struct {
				Type   string            `json:"protocolMapper"`
				Config map[string]string `json:"config"`
			} `json:"protocolMappers"`
		} `json:"clientScopes"`
	}
	if err := json.Unmarshal(data, &realm); err != nil {
		t.Fatal(err)
	}
	if realm.Realm != "pmss-test" {
		t.Fatalf("unexpected realm: %q", realm.Realm)
	}
	foundClient := false
	for _, client := range realm.Clients {
		if client.ID != "pmss-api-tester" {
			continue
		}
		if foundClient {
			t.Fatal("duplicate tester client")
		}
		foundClient = true
		if !client.Enabled || client.Public || client.Authenticator != "client-secret" ||
			!client.StandardFlow || client.ImplicitFlow || client.DirectGrants || client.ServiceAccount {
			t.Fatal("tester client has incorrect authentication flows")
		}
		if !slices.Equal(client.Redirects, []string{"${PMSS_API_TESTER_URL:http://127.0.0.1:8080}/auth/callback"}) ||
			client.Attributes["post.logout.redirect.uris"] != "${PMSS_API_TESTER_URL:http://127.0.0.1:8080}/" ||
			client.Attributes["pkce.code.challenge.method"] != "S256" ||
			client.Attributes["standard.token.exchange.enabled"] != "true" {
			t.Fatal("tester client has incorrect redirects, PKCE or token exchange settings")
		}
		if !slices.Equal(client.DefaultScopes, []string{"basic"}) ||
			!slices.Equal(client.OptionalScopes, []string{"read"}) {
			t.Fatal("tester client must be limited to basic and read scopes")
		}
	}
	if !foundClient {
		t.Fatal("tester client is missing")
	}
	foundAudience := false
	for _, scope := range realm.ClientScopes {
		if scope.Name != "read" {
			continue
		}
		for _, mapper := range scope.Mappers {
			if mapper.Type == "oidc-audience-mapper" &&
				mapper.Config["included.client.audience"] == "phylax-mqtt-security-service" &&
				mapper.Config["access.token.claim"] == "true" {
				foundAudience = true
			}
		}
	}
	if !foundAudience {
		t.Fatal("read scope is missing the PMSS access-token audience mapper")
	}
}
