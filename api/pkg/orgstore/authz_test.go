package orgstore

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

type fakeQueryer struct {
	memberships map[string]*types.OrganizationMembership
}

func (f *fakeQueryer) GetOrganizationMembership(_ context.Context, q *GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	m, ok := f.memberships[q.OrganizationID+"/"+q.UserID]
	if !ok {
		return nil, context.DeadlineExceeded
	}
	return m, nil
}

func (f *fakeQueryer) ListTeams(_ context.Context, _ *ListTeamsQuery) ([]*types.Team, error) {
	return nil, nil
}

func (f *fakeQueryer) ListAccessGrants(_ context.Context, _ *ListAccessGrantsQuery) ([]*types.AccessGrant, error) {
	return nil, nil
}

func newFakeQueryer() *fakeQueryer {
	return &fakeQueryer{
		memberships: map[string]*types.OrganizationMembership{
			// org-2 is seeded with an owner role on purpose: without
			// enforceKeyOrgScope both cross-org assertions would succeed,
			// so the negative tests fail if the guard is removed.
			"org-1/user-1": {OrganizationID: "org-1", UserID: "user-1", Role: types.OrganizationRoleMember},
			"org-2/user-1": {OrganizationID: "org-2", UserID: "user-1", Role: types.OrganizationRoleOwner},
		},
	}
}

func TestAuthorizeOrgMember_OrgScopedUserDeniedOtherOrg(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	user := &types.User{ID: "user-1", OrganizationID: "org-1"}

	if _, err := a.AuthorizeOrgMember(context.Background(), user, "org-2"); err == nil {
		t.Fatal("org-scoped credential must not authorize another organization")
	}
	if _, err := a.AuthorizeOrgOwner(context.Background(), user, "org-2"); err == nil {
		t.Fatal("org-scoped credential must not authorize another organization as owner")
	}
}

func TestAuthorizeOrgMember_OrgScopedUserAllowedOwnOrg(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	user := &types.User{ID: "user-1", OrganizationID: "org-1"}

	m, err := a.AuthorizeOrgMember(context.Background(), user, "org-1")
	if err != nil {
		t.Fatalf("org-scoped credential should work in its own org: %v", err)
	}
	if m.Role != types.OrganizationRoleMember {
		t.Fatalf("expected member role, got %s", m.Role)
	}
}

func TestAuthorizeOrgMember_UnscopedUserUnaffected(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())

	if _, err := a.AuthorizeOrgMember(context.Background(), &types.User{ID: "user-1"}, "org-1"); err != nil {
		t.Fatalf("unscoped user should authorize as before: %v", err)
	}
}
