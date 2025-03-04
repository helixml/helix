package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	ErrNotFound = errors.New("not found")
	ErrMultiple = errors.New("multiple found")
)

type CookieManager struct {
	SecureCookies bool
}

func NewCookieManager(config *config.ServerConfig) *CookieManager {
	return &CookieManager{
		SecureCookies: config.OIDC.SecureCookies,
	}
}

func (cm *CookieManager) Set(w http.ResponseWriter, r *http.Request, path, name, value string) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   cm.SecureCookies,
		HttpOnly: true,
		Path:     path,
		Domain:   "",
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, c)
}

func (cm *CookieManager) Get(r *http.Request, name string) (string, error) {
	// Check that there's only one cookie with this name, had issues before where there were
	// multiple cookies with this name
	cookies := r.Cookies()
	numNamedCookies := 0
	for _, cookie := range cookies {
		if cookie.Name == name {
			numNamedCookies++
		}
	}
	if numNamedCookies == 0 {
		return "", fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if numNamedCookies > 1 {
		return "", fmt.Errorf("%w: %s", ErrMultiple, name)
	}
	return cookies[0].Value, nil
}

func randString(nByte int) (string, error) {
	b := make([]byte, nByte)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// login godoc
// @Summary Login
// @Description Login to the application
// @Tags    auth
// @Success 200
// @Param request    body types.LoginRequest true "Request body with redirect URI.")
// @Router /api/v1/auth/login [post]
func (s *HelixAPIServer) login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	var loginRequest types.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&loginRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode login request")
		http.Error(w, "Failed to decode login request", http.StatusBadRequest)
		return
	}

	state, err := randString(64)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate state")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	nonce, err := randString(64)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate nonce")
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	cookieManager := NewCookieManager(s.Cfg)
	cookieManager.Set(w, r, "", "state", state)
	cookieManager.Set(w, r, "", "nonce", nonce)
	// Store the original URL if provided in the "redirect_uri" query parameter
	if loginRequest.RedirectURI != "" {
		// Validate the redirect URI
		if !strings.HasPrefix(loginRequest.RedirectURI, s.Cfg.WebServer.URL) {
			log.Debug().Str("server_url", s.Cfg.WebServer.URL).Str("redirect_uri", loginRequest.RedirectURI).Msg("Invalid redirect URI")
			http.Error(w, "Invalid redirect URI", http.StatusBadRequest)
			return
		}
		cookieManager.Set(w, r, "", "redirect_uri", loginRequest.RedirectURI)
	}

	log.Trace().Str("auth_url", s.oidcClient.GetAuthURL(state, nonce)).Msg("Redirecting to auth URL")
	http.Redirect(w, r, s.oidcClient.GetAuthURL(state, nonce), http.StatusFound)
}

