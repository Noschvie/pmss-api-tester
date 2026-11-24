package config

import "testing"

func TestLoadUsesDefaultAddress(t *testing.T) {
	t.Setenv("HTTP_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want %q", cfg.HTTPAddr, ":8080")
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
