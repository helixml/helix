package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	jwt "github.com/golang-jwt/jwt/v5"
	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	// Allowed paths for app API keys. Currently we support
	// only OpenAI compatible chat completions API
	AppAPIKeyPaths = map[string]bool{
		"/v1/chat/completions":  true,
		"/api/v1/sessions/chat": true,
	}
)

var (
	ErrNoAPIKeyFound           = errors.New("no API key found")
	ErrNoUserIDFound           = errors.New("no user ID found")
	ErrAppAPIKeyPathNotAllowed = errors.New("path not allowed for app API keys, use your personal account key from your /account page instead")
	// ErrHelixTokenWithOIDC is returned when a Helix-issued JWT is used while OIDC authentication
	// is configured. This can happen when a user has stale cookies from when the server was using
	// regular auth. The user needs to clear their cookies and log in again via OIDC.
	ErrHelixTokenWithOIDC = errors.New("token format mismatch: please log out and log in again")
)

type authMiddlewareConfig struct {
	// adminUserIDs is a list of user IDs that should be admins.
	// Can contain "all" for dev mode (everyone is admin), or specific user IDs.
	adminUserIDs []string
	runnerToken  string
}

type authMiddleware struct {
	authenticator  authpkg.Authenticator
	oidcClient     authpkg.OIDC // For OIDC token validation (nil if not using OIDC)
	store          store.Store
	cfg            authMiddlewareConfig
	serverCfg      *config.ServerConfig // Server config for cookie management
	sessionManager *authpkg.SessionManager // BFF session manager (nil if not using sessions)
}

func newAuthMiddleware(
	authenticator authpkg.Authenticator,
	oidcClient authpkg.OIDC,
	store store.Store,
	cfg authMiddlewareConfig,
	serverCfg *config.ServerConfig,
	sessionManager *authpkg.SessionManager,
) *authMiddleware {
	return &authMiddleware{
		authenticator:  authenticator,
		oidcClient:     oidcClient,
		store:          store,
		cfg:            cfg,
		serverCfg:      serverCfg,
		sessionManager: sessionManager,
	}
}

type tokenAcct struct {
	jwt    *jwt.Token
	userID string
}

type account struct {
	userID string
	token  *tokenAcct
}

type accountType string

const (
	accountTypeUser    accountType = "user"
	accountTypeToken   accountType = "token"
	accountTypeInvalid accountType = "invalid"
)

func (a *account) Type() accountType {
	switch {
	case a.userID != "":
		return accountTypeUser
	case a.token != nil:
		return accountTypeToken
	}
	return accountTypeInvalid
}

// looksLikeHelixJWT checks if a token appears to be a Helix-issued JWT
// by parsing the token without verification and checking the issuer claim.
// This is used to detect stale Helix JWTs when OIDC authentication is configured,
// which can happen when a user has cookies from when the server used regular auth.
func looksLikeHelixJWT(token string) bool {
	// Parse token without validation to check claims
	parser := jwt.NewParser()
	parsedToken, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		// Not a valid JWT structure - could be a Google opaque token
		return false
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return false
	}

	// Check if issuer is "helix" - this indicates a Helix-generated JWT
	issuer, _ := claims["iss"].(string)
	return issuer == "helix"
}

// isAdminWithContext checks admin status.
// If ADMIN_USER_IDS contains "all", everyone is admin (dev mode).
// If ADMIN_USER_IDS contains the user's ID, they are admin.
// Otherwise, fetches the admin status from the database user record.
func (auth *authMiddleware) isAdminWithContext(ctx context.Context, userID string) bool {
	if userID == "" {
		return false
	}

	// Check if user is in the admin list (includes "all" check)
	for _, adminID := range auth.cfg.adminUserIDs {
		if adminID == config.AdminAllUsers {
			return true // "all" means everyone is admin
		}
		if adminID == userID {
			return true // User is explicitly in the admin list
		}
	}

	// If admin list is empty or user not in list, fetch admin status from database
	dbUser, err := auth.store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil {
		log.Warn().Err(err).Str("user_id", userID).Msg("failed to get user from db for admin check")
		return false
	}
	if dbUser == nil {
		return false
	}
	return dbUser.Admin
}

