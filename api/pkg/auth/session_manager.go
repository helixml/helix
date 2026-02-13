package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

const (
	// SessionCookieName is the name of the HttpOnly session cookie
	SessionCookieName = "helix_session"

	// CSRFCookieName is the name of the CSRF token cookie (readable by JS)
	CSRFCookieName = "helix_csrf"

	// CSRFHeaderName is the name of the header that must contain the CSRF token
	CSRFHeaderName = "X-CSRF-Token"

	// DefaultSessionDuration is the default session lifetime (30 days)
	DefaultSessionDuration = 30 * 24 * time.Hour

	// DesktopSessionDuration is the session lifetime for Helix Desktop auto-login.
	// Effectively permanent — desktop users should never be prompted to re-authenticate.
	// Go's time.Duration is int64 nanoseconds so max is ~292 years; we use 200 years.
	DesktopSessionDuration = 200 * 365 * 24 * time.Hour
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

// SessionManager handles user session lifecycle in the BFF pattern
// It stores session IDs in HttpOnly cookies and manages OIDC token refresh transparently
type SessionManager struct {
	store      store.Store
	oidcClient OIDC
	cfg        *config.ServerConfig
}

// NewSessionManager creates a new session manager
func NewSessionManager(store store.Store, oidcClient OIDC, cfg *config.ServerConfig) *SessionManager {
	return &SessionManager{
		store:      store,
		oidcClient: oidcClient,
		cfg:        cfg,
	}
}

// CreateSession creates a new user session and sets the session cookie.
// Use CreateDesktopSession for Helix Desktop auto-login with a longer expiry.
func (sm *SessionManager) CreateSession(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	authProvider types.AuthProvider,
	oidcAccessToken, oidcRefreshToken string,
	oidcTokenExpiry time.Time,
) (*types.UserSession, error) {
	return sm.createSessionWithDuration(ctx, w, r, userID, authProvider, oidcAccessToken, oidcRefreshToken, oidcTokenExpiry, DefaultSessionDuration)
}

// CreateDesktopSession creates a user session with a very long expiry
// for the Helix Desktop app, so the user is never prompted to re-authenticate.
// It uses SameSite=None so the cookie works inside cross-origin iframes
// (the Wails WKWebView parent is wails:// while the iframe is http://localhost).
func (sm *SessionManager) CreateDesktopSession(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	userID string,
) (*types.UserSession, error) {
	session := &types.UserSession{
		ID:           system.GenerateUserSessionID(),
		UserID:       userID,
		AuthProvider: types.AuthProviderRegular,
		ExpiresAt:    time.Now().Add(DesktopSessionDuration),
		UserAgent:    r.UserAgent(),
		IPAddress:    getClientIP(r),
	}

	createdSession, err := sm.store.CreateUserSession(ctx, session)
	if err != nil {
		return nil, err
	}

	// Set cookies with SameSite=None for cross-origin iframe compatibility.
	// localhost is exempt from the Secure requirement in WebKit.
	sm.setDesktopSessionCookie(w, createdSession.ID, createdSession.ExpiresAt)

	log.Info().
		Str("session_id", createdSession.ID).
		Str("user_id", userID).
		Str("auth_provider", string(types.AuthProviderRegular)).
		Msg("Created new desktop session")

	return createdSession, nil
}

func (sm *SessionManager) createSessionWithDuration(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	authProvider types.AuthProvider,
	oidcAccessToken, oidcRefreshToken string,
	oidcTokenExpiry time.Time,
	duration time.Duration,
) (*types.UserSession, error) {
	session := &types.UserSession{
		ID:               system.GenerateUserSessionID(),
		UserID:           userID,
		AuthProvider:     authProvider,
		ExpiresAt:        time.Now().Add(duration),
		OIDCAccessToken:  oidcAccessToken,
		OIDCRefreshToken: oidcRefreshToken,
		OIDCTokenExpiry:  oidcTokenExpiry,
		UserAgent:        r.UserAgent(),
		IPAddress:        getClientIP(r),
	}

	createdSession, err := sm.store.CreateUserSession(ctx, session)
	if err != nil {
		return nil, err
	}

	// Set the session cookie
	sm.setSessionCookie(w, createdSession.ID, createdSession.ExpiresAt)

	log.Info().
		Str("session_id", createdSession.ID).
		Str("user_id", userID).
		Str("auth_provider", string(authProvider)).
		Msg("Created new user session")

	return createdSession, nil
}

// GetSessionFromRequest extracts and validates the session from the request cookie
// If the session has OIDC tokens that need refresh, it refreshes them transparently
func (sm *SessionManager) GetSessionFromRequest(ctx context.Context, r *http.Request) (*types.UserSession, error) {
	sessionCookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		log.Debug().Err(err).Str("path", r.URL.Path).Msg("No session cookie found")
		return nil, ErrSessionNotFound
	}

	sessionID := sessionCookie.Value
	if sessionID == "" {
		log.Debug().Str("path", r.URL.Path).Msg("Session cookie is empty")
		return nil, ErrSessionNotFound
	}

	session, err := sm.store.GetUserSession(ctx, sessionID)
	if err != nil {
		log.Debug().Err(err).Str("session_id", sessionID).Str("path", r.URL.Path).Msg("Session lookup failed in store")
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}

	// Check if session is expired
	if session.IsExpired() {
		// Clean up expired session
		_ = sm.store.DeleteUserSession(ctx, session.ID)
		return nil, ErrSessionExpired
	}

	// For OIDC sessions, check if access token needs refresh
	if session.NeedsOIDCRefresh() && sm.oidcClient != nil {
		if err := sm.refreshOIDCToken(ctx, session); err != nil {
			log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to refresh OIDC token")
			// Continue with potentially expired token - the API call might still work
			// or will fail and force re-login
		}
	}

	// Touch the session periodically (not every request to avoid DB load)
	// Update if last used more than 1 hour ago
	if time.Since(session.LastUsedAt) > time.Hour {
		session.Touch()
		_, _ = sm.store.UpdateUserSession(ctx, session)
	}

	return session, nil
}

