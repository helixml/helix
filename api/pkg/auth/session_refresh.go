package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

	// endedSessionLimit caps how many sessions may be retired (marked dead or
	// deleted) per endedSessionWindow. A realm-wide event such as a provider
	// restore or key rotation then cannot retire every session at once.
	endedSessionLimit  = 20
	endedSessionWindow = time.Minute
)

// ErrSessionRevoked is returned for a session whose identity-provider
// session has ended when OIDC_END_SESSION_WHEN_IDP_SESSION_ENDS is enabled.
// The session has been deleted.
var ErrSessionRevoked = errors.New("identity provider session has ended")

var oidcRefreshFailures = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "helix_oidc_session_refresh_failures_total",
	Help: "Failed OIDC token refreshes for browser sessions, by outcome.",
}, []string{"reason"})

const (
	refreshFailureTransient      = "transient"
	refreshFailureClientRejected = "client_rejected"
	refreshFailureSessionEnded   = "idp_session_ended"
	refreshFailureLimitReached   = "ended_session_limit"
)

// IsSessionDiscarded reports whether err means the request's session cookie
// names a session that no longer exists, so the cookie should be cleared.
func IsSessionDiscarded(err error) bool {
	return errors.Is(err, ErrSessionExpired) ||
		errors.Is(err, ErrSessionProviderMismatch) ||
		errors.Is(err, ErrSessionRevoked)
}

// refreshErrorReason classifies a token-endpoint failure (RFC 6749 §5.2).
// invalid_grant concerns this session's refresh token alone. invalid_client
// and unauthorized_client mean Helix's own client credentials are rejected:
// that affects every session and is fixed by configuration, so it is
// handled like any other transient failure.
func refreshErrorReason(err error) string {
	var retrieveErr *oauth2.RetrieveError
	if !errors.As(err, &retrieveErr) {
		return refreshFailureTransient
	}
	switch retrieveErr.ErrorCode {
	case "invalid_grant":
		return refreshFailureSessionEnded
	case "invalid_client", "unauthorized_client":
		return refreshFailureClientRejected
	}
	return refreshFailureTransient
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

// rateLimiter counts events in a sliding window.
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	events []time.Time
}

func (l *rateLimiter) full(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-l.window)
	kept := l.events[:0]
	for _, t := range l.events {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.events = kept
	return len(l.events) >= l.limit
}

func (l *rateLimiter) record(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, now)
}

// refreshSessionOIDC gets the session a fresh OIDC access token. Concurrent
// callers for one session share a single token-endpoint call, and a session
// whose refresh recently failed is returned unchanged until the retry
// interval passes. When the provider reports that the user's session has
// ended, the session is either marked so it is never refreshed again
// (default) or deleted, returning ErrSessionRevoked. Any other error leaves
// the session in place.
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
	session, err := sm.loadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if !session.NeedsOIDCRefresh() {
		return session, nil
	}

	token, err := sm.oidcClient.RefreshAccessToken(ctx, session.OIDCRefreshToken)
	if err != nil {
		switch reason := refreshErrorReason(err); reason {
		case refreshFailureSessionEnded:
			return sm.retireEndedSession(ctx, session, err)
		case refreshFailureClientRejected:
			oidcRefreshFailures.WithLabelValues(reason).Inc()
			log.Error().Err(err).Str("session_id", sessionID).
				Msg("Identity provider rejected Helix's OIDC client credentials while refreshing a session; check OIDC_CLIENT_ID / OIDC_CLIENT_SECRET")
		default:
			oidcRefreshFailures.WithLabelValues(reason).Inc()
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

func (sm *SessionManager) loadSession(ctx context.Context, sessionID string) (*types.UserSession, error) {
	session, err := sm.store.GetUserSession(ctx, sessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	return session, err
}

func (sm *SessionManager) endSessionWhenIdPSessionEnds() bool {
	return sm.cfg != nil && sm.cfg.Auth.OIDC.EndSessionWhenIdPSessionEnds
}

// retireEndedSession handles a refresh token the provider rejected because
// the user's session there has ended. Both the mark and the delete apply
// only while the session still holds that refresh token, so a session that
// another API replica refreshed in the meantime is left alone.
func (sm *SessionManager) retireEndedSession(ctx context.Context, session *types.UserSession, refreshErr error) (*types.UserSession, error) {
	now := time.Now()
	if sm.endedSessions.full(now) {
		oidcRefreshFailures.WithLabelValues(refreshFailureLimitReached).Inc()
		log.Error().Err(refreshErr).Str("session_id", session.ID).
			Int("limit", endedSessionLimit).Dur("window", endedSessionWindow).
			Msg("Too many sessions ended by the identity provider; leaving this one in place and retrying later")
		sm.refreshRetry.failed(session.ID, now)
		return nil, refreshErr
	}

	var (
		applied bool
		err     error
	)
	if sm.endSessionWhenIdPSessionEnds() {
		applied, err = sm.store.DeleteUserSessionWithRefreshToken(ctx, session.ID, session.OIDCRefreshToken)
	} else {
		applied, err = sm.store.MarkUserSessionOIDCRefreshDead(ctx, session.ID, session.OIDCRefreshToken)
	}
	if err != nil {
		sm.refreshRetry.failed(session.ID, now)
		return nil, err
	}
	if !applied {
		// The stored refresh token changed (or the session went away) while
		// this refresh was in flight: use what is stored now.
		latest, err := sm.loadSession(ctx, session.ID)
		if err != nil && !errors.Is(err, ErrSessionNotFound) {
			sm.refreshRetry.failed(session.ID, now)
		}
		return latest, err
	}

	sm.endedSessions.record(now)
	sm.refreshRetry.succeeded(session.ID)
	oidcRefreshFailures.WithLabelValues(refreshFailureSessionEnded).Inc()

	if sm.endSessionWhenIdPSessionEnds() {
		log.Info().Err(refreshErr).Str("session_id", session.ID).Str("user_id", session.UserID).
			Msg("Identity provider session ended; Helix session deleted")
		return nil, fmt.Errorf("%w: %v", ErrSessionRevoked, refreshErr)
	}
	log.Info().Err(refreshErr).Str("session_id", session.ID).Str("user_id", session.UserID).
		Msg("Identity provider session ended; the Helix session will no longer refresh its OIDC token")
	session.OIDCRefreshDead = true
	return session, nil
}