func (auth *authMiddleware) getUserFromToken(ctx context.Context, token string) (*types.User, error) {
	if token == "" {
		return nil, nil
	}

	if token == auth.cfg.runnerToken {
		// if the api key is our runner token then we are in runner mode
		return &types.User{
			ID:        "runner-system", // Add a system user ID for runner tokens
			Type:      types.OwnerTypeUser,
			Token:     token,
			TokenType: types.TokenTypeRunner,
		}, nil
	}

	if strings.HasPrefix(token, types.APIKeyPrefix) {
		// we have an API key - we should load it from the database and construct our user that way
		apiKey, err := auth.store.GetAPIKey(ctx, &types.ApiKey{
			Key: token,
		})
		if err != nil {
			return nil, fmt.Errorf("error getting API key: %s", err.Error())
		}
		if apiKey == nil {
			return nil, fmt.Errorf("error getting API key: %w", ErrNoAPIKeyFound)
		}

		// Try to get user from local database first, fall back to Keycloak if available
		var user *types.User

		dbUser, err := auth.store.GetUser(ctx, &store.GetUserQuery{ID: apiKey.Owner})
		if err == nil && dbUser != nil {
			// User found in local database
			user = dbUser
		} else if auth.authenticator != nil {
			// Fall back to Keycloak
			user, err = auth.authenticator.GetUserByID(ctx, apiKey.Owner)
			if err != nil {
				return nil, fmt.Errorf("error loading user from keycloak: %s", err.Error())
			}
		} else {
			// No local user and no Keycloak - create minimal user from API key owner
			// TODO: remove???
			user = &types.User{
				ID:   apiKey.Owner,
				Type: apiKey.OwnerType,
			}
		}

		user.Token = token
		user.TokenType = types.TokenTypeAPIKey
		user.ID = apiKey.Owner
		user.Type = apiKey.OwnerType
		user.Admin = auth.isAdminWithContext(ctx, user.ID)
		if apiKey.AppID != nil && apiKey.AppID.Valid {
			user.AppID = apiKey.AppID.String
		}
		user.ProjectID = apiKey.ProjectID
		user.SpecTaskID = apiKey.SpecTaskID
		user.SessionID = apiKey.SessionID

		// Ensure user_meta exists with slug for GitHub-style URLs
		if user.ID != "" {
			_, err := auth.store.EnsureUserMeta(ctx, types.UserMeta{
				ID: user.ID,
			})
			if err != nil {
				log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to ensure user meta")
				// Don't fail auth if user_meta creation fails - just log the warning
			}
		}

		return user, nil
	}

	// Try to decode the token - use OIDC client if available, otherwise use Helix authenticator
	var user *types.User
	var err error
	if auth.oidcClient != nil {
		// OIDC is configured - check for stale Helix JWTs before calling Google's userinfo endpoint.
		// This can happen when a user has cookies from when the server was using regular auth,
		// or when AUTH_PROVIDER was changed from "regular" to "oidc".
		// Detecting this early provides a clearer error message instead of "Invalid Credentials".
		if looksLikeHelixJWT(token) {
			log.Warn().Msg("detected Helix JWT token while OIDC is configured - user needs to clear cookies and re-login")
			return nil, ErrHelixTokenWithOIDC
		}
		// Use OIDC client for token validation (validates via userinfo endpoint)
		user, err = auth.oidcClient.ValidateUserToken(ctx, token)
	} else {
		// Fall back to Helix authenticator (validates Helix-issued JWTs)
		user, err = auth.authenticator.ValidateUserToken(ctx, token)
	}
	if err != nil {
		log.Error().Err(err).Str("token", token).Msg("error validating user token")
		return nil, err
	}

	// Ensure user_meta exists with slug for GitHub-style URLs
	if user != nil && user.ID != "" {
		_, err := auth.store.EnsureUserMeta(ctx, types.UserMeta{
			ID: user.ID,
		})
		if err != nil {
			log.Warn().Err(err).Str("user_id", user.ID).Msg("failed to ensure user meta")
			// Don't fail auth if user_meta creation fails - just log the warning
		}
	}

	return user, nil
}

// getUserFromSession checks for a valid BFF session and returns the user
// This is the primary auth method for browser-based clients using HttpOnly session cookies
func (auth *authMiddleware) getUserFromSession(ctx context.Context, r *http.Request) (*types.User, error) {
	if auth.sessionManager == nil {
		return nil, authpkg.ErrSessionNotFound
	}

	session, err := auth.sessionManager.GetSessionFromRequest(ctx, r)
	if err != nil {
		return nil, err
	}

	// Load user from database
	dbUser, err := auth.store.GetUser(ctx, &store.GetUserQuery{ID: session.UserID})
	if err != nil {
		return nil, fmt.Errorf("failed to load user for session: %w", err)
	}
	if dbUser == nil {
		return nil, fmt.Errorf("user not found for session: %s", session.UserID)
	}

	// Set token type based on auth provider
	if session.AuthProvider == types.AuthProviderOIDC {
		dbUser.Token = session.OIDCAccessToken
		dbUser.TokenType = types.TokenTypeOIDC
	} else {
		dbUser.TokenType = types.TokenTypeSession
	}

	dbUser.Admin = auth.isAdminWithContext(ctx, dbUser.ID)

	return dbUser, nil
}

