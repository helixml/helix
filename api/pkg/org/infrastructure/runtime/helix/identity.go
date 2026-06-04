package helix

import "context"

// HelixIdentity is the per-request caller identity in the helix runtime:
// UserID, OrganizationID, and BearerToken (empty → static service key).
// Zero value means "no identity" (service-account paths); WithHelixIdentity
// treats it as a no-op.
type HelixIdentity struct {
	UserID         string
	OrganizationID string
	BearerToken    string
}

// IsZero reports the zero-identity "no caller" state.
func (i HelixIdentity) IsZero() bool {
	return i == HelixIdentity{}
}

type helixIdentityKey struct{}

// WithHelixIdentity stashes id on ctx. The zero identity is a no-op
// (returns ctx unchanged) so callers can pass the result of a lookup
// without gating on IsZero.
func WithHelixIdentity(ctx context.Context, id HelixIdentity) context.Context {
	if id.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, helixIdentityKey{}, id)
}

// HelixIdentityFromContext returns the stashed identity and a flag
// indicating whether one was present. The zero identity is never
// returned with ok=true (WithHelixIdentity refuses to stash it).
func HelixIdentityFromContext(ctx context.Context) (HelixIdentity, bool) {
	v, ok := ctx.Value(helixIdentityKey{}).(HelixIdentity)
	if !ok {
		return HelixIdentity{}, false
	}
	return v, true
}
