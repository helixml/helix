package server

import (
	"context"
	"net/http"
	"testing"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const harnessPolicyOrgID = "org_harness_policy"

// harnessPolicyStore disables one runtime for harnessPolicyOrgID only, so a
// check against any other organization ID sees the default (enabled) policy.
type harnessPolicyStore struct {
	*memorystore.MemoryStore
	disabled types.CodeAgentRuntime
}

func (s *harnessPolicyStore) GetOrganizationMembership(_ context.Context, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	if q.OrganizationID != harnessPolicyOrgID {
		return nil, store.ErrNotFound
	}
	return &types.OrganizationMembership{OrganizationID: q.OrganizationID, UserID: q.UserID, Role: types.OrganizationRoleMember}, nil
}

func (s *harnessPolicyStore) GetOrgCodeAgentHarness(_ context.Context, orgID string, runtime types.CodeAgentRuntime) (*types.OrgCodeAgentHarness, error) {
	if orgID != harnessPolicyOrgID || runtime != s.disabled {
		return nil, store.ErrNotFound
	}
	return &types.OrgCodeAgentHarness{OrganizationID: orgID, Runtime: runtime, Enabled: false}, nil
}

func newHarnessPolicyTestServer(t *testing.T, disabled types.CodeAgentRuntime) (*HelixAPIServer, *memorystore.MemoryStore, *types.User, *types.Session) {
	t.Helper()
	mem := memorystore.New()
	ps, err := pubsub.NewInMemoryNats()
	require.NoError(t, err)
	srv := NewTestServer(&harnessPolicyStore{MemoryStore: mem, disabled: disabled}, ps)

	user := &types.User{ID: "user_a", Type: types.OwnerTypeUser}
	seedCodingAgent(mem, "app_parent", "anthropic", "claude-opus-4-7")
	mem.SeedApp(&types.App{ID: "app_target", AgentKind: types.AgentKindCoding, OrganizationID: harnessPolicyOrgID, Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal, CodeAgentRuntime: disabled,
		}},
	}}})
	session := newTestParentSession(user.ID)
	session.OrganizationID = harnessPolicyOrgID
	seedParentWithInteractions(t, mem, session, 1)
	return srv, mem, user, session
}

func TestSwitchAgentRejectsRuntimeDisabledForOrg(t *testing.T) {
	srv, mem, user, session := newHarnessPolicyTestServer(t, types.CodeAgentRuntimeZedAgent)

	rr := callSwitchAgentHTTP(t, srv, user, session.ID, SwitchAgentRequest{HelixAppID: "app_target"})
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), `coding-agent runtime "zed_agent" is not enabled for this organization`)

	unchanged, err := mem.GetSession(context.Background(), session.ID)
	require.NoError(t, err)
	assert.Equal(t, session.ParentApp, unchanged.ParentApp)
}

func TestForkRejectsRuntimeDisabledForOrg(t *testing.T) {
	srv, mem, user, session := newHarnessPolicyTestServer(t, types.CodeAgentRuntimeZedAgent)

	rr := callForkHTTP(t, srv, user, session.ID, ForkSessionRequest{HelixAppID: "app_target"})
	require.Equal(t, http.StatusUnprocessableEntity, rr.Code, "body: %s", rr.Body.String())
	assert.Contains(t, rr.Body.String(), `coding-agent runtime "zed_agent" is not enabled for this organization`)

	parent, err := mem.GetSession(context.Background(), session.ID)
	require.NoError(t, err)
	assert.False(t, parent.Metadata.Paused, "a rejected fork must not pause the parent")
}
