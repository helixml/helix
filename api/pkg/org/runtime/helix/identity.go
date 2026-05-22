package helix

import "context"

// HelixIdentity is the per-request caller identity in the helix
// runtime. Bundles the three things every downstream call needs
// to know about the caller:
//
//   - UserID: the helix.User the request is acting on behalf of.
//     Stable across requests; threaded by middleware from the
//     authenticated session.
//   - OrganizationID: the helix.Organization the user is acting
//     within. The same OrgID that gets stamped onto Workers
//     hired in this request (H5.2 / H5.3) and onto every Activation
//     row scoped to this tenant.
//   - BearerToken: the API key downstream HTTP calls authenticate
//     with. Empty when the runtime should fall back to the static
//     service key.
//
// Replaces the three-stash bearer-then-resolve dance (BearerFromContext
// + UserIDFromContext + UserFromContext) with a single typed value
// that flows through the request context. The legacy helpers stay
// for the migration window; new code reads/writes HelixIdentity.
//
// The zero value represents "no identity" — service-account / system
// paths where the caller hasn't been identified. IsZero reports it;
// WithHelixIdentity treats it as a no-op so callers don't have to
// gate.
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
