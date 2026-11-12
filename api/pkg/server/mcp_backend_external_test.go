package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.uber.org/mock/gomock"

	mcpclient "github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type externalMCPResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func decodeExternalMCPResponse(t *testing.T, body []byte) string {
	t.Helper()
	var response externalMCPResponse
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode MCP response %q: %v", body, err)
	}
	if response.Error != nil {
		t.Fatalf("MCP error: %s", response.Error.Message)
	}
	return string(response.Result)
}

func serveExternalMCPRequest(t *testing.T, httpServer http.Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	httpServer.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("MCP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeExternalMCPResponse(t, rec.Body.Bytes())
}

func serveExternalBackendRequest(t *testing.T, backend *ExternalMCPBackend, user *types.User, mcpName, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/external/"+mcpName, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req = mux.SetURLVars(req, map[string]string{"path": mcpName})
	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, user)
	if rec.Code != http.StatusOK {
		t.Fatalf("external MCP status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return decodeExternalMCPResponse(t, rec.Body.Bytes())
}

func TestExternalMCPBackendUsesProjectMCPBeforeStaleParentApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	mockClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID  = "ses_project_mcp"
		userID     = "usr_project_mcp"
		staleAppID = "app_missing"
		projectID  = "prj_project_mcp"
	)
	projectMCP := types.AssistantMCP{Name: "HelixOS", URL: "https://project.example/mcp"}
	project := &types.Project{
		ID:     projectID,
		Skills: &types.AssistantSkills{MCPs: []types.AssistantMCP{projectMCP}},
	}

	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID, Owner: userID, ParentApp: staleAppID, ProjectID: projectID,
	}, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(project, nil)
	var clientCtx context.Context
	mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&projectMCP)).DoAndReturn(func(ctx context.Context, _ interface{}, _ interface{}, _ interface{}) (mcpclient.Client, error) {
		clientCtx = ctx
		return mockClient, nil
	})
	mockClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil)

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	backend.connectTimeout = 20 * time.Millisecond
	defer backend.Stop()

	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: userID}, sessionID, "helixos"); err != nil {
		t.Fatalf("get project MCP proxy: %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	if err := clientCtx.Err(); err != nil {
		t.Fatalf("successful MCP client context was canceled: %v", err)
	}
}

func TestExternalMCPBackendFallsBackToAppWhenProjectHasNoMatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	mockClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID = "ses_app_fallback_mcp"
		userID    = "usr_app_fallback_mcp"
		appID     = "app_fallback_mcp"
		projectID = "prj_app_fallback_mcp"
	)
	projectMCP := types.AssistantMCP{Name: "Other", URL: "https://project.example/other"}
	appMCP := types.AssistantMCP{Name: "HelixOS", URL: "https://app.example/mcp"}
	app := &types.App{
		ID: appID,
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			MCPs: []types.AssistantMCP{appMCP},
		}}}},
	}
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID, Owner: userID, ParentApp: appID, ProjectID: projectID,
	}, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID: projectID, Skills: &types.AssistantSkills{MCPs: []types.AssistantMCP{projectMCP}},
	}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), appID).Return(app, nil)
	mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&appMCP)).Return(mockClient, nil)
	mockClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil)

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	defer backend.Stop()

	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: userID}, sessionID, "helixos"); err != nil {
		t.Fatalf("get App MCP fallback: %v", err)
	}
}

