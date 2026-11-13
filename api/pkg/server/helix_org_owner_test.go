package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type ownerResolutionStore struct {
	helixstore.Store
	org         *types.Organization
	memberships []*types.OrganizationMembership
	users       map[string]*types.User
	projects    map[string]*types.Project
}

func (s *ownerResolutionStore) GetOrganization(_ context.Context, _ *helixstore.GetOrganizationQuery) (*types.Organization, error) {
	return s.org, nil
}

func (s *ownerResolutionStore) ListOrganizationMemberships(_ context.Context, _ *helixstore.ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
	return s.memberships, nil
}

func (s *ownerResolutionStore) GetOrganizationMembership(_ context.Context, q *helixstore.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	for _, m := range s.memberships {
		if m.UserID == q.UserID && m.OrganizationID == q.OrganizationID {
			return m, nil
		}
	}
	return nil, errors.New("not a member")
}

func (s *ownerResolutionStore) GetUser(_ context.Context, q *helixstore.GetUserQuery) (*types.User, error) {
	u, ok := s.users[q.ID]
	if !ok {
		return nil, errors.New("user not found")
	}
	return u, nil
}

func (s *ownerResolutionStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	p, ok := s.projects[id]
	if !ok {
		return nil, errors.New("project not found")
	}
	return p, nil
}

func (s *ownerResolutionStore) ListTeams(context.Context, *helixstore.ListTeamsQuery) ([]*types.Team, error) {
	return nil, nil
}

func (s *ownerResolutionStore) ListAccessGrants(context.Context, *helixstore.ListAccessGrantsQuery) ([]*types.AccessGrant, error) {
	return nil, nil
}

func membership(org, user string, role types.OrganizationRole, created time.Time) *types.OrganizationMembership {
	return &types.OrganizationMembership{OrganizationID: org, UserID: user, Role: role, CreatedAt: created}
}

func TestResolveOrgOwner(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	users := map[string]*types.User{"u-founder": {ID: "u-founder"}, "u-second": {ID: "u-second"}, "u-third": {ID: "u-third"}, "u-member": {ID: "u-member"}}

	cases := []struct {
		name        string
		owner       string
		memberships []*types.OrganizationMembership
		want        string
	}{
		{"recorded owner still a member", "u-founder", []*types.OrganizationMembership{
			membership("org", "u-second", types.OrganizationRoleOwner, t0),
			membership("org", "u-founder", types.OrganizationRoleOwner, t0.Add(time.Hour)),
		}, "u-founder"},
		{"recorded owner left: earliest owner-role membership", "u-founder", []*types.OrganizationMembership{
			membership("org", "u-member", types.OrganizationRoleMember, t0),
			membership("org", "u-third", types.OrganizationRoleOwner, t0.Add(2*time.Hour)),
			membership("org", "u-second", types.OrganizationRoleOwner, t0.Add(time.Hour)),
		}, "u-second"},
		{"no recorded owner", "", []*types.OrganizationMembership{
			membership("org", "u-second", types.OrganizationRoleOwner, t0),
		}, "u-second"},
		{"no owner left", "u-founder", []*types.OrganizationMembership{
			membership("org", "u-member", types.OrganizationRoleMember, t0),
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &ownerResolutionStore{org: &types.Organization{ID: "org", Owner: tc.owner}, memberships: tc.memberships, users: users}
			got, err := resolveOrgOwner(context.Background(), st, "org")
			if tc.want == "" {
				if err == nil {
					t.Fatalf("resolved %s, want failure", got.ID)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveOrgOwner: %v", err)
			}
			if got.ID != tc.want {
				t.Fatalf("owner = %s, want %s", got.ID, tc.want)
			}
		})
	}
}

// A bot without a persisted hiring user acts as the organization owner, so
// its project reach is the owner's; a bot with one is bounded by that user.
func TestInProcAuthorizeProjectFallsBackToOrganizationOwner(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	st := &ownerResolutionStore{
		org: &types.Organization{ID: "org", Owner: "u-owner"},
		memberships: []*types.OrganizationMembership{
			membership("org", "u-owner", types.OrganizationRoleOwner, t0),
			membership("org", "u-member", types.OrganizationRoleMember, t0),
		},
		users:    map[string]*types.User{"u-owner": {ID: "u-owner"}, "u-member": {ID: "u-member"}},
		projects: map[string]*types.Project{"prj": {ID: "prj", OrganizationID: "org", UserID: "u-owner"}},
	}
	client := &inProcHelixClient{server: &HelixAPIServer{Store: st}}
	ctx := context.Background()
	if err := client.AuthorizeProject(ctx, "org", "", "prj", types.ActionCreate); err != nil {
		t.Fatalf("no hiring user should act as the owner: %v", err)
	}
	if err := client.AuthorizeProject(ctx, "org", "u-member", "prj", types.ActionGet); err == nil {
		t.Fatal("member without a grant must not reach the owner's project")
	}
	if err := client.AuthorizeProject(ctx, "org", "u-owner", "prj", types.ActionUpdate); err != nil {
		t.Fatalf("owner: %v", err)
	}

	// No owner left: fail closed rather than act as nobody.
	st.memberships = st.memberships[1:]
	if err := client.AuthorizeProject(ctx, "org", "", "prj", types.ActionGet); err == nil {
		t.Fatal("no owner left must refuse")
	}
}

// The write-time check passes the bot's persisted hiring user through, or an
// empty id for a legacy bot so the host resolves the owner.
func TestBotProjectAuthorizerUsesHiringUser(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := orggorm.GetOrgTestDB(t)
	var calls []string
	auth := botProjectAuthorizer{orgStore: st, authorize: func(_ context.Context, orgID, userID, projectID string, action types.Action) error {
		calls = append(calls, orgID+"/"+userID+"/"+projectID+"/"+string(action))
		return nil
	}}
	const orgID = "org-bot-projects"
	if err := (&runtimehelix.Hire{Store: st}).OnHire(ctx, orgID, orgchart.NodeID("w-hired"), "u-hirer"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AuthorizeBotProject(ctx, orgID, "w-hired", "prj"); err != nil {
		t.Fatal(err)
	}
	if err := auth.AuthorizeBotProject(ctx, orgID, "w-legacy", "prj"); err != nil {
		t.Fatal(err)
	}
	want := []string{orgID + "/u-hirer/prj/" + string(types.ActionGet), orgID + "//prj/" + string(types.ActionGet)}
	if len(calls) != 2 || calls[0] != want[0] || calls[1] != want[1] {
		t.Fatalf("authorize calls = %v, want %v", calls, want)
	}
}