// callback godoc
// @Summary Callback from OIDC provider
// @Description The callback receiver from the OIDC provider
// @Tags    auth
// @Success 200
// @Param code query string true "The code from the OIDC provider"
// @Param state query string true "The state from the OIDC provider"
// @Router /api/v1/auth/callback [get]
func (s *HelixAPIServer) callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	state, err := r.Cookie("state")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get state")
		http.Error(w, "state not found", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != state.Value {
		log.Debug().Str("state", r.URL.Query().Get("state")).Str("cookie", state.Value).Msg("state did not match")
		http.Error(w, "state did not match", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Debug().Msg("No code found in callback")
		http.Error(w, "No code found in callback", http.StatusBadRequest)
		return
	}

	oauth2Token, err := s.oidcClient.Exchange(ctx, code)
	if err != nil {
		log.Error().Err(err).Msg("Failed to exchange token")
		http.Error(w, "Failed to exchange token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	idToken, err := s.oidcClient.VerifyIDToken(ctx, oauth2Token)
	if err != nil {
		log.Error().Err(err).Msg("Failed to verify ID Token")
		http.Error(w, "Failed to verify ID Token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nonce, err := r.Cookie("nonce")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get nonce")
		http.Error(w, "nonce not found", http.StatusBadRequest)
		return
	}
	if idToken.Nonce != nonce.Value {
		log.Debug().Str("nonce", idToken.Nonce).Str("cookie", nonce.Value).Msg("nonce did not match")
		http.Error(w, "nonce did not match", http.StatusBadRequest)
		return
	}

	// Set access_token cookie
	cookieManager := NewCookieManager(s.Cfg)
	cookieManager.Set(w, r, "/", "access_token", oauth2Token.AccessToken)
	cookieManager.Set(w, r, "/", "refresh_token", oauth2Token.RefreshToken)

	// Check if we have a stored redirect URI
	redirectURI := s.Cfg.WebServer.URL // default redirect
	if cookie, err := r.Cookie("redirect_uri"); err == nil {
		redirectURI = cookie.Value
	}

	http.Redirect(w, r, redirectURI, http.StatusTemporaryRedirect)
}

// user godoc
// @Summary User information
// @Description Get the current user's information
// @Tags    auth
// @Success 200 {object} types.UserResponse
// @Router /api/v1/auth/user [get]
func (s *HelixAPIServer) user(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Method == http.MethodOptions {
		return
	}

	access_token, err := r.Cookie("access_token")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get access_token")
		http.Error(w, "access_token not found", http.StatusUnauthorized)
		return
	}

	userInfo, err := s.oidcClient.GetUserInfo(ctx, access_token.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get userinfo")
		http.Error(w, "Failed to get userinfo: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user, err := s.Store.GetUser(ctx, &store.GetUserQuery{
		Email: userInfo.Email,
	})
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get user")
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := types.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Token: access_token.Value,
		Name:  user.FullName,
	}
	json.NewEncoder(w).Encode(response)
}

// user godoc
// @Summary Logout
// @Description Logout the user
// @Tags    auth
// @Success 200
// @Router /api/v1/auth/logout [post]
func (s *HelixAPIServer) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	http.Redirect(w, r, s.oidcClient.GetLogoutURL(s.Cfg.WebServer.URL), http.StatusTemporaryRedirect)
}

// user godoc
// @Summary Authenticated
// @Description Check if the user is authenticated
// @Tags    auth
// @Success 200 {object} types.AuthenticatedResponse
// @Router /api/v1/auth/authenticated [get]
func (s *HelixAPIServer) authenticated(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Method == http.MethodOptions {
		return
	}

	accessToken, err := r.Cookie("access_token")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get access_token")
		http.Error(w, "access_token not found", http.StatusUnauthorized)
		return
	}

	err = s.oidcClient.VerifyAccessToken(ctx, accessToken.Value)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to verify access_token")
		json.NewEncoder(w).Encode(types.AuthenticatedResponse{
			Authenticated: false,
		})
		return
	}

	json.NewEncoder(w).Encode(types.AuthenticatedResponse{
		Authenticated: true,
	})
}

// user godoc
// @Summary Refresh the access token
// @Description Refresh the access token
// @Tags    auth
// @Success 204
// @Router /api/v1/auth/refresh [post]
func (s *HelixAPIServer) refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if r.Method == http.MethodOptions {
		return
	}

	refreshToken, err := r.Cookie("refresh_token")
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get refresh_token")
		http.Error(w, "refresh_token not found", http.StatusUnauthorized)
		return
	}

	newAccessToken, err := s.oidcClient.RefreshAccessToken(ctx, refreshToken.Value)
	if err != nil {
		log.Error().Err(err).Msg("Failed to refresh access_token")
		http.Error(w, "Failed to refresh access_token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cookieManager := NewCookieManager(s.Cfg)
	cookieManager.Set(w, r, "/", "access_token", newAccessToken.AccessToken)
	cookieManager.Set(w, r, "/", "refresh_token", newAccessToken.RefreshToken)

	w.WriteHeader(http.StatusNoContent)
}
