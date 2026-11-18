package orgstore

import (
	"context"
	"errors"
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
			// EnforceKeyOrgScope both cross-org assertions would succeed,
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

func TestAuthorizeOrg_AdminKeepsOwnerAccessAfterJoining(t *testing.T) {
	ctx := context.Background()
	for name, membership := range map[string]*types.OrganizationMembership{
		"no membership": nil,
		"member":        {OrganizationID: "org-1", UserID: "admin-1", Role: types.OrganizationRoleMember},
		"owner":         {OrganizationID: "org-1", UserID: "admin-1", Role: types.OrganizationRoleOwner},
	} {
		t.Run(name, func(t *testing.T) {
			queryer := &fakeQueryer{memberships: map[string]*types.OrganizationMembership{}}
			originalRole := types.OrganizationRole("")
			if membership != nil {
				queryer.memberships["org-1/admin-1"] = membership
				originalRole = membership.Role
			}
			a := NewAuthorizer(queryer)
			user := &types.User{ID: "admin-1", Admin: true}

			for helper, authorize := range map[string]func() (*types.OrganizationMembership, error){
				"member": func() (*types.OrganizationMembership, error) {
					return a.AuthorizeOrgMember(ctx, user, "org-1")
				},
				"owner": func() (*types.OrganizationMembership, error) {
					return a.AuthorizeOrgOwner(ctx, user, "org-1")
				},
			} {
				got, err := authorize()
				if err != nil {
					t.Fatalf("%s helper: %v", helper, err)
				}
				if got.OrganizationID != "org-1" || got.UserID != "admin-1" || got.Role != types.OrganizationRoleOwner {
					t.Fatalf("%s helper returned %#v, want synthetic owner", helper, got)
				}
			}
			if membership != nil && queryer.memberships["org-1/admin-1"].Role != originalRole {
				t.Fatal("authorization changed the stored membership")
			}
		})
	}
}

func TestAuthorizeOrg_AdminScopeGuardsStillApply(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	ctx := context.Background()
	orgScoped := &types.User{ID: "user-1", Admin: true, OrganizationID: "org-1"}
	if _, err := a.AuthorizeOrgMember(ctx, orgScoped, "org-2"); !errors.Is(err, ErrKeyOrgScope) {
		t.Fatalf("member helper: expected ErrKeyOrgScope, got %v", err)
	}
	if _, err := a.AuthorizeOrgOwner(ctx, orgScoped, "org-2"); !errors.Is(err, ErrKeyOrgScope) {
		t.Fatalf("owner helper: expected ErrKeyOrgScope, got %v", err)
	}

	for name, user := range map[string]*types.User{
		"project key": {ID: "user-1", Admin: true, OrganizationID: "org-2", ProjectID: "prj-1"},
		"session key": {ID: "user-1", Admin: true, OrganizationID: "org-2", SessionID: "ses-1"},
	} {
		if _, err := a.AuthorizeOrgOwner(ctx, user, "org-2"); !errors.Is(err, ErrNotOrgOwner) {
			t.Errorf("%s: expected ErrNotOrgOwner, got %v", name, err)
		}
	}
}

// org-2 seeds user-1 as owner, so without enforceKeyNotAgentScoped every
// scoped case below would be authorized.
func TestAuthorizeOrgOwner_AgentScopedKeysRefused(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	for name, user := range map[string]*types.User{
		"project key": {ID: "user-1", OrganizationID: "org-2", ProjectID: "prj-1", SessionID: "ses-1", SpecTaskID: "spt-1"},
		"session key": {ID: "user-1", SessionID: "ses-1"},
		"worker key":  {ID: "user-1", OrganizationID: "org-2", SessionID: "ses-2"},
	} {
		// ErrNotOrgOwner, so handlers answer 403 rather than 500.
		if _, err := a.AuthorizeOrgOwner(context.Background(), user, "org-2"); !errors.Is(err, ErrNotOrgOwner) {
			t.Errorf("%s: expected ErrNotOrgOwner, got %v", name, err)
		}
	}
}

func TestAuthorizeOrgOwner_OrgKeyAndPersonalKeyKeepOwnerRole(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	for name, user := range map[string]*types.User{
		"personal key": {ID: "user-1"},
		"org key":      {ID: "user-1", OrganizationID: "org-2"},
	} {
		if _, err := a.AuthorizeOrgOwner(context.Background(), user, "org-2"); err != nil {
			t.Errorf("%s: expected owner authority, got %v", name, err)
		}
	}
}

// Membership (read) checks are unchanged for scoped keys: an agent still
// works inside its organization.
func TestAuthorizeOrgMember_AgentScopedKeyKeepsMembership(t *testing.T) {
	a := NewAuthorizer(newFakeQueryer())
	user := &types.User{ID: "user-1", OrganizationID: "org-2", ProjectID: "prj-1", SessionID: "ses-1"}
	if _, err := a.AuthorizeOrgMember(context.Background(), user, "org-2"); err != nil {
		t.Fatalf("expected membership, got %v", err)
	}
}