// refreshOIDCToken refreshes the OIDC access token using the refresh token
func (sm *SessionManager) refreshOIDCToken(ctx context.Context, session *types.UserSession) error {
	if session.OIDCRefreshToken == "" {
		return errors.New("no refresh token available")
	}

	newToken, err := sm.oidcClient.RefreshAccessToken(ctx, session.OIDCRefreshToken)
	if err != nil {
		return err
	}

	// Update session with new tokens
	session.UpdateOIDCTokens(newToken.AccessToken, newToken.RefreshToken, newToken.Expiry)

	// Persist the updated session
	_, err = sm.store.UpdateUserSession(ctx, session)
	if err != nil {
		return err
	}

	log.Debug().
		Str("session_id", session.ID).
		Time("new_expiry", newToken.Expiry).
		Msg("Refreshed OIDC token for session")

	return nil
}

// DeleteSession removes a session and clears the session cookie
func (sm *SessionManager) DeleteSession(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	err := sm.store.DeleteUserSession(ctx, sessionID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// Clear the session cookie
	sm.ClearSessionCookie(w)

	log.Info().Str("session_id", sessionID).Msg("Deleted user session")
	return nil
}

// DeleteAllUserSessions removes all sessions for a user (logout from all devices)
func (sm *SessionManager) DeleteAllUserSessions(ctx context.Context, w http.ResponseWriter, userID string) error {
	err := sm.store.DeleteUserSessionsByUser(ctx, userID)
	if err != nil {
		return err
	}

	// Clear the session cookie
	sm.ClearSessionCookie(w)

	log.Info().Str("user_id", userID).Msg("Deleted all user sessions")
	return nil
}

// setSessionCookie sets the session cookie with proper security settings
// It also sets a companion CSRF token cookie (readable by JS)
func (sm *SessionManager) setSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	secure := sm.isSecureCookies()

	// Set the HttpOnly session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	// Set the CSRF token cookie (readable by JS, used for X-CSRF-Token header)
	csrfToken := generateCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: false, // JS needs to read this
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// setDesktopSessionCookie sets cookies with SameSite=None for cross-origin
// iframe contexts (Helix Desktop WKWebView). This is necessary because
// SameSite=Lax cookies are not sent in cross-origin iframe navigations.
func (sm *SessionManager) setDesktopSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: true,
		Secure:   false, // localhost doesn't use HTTPS
		SameSite: http.SameSiteNoneMode,
	})

	csrfToken := generateCSRFToken()
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		HttpOnly: false,
		Secure:   false,
		SameSite: http.SameSiteNoneMode,
	})
}

// ClearSessionCookie clears the session and CSRF cookies
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// generateCSRFToken generates a cryptographically secure random CSRF token
func generateCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to ULID if crypto/rand fails (shouldn't happen)
		return system.GenerateID()
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// ValidateCSRF checks that the X-CSRF-Token header matches the helix_csrf cookie
// Returns true if valid, false otherwise
func ValidateCSRF(r *http.Request) bool {
	csrfCookie, err := r.Cookie(CSRFCookieName)
	if err != nil || csrfCookie.Value == "" {
		return false
	}

	csrfHeader := r.Header.Get(CSRFHeaderName)
	if csrfHeader == "" {
		return false
	}

	return csrfCookie.Value == csrfHeader
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (for proxied requests)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the chain
		if idx := len(xff); idx > 0 {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// StartBackgroundRefresh starts a background goroutine that refreshes OIDC tokens
// before they expire, similar to OAuth manager's RefreshExpiredTokens
func (sm *SessionManager) StartBackgroundRefresh(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				sm.refreshExpiredSessions(ctx)
			}
		}
	}()
}

// refreshExpiredSessions refreshes OIDC tokens for sessions approaching expiry
func (sm *SessionManager) refreshExpiredSessions(ctx context.Context) {
	// Get sessions with tokens expiring in the next 5 minutes
	sessions, err := sm.store.GetUserSessionsNearOIDCExpiry(ctx, time.Now().Add(5*time.Minute))
	if err != nil {
		log.Error().Err(err).Msg("Failed to get sessions near OIDC expiry")
		return
	}

	if len(sessions) == 0 {
		return
	}

	log.Debug().Int("count", len(sessions)).Msg("Refreshing OIDC tokens for sessions")

	for _, session := range sessions {
		if err := sm.refreshOIDCToken(ctx, session); err != nil {
			log.Warn().
				Err(err).
				Str("session_id", session.ID).
				Str("user_id", session.UserID).
				Msg("Failed to refresh OIDC token in background job")
		}
	}
}

// CleanupExpiredSessions removes expired sessions from the database
// This should be called periodically (e.g., daily)
func (sm *SessionManager) CleanupExpiredSessions(ctx context.Context) error {
	return sm.store.DeleteExpiredUserSessions(ctx)
}

// isSecureCookies determines if cookies should have the Secure flag set.
// Returns true if OIDC_SECURE_COOKIES is explicitly set to true,
// or if SERVER_URL starts with https://
func (sm *SessionManager) isSecureCookies() bool {
	if sm.cfg == nil {
		return true // Safe default
	}
	// Explicit setting takes precedence
	if sm.cfg.Auth.OIDC.SecureCookies {
		return true
	}
	// Auto-detect from server URL
	return strings.HasPrefix(sm.cfg.WebServer.URL, "https://")
}
