package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"golang.org/x/oauth2"
)

func newProvider(t *testing.T, userinfo map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	mux.HandleFunc("GET /.well-known/openid-configuration", func(wr http.ResponseWriter, req *http.Request) {
		_ = json.NewEncoder(wr).Encode(discovery{
			Issuer:                srv.URL,
			AuthorizationEndpoint: srv.URL + "/authorize",
			TokenEndpoint:         srv.URL + "/token",
			UserinfoEndpoint:      srv.URL + "/userinfo",
		})
	})
	mux.HandleFunc("POST /token", func(wr http.ResponseWriter, req *http.Request) {
		if req.FormValue("code") != "good-code" || req.FormValue("code_verifier") == "" {
			http.Error(wr, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		wr.Header().Set("Content-Type", "application/json")
		_, _ = wr.Write([]byte(`{"access_token":"at","token_type":"Bearer"}`))
	})
	mux.HandleFunc("GET /userinfo", func(wr http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "Bearer at" {
			http.Error(wr, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(wr).Encode(userinfo)
	})
	return srv
}

func TestLoginFlow(t *testing.T) {
	srv := newProvider(t, map[string]any{"sub": "abc123", "preferred_username": "alice"})

	auth, err := NewAuth(context.Background(), srv.URL+"/", "client", "secret", "https://panel/oidc/redirect", []string{"openid"}, "preferred_username")
	if err != nil {
		t.Fatal(err)
	}

	// Start login
	rec := httptest.NewRecorder()
	auth.RedirectToProvider(rec, httptest.NewRequest(http.MethodPost, "/login", nil))
	loc, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("missing PKCE challenge: %s", loc)
	}
	state := loc.Query().Get("state")
	cookies := rec.Result().Cookies()

	redirect := func() (*UserInfo, error) {
		req := httptest.NewRequest(http.MethodGet, "/oidc/redirect?code=good-code&state="+url.QueryEscape(state), nil)
		for _, c := range cookies {
			req.AddCookie(c)
		}
		return auth.HandleRedirect(httptest.NewRecorder(), req)
	}

	// Handle redirect
	info, err := redirect()
	if err != nil {
		t.Fatal(err)
	}
	if info.Subject != "abc123" || info.Username != "alice" {
		t.Fatalf("unexpected user info: %+v", info)
	}

	// State must not be reusable
	if _, err := redirect(); err == nil {
		t.Fatal("expected state reuse to fail")
	}
}

func TestIssuerMismatch(t *testing.T) {
	srv := newProvider(t, nil)
	_, err := NewAuth(context.Background(), srv.URL+"/other", "client", "secret", "", nil, "")
	if err == nil {
		t.Fatal("expected discovery to fail")
	}
}

func TestUsernameFallback(t *testing.T) {
	srv := newProvider(t, map[string]any{"sub": "abc123", "email": "bob@example.com"})
	auth, err := NewAuth(context.Background(), srv.URL, "client", "secret", "", nil, "nickname")
	if err != nil {
		t.Fatal(err)
	}
	info, err := auth.GetUserInfo(context.Background(), &oauth2.Token{AccessToken: "at", TokenType: "Bearer"})
	if err != nil {
		t.Fatal(err)
	}
	if info.Username != "bob@example.com" {
		t.Fatalf("expected email fallback, got %q", info.Username)
	}
}
