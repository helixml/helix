package server

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/mock/gomock"

	mcpclient "github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestExternalMCPBackendUsesProjectMCPOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	mockClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID = "ses_project_mcp"
		userID    = "usr_project_mcp"
		appID     = "app_project_mcp"
		projectID = "prj_project_mcp"
	)
	appMCP := types.AssistantMCP{Name: "HelixOS", URL: "https://app.example/mcp"}
	projectMCP := types.AssistantMCP{Name: "HelixOS", URL: "https://project.example/mcp"}
	app := &types.App{
		ID: appID,
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			MCPs: []types.AssistantMCP{appMCP},
		}}}},
	}
	project := &types.Project{
		ID:     projectID,
		Skills: &types.AssistantSkills{MCPs: []types.AssistantMCP{projectMCP}},
	}

	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID, Owner: userID, ParentApp: appID, ProjectID: projectID,
	}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil)
	mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&projectMCP)).Return(mockClient, nil)
	mockClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil)

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	defer backend.Stop()

	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: userID}, sessionID, "helixos"); err != nil {
		t.Fatalf("get project MCP proxy: %v", err)
	}
}
