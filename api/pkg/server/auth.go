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
	var secureCookies bool

	if config.Auth.OIDC.SecureCookies {
		// Explicit override: OIDC_SECURE_COOKIES=true forces secure cookies
		// regardless of SERVER_URL protocol (e.g., behind HTTPS proxy)
		secureCookies = true
		log.Debug().Msg("OIDC_SECURE_COOKIES=true - forcing secure cookies")
	} else {
		// Auto-detect secure cookies based on SERVER_URL protocol
		// - HTTPS: use Secure cookies (standard secure behavior)
		// - HTTP: use non-Secure cookies (Safari strictly rejects Secure cookies over HTTP)
		secureCookies = strings.HasPrefix(config.WebServer.URL, "https://")
		if !secureCookies {
			log.Debug().
				Str("server_url", config.WebServer.URL).
				Msg("SERVER_URL is HTTP - using non-secure cookies for browser compatibility")
		}
	}

	return &CookieManager{
		SecureCookies: secureCookies,
	}
}

func (cm *CookieManager) Set(w http.ResponseWriter, c Cookie, value string) {
	cookie := &http.Cookie{
		Name:     c.Name,
		Path:     c.Path,
		Value:    value,
		MaxAge:   int(0),
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

// register godoc
// @Summary Register
// @Description Register a new user
// @Tags    auth
// @Success 200
// @Param request    body types.RegisterRequest true "Request body with email and password.")
// @Router /api/v1/auth/register [post]
func (s *HelixAPIServer) register(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.Auth.RegistrationEnabled {
		http.Error(w, "Registration is disabled", http.StatusForbidden)
		return
	}

	ctx := r.Context()

	var registerRequest types.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&registerRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode register request")
		http.Error(w, "Failed to decode register request", http.StatusBadRequest)
		return
	}

	if registerRequest.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if registerRequest.Password == "" {
		http.Error(w, "Password is required", http.StatusBadRequest)
		return
	}

	if registerRequest.Password != registerRequest.PasswordConfirm {
		http.Error(w, "Password and password confirmation do not match", http.StatusBadRequest)
		return
	}

	existingUser, err := s.Store.GetUser(ctx, &store.GetUserQuery{
		Email: registerRequest.Email,
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Error().Err(err).Msg("Failed to check if user exists")
		http.Error(w, "Failed to check if user exists", http.StatusInternalServerError)
		return
	}
	if existingUser != nil {
		http.Error(w, "Email is already taken", http.StatusConflict)
		return
	}

	userID := system.GenerateUserID()
	user := &types.User{
		ID:           userID,
		Email:        registerRequest.Email,
		FullName:     registerRequest.FullName,
		Username:     registerRequest.Email,
		Password:     registerRequest.Password,
		Type:         types.OwnerTypeUser,
		AuthProvider: types.AuthProviderRegular,
		CreatedAt:    time.Now(),
	}

	createdUser, err := s.authenticator.CreateUser(ctx, user)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create user")
		http.Error(w, "Failed to create user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	token, err := s.authenticator.GenerateUserToken(ctx, createdUser)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate user token")
		http.Error(w, "Failed to generate user token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cookieManager := NewCookieManager(s.Cfg)
	cookieManager.Set(w, accessTokenCookie, token)
	cookieManager.Set(w, refreshTokenCookie, token)

	response := types.UserResponse{
		ID:    createdUser.ID,
		Email: createdUser.Email,
		Token: token,
		Name:  createdUser.FullName,
	}
	writeResponse(w, response, http.StatusOK)
}

// accountUpdate godoc
// @Summary Update Account
// @Description Update the account information for the authenticated user
// @Tags    auth
// @Success 200 {object} types.UserResponse
// @Param request    body types.AccountUpdateRequest true "Request body with full name."
// @Router /api/v1/auth/update [post]
// @Security BearerAuth
func (s *HelixAPIServer) accountUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	ctx := r.Context()

	user := getRequestUser(r)
	if user == nil || user.ID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var accountUpdateRequest types.AccountUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&accountUpdateRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode account update request")
		http.Error(w, "Failed to decode account update request", http.StatusBadRequest)
		return
	}

	existingUser, err := s.Store.GetUser(ctx, &store.GetUserQuery{
		ID: user.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to get user")
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if existingUser == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	existingUser.FullName = accountUpdateRequest.FullName

	updatedUser, err := s.Store.UpdateUser(ctx, existingUser)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to update user")
		http.Error(w, "Failed to update user: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cm := NewCookieManager(s.Cfg)
	accessToken, err := cm.Get(r, accessTokenCookie)
	if err != nil {
		log.Debug().Err(err).Str("cookie", accessTokenCookie.Name).Msg("failed to get cookie")
		accessToken = ""
	}

	response := types.UserResponse{
		ID:    updatedUser.ID,
		Email: updatedUser.Email,
		Token: accessToken,
		Name:  updatedUser.FullName,
	}
	writeResponse(w, response, http.StatusOK)
}

// passwordReset godoc
// @Summary Password Reset
// @Description Reset the password for a user
// @Tags    auth
// @Success 200
// @Param request    body types.PasswordResetRequest true "Request body with email.")
// @Router /api/v1/auth/password-reset [post]
func (s *HelixAPIServer) passwordReset(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	var passwordResetRequest types.PasswordResetRequest
	if err := json.NewDecoder(r.Body).Decode(&passwordResetRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode password reset request")
		http.Error(w, "Failed to decode password reset request", http.StatusBadRequest)
		return
	}

	if passwordResetRequest.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	user, err := s.Store.GetUser(r.Context(), &store.GetUserQuery{
		Email: passwordResetRequest.Email,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to get user")
		http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if user == nil {
		log.Error().Str("email", passwordResetRequest.Email).Msg("User not found")
		http.Error(w, "User not found", http.StatusUnauthorized)
		return
	}

	err = s.authenticator.RequestPasswordReset(r.Context(), user.Email)
	if err != nil {
		log.Error().Err(err).Msg("Failed to request password reset")
		http.Error(w, "Failed to request password reset: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// passwordResetComplete godoc
// @Summary Complete Password Reset
// @Description Complete the password reset process using the access token from the reset email
// @Tags    auth
// @Success 200
// @Param request    body types.PasswordResetCompleteRequest true "Request body with access token and new password."
// @Router /api/v1/auth/password-reset-complete [post]
func (s *HelixAPIServer) passwordResetComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	ctx := r.Context()

	var passwordResetCompleteRequest types.PasswordResetCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&passwordResetCompleteRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode password reset complete request")
		http.Error(w, "Failed to decode password reset complete request", http.StatusBadRequest)
		return
	}

	if passwordResetCompleteRequest.AccessToken == "" {
		http.Error(w, "Access token is required", http.StatusBadRequest)
		return
	}

	if passwordResetCompleteRequest.NewPassword == "" {
		http.Error(w, "New password is required", http.StatusBadRequest)
		return
	}

	err := s.authenticator.PasswordResetComplete(ctx, passwordResetCompleteRequest.AccessToken, passwordResetCompleteRequest.NewPassword)
	if err != nil {
		log.Error().Err(err).Msg("Failed to complete password reset")
		http.Error(w, "Failed to complete password reset: "+err.Error(), http.StatusBadRequest)
		return
	}

	user, err := s.authenticator.ValidateUserToken(ctx, passwordResetCompleteRequest.AccessToken)
	if err != nil {
		log.Error().Err(err).Msg("Failed to validate user token")
		http.Error(w, "Failed to validate user token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Generate new token as well
	token, err := s.authenticator.GenerateUserToken(ctx, user)
	if err != nil {
		log.Error().Err(err).Msg("Failed to generate user token")
		http.Error(w, "Failed to generate user token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	cookieManager := NewCookieManager(s.Cfg)
	cookieManager.Set(w, accessTokenCookie, token)
	cookieManager.Set(w, refreshTokenCookie, token)

	response := types.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Token: token,
		Name:  user.FullName,
	}
	writeResponse(w, response, http.StatusOK)
}

// passwordUpdate godoc
// @Summary Update Password
// @Description Update the password for the authenticated user
// @Tags    auth
// @Success 200
// @Param request    body types.PasswordUpdateRequest true "Request body with new password."
// @Router /api/v1/auth/password-update [post]
// @Security BearerAuth
func (s *HelixAPIServer) passwordUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		return
	}

	ctx := r.Context()

	user := getRequestUser(r)
	if user == nil || user.ID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var passwordUpdateRequest types.PasswordUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&passwordUpdateRequest); err != nil {
		log.Error().Err(err).Msg("Failed to decode password update request")
		http.Error(w, "Failed to decode password update request", http.StatusBadRequest)
		return
	}

	if passwordUpdateRequest.NewPassword == "" {
		http.Error(w, "New password is required", http.StatusBadRequest)
		return
	}

	err := s.authenticator.UpdatePassword(ctx, user.ID, passwordUpdateRequest.NewPassword)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to update password")
		http.Error(w, "Failed to update password: "+err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
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
			http.Error(w, fmt.Sprintf("invalid redirect URI: %s != %s", loginRequest.RedirectURI, s.Cfg.WebServer.URL), http.StatusBadRequest)
			return
		}
		cookieManager.Set(w, redirectURICookie, loginRequest.RedirectURI)
	}

	if loginRequest.Email != "" && loginRequest.Password != "" {
		user, err := s.Store.GetUser(r.Context(), &store.GetUserQuery{
			Email: loginRequest.Email,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				log.Error().Str("email", loginRequest.Email).Msg("User not found")
				http.Error(w, "User with this email does not exist", http.StatusUnauthorized)
				return
			}
			log.Error().Err(err).Msg("Failed to get user")
			http.Error(w, "Failed to get user: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if user == nil {
			log.Error().Str("email", loginRequest.Email).Msg("User not found")
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		err = s.authenticator.ValidatePassword(r.Context(), user, loginRequest.Password)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate password")
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}

		// Generate a new token
		token, err := s.authenticator.GenerateUserToken(r.Context(), user)
		if err != nil {
			log.Error().Err(err).Msg("Failed to generate user token")
			http.Error(w, "Failed to generate user token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// OK, set authentication cookies and redirect
		cookieManager.Set(w, accessTokenCookie, token)
		cookieManager.Set(w, refreshTokenCookie, token)

		response := types.UserResponse{
			ID:    user.ID,
			Email: user.Email,
			Token: token,
			Name:  user.FullName,
		}
		writeResponse(w, response, http.StatusOK)

		return
	}
	// OIDC - only available if not using regular auth
	if s.Cfg.Auth.Provider != types.AuthProviderRegular {
		redirectURL := s.oidcClient.GetAuthURL(state, nonce)
		if redirectURL == "" {
			log.Error().Msg("empty redirect URL")
			http.Error(w, "empty redirect URL", http.StatusBadGateway)
			return
		}
		log.Trace().Str("auth_url", redirectURL).Msg("Redirecting to auth URL")

		// If get_url=true query param is set, return the URL as JSON instead of redirecting
		// This is needed because browsers can't follow cross-origin redirects from fetch()
		if r.URL.Query().Get("get_url") == "true" {
			writeResponse(w, map[string]string{"url": redirectURL}, http.StatusOK)
			return
		}

		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}
	// Regular auth provider - email and password are required
	log.Error().Msg("Email and password are required for regular authentication")
	http.Error(w, "Email and password are required", http.StatusBadRequest)
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
	if s.Cfg.Auth.Provider == types.AuthProviderRegular {
		http.Error(w, "OIDC not turned on", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	cm := NewCookieManager(s.Cfg)
	state, err := cm.Get(r, stateCookie)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get state")
		// Redirect to the homepage
		http.Redirect(w, r, s.Cfg.WebServer.URL, http.StatusFound)
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
		log.Error().
			Err(err).
			Str("provider_url", s.Cfg.Auth.OIDC.URL).
			Str("callback_url", fmt.Sprintf("%s/api/v1/auth/callback", s.Cfg.WebServer.URL)).
			Str("auth_code", code).
			Msg("Failed to exchange token")
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

	var user *types.User

	switch s.Cfg.Auth.Provider {
	case types.AuthProviderRegular:
		user, err = s.authenticator.ValidateUserToken(ctx, accessToken)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate user token")
			http.Error(w, "Failed to validate user token: "+err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		// OIDC-based auth (keycloak, oidc, or any future OIDC provider)
		// Uses ValidateUserToken which properly computes admin status from ADMIN_USER_IDS
		user, err = s.oidcClient.ValidateUserToken(ctx, accessToken)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate user token")
			// If the error contains "401" or "unauthorized", it's likely an expired/invalid token
			errStr := strings.ToLower(err.Error())
			if strings.Contains(errStr, "401") || strings.Contains(errStr, "unauthorized") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			} else {
				http.Error(w, "Failed to validate user token: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}
	}

	response := types.UserResponse{
		ID:    user.ID,
		Email: user.Email,
		Token: accessToken,
		Name:  user.FullName,
		Admin: user.Admin,
	}
	writeResponse(w, response, http.StatusOK)
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

	if s.Cfg.Auth.Provider == types.AuthProviderRegular {
		// If get_url=true query param is set, return the URL as JSON instead of redirecting
		if r.URL.Query().Get("get_url") == "true" {
			writeResponse(w, map[string]string{"url": s.Cfg.WebServer.URL}, http.StatusOK)
			return
		}
		http.Redirect(w, r, s.Cfg.WebServer.URL, http.StatusFound)
		return
	}

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

	// If get_url=true query param is set, return the URL as JSON instead of redirecting
	// This is needed because browsers can't follow cross-origin redirects from fetch()
	if r.URL.Query().Get("get_url") == "true" {
		writeResponse(w, map[string]string{"url": logoutURL}, http.StatusOK)
		return
	}

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

		writeResponse(w, types.AuthenticatedResponse{
			Authenticated: false,
		}, http.StatusOK)
		return
	}

	_, err = s.authenticator.ValidateUserToken(ctx, accessToken)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to validate user token")
		writeResponse(w, types.AuthenticatedResponse{
			Authenticated: false,
		}, http.StatusOK)
		return
	}

	// Authenticated and verified

	writeResponse(w, types.AuthenticatedResponse{
		Authenticated: true,
	}, http.StatusOK)
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

	var (
		newAccessToken  string
		newRefreshToken string
	)

	switch s.Cfg.Auth.Provider {
	case types.AuthProviderRegular:

		user, err := s.authenticator.ValidateUserToken(ctx, refreshToken)
		if err != nil {
			log.Error().Err(err).Msg("Failed to validate user token")
			http.Error(w, "Failed to validate user token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		token, err := s.authenticator.GenerateUserToken(ctx, user)
		if err != nil {
			log.Error().Err(err).Msg("Failed to generate user token")
			http.Error(w, "Failed to generate user token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		newAccessToken = token
		newRefreshToken = token
	default:
		// OIDC-based auth (keycloak, oidc, or any future OIDC provider)
		token, err := s.oidcClient.RefreshAccessToken(ctx, refreshToken)
		if err != nil {
			log.Error().Err(err).Msg("Failed to refresh access_token")
			http.Error(w, "Failed to refresh access_token: "+err.Error(), http.StatusInternalServerError)
			return
		}

		newAccessToken = token.AccessToken
		newRefreshToken = token.RefreshToken
	}

	cm.Set(w, accessTokenCookie, newAccessToken)
	cm.Set(w, refreshTokenCookie, newRefreshToken)

	w.WriteHeader(http.StatusNoContent)
}
