package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// botCaller adapts an orgchart.Bot's identity to the tool.Caller
// interface. The Bot aggregate is a plain struct (no ID()/OrganizationID()
// methods), so this tiny value carries the two fields a tool invocation
// needs to attribute the caller.
type botCaller struct{ id, orgID string }

func (c botCaller) ID() string             { return c.id }
func (c botCaller) OrganizationID() string { return c.orgID }

// mcpHandler returns an http.Handler that speaks MCP over the Streamable
// HTTP transport. It is mounted at /workers/{id}/mcp; the bot ID in
// the URL identifies the caller, and the server exposes only the tools
// listed in that bot's Tools.
//
// Stateless mode is used: each request stands on its own. The server has
// no need to push notifications to clients, so session state buys us
// nothing here and adds an obligation to track session IDs.
func (s *Server) mcpHandler() http.Handler {
	inner := mcp.NewStreamableHTTPHandler(s.buildMCPServer, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		Logger:                     s.logger,
		DisableLocalhostProtection: true, // helix-org is reverse-proxied through tunnels (cloudflared) when Helix's runner is on a different host; the SDK's DNS-rebinding guard rejects non-loopback Host headers, which kills the tunnel path.
	})
	// Hoist the HTTP request's bearer onto the request context so
	// tools (and anything they call into via the in-proc Helix
	// adapter) can use runtimehelix.BearerFromContext to discover
	// the caller's identity. In the embedded SaaS this is the picking user's
	// own api_key; tools like create_bot persist it onto the new
	// Bot so subsequent activations run as the same user. In
	// standalone helix-org the request carries no Authorization
	// header and this is a no-op.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if auth := r.Header.Get("Authorization"); auth != "" {
			if token := strings.TrimPrefix(auth, "Bearer "); token != auth && token != "" {
				ctx = runtimehelix.WithBearerToken(ctx, token)
			}
		}
		// Embedding hosts (e.g. the SaaS alpha) forward the calling
		// user's stable identifier in this header so tools like
		// create_bot can persist it onto a Bot's runtime state
		// — letting the Spawner mint a fresh per-user api_key at
		// activation time instead of stashing a token at rest.
		if uid := strings.TrimSpace(r.Header.Get("X-Helix-Org-User-Id")); uid != "" {
			ctx = runtimehelix.WithUserID(ctx, uid)
		}
		if ctx != r.Context() {
			r = r.WithContext(ctx)
		}
		inner.ServeHTTP(w, r)
	})
}

// buildMCPServer assembles a fresh *mcp.Server tailored to the bot in
// the request URL. The advertised tools are derived live from the Bot's
// Tools: editing a Bot's Tools changes its capability on the next MCP
// request. There is no separate role record — the Bot IS its own job
// description, and Bot.Tools is the whole story.
//
// Returning nil causes the SDK to respond 400 Bad Request.
func (s *Server) buildMCPServer(r *http.Request) *mcp.Server {
	botID := orgchart.BotID(r.PathValue("id"))
	if botID == "" {
		return nil
	}

	ctx := r.Context()
	orgID := OrgIDFromContext(ctx)
	if orgID == "" {
		s.logger.Info("mcp.missing_org_scope", "bot", botID)
		return nil
	}
	bot, err := s.queries.GetBot(ctx, orgID, botID)
	if err != nil {
		s.logger.Info("mcp.unknown_bot", "bot", botID, "err", err.Error())
		return nil
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "helix-org",
		Version: "0.1.0",
	}, nil)

	caller := botCaller{id: string(bot.ID), orgID: bot.OrganizationID}
	botTools := make(map[tool.Name]bool, len(bot.Tools))
	for _, toolName := range bot.Tools {
		botTools[toolName] = true
		t, err := s.registry.Get(toolName)
		if err != nil {
			// Bot lists a tool the server doesn't know about. Skip
			// silently; removing it is the owner's job (PATCH /bots/{id}).
			s.logger.Info("mcp.unknown_tool_on_bot", "bot", botID, "tool", toolName)
			continue
		}
		registerToolForBot(srv, t, caller, s.logger.With("bot", botID, "tool", toolName))
	}

	if s.prompts != nil {
		for _, p := range s.prompts.All() {
			if req := p.RequiresTool(); req != "" && !botTools[req] {
				continue
			}
			registerPromptForBot(srv, p, s.logger.With("bot", botID, "prompt", p.Name()))
		}
	}

	return srv
}

// registerToolForBot binds a single tool onto the per-Bot MCP server.
// The handler closes over the caller so each invocation dispatches with
// the right Invocation without re-querying the store. Authorisation is
// by virtue of the tool appearing in the Bot's Tools; there is no
// separate tool record to consult at call time.
func registerToolForBot(srv *mcp.Server, t tool.Tool, caller tool.Caller, logger interface {
	Info(msg string, args ...any)
}) {
	srv.AddTool(&mcp.Tool{
		Name:        string(t.Name()),
		Description: t.Description(),
		InputSchema: t.InputSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		result, err := t.Invoke(ctx, tool.Invocation{
			Caller: caller,
			Args:   args,
		})
		if err != nil {
			logger.Info("mcp.tool_error", "err", err.Error())
			out := &mcp.CallToolResult{}
			out.SetError(err)
			return out, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
		}, nil
	})
}

// registerPromptForBot binds a single prompt onto the per-bot MCP
// server. The handler renders the prompt's template into seed messages;
// the LLM consumes those and drives the conversation, usually ending in
// a tool call (create_bot, create_topic, …).
//
// Visibility is decided in buildMCPServer; by the time we get here the
// prompt is already in the bot's allowed set.
func registerPromptForBot(srv *mcp.Server, p prompts.Prompt, logger interface {
	Info(msg string, args ...any)
}) {
	args := p.Arguments()
	mcpArgs := make([]*mcp.PromptArgument, 0, len(args))
	for _, a := range args {
		mcpArgs = append(mcpArgs, &mcp.PromptArgument{
			Name:        a.Name,
			Title:       a.Title,
			Description: a.Description,
			Required:    a.Required,
		})
	}
	srv.AddPrompt(&mcp.Prompt{
		Name:        string(p.Name()),
		Title:       p.Title(),
		Description: p.Description(),
		Arguments:   mcpArgs,
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		messages, err := p.Render(ctx, req.Params.Arguments)
		if err != nil {
			logger.Info("mcp.prompt_error", "err", err.Error())
			return nil, err
		}
		out := make([]*mcp.PromptMessage, 0, len(messages))
		for _, m := range messages {
			out = append(out, &mcp.PromptMessage{
				Role:    mcp.Role(m.Role),
				Content: &mcp.TextContent{Text: m.Text},
			})
		}
		return &mcp.GetPromptResult{
			Description: p.Description(),
			Messages:    out,
		}, nil
	})
}
