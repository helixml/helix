package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const (
	// oidcRefreshRetryInterval is how long a session waits after a failed
	// refresh before it asks the identity provider again.
	oidcRefreshRetryInterval = 30 * time.Second

	// oidcRefreshTimeout bounds one refresh call. The refresh is shared by
	// every request waiting on it, so it does not inherit any one request's
	// cancellation.
	oidcRefreshTimeout = 15 * time.Second
)

// ErrSessionRevoked is returned for a session whose identity-provider
// session has ended: its refresh token was rejected, so the session can
// never get a valid access token again. The session has been deleted.
var ErrSessionRevoked = errors.New("identity provider session has ended")

// IsSessionDiscarded reports whether err means the request's session cookie
// names a session that no longer exists, so the cookie should be cleared.
func IsSessionDiscarded(err error) bool {
	return errors.Is(err, ErrSessionExpired) ||
		errors.Is(err, ErrSessionProviderMismatch) ||
		errors.Is(err, ErrSessionRevoked)
}

// isTerminalRefreshError reports whether the token endpoint rejected the
// refresh in a way no retry can fix (RFC 6749 §5.2).
func isTerminalRefreshError(err error) bool {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return false
	}
	switch retrieveErr.ErrorCode {
	case "invalid_grant", "invalid_client", "unauthorized_client":
		return true
	}
	return false
}

// refreshRetryGate remembers sessions whose last refresh failed, so they are
// not refreshed again until the retry interval has passed.
type refreshRetryGate struct {
	mu       sync.Mutex
	interval time.Duration
	until    map[string]time.Time
}

func (g *refreshRetryGate) blocked(sessionID string, now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	until, ok := g.until[sessionID]
	if !ok {
		return false
	}
	if now.Before(until) {
		return true
	}
	delete(g.until, sessionID)
	return false
}

func (g *refreshRetryGate) failed(sessionID string, now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.until == nil {
		g.until = make(map[string]time.Time)
	}
	for id, until := range g.until {
		if !now.Before(until) {
			delete(g.until, id)
		}
	}
	g.until[sessionID] = now.Add(g.interval)
}

func (g *refreshRetryGate) succeeded(sessionID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.until, sessionID)
}

// refreshSessionOIDC gets the session a fresh OIDC access token. Concurrent
// callers for one session share a single token-endpoint call, and a session
// whose refresh recently failed is returned unchanged until the retry
// interval passes. When the provider rejects the refresh token the session
// is deleted and ErrSessionRevoked is returned. Any other error leaves the
// session in place; the caller may carry on with its current token.
func (sm *SessionManager) refreshSessionOIDC(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
	if sm.refreshRetry.blocked(session.ID, time.Now()) {
		return session, nil
	}
	result, err, _ := sm.refreshGroup.Do(session.ID, func() (any, error) {
		refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), oidcRefreshTimeout)
		defer cancel()
		return sm.refreshStoredSession(refreshCtx, session.ID)
	})
	if err != nil {
		return nil, err
	}
	refreshed := *result.(*types.UserSession)
	return &refreshed, nil
}

// refreshStoredSession refreshes the session as currently stored, which may
// already carry tokens from a refresh that finished after the caller read it.
func (sm *SessionManager) refreshStoredSession(ctx context.Context, sessionID string) (*types.UserSession, error) {
	session, err := sm.store.GetUserSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSessionNotFound
		}
		return nil, err
	}
	if !session.NeedsOIDCRefresh() {
		return session, nil
	}

	token, err := sm.oidcClient.RefreshAccessToken(ctx, session.OIDCRefreshToken)
	if err != nil {
		if isTerminalRefreshError(err) {
			return sm.discardRevokedSession(ctx, session, err)
		}
		sm.refreshRetry.failed(sessionID, time.Now())
		return nil, err
	}

	session.UpdateOIDCTokens(token.AccessToken, token.RefreshToken, token.Expiry)
	if _, err := sm.store.UpdateUserSession(ctx, session); err != nil {
		sm.refreshRetry.failed(sessionID, time.Now())
		return nil, err
	}
	sm.refreshRetry.succeeded(sessionID)

	log.Debug().
		Str("session_id", session.ID).
		Time("new_expiry", token.Expiry).
		Msg("Refreshed OIDC token for session")
	return session, nil
}

// discardRevokedSession deletes a session whose refresh token the provider
// rejected, unless another API replica has meanwhile stored a newer one.
func (sm *SessionManager) discardRevokedSession(ctx context.Context, session *types.UserSession, refreshErr error) (*types.UserSession, error) {
	if latest, err := sm.store.GetUserSession(ctx, session.ID); err == nil &&
		latest.OIDCRefreshToken != session.OIDCRefreshToken {
		return latest, nil
	}

	if err := sm.store.DeleteUserSession(ctx, session.ID); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to delete session after its refresh token was rejected")
		sm.refreshRetry.failed(session.ID, time.Now())
	} else {
		sm.refreshRetry.succeeded(session.ID)
	}

	log.Info().
		Err(refreshErr).
		Str("session_id", session.ID).
		Str("user_id", session.UserID).
		Msg("Identity provider rejected the session's refresh token; session deleted")
	return nil, fmt.Errorf("%w: %v", ErrSessionRevoked, refreshErr)
}
