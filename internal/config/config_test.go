package config

import "testing"

func TestLoadUsesDefaultAddress(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("OIDC_ISSUER_URL", "")
	t.Setenv("OIDC_INTERNAL_URL", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("OIDC_CLIENT_SECRET", "")
	t.Setenv("OIDC_CALLBACK_URL", "")
	t.Setenv("PMSS_AUDIENCE", "")
	t.Setenv("OIDC_COOKIE_SECURE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
	}
	if cfg.OIDC.CallbackURL != "http://localhost:8080/auth/callback" || cfg.OIDC.Audience != "phylax-mqtt-security-service" || !cfg.OIDC.CookieSecure {
		t.Fatalf("unexpected OIDC defaults: %+v", cfg.OIDC)
	}
}

func TestLoadRejectsInvalidAddress(t *testing.T) {
	for _, addr := range []string{"8080", ":not-a-port", ":0", ":65536"} {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("HTTP_ADDR", addr)
			if _, err := Load(); err == nil {
				t.Fatalf("Load(%q) succeeded, want an error", addr)
			}
		})
	}
}

func TestLoadOIDCConfig(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "http://127.0.0.1:8180/realms/pmss-test/")
	t.Setenv("OIDC_INTERNAL_URL", "http://host.docker.internal:8180/realms/pmss-test")
	t.Setenv("OIDC_CLIENT_ID", "tasmofleet-test")
	t.Setenv("OIDC_CLIENT_SECRET", "test-secret")
	t.Setenv("OIDC_CALLBACK_URL", "http://localhost:8080/auth/callback")
	t.Setenv("PMSS_AUDIENCE", "")
	t.Setenv("OIDC_COOKIE_SECURE", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OIDC.IssuerURL != "http://127.0.0.1:8180/realms/pmss-test" || cfg.OIDC.InternalURL != "http://host.docker.internal:8180/realms/pmss-test" {
		t.Fatalf("OIDC URLs were not normalized: %+v", cfg.OIDC)
	}
	if cfg.OIDC.ClientID != "tasmofleet-test" || cfg.OIDC.CookieSecure {
		t.Fatalf("unexpected OIDC config: %+v", cfg.OIDC)
	}
}

func TestLoadRejectsIncompleteOIDCConfig(t *testing.T) {
	t.Setenv("OIDC_ISSUER_URL", "http://127.0.0.1:8180/realms/pmss-test")
	t.Setenv("OIDC_INTERNAL_URL", "")
	t.Setenv("OIDC_CLIENT_ID", "tasmofleet-test")
	t.Setenv("OIDC_CLIENT_SECRET", "")
	t.Setenv("OIDC_CALLBACK_URL", "")
	t.Setenv("PMSS_AUDIENCE", "")
	t.Setenv("OIDC_COOKIE_SECURE", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load succeeded with incomplete OIDC settings")
	}
}
