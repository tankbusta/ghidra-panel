package main

import (
	"encoding/json"
	"testing"
)

func TestResolveEnv(t *testing.T) {
	t.Setenv("TEST_OIDC_SECRET", "s3cret")

	var cfg config
	err := json.Unmarshal([]byte(`{
		"discord": {"client_id": "literal"},
		"oidc": {"issuer": "https://idp", "client_secret": "$env:TEST_OIDC_SECRET"}
	}`), &cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.resolveEnv(); err != nil {
		t.Fatal(err)
	}
	if cfg.OIDC.ClientSecret != "s3cret" {
		t.Errorf("client_secret = %q, want env value", cfg.OIDC.ClientSecret)
	}
	if cfg.Discord.ClientID != "literal" || cfg.OIDC.Issuer != "https://idp" {
		t.Errorf("literal values changed: %q %q", cfg.Discord.ClientID, cfg.OIDC.Issuer)
	}

	cfg.Discord.BotToken = "$env:TEST_UNSET_VARIABLE"
	if err := cfg.resolveEnv(); err == nil {
		t.Error("expected error for unset environment variable")
	}
}
