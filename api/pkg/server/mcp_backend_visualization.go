package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/visualization"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// VisualizationMCPBackend serves the html_render tool: an agent publishes a
// self-contained HTML page that renders inline in its own chat. It is its own
// backend (wired as the helix-viz context server) rather than a tool on
// helix-session, so every agent — including minimal org bot instances — can
// visualize without also being granted session navigation.
type VisualizationMCPBackend struct {
	store      store.Store
	controller *controller.Controller
	httpServer *server.StreamableHTTPServer
}

func NewVisualizationMCPBackend(s store.Store, ctrl *controller.Controller) *VisualizationMCPBackend {
	backend := &VisualizationMCPBackend{store: s, controller: ctrl}

	mcpServer := server.NewMCPServer(
		"Helix Visualization",
		"1.0.0",
		server.WithResourceCapabilities(false, false),
		server.WithLogging(),
	)
	mcpServer.AddTool(mcp.NewTool(visualization.ToolName,
		mcp.WithDescription("Show a finished HTML page (chart, table, diagram, collage, mockup) inline in this thread, above your final text reply; call it before writing that reply. The reader already sees the page, so the reply should not announce it, say where it is, or restate it: add only what the page doesn't say. Write one self-contained document with inline <style> and <script>; remote http(s) URLs such as a CDN chart library load as-is. The frame fits the page's height automatically. "+visualization.LayoutGuide+" "+visualization.ThemeGuide),
		mcp.WithString("html",
			mcp.Required(),
			mcp.Description("A complete, self-contained HTML document."),
		),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Short name for the page."),
		),
		mcp.WithNumber("height",
			mcp.Description(fmt.Sprintf("Optional frame height in CSS pixels, %d-%d. The frame auto-fits the page, so this is only an initial hint; omit it to let the page set its own height.", visualization.MinHeight, visualization.MaxHeight)),
		),
	), backend.handleHTMLRender)

	backend.httpServer = server.NewStreamableHTTPServer(mcpServer, server.WithStateLess(true))
	return backend
}

// ServeHTTP implements MCPBackend. The session is the one named by the
// session_id query parameter; for a bot instance key the auth middleware has
// already bound that to the key's own session.
func (b *VisualizationMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	ctx := context.WithValue(r.Context(), "user", user)
	ctx = context.WithValue(ctx, "session_id", r.URL.Query().Get("session_id"))
	b.httpServer.ServeHTTP(w, r.WithContext(ctx))
}

func (b *VisualizationMCPBackend) handleHTMLRender(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, _ := ctx.Value("session_id").(string)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	html, err := request.RequireString("html")
	if err != nil || strings.TrimSpace(html) == "" {
		return mcp.NewToolResultError("html is required"), nil
	}
	if len(html) > visualization.MaxHTMLBytes {
		return mcp.NewToolResultError(fmt.Sprintf("html is %d bytes; the limit is %d", len(html), visualization.MaxHTMLBytes)), nil
	}
	title := visualization.CleanTitle(request.GetString("title", ""))
	// 0 lets the page set its own height; the client fits the frame either way.
	height := 0
	if raw := request.GetFloat("height", 0); raw > 0 {
		height = visualization.ClampHeight(int(raw))
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	user, ok := ctx.Value("user").(*types.User)
	if !ok || user == nil || session.Owner != user.ID {
		return mcp.NewToolResultError("not authorized to publish to this session"), nil
	}

	vizID := "viz_" + system.GenerateID()
	prepared := visualization.InjectBootstrap(html)
	if _, err := b.controller.FilestoreVisualizationWrite(ctx, session.Owner, sessionID, vizID, strings.NewReader(prepared)); err != nil {
		return mcp.NewToolResultError("failed to store visualization: " + err.Error()), nil
	}

	payload, err := json.Marshal(visualization.Reference{ID: vizID, Title: title, Height: height})
	if err != nil {
		return mcp.NewToolResultError("failed to encode visualization reference: " + err.Error()), nil
	}
	// The marker lets the frontend find the reference however a harness wraps
	// tool output.
	return mcp.NewToolResultText(fmt.Sprintf(
		"%s %s\nShown to the reader above your reply. Don't mention or describe the page; reply with only what it doesn't already say.",
		visualization.ResultMarker, payload,
	)), nil
}
