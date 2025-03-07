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
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	ErrNotFound = errors.New("not found")
	ErrMultiple = errors.New("multiple found")
)

// Cookies are set, used and deleted at different points in the lifecycle of the request.
// If found it hard to line these up correctly, especially the paths, so we'll use a struct to
// represent each cookie.
type Cookie struct {
	Name string
	Path string
}

var (
	accessTokenCookie = Cookie{
		Name: "access_token",
		Path: "/",
	}
	refreshTokenCookie = Cookie{
		Name: "refresh_token",
		Path: "/",
	}
	stateCookie = Cookie{
		Name: "state",
		Path: "",
	}
	nonceCookie = Cookie{
		Name: "nonce",
		Path: "",
	}
	redirectURICookie = Cookie{
		Name: "redirect_uri",
		Path: "",
	}
)

// CookieManager is a helper for setting, getting and deleting cookies.
type CookieManager struct {
	SecureCookies bool
}

func NewCookieManager(config *config.ServerConfig) *CookieManager {
	return &CookieManager{
		SecureCookies: config.OIDC.SecureCookies,
	}
}

func (cm *CookieManager) Set(w http.ResponseWriter, c Cookie, value string) {
	cookie := &http.Cookie{
		Name:     c.Name,
		Path:     c.Path,
		Value:    value,
		MaxAge:   int(time.Hour.Seconds()),
		Secure:   cm.SecureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

func (cm *CookieManager) Get(r *http.Request, c Cookie) (string, error) {
	// Check that there's only one cookie with this name, had issues before where there were
	// multiple cookies with this name
	cookies := r.CookiesNamed(c.Name)
	numNamedCookies := 0
	for _, cookie := range cookies {
		if cookie.Name == c.Name {
			numNamedCookies++
		}
	}
	if numNamedCookies == 0 {
		return "", fmt.Errorf("%w: %s", ErrNotFound, c.Name)
	}
	if numNamedCookies > 1 {
		return "", fmt.Errorf("%w: %s", ErrMultiple, c.Name)
	}
	return cookies[0].Value, nil
}

func (cm *CookieManager) DeleteAllCookies(w http.ResponseWriter) {
	cm.delete(w, accessTokenCookie)
	cm.delete(w, refreshTokenCookie)
	cm.delete(w, stateCookie)
	cm.delete(w, nonceCookie)
	cm.delete(w, redirectURICookie)
}

func (cm *CookieManager) delete(w http.ResponseWriter, c Cookie) {
	cookie := &http.Cookie{
		Name:     c.Name,
		Path:     c.Path,
		Value:    "",
		MaxAge:   -1,
		Secure:   cm.SecureCookies,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
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
	cookieManager.Set(w, stateCookie, state)
	cookieManager.Set(w, nonceCookie, nonce)
	// Store the original URL if provided in the "redirect_uri" query parameter
	if loginRequest.RedirectURI != "" {
		// Validate the redirect URI
		if s.Cfg.WebServer.URL == "" {
			log.Error().Msg("WebServer.URL is not set, unable to validate redirect URI")
			http.Error(w, "unable to validate redirect URI", http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(loginRequest.RedirectURI, s.Cfg.WebServer.URL) {
			log.Debug().Str("server_url", s.Cfg.WebServer.URL).Str("redirect_uri", loginRequest.RedirectURI).Msg("Invalid redirect URI")
			http.Error(w, "invalid redirect URI", http.StatusBadRequest)
			return
		}
		cookieManager.Set(w, redirectURICookie, loginRequest.RedirectURI)
	}

	redirectURL := s.oidcClient.GetAuthURL(state, nonce)
	if redirectURL == "" {
		log.Error().Msg("empty redirect URL")
		http.Error(w, "empty redirect URL", http.StatusBadGateway)
		return
	}
	log.Trace().Str("auth_url", redirectURL).Msg("Redirecting to auth URL")
	http.Redirect(w, r, redirectURL, http.StatusFound)
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
	cm := NewCookieManager(s.Cfg)
	state, err := cm.Get(r, stateCookie)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get state")
		http.Error(w, "failed to get state cookie", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != state {
		log.Debug().Str("state", r.URL.Query().Get("state")).Str("cookie", state).Msg("state did not match")
		// Remove the cookies and start again
		cm.DeleteAllCookies(w)
		// Redirect to the homepage
		http.Redirect(w, r, s.Cfg.WebServer.URL, http.StatusFound)
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

	nonce, err := cm.Get(r, nonceCookie)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get nonce")
		http.Error(w, "nonce not found", http.StatusBadRequest)
		return
	}
	if idToken.Nonce != nonce {
		log.Debug().Str("nonce", idToken.Nonce).Str("cookie", nonce).Msg("nonce did not match")
		http.Error(w, "nonce did not match", http.StatusBadRequest)
		return
	}

	// Set cookies, if applicable
	if oauth2Token.AccessToken != "" {
		cm.Set(w, accessTokenCookie, oauth2Token.AccessToken)
	} else {
		log.Debug().Msg("access_token is empty")
		http.Error(w, "access_token is empty", http.StatusBadRequest)
		return
	}
	if oauth2Token.RefreshToken != "" {
		cm.Set(w, refreshTokenCookie, oauth2Token.RefreshToken)
	} else {
		log.Debug().Msg("refresh_token is empty, ignoring")
	}

	// Check if we have a stored redirect URI
	redirectURI := s.Cfg.WebServer.URL // default redirect
	if cookie, err := cm.Get(r, redirectURICookie); err == nil {
		redirectURI = cookie
	}

	http.Redirect(w, r, redirectURI, http.StatusFound)
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

	cm := NewCookieManager(s.Cfg)
	accessToken, err := cm.Get(r, accessTokenCookie)
	if err != nil {
		log.Debug().Err(err).Str("cookie", accessTokenCookie.Name).Msg("failed to get cookie")
		http.Error(w, fmt.Sprintf("failed to get cookie: %s", accessTokenCookie.Name), http.StatusUnauthorized)
		return
	}

	userInfo, err := s.oidcClient.GetUserInfo(ctx, accessToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get userinfo")
		http.Error(w, "Failed to get userinfo: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Trace().Interface("userinfo", userInfo).Msg("Userinfo")

	user, err := s.Store.GetUser(ctx, &store.GetUserQuery{
		Email: userInfo.Email,
	})
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Debug().Err(err).Msg("Failed to get user")
			http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Extract the full name from the userinfo
	fullName := "unknown"
	if userInfo.Name != "" {
		fullName = userInfo.Name
	} else if userInfo.GivenName != "" && userInfo.FamilyName != "" {
		fullName = fmt.Sprintf("%s %s", userInfo.GivenName, userInfo.FamilyName)
	}

	if user == nil {
		user, err = s.Store.CreateUser(ctx, &types.User{
			ID:        system.GenerateUserID(),
			Username:  userInfo.Email,
			Email:     userInfo.Email,
			FullName:  fullName,
			CreatedAt: time.Now(),
		})
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return
			}
			log.Error().Err(err).Msg("Failed to create user")
			http.Error(w, "Failed to create user: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// If existing has changed, update the user
	if user.Email != userInfo.Email || user.FullName != fullName || user.Username != userInfo.Email {
		user.Email = userInfo.Email
		user.FullName = fullName
		user.Username = userInfo.Email
		_, err = s.Store.UpdateUser(ctx, user)
		if err != nil {
			log.Error().Err(err).Msg("Failed to update user")
			http.Error(w, "Failed to update user: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	response := types.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Token: accessToken,
		Name:  user.FullName,
	}
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode user response")
		http.Error(w, "Failed to encode user response: "+err.Error(), http.StatusInternalServerError)
		return
	}
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

	// Remove cookies
	NewCookieManager(s.Cfg).DeleteAllCookies(w)

	logoutURL, err := s.oidcClient.GetLogoutURL()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get logout URL")
		http.Error(w, "Failed to get logout URL: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if logoutURL == "" {
		log.Error().Msg("empty logout URL")
		http.Error(w, "empty logout URL", http.StatusBadGateway)
		return
	}
	log.Debug().Str("logout_url", logoutURL).Msg("Redirecting to logout URL")
	http.Redirect(w, r, logoutURL, http.StatusFound)
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

	cm := NewCookieManager(s.Cfg)
	accessToken, err := cm.Get(r, accessTokenCookie)
	if err != nil {
		log.Debug().Err(err).Str("cookie", accessTokenCookie.Name).Msg("failed to get cookie")
		err = json.NewEncoder(w).Encode(types.AuthenticatedResponse{
			Authenticated: false,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to encode authenticated response")
			http.Error(w, "Failed to encode authenticated response: "+err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	err = s.oidcClient.VerifyAccessToken(ctx, accessToken)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to verify access_token")
		err = json.NewEncoder(w).Encode(types.AuthenticatedResponse{
			Authenticated: false,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to encode authenticated response")
			http.Error(w, "Failed to encode authenticated response: "+err.Error(), http.StatusInternalServerError)
			return
		}
		return
	}

	err = json.NewEncoder(w).Encode(types.AuthenticatedResponse{
		Authenticated: true,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode authenticated response")
		http.Error(w, "Failed to encode authenticated response: "+err.Error(), http.StatusInternalServerError)
		return
	}
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

	cm := NewCookieManager(s.Cfg)
	refreshToken, err := cm.Get(r, refreshTokenCookie)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get refresh_token, skipping refresh")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if refreshToken == "" {
		log.Debug().Msg("refresh_token is empty, skipping refresh")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	newAccessToken, err := s.oidcClient.RefreshAccessToken(ctx, refreshToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to refresh access_token")
		http.Error(w, "Failed to refresh access_token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cm.Set(w, accessTokenCookie, newAccessToken.AccessToken)
	cm.Set(w, refreshTokenCookie, newAccessToken.RefreshToken)

	w.WriteHeader(http.StatusNoContent)
}
