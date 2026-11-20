package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Two tenants. Tenant A's bot b-recruiter lives in org_a and project prj_a.
// Every verb on its proposals and settings must refuse tenant B, a member of
// org_a without access to prj_a, and a session-scoped key of A's own user —
// and must change nothing.
type orgBotAuthzFixture struct {
	server   *HelixAPIServer
	orgStore *helixorgstore.Store
	proposal *types.SpecTaskPRProposal
}

func newOrgBotAuthzFixture(t *testing.T) *orgBotAuthzFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	orgStore := orgmemory.New()
	ctx := context.Background()

	node, err := orgchart.NewNode("b-recruiter", "# Recruiter", nil, time.Now().UTC(), "org_a")
	require.NoError(t, err)
	node.AgentID = "app_a"
	require.NoError(t, orgStore.Nodes.Create(ctx, node))
	require.NoError(t, runtimehelix.SaveProject(ctx, orgStore, "org_a", node.ID, "prj_a", "app_a", ""))

	orgs := map[string]*types.Organization{"org_a": {ID: "org_a"}, "org_b": {ID: "org_b"}}
	helixStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, q *store.GetOrganizationQuery) (*types.Organization, error) {
			if o, ok := orgs[q.ID]; ok {
				return o, nil
			}
			return nil, store.ErrNotFound
		})
	members := map[string]types.OrganizationRole{
		"usr_a/org_a":      types.OrganizationRoleOwner,
		"usr_b/org_b":      types.OrganizationRoleOwner,
		"usr_member/org_a": types.OrganizationRoleMember,
	}
	helixStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(
		func(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			if role, ok := members[q.UserID+"/"+q.OrganizationID]; ok {
				return &types.OrganizationMembership{UserID: q.UserID, OrganizationID: q.OrganizationID, Role: role}, nil
			}
			return nil, store.ErrNotFound
		})
	helixStore.EXPECT().GetProject(gomock.Any(), "prj_a").AnyTimes().Return(
		&types.Project{ID: "prj_a", OrganizationID: "org_a", UserID: "usr_a"}, nil)
	// The member without project access falls through to access grants.
	helixStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
	helixStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)

	proposal := &types.SpecTaskPRProposal{
		ID: "prp_a", OrgBotID: "b-recruiter", OrganizationID: "org_a", ProjectID: "prj_a",
		HeadBranch: "feature/craft", BaseBranch: "main", Status: types.PRProposalStatusPending,
	}
	helixStore.EXPECT().GetSpecTaskPRProposal(gomock.Any(), "prp_a").AnyTimes().Return(proposal, nil)
	helixStore.EXPECT().ListSpecTaskPRProposals(gomock.Any(), gomock.Any()).AnyTimes().Return([]*types.SpecTaskPRProposal{proposal}, nil)
	// No UpdateSpecTaskPRProposal / CreateSpecTaskPRProposal expectation: any
	// write to a proposal fails the test.

	prProposals := services.NewPRProposalService(helixStore, nil, nil, "")
	prProposals.SetOrgBotHost(orgBotPRHost{st: orgStore, now: time.Now})
	return &orgBotAuthzFixture{
		server: &HelixAPIServer{
			Store:       helixStore,
			prProposals: prProposals,
			helixOrg:    &helixOrgHandlers{store: orgStore},
		},
		orgStore: orgStore,
		proposal: proposal,
	}
}

type orgBotCall struct {
	name    string
	method  string
	vars    map[string]string
	body    string
	handler func(*HelixAPIServer) http.HandlerFunc
}

