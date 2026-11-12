package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// lastSeenThrottle limits how often we write users.last_seen_at.
// Hot-path auth runs on every request; one write per user per minute keeps
// the rate trivially low while giving org presence (types.PresenceOnlineWindow)
// minute-level resolution. Keep it below PresenceOnlineWindow.
const lastSeenThrottle = time.Minute

var (
	// Allowed paths for app API keys. Currently we support
	// only OpenAI compatible chat completions API
	AppAPIKeyPaths = map[string]bool{
		"/v1/chat/completions":  true,
		"/api/v1/sessions/chat": true,
	}
)

var (
	ErrNoAPIKeyFound            = errors.New("no API key found")
	ErrNoUserIDFound            = errors.New("no user ID found")
	ErrAPIKeyOwnerNotFound      = errors.New("the owner of this API key no longer exists")
	ErrAppAPIKeyPathNotAllowed  = errors.New("path not allowed for app API keys, use your personal account key from your /account page instead")
	ErrEmbedKeyNotAllowed       = errors.New("this embed key is scoped to a single task and may not access this resource")
	ErrBotInstanceKeyNotAllowed = errors.New("this bot instance key is scoped to its own sandbox and may not access this resource")
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
	serverCfg      *config.ServerConfig    // Server config for cookie management
	sessionManager *authpkg.SessionManager // BFF session manager (nil if not using sessions)

	// lastSeenCache tracks the last time we wrote last_seen_at for each user ID
	// so we can skip the DB write on most requests. Values are time.Time.
	lastSeenCache sync.Map
}

