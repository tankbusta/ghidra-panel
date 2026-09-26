package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.mkw.re/ghidra-panel/discord"
	"go.mkw.re/ghidra-panel/oidc"
)

func TestLoginRender(t *testing.T) {
	tests := []struct {
		name        string
		state       LoginState
		wantDiscord bool
		wantOIDC    bool
	}{
		{"both", LoginState{Discord: true, OIDCName: "Example SSO"}, true, true},
		{"oidc only", LoginState{OIDCName: "Example SSO"}, false, true},
		{"discord only", LoginState{Discord: true}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.state.State = &State{}
			var b strings.Builder
			if err := loginPage.Execute(&b, tt.state); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(b.String(), "Continue with Discord"); got != tt.wantDiscord {
				t.Errorf("Discord button shown = %v, want %v", got, tt.wantDiscord)
			}
			if got := strings.Contains(b.String(), "Continue with Example SSO"); got != tt.wantOIDC {
				t.Errorf("OIDC button shown = %v, want %v", got, tt.wantOIDC)
			}
		})
	}
}

func newTestOIDC(t *testing.T) *oidc.Auth {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(wr http.ResponseWriter, req *http.Request) {
		_, _ = fmt.Fprintf(wr, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"userinfo_endpoint":%q}`,
			srv.URL, srv.URL+"/authorize", srv.URL+"/token", srv.URL+"/userinfo")
	}))
	t.Cleanup(srv.Close)
	auth, err := oidc.NewAuth(context.Background(), srv.URL, "client", "secret", "https://panel/oidc/redirect", []string{"openid"}, "")
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestLoginAutoRedirect(t *testing.T) {
	oidcAuth := newTestOIDC(t)
	discordAuth := discord.NewAuth("client", "secret", "https://panel/redirect")

	tests := []struct {
		name     string
		server   *Server
		url      string
		wantAuto bool
	}{
		{"oidc only", &Server{Config: &Config{}, OIDC: oidcAuth}, "/login", true},
		{"oidc and discord", &Server{Config: &Config{}, OIDC: oidcAuth, Auth: discordAuth}, "/login", false},
		{"discord only", &Server{Config: &Config{}, Auth: discordAuth}, "/login", false},
		{"after logout", &Server{Config: &Config{}, OIDC: oidcAuth}, "/login?status=logged_out", false},
		{"access denied", &Server{Config: &Config{}, OIDC: oidcAuth}, "/login?status=access_denied", false},
		{"dev mode", &Server{Config: &Config{Dev: true}, OIDC: oidcAuth}, "/login", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.server.handleLogin(rec, httptest.NewRequest(http.MethodGet, tt.url, nil))
			gotAuto := rec.Code == http.StatusSeeOther && strings.HasPrefix(rec.Header().Get("Location"), oidcAuth.Endpoint.AuthURL)
			if gotAuto != tt.wantAuto {
				t.Errorf("auto redirect = %v (status %d, location %q), want %v",
					gotAuto, rec.Code, rec.Header().Get("Location"), tt.wantAuto)
			}
		})
	}
}
