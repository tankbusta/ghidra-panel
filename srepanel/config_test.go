package main

import (
	"encoding/json"
	"testing"
)

func TestResolveEnv(t *testing.T) {
	t.Setenv("TEST_OIDC_SECRET", "s3cret")
	t.Setenv("TEST_DATABASE_URL", "postgres://u@h/db")

	var cfg config
	err := json.Unmarshal([]byte(`{
		"database": "$env:TEST_DATABASE_URL",
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
	if cfg.Database != "postgres://u@h/db" {
		t.Errorf("database = %q, want env value", cfg.Database)
	}
	if cfg.Discord.ClientID != "literal" || cfg.OIDC.Issuer != "https://idp" {
		t.Errorf("literal values changed: %q %q", cfg.Discord.ClientID, cfg.OIDC.Issuer)
	}

	cfg.Discord.BotToken = "$env:TEST_UNSET_VARIABLE"
	if err := cfg.resolveEnv(); err == nil {
		t.Error("expected error for unset environment variable")
	}
}

func TestDatabaseDSN(t *testing.T) {
	t.Setenv("TEST_DATABASE_URL", "postgres://u@h/db")
	for _, tc := range []struct{ flag, config, want string }{
		{"", "", defaultDatabase},
		{"", "from_config.db", "from_config.db"},
		{"from_flag.db", "from_config.db", "from_flag.db"},
		{"$env:TEST_DATABASE_URL", "from_config.db", "postgres://u@h/db"},
	} {
		got, err := databaseDSN(tc.flag, tc.config)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("databaseDSN(%q, %q) = %q, want %q", tc.flag, tc.config, got, tc.want)
		}
	}
	if _, err := databaseDSN("$env:TEST_UNSET_VARIABLE", ""); err == nil {
		t.Error("expected error for unset environment variable")
	}
}