// this will extract the token from the request and then load the correct
// user based on what type of token it is
// if there is no token, a default user object will be written to the
// request context
func (auth *authMiddleware) extractMiddleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		var user *types.User
		var err error

		// First, try BFF session authentication (from helix_session cookie)
		// This is the primary auth method for browser clients
		if auth.sessionManager != nil {
			user, err = auth.getUserFromSession(r.Context(), r)
			if err == nil && user != nil {
				// Successfully authenticated via session
				r = r.WithContext(setRequestUser(r.Context(), *user))
				next.ServeHTTP(w, r)
				return
			}

			// Session expired - clear the session cookie
			if errors.Is(err, authpkg.ErrSessionExpired) {
				auth.sessionManager.ClearSessionCookie(w)
			}

			// If session auth failed but it's not a "not found" error, log it
			if err != nil && !errors.Is(err, authpkg.ErrSessionNotFound) {
				log.Debug().Err(err).Str("path", r.URL.Path).Msg("BFF session auth failed, trying token auth")
			}

			// Fall through to token-based auth
			err = nil
		}

		// Fall back to token-based authentication (API keys, runner tokens, OIDC tokens)
		user, err = auth.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			// Check if error is due to server not ready vs invalid token
			// Return 503 for server errors so frontend doesn't auto-logout during API restart
			if errors.Is(err, authpkg.ErrProviderNotReady) {
				log.Warn().Err(err).Str("path", r.URL.Path).Msg("OIDC provider not ready during auth check")
				http.Error(w, "Authentication service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			// If the error is due to stale Helix JWT while OIDC is configured,
			// clear all cookies so the user is forced to re-login via OIDC
			if errors.Is(err, ErrHelixTokenWithOIDC) && auth.serverCfg != nil {
				log.Warn().Str("path", r.URL.Path).Msg("Clearing stale Helix JWT cookies - user needs to re-login via OIDC")
				NewCookieManager(auth.serverCfg).DeleteAllCookies(w)
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			// With BFF pattern, token refresh is handled by SessionManager
			// API keys and runner tokens don't need refresh
			if err != nil {
				log.Debug().Err(err).Str("path", r.URL.Path).Msg("Auth error - returning 401")
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}
		}
		if user == nil {
			user = &types.User{}
		}

		// If app API key, check if the path is in the allowed list
		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		r = r.WithContext(setRequestUser(r.Context(), *user))
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}

func (auth *authMiddleware) auth(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.getUserFromToken(r.Context(), getRequestToken(r))
		if err != nil {
			// Check if error is due to server not ready vs invalid token
			// Return 503 for server errors so frontend doesn't auto-logout during API restart
			if errors.Is(err, authpkg.ErrProviderNotReady) {
				log.Warn().Err(err).Str("path", r.URL.Path).Msg("OIDC provider not ready during auth check")
				http.Error(w, "Authentication service temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			// If the error is due to stale Helix JWT while OIDC is configured,
			// clear all cookies so the user is forced to re-login via OIDC
			if errors.Is(err, ErrHelixTokenWithOIDC) && auth.serverCfg != nil {
				log.Warn().Str("path", r.URL.Path).Msg("Clearing stale Helix JWT cookies - user needs to re-login via OIDC")
				NewCookieManager(auth.serverCfg).DeleteAllCookies(w)
			}
			log.Debug().Err(err).Str("path", r.URL.Path).Msg("Auth error - returning 401")
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		if user == nil {
			user = &types.User{}
		}

		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		r = r.WithContext(setRequestUser(r.Context(), *user))

		f(w, r)
	}
}

// csrfExemptPaths are paths that don't require CSRF protection
// These are typically auth endpoints or APIs used before/during session creation
var csrfExemptPaths = map[string]bool{
	"/api/v1/auth/login":         true, // Login doesn't have CSRF cookie yet
	"/api/v1/auth/logout":        true, // Logout clears the session
	"/api/v1/auth/oidc":          true, // OIDC redirect
	"/api/v1/auth/oidc/callback": true, // OIDC callback
	"/api/v1/auth/authenticated": true, // Read-only check
	"/api/v1/auth/session":       true, // Session info (GET-like semantics)
	"/api/v1/auth/user":          true, // Get user info
}

// csrfMiddleware validates CSRF tokens for state-changing requests
// when the request uses cookie-based session authentication.
// API key and runner token authenticated requests skip CSRF validation.
func (auth *authMiddleware) csrfMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check CSRF for state-changing methods
		if r.Method != "POST" && r.Method != "PUT" && r.Method != "DELETE" && r.Method != "PATCH" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if path is exempt from CSRF
		if csrfExemptPaths[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		// Check if this request was authenticated via session cookie
		// If using API key or runner token, skip CSRF validation
		_, err := r.Cookie(authpkg.SessionCookieName)
		if err != nil {
			// No session cookie - this is API key or runner token auth, skip CSRF
			next.ServeHTTP(w, r)
			return
		}

		// Session cookie exists - validate CSRF token
		if !authpkg.ValidateCSRF(r) {
			log.Warn().
				Str("path", r.URL.Path).
				Str("method", r.Method).
				Msg("CSRF validation failed")
			http.Error(w, "CSRF validation failed", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
