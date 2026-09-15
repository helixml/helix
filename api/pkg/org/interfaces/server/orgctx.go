package server

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

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

// orgAuthorizationKey carries the organization membership resolved by the
// authenticated outer API middleware. The standalone org server deliberately
// has no implicit privileged identity: callers that do not pass through that
// middleware cannot grant organization-management tools over REST.
type orgAuthorizationKey struct{}

type OrgAuthorization struct {
	MembershipRole types.OrganizationRole
	PlatformAdmin  bool
}

func WithOrgAuthorization(ctx context.Context, role types.OrganizationRole, platformAdmin bool) context.Context {
	return context.WithValue(ctx, orgAuthorizationKey{}, OrgAuthorization{
		MembershipRole: role,
		PlatformAdmin:  platformAdmin,
	})
}

func OrgAuthorizationFromContext(ctx context.Context) (OrgAuthorization, bool) {
	auth, ok := ctx.Value(orgAuthorizationKey{}).(OrgAuthorization)
	return auth, ok
}

func CanManageOrganization(ctx context.Context) bool {
	auth, ok := OrgAuthorizationFromContext(ctx)
	return ok && (auth.PlatformAdmin || auth.MembershipRole == types.OrganizationRoleOwner)
}
