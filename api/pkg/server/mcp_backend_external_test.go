package server

import (
	"context"
	"testing"
	"time"

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

func TestExternalMCPBackendRebuildsCachedServerWhenMCPConfigChanges(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	oldClient := mcpclient.NewMockClient(ctrl)
	newClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID = "ses_rotated_mcp"
		userID    = "usr_rotated_mcp"
		appID     = "app_rotated_mcp"
	)
	oldMCP := types.AssistantMCP{Name: "helix", URL: "https://org.example/mcp", Headers: map[string]string{"Authorization": "Bearer old"}}
	newMCP := oldMCP
	newMCP.Headers = map[string]string{"Authorization": "Bearer new"}
	appWith := func(config types.AssistantMCP) *types.App {
		return &types.App{ID: appID, Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{config}}}}}}
	}
	session := &types.Session{ID: sessionID, Owner: userID, ParentApp: appID}
	gomock.InOrder(
		mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil),
		mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(appWith(oldMCP), nil),
		mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&oldMCP)).Return(oldClient, nil),
		oldClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil),
		mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(session, nil),
		mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(appWith(newMCP), nil),
		mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&newMCP)).Return(newClient, nil),
		newClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil),
	)

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	defer backend.Stop()
	user := &types.User{ID: userID}
	oldServer, err := backend.getOrCreateServer(context.Background(), user, sessionID, "helix")
	if err != nil {
		t.Fatalf("create old MCP proxy: %v", err)
	}
	newServer, err := backend.getOrCreateServer(context.Background(), user, sessionID, "helix")
	if err != nil {
		t.Fatalf("create rotated MCP proxy: %v", err)
	}
	if oldServer == newServer {
		t.Fatal("rotated MCP config reused cached proxy server")
	}
}

func TestExternalMCPBackendCoalescesConcurrentInitialization(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	mockClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID = "ses_concurrent_mcp"
		userID    = "usr_concurrent_mcp"
		appID     = "app_concurrent_mcp"
	)
	mcpConfig := types.AssistantMCP{Name: "helix", URL: "https://org.example/mcp"}
	app := &types.App{ID: appID, Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{mcpConfig}}}}}}
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{ID: sessionID, Owner: userID, ParentApp: appID}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&mcpConfig)).Return(mockClient, nil)
	entered := make(chan struct{})
	release := make(chan struct{})
	mockClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
		close(entered)
		<-release
		return &mcp.ListToolsResult{}, nil
	})

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	defer backend.Stop()
	user := &types.User{ID: userID}
	results := make(chan error, 2)
	go func() {
		_, err := backend.getOrCreateServer(context.Background(), user, sessionID, "helix")
		results <- err
	}()
	<-entered
	go func() {
		_, err := backend.getOrCreateServer(context.Background(), user, sessionID, "helix")
		results <- err
	}()
	select {
	case err := <-results:
		t.Fatalf("concurrent initialization returned before the first completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("get concurrent MCP proxy: %v", err)
		}
	}
}

func TestExternalMCPBackendCleanupPreservesReplacement(t *testing.T) {
	oldServer := &externalMCPServer{}
	freshServer := &externalMCPServer{}
	backend := &ExternalMCPBackend{servers: map[string]*externalMCPServer{"session:mcp": freshServer}}

	removed := backend.deleteExpiredServers(map[string]*externalMCPServer{"session:mcp": oldServer})
	if len(removed) != 0 {
		t.Fatalf("removed %d servers from stale snapshot, want 0", len(removed))
	}
	if backend.servers["session:mcp"] != freshServer {
		t.Fatal("cleanup deleted the fresh replacement server")
	}
}
