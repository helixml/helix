package server

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/visualization"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// VisualizationMCPSuite covers the html_render tool on the session MCP backend.
type VisualizationMCPSuite struct {
	suite.Suite
	ctrl      *gomock.Controller
	mockStore *store.MockStore
	notifier  *notification.MockNotifier
	appCtrl   *controller.Controller
	backend   *SessionMCPBackend
	tmpDir    string
}

func TestVisualizationMCPSuite(t *testing.T) {
	suite.Run(t, new(VisualizationMCPSuite))
}

func (suite *VisualizationMCPSuite) SetupTest() {
	suite.ctrl = gomock.NewController(suite.T())
	suite.mockStore = store.NewMockStore(suite.ctrl)
	suite.notifier = notification.NewMockNotifier(suite.ctrl)
	suite.tmpDir = suite.T().TempDir()

	fs := filestore.NewFileSystemStorage(suite.tmpDir, "http://localhost", "test-secret")
	cfg := &config.ServerConfig{}
	cfg.Controller.FilePrefixGlobal = "dev"
	suite.appCtrl = &controller.Controller{
		Ctx:     context.Background(),
		Options: controller.Options{Filestore: fs, Config: cfg},
	}
	suite.backend = NewSessionMCPBackend(suite.mockStore, suite.notifier, suite.appCtrl)
}

func (suite *VisualizationMCPSuite) TearDownTest() {
	suite.ctrl.Finish()
}

func (suite *VisualizationMCPSuite) renderCtx() context.Context {
	ctx := context.WithValue(context.Background(), "session_id", "session-viz")
	return context.WithValue(ctx, "user", &types.User{ID: "owner-1"})
}

func (suite *VisualizationMCPSuite) renderReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{Params: mcp.CallToolParams{Arguments: args}}
}

func (suite *VisualizationMCPSuite) TestSuccess() {
	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-viz").
		Return(&types.Session{ID: "session-viz", Owner: "owner-1"}, nil)

	req := suite.renderReq(map[string]any{
		"html":   "<html><head></head><body><h1>Chart</h1></body></html>",
		"title":  "Revenue",
		"height": float64(480),
	})
	result, err := suite.backend.handleHTMLRender(suite.renderCtx(), req)
	suite.NoError(err)
	suite.False(result.IsError)

	text := result.Content[0].(mcp.TextContent).Text
	suite.Contains(text, visualization.ResultMarker)

	// The marker payload parses into a reference with a viz_ id and the clamped height.
	_, jsonPart, found := strings.Cut(text, visualization.ResultMarker+" ")
	suite.True(found)
	jsonPart = strings.SplitN(jsonPart, "\n", 2)[0]
	var ref visualization.Reference
	suite.NoError(json.Unmarshal([]byte(jsonPart), &ref))
	suite.Equal("Revenue", ref.Title)
	suite.Equal(480, ref.Height)
	suite.True(strings.HasPrefix(ref.ID, "viz_"))

	// The stored file is readable and carries the injected bootstrap + content.
	reader, err := suite.appCtrl.FilestoreVisualizationRead(context.Background(), "owner-1", "session-viz", ref.ID)
	suite.NoError(err)
	defer reader.Close()
	stored, err := io.ReadAll(reader)
	suite.NoError(err)
	suite.Contains(string(stored), `<style id="helix-viz-theme">`)
	suite.Contains(string(stored), "<h1>Chart</h1>")
}

func (suite *VisualizationMCPSuite) TestMissingSessionID() {
	result, err := suite.backend.handleHTMLRender(context.Background(), suite.renderReq(map[string]any{"html": "<p>x</p>", "title": "t"}))
	suite.NoError(err)
	suite.True(result.IsError)
	suite.Contains(result.Content[0].(mcp.TextContent).Text, "session_id is required")
}

func (suite *VisualizationMCPSuite) TestMissingHTML() {
	// session_id present via context but html missing → validation error before store access.
	ctx := context.WithValue(context.Background(), "session_id", "session-viz")
	ctx = context.WithValue(ctx, "user", &types.User{ID: "owner-1"})
	result, err := suite.backend.handleHTMLRender(ctx, suite.renderReq(map[string]any{"title": "t"}))
	suite.NoError(err)
	suite.True(result.IsError)
	suite.Contains(result.Content[0].(mcp.TextContent).Text, "html is required")
}

func (suite *VisualizationMCPSuite) TestNotAuthorized() {
	suite.mockStore.EXPECT().
		GetSession(gomock.Any(), "session-viz").
		Return(&types.Session{ID: "session-viz", Owner: "someone-else"}, nil)

	result, err := suite.backend.handleHTMLRender(suite.renderCtx(), suite.renderReq(map[string]any{"html": "<p>x</p>", "title": "t"}))
	suite.NoError(err)
	suite.True(result.IsError)
	suite.Contains(result.Content[0].(mcp.TextContent).Text, "not authorized")
}

func (suite *VisualizationMCPSuite) TestNoControllerDisabled() {
	backend := NewSessionMCPBackend(suite.mockStore, suite.notifier, nil)
	result, err := backend.handleHTMLRender(suite.renderCtx(), suite.renderReq(map[string]any{"html": "<p>x</p>", "title": "t"}))
	suite.NoError(err)
	suite.True(result.IsError)
	suite.Contains(result.Content[0].(mcp.TextContent).Text, "not available")
}
