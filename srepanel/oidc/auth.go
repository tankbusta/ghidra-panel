package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"go.mkw.re/ghidra-panel/csrf"
)

// verifierCookie stores the PKCE code verifier between the login and redirect requests.
const verifierCookie = "oidc_verifier"

// discovery https://openid.net/specs/openid-connect-discovery-1_0.html#ProviderMetadata
type discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// UserInfo is the subset of the identity returned by the OIDC provider.
type UserInfo struct {
	Subject  string
	Username string
}

type Auth struct {
	oauth2.Config
	Issuer        string
	usernameClaim string
	userinfoURL   string
	prot          *csrf.OneTime
}

// NewAuth fetches the provider's discovery document and returns a configured client.
func NewAuth(ctx context.Context, issuer, clientID, clientSecret, redirectURL string, scopes []string, usernameClaim string) (*Auth, error) {
	issuer = strings.TrimSuffix(issuer, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery failed: %s", res.Status)
	}

	var disc discovery
	if err := json.NewDecoder(res.Body).Decode(&disc); err != nil {
		return nil, fmt.Errorf("oidc discovery failed: %w", err)
	}
	if strings.TrimSuffix(disc.Issuer, "/") != issuer {
		return nil, fmt.Errorf("oidc discovery issuer mismatch: expected %q, got %q", issuer, disc.Issuer)
	}
	if disc.AuthorizationEndpoint == "" || disc.TokenEndpoint == "" || disc.UserinfoEndpoint == "" {
		return nil, errors.New("oidc discovery document missing required endpoints")
	}

	return &Auth{
		Config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Endpoint: oauth2.Endpoint{
				AuthURL:  disc.AuthorizationEndpoint,
				TokenURL: disc.TokenEndpoint,
			},
			RedirectURL: redirectURL,
			Scopes:      scopes,
		},
		Issuer:        disc.Issuer,
		usernameClaim: usernameClaim,
		userinfoURL:   disc.UserinfoEndpoint,
		prot:          csrf.NewOneTime(),
	}, nil
}

// RedirectToProvider starts the authorization code flow with PKCE.
func (c *Auth) RedirectToProvider(wr http.ResponseWriter, req *http.Request) {
	verifier := oauth2.GenerateVerifier()
	http.SetCookie(wr, &http.Cookie{
		Name:     verifierCookie,
		Value:    verifier,
		Path:     "/oidc",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	authURL := c.Config.AuthCodeURL(c.prot.Issue(), oauth2.S256ChallengeOption(verifier))
	http.Redirect(wr, req, authURL, http.StatusSeeOther)
}

// HandleRedirect handles an OAuth2 redirect from the identity provider.
func (c *Auth) HandleRedirect(wr http.ResponseWriter, req *http.Request) (info *UserInfo, err error) {
	ctx := req.Context()

	errID := req.FormValue("error")
	errDescription := req.FormValue("error_description")
	if errID != "" {
		if errID == "access_denied" {
			// The status prevents the login page from redirecting back to the provider
			http.Redirect(wr, req, "/login?status=access_denied", http.StatusSeeOther)
			return nil, nil
		}
		http.Error(wr, errDescription, http.StatusUnauthorized)
		return nil, nil
	}

	query := req.URL.Query()
	code := query.Get("code")
	state := query.Get("state")

	// Check CSRF token validity -- do not consume yet
	csrfID, err := c.prot.Check(state)
	if err != nil {
		return nil, err
	}

	cookie, err := req.Cookie(verifierCookie)
	if err != nil || cookie.Value == "" {
		return nil, errors.New("missing PKCE verifier cookie")
	}
	http.SetCookie(wr, &http.Cookie{
		Name:   verifierCookie,
		Value:  "",
		Path:   "/oidc",
		MaxAge: -1,
	})

	// Request authorization token from the provider
	token, err := c.Config.Exchange(ctx, code, oauth2.VerifierOption(cookie.Value))
	if err != nil {
		return nil, err
	}

	// The token was received directly from the token endpoint over TLS,
	// so the userinfo endpoint is trusted to identify the user.
	info, err = c.GetUserInfo(ctx, token)
	if err != nil {
		return nil, err
	}

	// Prevent CSRF token reuse
	err = c.prot.Consume(csrfID)
	return
}

func (c *Auth) GetUserInfo(ctx context.Context, token *oauth2.Token) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.userinfoURL, nil)
	if err != nil {
		return nil, err
	}

	res, err := c.Config.Client(ctx, token).Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed: %s", res.Status)
	}

	var claims map[string]any
	if err := json.NewDecoder(res.Body).Decode(&claims); err != nil {
		return nil, err
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, errors.New("userinfo response missing sub")
	}

	// Pick the first available username claim
	username := sub
	for _, key := range []string{c.usernameClaim, "preferred_username", "name", "email"} {
		if v, _ := claims[key].(string); v != "" {
			username = v
			break
		}
	}

	return &UserInfo{
		Subject:  sub,
		Username: username,
	}, nil
}
