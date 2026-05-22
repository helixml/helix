package helix

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// Auth-context helpers lifted from helix-org/helix/helixclient in H1.0
// — same shape as before, new home so the helixclient package can
// shrink toward deletion.
//
// Two independent stashes on the request context:
//
//   - bearerToken: the per-request API key. realClient (helixclient)
//     reads it via BearerFromContext to override its static apiKey.
//     Used by the chat-bridge path (middleware mints a bearer for the
//     logged-in user) and the spawner path (BearerForUser callback
//     mints one per activation).
//
//   - userID: the upstream caller's identifier (typically a Helix
//     user_id). Independent of the bearer because we may know "who"
//     without holding their api_key — e.g. when the spawner has the
//     hiring user's persisted userID but needs to ask the host to
//     mint a bearer on-demand.
//
// H1.x will progressively retire the bearer path as direct controller
// calls land; WithUser / UserFromContext (the *types.User-shaped
// stash) is the replacement that in-process call sites use to thread
// per-user identity end-to-end without going through the
// bearer-then-resolve dance.

type bearerTokenKey struct{}
type userIDKey struct{}
type userKey struct{}

// WithBearerToken returns a context carrying the given token as the
// bearer realClient should use on its next request. Empty token is a
// no-op.
func WithBearerToken(ctx context.Context, token string) context.Context {
	if token == "" {
		return ctx
	}
	return context.WithValue(ctx, bearerTokenKey{}, token)
}

// BearerFromContext returns the per-request bearer stashed by
// WithBearerToken, or "" if none.
func BearerFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(bearerTokenKey{}).(string); ok {
		return v
	}
	return ""
}

// WithUserID returns a context carrying userID. Empty userID is a
// no-op.
func WithUserID(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFromContext returns the user identifier stashed by
// WithUserID, or "" if none.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey{}).(string); ok {
		return v
	}
	return ""
}

// WithUser stashes the resolved *types.User on the context. After H1
// completes, this replaces the bearer-then-resolve dance for
// in-process call sites — direct controller calls take a *types.User
// and the rest of the runtime threads it via this helper. Nil user
// is a no-op.
func WithUser(ctx context.Context, u *types.User) context.Context {
	if u == nil {
		return ctx
	}
	return context.WithValue(ctx, userKey{}, u)
}

// UserFromContext returns the resolved *types.User stashed by
// WithUser, or nil if none.
func UserFromContext(ctx context.Context) *types.User {
	if v, ok := ctx.Value(userKey{}).(*types.User); ok {
		return v
	}
	return nil
}