func TestExternalMCPBackendUsesProjectMCPWithoutParentApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockGetter := mcpclient.NewMockClientGetter(ctrl)
	mockClient := mcpclient.NewMockClient(ctrl)

	const (
		sessionID = "ses_project_only_mcp"
		userID    = "usr_project_only_mcp"
		projectID = "prj_project_only_mcp"
	)
	projectMCP := types.AssistantMCP{Name: "HelixOS", URL: "https://project.example/mcp"}
	mockStore.EXPECT().GetSession(gomock.Any(), sessionID).Return(&types.Session{
		ID: sessionID, Owner: userID, ParentApp: "", ProjectID: projectID,
	}, nil)
	mockStore.EXPECT().GetProject(gomock.Any(), projectID).Return(&types.Project{
		ID: projectID, Skills: &types.AssistantSkills{MCPs: []types.AssistantMCP{projectMCP}},
	}, nil)
	mockGetter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&projectMCP)).Return(mockClient, nil)
	mockClient.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil)

	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = mockGetter
	defer backend.Stop()

	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: userID}, sessionID, "helixos"); err != nil {
		t.Fatalf("get project MCP proxy without parent App: %v", err)
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

func TestExternalMCPBackendStartsDegradedAndReportsRecovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	client := mcpclient.NewMockClient(ctrl)
	config := types.AssistantMCP{Name: "Cyber", URL: "https://cyber.example/mcp"}
	app := &types.App{ID: "app_1", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{config}}}}}}
	session := &types.Session{ID: "ses_1", Owner: "user_1", ParentApp: app.ID}
	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil).AnyTimes()
	gomock.InOrder(
		getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&config)).Return(nil, errors.New("transport error: unauthorized (401)")),
		getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&config)).Return(client, nil),
		client.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil),
		client.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil),
	)
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	defer backend.Stop()

	user := &types.User{ID: session.Owner, SessionID: session.ID}
	optionsReq := httptest.NewRequest(http.MethodOptions, "/api/v1/mcp/external/cyber", nil)
	optionsReq = mux.SetURLVars(optionsReq, map[string]string{"path": "cyber"})
	optionsRec := httptest.NewRecorder()
	backend.ServeHTTP(optionsRec, optionsReq, user)
	if optionsRec.Code >= 500 || optionsRec.Code == http.StatusUnauthorized || optionsRec.Code == http.StatusForbidden {
		t.Fatalf("degraded OPTIONS status = %d, body = %s", optionsRec.Code, optionsRec.Body.String())
	}
	initialized := serveExternalBackendRequest(t, backend, user, "cyber", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if !strings.Contains(initialized, "did not accept its configured credentials") {
		t.Fatalf("initialize response did not explain authentication failure: %s", initialized)
	}
	if strings.Contains(initialized, config.URL) || strings.Contains(initialized, "401") {
		t.Fatalf("initialize response leaked diagnostics: %s", initialized)
	}
	listed := serveExternalBackendRequest(t, backend, user, "cyber", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !strings.Contains(listed, "helix_connection_status") {
		t.Fatalf("degraded tools = %s, want connection status", listed)
	}
	unavailable := serveExternalBackendRequest(t, backend, user, "cyber", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"helix_connection_status","arguments":{}}}`)
	if !strings.Contains(unavailable, "tools are unavailable") || strings.Contains(unavailable, "is connected") {
		t.Fatalf("unavailable response = %s", unavailable)
	}
	backend.serversMu.Lock()
	backend.servers[session.ID+":cyber"].createdAt = time.Now().Add(-externalMCPDegradedTTL - time.Second)
	backend.serversMu.Unlock()
	called := serveExternalBackendRequest(t, backend, user, "cyber", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"helix_connection_status","arguments":{}}}`)
	if !strings.Contains(called, "Cyber is connected") || !strings.Contains(called, "Reconnect the coding agent") {
		t.Fatalf("recovery response = %s", called)
	}
}

func TestExternalMCPBackendConfigChangeBypassesDegradedCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	client := mcpclient.NewMockClient(ctrl)
	oldConfig := types.AssistantMCP{Name: "Cyber", URL: "https://cyber.example/mcp"}
	newConfig := oldConfig
	newConfig.Headers = map[string]string{"Authorization": "Bearer fixed"}
	app := &types.App{ID: "app_1", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{oldConfig}}}}}}
	session := &types.Session{ID: "ses_1", Owner: "user_1", ParentApp: app.ID}
	mockStore.EXPECT().GetSession(gomock.Any(), session.ID).Return(session, nil).AnyTimes()
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil).AnyTimes()
	gomock.InOrder(
		getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&oldConfig)).Return(nil, errors.New("unauthorized")),
		getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&newConfig)).Return(client, nil),
		client.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(&mcp.ListToolsResult{}, nil),
	)
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	defer backend.Stop()
	user := &types.User{ID: session.Owner}

	if _, err := backend.getOrCreateServer(context.Background(), user, session.ID, "cyber"); err != nil {
		t.Fatal(err)
	}
	app.Config.Helix.Assistants[0].MCPs[0] = newConfig
	if _, err := backend.getOrCreateServer(context.Background(), user, session.ID, "cyber"); err != nil {
		t.Fatal(err)
	}
	if backend.servers[session.ID+":cyber"].degraded {
		t.Fatal("updated credentials reused degraded cache")
	}
}

func TestExternalMCPDegradedCacheExpiresDespiteTraffic(t *testing.T) {
	server := &externalMCPServer{
		createdAt: time.Now().Add(-externalMCPDegradedTTL - time.Second),
		lastUsed:  time.Now(),
		degraded:  true,
	}
	server.touch()
	if !server.isExpired(5 * time.Minute) {
		t.Fatal("degraded cache traffic extended its absolute TTL")
	}
}

func TestExternalMCPBackendRejectsWrongSessionOwnerBeforeConnecting(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_other").Return(&types.Session{ID: "ses_other", Owner: "user_a"}, nil)
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	defer backend.Stop()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/external/cyber", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	req = mux.SetURLVars(req, map[string]string{"path": "cyber"})
	rec := httptest.NewRecorder()
	backend.ServeHTTP(rec, req, &types.User{ID: "user_b", SessionID: "ses_other"})
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "forbidden") {
		t.Fatalf("wrong-owner status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestExternalMCPBackendListToolsFailureIsDegraded(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	client := mcpclient.NewMockClient(ctrl)
	config := types.AssistantMCP{Name: "Scanner", URL: "https://scanner.example/mcp"}
	app := &types.App{ID: "app_1", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{config}}}}}}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "user_1", ParentApp: app.ID}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&config)).Return(client, nil)
	client.EXPECT().ListTools(gomock.Any(), gomock.Any()).Return(nil, errors.New("upstream unavailable"))
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	defer backend.Stop()

	httpServer, err := backend.getOrCreateServer(context.Background(), &types.User{ID: "user_1"}, "ses_1", "scanner")
	if err != nil {
		t.Fatalf("get degraded MCP proxy: %v", err)
	}
	listed := serveExternalMCPRequest(t, httpServer, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(listed, "helix_connection_status") {
		t.Fatalf("degraded tools = %s, want connection status", listed)
	}
}

