package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func externalAgentApp(id, name string) *types.Agent {
	return &types.Agent{
		ID:             id,
		OrganizationID: "org-test",
		Owner:          "user-someone-else",
		AgentKind:      types.AgentKindCoding,
		Config: types.AgentConfig{Helix: types.AgentHelixConfig{
			Name:             name,
			DefaultAgentType: types.AgentTypeZedExternal,
			Assistants: []types.AssistantConfig{{
				CodeAgentRuntime: types.CodeAgentRuntimeCodexCLI,
			}},
		}},
	}
}

func specTaskAgentsRequest(ctx context.Context) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/prj_1/spec-task-agents", http.NoBody)
	req = req.WithContext(ctx)
	return mux.SetURLVars(req, map[string]string{"id": "prj_1"})
}

// The returned ids are directly usable as CreateTaskRequest.app_id, and
// createTaskFromPrompt authorizes the project but NOT the app — so this endpoint
// is the only thing standing between a member and an agent they weren't granted.
func TestListProjectSpecTaskAgents_HidesAgentsTheMemberCannotAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	user := &types.User{ID: "user-member"}
	ctx := setRequestUser(context.Background(), *user)

	helixStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{
		ID: "prj_1", OrganizationID: "org-test", UserID: user.ID,
	}, nil)
	helixStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
		&types.OrganizationMembership{
			OrganizationID: "org-test", UserID: user.ID, Role: types.OrganizationRoleMember,
		}, nil).AnyTimes()
	helixStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return([]*types.Agent{
		externalAgentApp("app-ungranted", "Someone else's agent"),
	}, nil)
	// No teams, no access grants, and no project references the agent: this
	// member was never given it by any route.
	helixStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	helixStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	helixStore.EXPECT().ListProjects(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	server := &HelixAPIServer{Store: helixStore}
	agents, httpErr := server.listProjectSpecTaskAgents(httptest.NewRecorder(), specTaskAgentsRequest(ctx))

	require.Nil(t, httpErr)
	require.Empty(t, agents)
}

func TestListProjectSpecTaskAgents_OrgOwnerSeesEligibleAgentsSorted(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	user := &types.User{ID: "user-owner"}
	ctx := setRequestUser(context.Background(), *user)

	helixStore.EXPECT().GetProject(gomock.Any(), "prj_1").Return(&types.Project{
		ID: "prj_1", OrganizationID: "org-test", UserID: user.ID,
	}, nil)
	helixStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).Return(
		&types.OrganizationMembership{
			OrganizationID: "org-test", UserID: user.ID, Role: types.OrganizationRoleOwner,
		}, nil).AnyTimes()
	helixStore.EXPECT().ListApps(gomock.Any(), gomock.Any()).Return([]*types.Agent{
		externalAgentApp("app-zeta", "Zeta"),
		externalAgentApp("app-alpha", "alpha"),
		{
			ID:             "app-helix",
			OrganizationID: "org-test",
			Config: types.AgentConfig{Helix: types.AgentHelixConfig{
				Name:             "Not an external agent",
				DefaultAgentType: types.AgentTypeHelixAgent,
			}},
		},
	}, nil)

	server := &HelixAPIServer{Store: helixStore}
	agents, httpErr := server.listProjectSpecTaskAgents(httptest.NewRecorder(), specTaskAgentsRequest(ctx))

	require.Nil(t, httpErr)
	require.Len(t, agents, 2)
	require.Equal(t, "app-alpha", agents[0].ID)
	require.Equal(t, types.CodeAgentRuntimeCodexCLI, agents[0].CodeAgentRuntime)
	require.Equal(t, "app-zeta", agents[1].ID)
}