func orgBotCalls(org string) []orgBotCall {
	bot := map[string]string{"org": org, "id": "b-recruiter"}
	decide := map[string]string{"org": org, "id": "b-recruiter", "proposal_id": "prp_a"}
	return []orgBotCall{
		{"list", http.MethodGet, bot, "", func(s *HelixAPIServer) http.HandlerFunc { return s.listOrgBotPRProposals }},
		{"decide", http.MethodPost, decide, `{"decision":"approve"}`, func(s *HelixAPIServer) http.HandlerFunc { return s.decideOrgBotPRProposal }},
		{"auto-approve", http.MethodPut, bot, `{"enabled":true}`, func(s *HelixAPIServer) http.HandlerFunc { return s.setOrgBotPRAutoApprove }},
		{"keep-alive", http.MethodPut, bot, `{"keep_alive":true}`, func(s *HelixAPIServer) http.HandlerFunc { return s.setOrgBotKeepAlive }},
	}
}

func (f *orgBotAuthzFixture) do(c orgBotCall, user types.User) int {
	req := httptest.NewRequest(c.method, "/", bytes.NewBufferString(c.body))
	req = mux.SetURLVars(req, c.vars)
	req = req.WithContext(setRequestUser(req.Context(), user))
	rec := httptest.NewRecorder()
	c.handler(f.server)(rec, req)
	return rec.Code
}

func (f *orgBotAuthzFixture) requireUnchanged(t *testing.T) {
	t.Helper()
	bot, err := f.orgStore.Nodes.Get(context.Background(), "org_a", "b-recruiter")
	require.NoError(t, err)
	require.False(t, bot.KeepAlive, "keep alive must be unchanged")
	require.False(t, bot.AutoApprovePullRequests, "auto-approve must be unchanged")
	require.Equal(t, types.PRProposalStatusPending, f.proposal.Status, "the proposal must be undecided")
}

func TestOrgBotProposalRoutesRefuseOtherTenants(t *testing.T) {
	outsider := types.User{ID: "usr_b"}
	member := types.User{ID: "usr_member"}
	for _, c := range orgBotCalls("org_a") {
		t.Run("tenant B on A's bot: "+c.name, func(t *testing.T) {
			f := newOrgBotAuthzFixture(t)
			require.Equal(t, http.StatusNotFound, f.do(c, outsider))
			f.requireUnchanged(t)
		})
		t.Run("org member without project access: "+c.name, func(t *testing.T) {
			f := newOrgBotAuthzFixture(t)
			require.Equal(t, http.StatusNotFound, f.do(c, member))
			f.requireUnchanged(t)
		})
	}
	// Tenant B naming its own org but A's bot id finds nothing.
	for _, c := range orgBotCalls("org_b") {
		t.Run("tenant B via own org: "+c.name, func(t *testing.T) {
			f := newOrgBotAuthzFixture(t)
			require.Equal(t, http.StatusNotFound, f.do(c, outsider))
			f.requireUnchanged(t)
		})
	}
}

// A bot's own key is session-scoped and owned by its hiring user: it must not
// approve its own pull requests or flip its own settings.
func TestOrgBotProposalDecisionsRefuseScopedKeys(t *testing.T) {
	scoped := types.User{ID: "usr_a", SessionID: "ses_bot"}
	for _, c := range orgBotCalls("org_a")[1:] {
		t.Run(c.name, func(t *testing.T) {
			f := newOrgBotAuthzFixture(t)
			require.Equal(t, http.StatusForbidden, f.do(c, scoped))
			f.requireUnchanged(t)
		})
	}
}

func TestOrgBotSettingsServeTheirOwner(t *testing.T) {
	owner := types.User{ID: "usr_a"}
	calls := orgBotCalls("org_a")

	f := newOrgBotAuthzFixture(t)
	require.Equal(t, http.StatusOK, f.do(calls[0], owner), "list")
	require.Equal(t, http.StatusNoContent, f.do(calls[2], owner), "auto-approve")
	require.Equal(t, http.StatusNoContent, f.do(calls[3], owner), "keep-alive")

	bot, err := f.orgStore.Nodes.Get(context.Background(), "org_a", "b-recruiter")
	require.NoError(t, err)
	require.True(t, bot.KeepAlive)
	require.True(t, bot.AutoApprovePullRequests)
	require.Equal(t, "usr_a", bot.AutoApprovePullRequestsBy)
}
