package helix

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// Auth-context helpers used by the in-proc Helix adapter and the
// spawner.
//
// Two independent stashes on the request context:
//
//   - bearerToken: the per-request API key. Used by the chat-bridge
//     path (middleware mints a bearer for the logged-in user) and the
//     spawner path (BearerForUser callback mints one per activation).
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
// WithBearerToken, falling back to the HelixIdentity stash (H6) if
// the legacy bearer key isn't set. Empty when neither is present —
// the runtime then uses its static service key.
func BearerFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(bearerTokenKey{}).(string); ok && v != "" {
		return v
	}
	if id, ok := HelixIdentityFromContext(ctx); ok && id.BearerToken != "" {
		return id.BearerToken
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
// WithUserID, falling back to the HelixIdentity stash (H6) if the
// legacy userID key isn't set.
func UserIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userIDKey{}).(string); ok && v != "" {
		return v
	}
	if id, ok := HelixIdentityFromContext(ctx); ok && id.UserID != "" {
		return id.UserID
	}
	return ""
}

// OrganizationIDFromContext returns the OrganizationID stashed via
// WithHelixIdentity (H6) — the helix.Organization the request acts
// within. Empty when the request hasn't been identified to an org
// (service paths / legacy single-tenant alpha). H5.x call sites
// use this to scope hires and lookups.
func OrganizationIDFromContext(ctx context.Context) string {
	if id, ok := HelixIdentityFromContext(ctx); ok {
		return id.OrganizationID
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