func TestExternalMCPBackendConnectTimeoutIsDegraded(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	config := types.AssistantMCP{Name: "Slow", URL: "https://slow.example/mcp"}
	app := &types.App{ID: "app_1", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{config}}}}}}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "user_1", ParentApp: app.ID}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&config)).DoAndReturn(func(ctx context.Context, _ interface{}, _ interface{}, _ interface{}) (mcpclient.Client, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	backend.connectTimeout = 20 * time.Millisecond
	defer backend.Stop()

	started := time.Now()
	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: "user_1"}, "ses_1", "slow"); err != nil {
		t.Fatalf("get degraded MCP proxy: %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("degraded proxy took %v, want bounded connection attempt", elapsed)
	}
}

func TestExternalMCPBackendListToolsCompletingAfterTimeoutIsDegraded(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	getter := mcpclient.NewMockClientGetter(ctrl)
	client := mcpclient.NewMockClient(ctrl)
	config := types.AssistantMCP{Name: "Slow", URL: "https://slow.example/mcp"}
	app := &types.App{ID: "app_1", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{MCPs: []types.AssistantMCP{config}}}}}}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: "user_1", ParentApp: app.ID}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
	getter.EXPECT().NewClient(gomock.Any(), gomock.Any(), nil, gomock.Eq(&config)).Return(client, nil)
	client.EXPECT().ListTools(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _ mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
		<-ctx.Done()
		return &mcp.ListToolsResult{}, nil
	})
	backend := NewExternalMCPBackend(mockStore)
	backend.clientGetter = getter
	backend.connectTimeout = 20 * time.Millisecond
	defer backend.Stop()

	if _, err := backend.getOrCreateServer(context.Background(), &types.User{ID: "user_1"}, "ses_1", "slow"); err != nil {
		t.Fatal(err)
	}
	if !backend.servers["ses_1:slow"].degraded {
		t.Fatal("client completing after timeout was cached as healthy")
	}
}

func TestExternalMCPAuthenticationErrorClassification(t *testing.T) {
	for _, message := range []string{"transport error: unauthorized (401)", "request failed with status 403", "unexpected status code: 401"} {
		if !isExternalMCPAuthenticationError(errors.New(message)) {
			t.Errorf("%q was not classified as authentication failure", message)
		}
	}
	if isExternalMCPAuthenticationError(errors.New("dial tcp 10.0.0.1:403: connection refused")) {
		t.Error("port 403 was classified as authentication failure")
	}
}

func TestExternalMCPBackendPreservesUpstreamConnectionStatusTool(t *testing.T) {
	upstream := mcpserver.NewMCPServer("test", "1")
	upstream.AddTool(mcp.NewTool("helix_connection_status", mcp.WithDescription("upstream status")), func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("upstream"), nil
	})
	(&ExternalMCPBackend{}).addConnectionStatusTool(upstream, "Cyber", mcpclient.NewMockClient(gomock.NewController(t)), nil)
	httpServer := mcpserver.NewStreamableHTTPServer(upstream, mcpserver.WithStateLess(true))
	listed := serveExternalMCPRequest(t, httpServer, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if !strings.Contains(listed, "upstream status") || strings.Contains(listed, "currently unavailable") {
		t.Fatalf("upstream connection status was replaced: %s", listed)
	}
}

func TestExternalMCPBackendCleanupRemovesDegradedServerWithoutCancel(t *testing.T) {
	backend := NewExternalMCPBackend(nil)
	defer backend.Stop()
	backend.servers["session:mcp"] = &externalMCPServer{
		createdAt: time.Now().Add(-externalMCPDegradedTTL - time.Second),
		lastUsed:  time.Now(),
		degraded:  true,
	}
	backend.cleanupExpiredServers(5 * time.Minute)
	if _, ok := backend.servers["session:mcp"]; ok {
		t.Fatal("expired degraded server was not removed")
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

// The external proxy is the only path Zed's tool calls take, so the acting
// session must travel with them. Without it an external MCP server cannot tell
// which user a call is for except by trusting a model-supplied argument — Find
// AI's tools failed closed with "requires a signed-in session" for exactly this
// reason, on production.
func TestProxyMetaCarriesTheActingSession(t *testing.T) {
	m := proxyMeta(&types.User{ID: "user_1", AppID: "app_1"}, "ses_real")
	if m.SessionID != "ses_real" {
		t.Errorf("SessionID = %q, want ses_real — the X-Helix-Session-Id header depends on it", m.SessionID)
	}
	if m.UserID != "user_1" {
		t.Errorf("UserID = %q, want user_1", m.UserID)
	}
	if m.AppID != "app_1" {
		t.Errorf("AppID = %q, want app_1", m.AppID)
	}
}
