package web

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"go.mkw.re/ghidra-panel/common"
)

func (s *Server) handleLogin(wr http.ResponseWriter, req *http.Request) {
	if _, ok := s.checkAuth(req); ok {
		s.redirectHome(wr, req)
		return
	}

	switch req.Method {
	case http.MethodGet:
		if s.autoLogin(req) {
			s.OIDC.RedirectToProvider(wr, req)
			return
		}
		state := s.stateWithNav(
			req,
			Nav{Route: "/", Name: "Ghidra"},
			Nav{Route: "/login", Name: "Login"},
		)
		loginState := LoginState{State: state, Discord: s.Auth != nil}
		if s.OIDC != nil {
			loginState.OIDCName = s.Config.OIDCDisplayName
		}
		err := loginPage.Execute(wr, loginState)
		if err != nil {
			log.Println("Failed to serve login:", err)
			_, _ = wr.Write([]byte("Failed to render the login page"))
		}
	case http.MethodPost:
		if s.Config.Dev {
			ident := &common.Identity{
				ID:       1,
				Username: "testuser",
			}
			s.setTokenCookie(wr, ident)
			s.redirectHome(wr, req)
			return
		}
		if req.FormValue("provider") == "oidc" && s.OIDC != nil {
			s.OIDC.RedirectToProvider(wr, req)
			return
		}
		if req.FormValue("provider") == "discord" && s.Auth != nil {
			http.Redirect(wr, req, s.Auth.AuthURL(), http.StatusSeeOther)
			return
		}
		http.Error(wr, "Unknown login provider", http.StatusBadRequest)
	default:
		http.Error(wr, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleOAuthRedirect(wr http.ResponseWriter, req *http.Request) {
	if s.Auth == nil {
		http.NotFound(wr, req)
		return
	}

	ident, err := s.Auth.HandleRedirect(wr, req)
	if err != nil {
		log.Println("Redirect request failed:", err)
		http.Error(wr, "Authorization failed", http.StatusUnauthorized)
		return
	}
	if ident == nil {
		return
	}

	s.setTokenCookie(wr, ident)
	s.redirectHome(wr, req)
}

func (s *Server) handleOIDCRedirect(wr http.ResponseWriter, req *http.Request) {
	if s.OIDC == nil {
		http.NotFound(wr, req)
		return
	}

	info, err := s.OIDC.HandleRedirect(wr, req)
	if err != nil {
		log.Println("OIDC redirect request failed:", err)
		http.Error(wr, "Authorization failed", http.StatusUnauthorized)
		return
	}
	if info == nil {
		return
	}

	// Map the OIDC subject to a stable local user ID
	id, err := s.DB.GetOrCreateOIDCUserID(req.Context(), s.OIDC.Issuer, info.Subject)
	if err != nil {
		log.Println("Failed to get OIDC user ID:", err)
		http.Error(wr, "Authorization failed", http.StatusInternalServerError)
		return
	}

	s.setTokenCookie(wr, &common.Identity{
		ID:          id,
		Username:    info.Username,
		OIDCSubject: info.Subject,
	})
	s.redirectHome(wr, req)
}

// setTokenCookie issues a session token for the identity.
func (s *Server) setTokenCookie(wr http.ResponseWriter, ident *common.Identity) {
	token, exp := s.Issuer.Issue(ident)
	http.SetCookie(wr, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   true,
	})
}

type LoginState struct {
	*State
	Discord  bool   // show the Discord login button
	OIDCName string // OIDC button label, empty if OIDC is disabled
}

func (s *Server) checkAuth(req *http.Request) (*common.Identity, bool) {
	cookie, err := req.Cookie("token")
	if err != nil || cookie == nil {
		return nil, false
	}
	ident, err := s.Issuer.Verify(cookie.Value)
	if err != nil {
		// Only log errors in development mode
		if s.Config.Dev {
			log.Print("failed to verify token: ", err)
		}
		return nil, false
	}
	return ident, true
}

func (s *Server) handleLogout(wr http.ResponseWriter, req *http.Request) {
	http.SetCookie(wr, &http.Cookie{
		Name:   "token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	// Show the login page rather than signing back in via the provider's session
	http.Redirect(wr, req, "/login?status=logged_out", http.StatusSeeOther)
}

// autoLogin reports whether the login page should be skipped in favor of the OIDC provider.
// This applies when OIDC is the only login provider, unless a status message is shown.
func (s *Server) autoLogin(req *http.Request) bool {
	return s.OIDC != nil && s.Auth == nil && !s.Config.Dev && req.URL.Query().Get("status") == ""
}

// redirectHome redirects to the home page or a stored redirect target.
func (s *Server) redirectHome(wr http.ResponseWriter, req *http.Request) {
	if toUrl := fetchRedirect(wr, req); toUrl != nil {
		http.Redirect(wr, req, toUrl.String(), http.StatusSeeOther)
	} else {
		http.Redirect(wr, req, "/", http.StatusSeeOther)
	}
}

// fetchRedirect fetches the redirect URL from the request cookies.
func fetchRedirect(wr http.ResponseWriter, req *http.Request) *url.URL {
	cookie, err := req.Cookie("redirect")
	if err != nil {
		return nil
	}
	// Delete the redirect cookie (MaxAge 0 would keep an empty cookie instead)
	http.SetCookie(wr, &http.Cookie{
		Name:   "redirect",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	// Only follow local paths. An empty or relative value would be resolved
	// against the callback path, e.g. /oidc/redirect -> /oidc/.
	if !strings.HasPrefix(cookie.Value, "/") || strings.HasPrefix(cookie.Value, "//") {
		return nil
	}
	toUrl, err := url.Parse(cookie.Value)
	if err != nil || toUrl.Scheme != "" || toUrl.Host != "" {
		return nil
	}
	return toUrl
}

// redirectLogin redirects to the login page, optionally storing the current URL as a redirect target.
func (s *Server) redirectLogin(wr http.ResponseWriter, req *http.Request, store bool) {
	if store {
		http.SetCookie(wr, &http.Cookie{
			Name:  "redirect",
			Value: req.RequestURI,
			Path:  "/",
		})
	}
	http.Redirect(wr, req, "/login", http.StatusSeeOther)
}
