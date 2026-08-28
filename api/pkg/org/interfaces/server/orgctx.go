package server

import "context"

// orgIDKey is the unexported context key for the resolved orgID.
type orgIDKey struct{}

// WithOrgID stores the orgID on ctx so downstream handlers and the
// store layer can scope all reads / writes to a single helix tenant.
// The middleware in api/pkg/server resolves the URL `{org}` segment
// to a canonical organisation ID via lookupOrg and stores it here.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, orgIDKey{}, orgID)
}

// OrgIDFromContext returns the orgID stored by WithOrgID, or empty
// when no middleware has set it. Empty orgID means "no helix-org
// scope" — handlers must error out (multi-tenant requires explicit
// scope; the old single-tenant fallback is gone).
func OrgIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(orgIDKey{}).(string)
	return v
}

// orgHandleKey is the unexported context key for the org handle exactly
// as it appeared in the request URL.
type orgHandleKey struct{}

// WithOrgHandle stores the raw `{org}` URL segment — a slug or an id,
// whichever the caller used. Handlers that build a URL for a human to
// copy render this rather than the canonical id, so the address they
// paste matches the one they navigated to. Both forms resolve.
func WithOrgHandle(ctx context.Context, handle string) context.Context {
	return context.WithValue(ctx, orgHandleKey{}, handle)
}

// OrgHandleFromContext returns the handle stored by WithOrgHandle, or
// empty when no middleware has set it. Callers fall back to the org id.
func OrgHandleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(orgHandleKey{}).(string)
	return v
}
