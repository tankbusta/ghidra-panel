package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"go.mkw.re/ghidra-panel/common"
)

type config struct {
	BaseURL string `json:"base_url"`
	// Database is a SQLite file path or postgres:// URL.
	Database string `json:"database"`
	Discord  struct {
		BotToken     string `json:"bot_token"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		WebhookURL   string `json:"webhook_url"`
	} `json:"discord"`
	Ghidra struct {
		Endpoint common.GhidraEndpoint `json:"endpoint"`
		GRPCAddr string                `json:"grpc_addr"`
	} `json:"ghidra"`
	Links       []common.Link `json:"links"`
	SuperAdmins []uint64      `json:"super_admins"`
	OIDC        oidcConfig    `json:"oidc"`
}

type oidcConfig struct {
	Issuer        string   `json:"issuer"`
	ClientID      string   `json:"client_id"`
	ClientSecret  string   `json:"client_secret"`
	DisplayName   string   `json:"display_name"`
	Scopes        []string `json:"scopes"`
	UsernameClaim string   `json:"username_claim"`
	SuperAdmins   []string `json:"super_admins"` // OIDC subjects
}

// OIDC login is enabled when an issuer is configured.
func (c *oidcConfig) enabled() bool {
	return c.Issuer != ""
}

func (c *oidcConfig) setDefaults() {
	if c.DisplayName == "" {
		c.DisplayName = "SSO"
	}
	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "profile", "email"}
	}
	if c.UsernameClaim == "" {
		c.UsernameClaim = "preferred_username"
	}
}

// Discord login is enabled when OAuth2 client credentials are configured.
// The bot token and webhook URL are used for notifications independently.
func (c *config) discordLoginEnabled() bool {
	return c.Discord.ClientID != ""
}

func (c *config) validate() {
	if c.discordLoginEnabled() && c.Discord.ClientSecret == "" {
		log.Fatal("discord.client_secret not set")
	}
	if !c.discordLoginEnabled() && c.Discord.ClientSecret != "" {
		log.Fatal("discord.client_id not set")
	}
	if !c.discordLoginEnabled() && !c.OIDC.enabled() {
		log.Fatal("no login provider configured, set discord.client_id or oidc.issuer")
	}
	if c.BaseURL == "" {
		log.Fatal("base_url not set")
	}
	if c.OIDC.enabled() {
		if c.OIDC.ClientID == "" {
			log.Fatal("oidc.client_id not set")
		}
		if c.OIDC.ClientSecret == "" {
			log.Fatal("oidc.client_secret not set")
		}
	}
}

// envPrefix marks a config value to be read from an environment variable,
// e.g. "$env:OIDC_CLIENT_SECRET".
const envPrefix = "$env:"

// resolveEnv replaces secret values referencing environment variables.
func (c *config) resolveEnv() error {
	fields := map[string]*string{
		"database":              &c.Database,
		"discord.bot_token":     &c.Discord.BotToken,
		"discord.client_id":     &c.Discord.ClientID,
		"discord.client_secret": &c.Discord.ClientSecret,
		"discord.webhook_url":   &c.Discord.WebhookURL,
		"oidc.issuer":           &c.OIDC.Issuer,
		"oidc.client_id":        &c.OIDC.ClientID,
		"oidc.client_secret":    &c.OIDC.ClientSecret,
	}
	for key, field := range fields {
		v, err := resolveEnvValue(*field)
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		*field = v
	}
	return nil
}

// resolveEnvValue returns the environment variable referenced by s,
// or s itself if it has no envPrefix.
func resolveEnvValue(s string) (string, error) {
	name, ok := strings.CutPrefix(s, envPrefix)
	if !ok {
		return s, nil
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return "", fmt.Errorf("environment variable %s not set", name)
	}
	return v, nil
}

const defaultDatabase = "ghidra_panel.db"

// databaseDSN picks the -db flag if set, then the config value, then the default.
func databaseDSN(flagValue, configValue string) (string, error) {
	switch {
	case flagValue != "":
		return resolveEnvValue(flagValue)
	case configValue != "":
		return configValue, nil
	default:
		return defaultDatabase, nil
	}
}