// rejectInactiveAccount answers 403 when the authenticated account may not
// use the API (waitlisted, deactivated or deleted; see
// types.User.AccountAccessError) and reports whether it did. Every entry point
// that authenticates a user calls it before serving anything. The only routes
// such an account still reaches are the insecure /api/v1/auth/* and /config
// routes, which resolve the caller themselves: that is all the waitlist page
// needs to show the account's status and log out.
func rejectInactiveAccount(w http.ResponseWriter, user *types.User) bool {
	err := user.AccountAccessError()
	if err == nil {
		return false
	}
	http.Error(w, err.Error(), http.StatusForbidden)
	return true
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

// touchUserLastSeen records that the user just authenticated. The write is
// throttled per-user by lastSeenThrottle to keep the cost negligible even on
// hot paths. Runner / empty users are skipped, as are session-scoped API keys:
// those are minted for sandboxes and org agents acting on a human's behalf,
// and an agent polling the API all night must not make its owner look online.
// The DB write runs in a detached goroutine so the request context
// cancellation does not abort it.
func (auth *authMiddleware) touchUserLastSeen(user *types.User) {
	if user == nil || user.ID == "" {
		return
	}
	if user.TokenType == types.TokenTypeRunner {
		return
	}
	if user.TokenType == types.TokenTypeAPIKey && user.SessionID != "" {
		return
	}

	now := time.Now()
	if prev, ok := auth.lastSeenCache.Load(user.ID); ok {
		if prevTime, ok := prev.(time.Time); ok && now.Sub(prevTime) < lastSeenThrottle {
			return
		}
	}
	auth.lastSeenCache.Store(user.ID, now)

	// Detach from the request context so the write is not cancelled when
	// the response finishes. The write is best-effort — log and move on.
	go func(userID string, at time.Time) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := auth.store.TouchUserLastSeen(bgCtx, userID, at); err != nil {
			log.Debug().Err(err).Str("user_id", userID).Msg("failed to update last_seen_at")
		}
	}(user.ID, now)
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

	// The shared runner token or a per-runner token: runner mode.
	if runner, ok, err := runnerUserFromToken(ctx, auth.store, auth.cfg.runnerToken, token); ok {
		return runner, err
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

		// The key acts as its owner, so it carries the owner's account state
		// (waitlisted, deactivated) into rejectInactiveAccount. Fail closed:
		// a key whose owner cannot be loaded authenticates nobody.
		user, err := auth.store.GetUser(ctx, &store.GetUserQuery{ID: apiKey.Owner})
		if errors.Is(err, store.ErrNotFound) || (err == nil && user == nil) {
			// Users are soft-deleted, so a deleted owner's row is invisible
			// here. A key must never outlive its owner.
			return nil, ErrAPIKeyOwnerNotFound
		}
		if err != nil {
			return nil, fmt.Errorf("error loading API key owner: %w", err)
		}

		user.Token = token
		user.TokenType = types.TokenTypeAPIKey
		user.APIKeyType = apiKey.Type
		user.ID = apiKey.Owner
		user.Type = apiKey.OwnerType
		// An org-labelled key is scoped to its organization: the bearer acts as
		// the key owner within that org (membership rules apply) and must not
		// inherit the owner's global admin — otherwise a global admin's org key
		// would reach every organization and admin endpoint. dbUser carries the
		// owner's Admin flag, so it must be cleared here, not just not set.
		// Keys minted for a project or a session (sandbox agents) are
		// confined the same way: admin would bypass that confinement.
		user.Admin = apiKey.OrganizationID == "" && apiKey.ProjectID == "" && apiKey.SessionID == "" &&
			auth.isAdminWithContext(ctx, user.ID)
		if apiKey.AppID != nil && apiKey.AppID.Valid {
			user.AppID = apiKey.AppID.String
		}
		user.ProjectID = apiKey.ProjectID
		user.SpecTaskID = apiKey.SpecTaskID
		user.SessionID = apiKey.SessionID
		user.EmbedInteractive = apiKey.Type == types.APIkeytypeEmbed && apiKey.EmbedInteractive
		user.OrganizationID = apiKey.OrganizationID

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
		log.Error().Err(err).Msg("error validating user token")
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
		// This is the primary auth method for browser clients — unless the
		// request names its own API key (see namesOwnAPIKey): an embed key in
		// an iframe on another product's page, sent by a browser that also
		// happens to hold the viewer's own Helix session. Letting the cookie win
		// there authorises the request as the viewer instead (a waitlisted
		// viewer gets "waiting for approval", anyone else "not found"), and the
		// embed spins forever, or its desktop stream never connects.
		if auth.sessionManager != nil && !namesOwnAPIKey(r) {
			user, err = auth.getUserFromSession(r.Context(), r)
			if err == nil && user != nil {
				if rejectInactiveAccount(w, user) {
					return
				}
				// Successfully authenticated via session
				auth.touchUserLastSeen(user)
				r = r.WithContext(setRequestUser(r.Context(), *user))
				next.ServeHTTP(w, r)
				return
			}

			// The session is gone (expired, from another auth provider, or its
			// identity-provider session ended) - clear the cookie
			if authpkg.IsSessionDiscarded(err) {
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
		if rejectInactiveAccount(w, user) {
			return
		}

		// If app API key, check if the path is in the allowed list
		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		// Embed keys live in an untrusted browser, so they are restricted to one
		// spec task's read surface. Fail closed.
		if user.APIKeyType == types.APIkeytypeEmbed && !embedKeyAllows(user, r) {
			// Some inventory endpoints are required by the embed page but must not
			// show a visitor real data. Serve an empty success rather than a 403,
			// which breaks the page without making anything safer.
			if body, ok := embedNeuteredResponse(r); ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
			http.Error(w, ErrEmbedKeyNotAllowed.Error(), http.StatusForbidden)
			return
		}

		// Bot instance keys live in a sandbox that reads untrusted input, so
		// they reach only their own session's needs. Fail closed.
		if user.APIKeyType == types.APIkeytypeBotInstance && !auth.botInstanceKeyAllows(r.Context(), user, r) {
			http.Error(w, ErrBotInstanceKeyNotAllowed.Error(), http.StatusForbidden)
			return
		}

		auth.touchUserLastSeen(user)
		r = r.WithContext(setRequestUser(r.Context(), *user))
		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}

func (auth *authMiddleware) auth(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// A browser signed in to Helix holds only the BFF session cookie (the
		// model picker reads /v1/models that way). These routes sit outside
		// the /api/v1 CSRF middleware, so the cookie is honoured for safe
		// methods only; a request naming its own API key uses that key.
		var user *types.User
		var err error
		if auth.sessionManager != nil && isSafeMethod(r.Method) && !namesOwnAPIKey(r) {
			if sessionUser, sessionErr := auth.getUserFromSession(r.Context(), r); sessionErr == nil && sessionUser != nil {
				user = sessionUser
			}
		}
		if user == nil {
			user, err = auth.getUserFromToken(r.Context(), getRequestToken(r))
		}
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
		if rejectInactiveAccount(w, user) {
			return
		}

		if user.AppID != "" {
			if _, ok := AppAPIKeyPaths[r.URL.Path]; !ok {
				http.Error(w, ErrAppAPIKeyPathNotAllowed.Error(), http.StatusForbidden)
				return
			}
		}

		// Embed keys live in an untrusted browser, so they are restricted to one
		// spec task's read surface. Fail closed.
		if user.APIKeyType == types.APIkeytypeEmbed && !embedKeyAllows(user, r) {
			// Some inventory endpoints are required by the embed page but must not
			// show a visitor real data. Serve an empty success rather than a 403,
			// which breaks the page without making anything safer.
			if body, ok := embedNeuteredResponse(r); ok {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
			http.Error(w, ErrEmbedKeyNotAllowed.Error(), http.StatusForbidden)
			return
		}

		// Bot instance keys live in a sandbox that reads untrusted input, so
		// they reach only their own session's needs. Fail closed.
		if user.APIKeyType == types.APIkeytypeBotInstance && !auth.botInstanceKeyAllows(r.Context(), user, r) {
			http.Error(w, ErrBotInstanceKeyNotAllowed.Error(), http.StatusForbidden)
			return
		}

		auth.touchUserLastSeen(user)
		r = r.WithContext(setRequestUser(r.Context(), *user))

		f(w, r)
	}
}

// optionalUser puts the caller on the request context when the request carries
// a valid session or token, and otherwise serves it anonymously. It never
// refuses a request, so it suits public routes whose response only narrows
// for anonymous callers; anything that must be authenticated belongs on
// authRouter instead.
func (auth *authMiddleware) optionalUser(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.getUserFromSession(r.Context(), r)
		if err != nil || user == nil {
			user, err = auth.getUserFromToken(r.Context(), getRequestToken(r))
		}
		if err == nil && hasUser(user) && user.AccountAccessError() == nil {
			r = r.WithContext(setRequestUser(r.Context(), *user))
		}
		f(w, r)
	}
}

// carriesCookieCredential reports whether the browser could authenticate this
// request by itself: it holds a session cookie or a legacy access_token cookie.
func carriesCookieCredential(r *http.Request) bool {
	for _, name := range []string{authpkg.SessionCookieName, accessTokenCookie.Name} {
		if c, err := r.Cookie(name); err == nil && c.Value != "" {
			return true
		}
	}
	return false
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

		// Only cookie-authenticated requests need a CSRF token: a browser
		// attaches cookies on its own, while an API key or runner token in a
		// header was put there by the caller. The legacy access_token cookie
		// (a Helix JWT, still accepted by getRequestToken) counts as much as
		// the session cookie: either one signs the request in by itself.
		if !carriesCookieCredential(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Cookie credential present - validate CSRF token
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

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}
