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
	Discord struct {
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

func (c *config) validate() {
	if c.Discord.ClientID == "" {
		log.Fatal("client_id not set")
	}
	if c.Discord.ClientSecret == "" {
		log.Fatal("client_secret not set")
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
		"discord.bot_token":     &c.Discord.BotToken,
		"discord.client_id":     &c.Discord.ClientID,
		"discord.client_secret": &c.Discord.ClientSecret,
		"discord.webhook_url":   &c.Discord.WebhookURL,
		"oidc.issuer":           &c.OIDC.Issuer,
		"oidc.client_id":        &c.OIDC.ClientID,
		"oidc.client_secret":    &c.OIDC.ClientSecret,
	}
	for key, field := range fields {
		name, ok := strings.CutPrefix(*field, envPrefix)
		if !ok {
			continue
		}
		v, ok := os.LookupEnv(name)
		if !ok {
			return fmt.Errorf("%s: environment variable %s not set", key, name)
		}
		*field = v
	}
	return nil
}
