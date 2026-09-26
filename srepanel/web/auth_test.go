package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectHome(t *testing.T) {
	tests := []struct {
		name   string
		cookie *http.Cookie
		want   string
	}{
		{"no cookie", nil, "/"},
		{"stored page", &http.Cookie{Name: "redirect", Value: "/repos/foo?x=1"}, "/repos/foo?x=1"},
		{"empty cookie", &http.Cookie{Name: "redirect", Value: ""}, "/"},
		{"absolute URL", &http.Cookie{Name: "redirect", Value: "https://evil.example/"}, "/"},
		{"protocol relative URL", &http.Cookie{Name: "redirect", Value: "//evil.example/"}, "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/oidc/redirect?code=x&state=y", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			rec := httptest.NewRecorder()
			(&Server{}).redirectHome(rec, req)

			if got := rec.Header().Get("Location"); got != tt.want {
				t.Errorf("Location = %q, want %q", got, tt.want)
			}
			if tt.cookie != nil {
				cleared := false
				for _, c := range rec.Result().Cookies() {
					if c.Name == "redirect" && c.MaxAge < 0 {
						cleared = true
					}
				}
				if !cleared {
					t.Error("redirect cookie not deleted")
				}
			}
		})
	}
}
