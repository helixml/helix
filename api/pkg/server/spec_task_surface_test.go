package server

import (
	"context"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	helixstore "github.com/helixml/helix/api/pkg/store"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgmemory "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
)

func boundAgentStore(t *testing.T, nodeID, orgID, projectID string, tools []tool.Name) *orgstore.Store {
	t.Helper()
	st := orgmemory.New()
	node, err := orgchart.NewNode(nodeID, "# bound", tools, time.Now().UTC(), orgID)
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(context.Background(), node))
	require.NoError(t, runtimehelix.SaveProject(context.Background(), st, orgID, nodeID, projectID, "app-"+nodeID, "repo-1"))
	return st
}

func surfaceProject() *types.Project {
	return &types.Project{ID: "prj-1", OrganizationID: "org-1", AgentTools: []string{"create_spectask"}}
}

func TestSpecTaskToolSurfaceUnionsBoundAgentTools(t *testing.T) {
	s := &HelixAPIServer{helixOrg: &helixOrgHandlers{store: boundAgentStore(t, "b-1", "org-1", "prj-1",
		[]tool.Name{"get_secret", "list_secrets", "create_spectask"})}}
	tools, bound := s.specTaskToolSurface(context.Background(), surfaceProject(), &types.SpecTask{AgentTools: []string{"get_spectask"}})
	require.Equal(t, orgchart.NodeID("b-1"), bound)
	require.ElementsMatch(t,
		[]tool.Name{"create_spectask", "get_spectask", "get_secret", "list_secrets"},
		tools)
}

func TestSpecTaskToolSurfaceUnboundProjectKeepsOwnGrantsOnly(t *testing.T) {
	s := &HelixAPIServer{helixOrg: &helixOrgHandlers{store: boundAgentStore(t, "b-1", "org-1", "prj-other",
		[]tool.Name{"get_secret"})}}
	tools, bound := s.specTaskToolSurface(context.Background(), surfaceProject(), &types.SpecTask{})
	require.Empty(t, bound)
	require.Equal(t, []tool.Name{"create_spectask"}, tools)
}

func TestSpecTaskToolSurfaceNilOrgKeepsOwnGrantsOnly(t *testing.T) {
	s := &HelixAPIServer{}
	tools, bound := s.specTaskToolSurface(context.Background(), surfaceProject(), &types.SpecTask{})
	require.Empty(t, bound)
	require.Equal(t, []tool.Name{"create_spectask"}, tools)
}

func TestSpecTaskToolSurfaceDropsBlockedAdminTools(t *testing.T) {
	s := &HelixAPIServer{helixOrg: &helixOrgHandlers{store: boundAgentStore(t, "b-1", "org-1", "prj-1",
		[]tool.Name{"get_secret", "attach_tool", "create_bot", "delete_bot", "chat"})}}
	tools, bound := s.specTaskToolSurface(context.Background(), surfaceProject(), &types.SpecTask{})
	require.Equal(t, orgchart.NodeID("b-1"), bound)
	require.ElementsMatch(t, []tool.Name{"create_spectask", "get_secret", "chat"}, tools)
	for _, banned := range []string{"attach_tool", "create_bot", "delete_bot"} {
		for _, name := range tools {
			require.NotEqual(t, tool.Name(banned), name)
		}
	}
}

func TestSpecTaskToolSurfaceAmbiguousOwnershipFailsClosed(t *testing.T) {
	st := boundAgentStore(t, "b-1", "org-1", "prj-1", []tool.Name{"get_secret"})
	node, err := orgchart.NewNode("b-2", "# dup", []tool.Name{"publish"}, time.Now().UTC(), "org-1")
	require.NoError(t, err)
	require.NoError(t, st.Nodes.Create(context.Background(), node))
	require.NoError(t, runtimehelix.SaveProject(context.Background(), st, "org-1", "b-2", "prj-1", "app-b-2", "repo-1"))
	s := &HelixAPIServer{helixOrg: &helixOrgHandlers{store: st}}
	tools, bound := s.specTaskToolSurface(context.Background(), surfaceProject(), &types.SpecTask{})
	require.Empty(t, bound)
	require.Equal(t, []tool.Name{"create_spectask"}, tools)
}

func TestSpecTaskAgentToolsTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	boundStore := boundAgentStore(t, "b-1", "org-1", "prj-1", []tool.Name{"get_secret", "chat"})

	task := &types.SpecTask{ID: "spt-1", ProjectID: "prj-1", UserID: "u-1"}
	orgProject := &types.Project{ID: "prj-1", OrganizationID: "org-1", AgentTools: []string{"create_spectask"}}
	unboundTask := &types.SpecTask{ID: "spt-2", ProjectID: "prj-none", UserID: "u-1"}
	unboundProject := &types.Project{ID: "prj-none", OrganizationID: "org-1", AgentTools: []string{"get_spectask"}}

	mockStore := helixstore.NewMockStore(ctrl)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt-1").Return(task, nil).AnyTimes()
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt-2").Return(unboundTask, nil).AnyTimes()
	mockStore.EXPECT().GetProject(gomock.Any(), "prj-1").Return(orgProject, nil).AnyTimes()
	mockStore.EXPECT().GetProject(gomock.Any(), "prj-none").Return(unboundProject, nil).AnyTimes()

	server := &HelixAPIServer{Store: mockStore, helixOrg: &helixOrgHandlers{store: boundStore}}

	nonTask := &types.Session{ID: "ses-1"}
	require.Nil(t, server.specTaskAgentTools(context.Background(), nonTask))
	require.Nil(t, server.specTaskAgentTools(context.Background(), nil))

	orgSession := &types.Session{ID: "ses-2", Metadata: types.SessionMetadata{SpecTaskID: "spt-1"}}
	got := server.specTaskAgentTools(context.Background(), orgSession)
	require.ElementsMatch(t, []string{"create_spectask", "get_secret", "chat"}, got)

	unboundSession := &types.Session{ID: "ses-3", Metadata: types.SessionMetadata{SpecTaskID: "spt-2"}}
	require.Equal(t, []string{"get_spectask"}, server.specTaskAgentTools(context.Background(), unboundSession))
}
