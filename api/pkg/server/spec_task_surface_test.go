package server

import (
	"context"
	"testing"
	"time"

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
