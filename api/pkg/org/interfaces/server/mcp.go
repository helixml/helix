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

// mcpHandler returns an http.Handler that speaks MCP over the Streamable
// HTTP transport. It is mounted at /workers/{id}/mcp; the worker ID in
// the URL identifies the caller, and the server exposes only the tools
// that worker holds grants for.
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
	// own api_key; tools like hire_worker persist it onto the new
	// Worker so subsequent activations run as the same user. In
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
		// hire_worker can persist it onto a Worker's runtime state
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

// buildMCPServer assembles a fresh *mcp.Server tailored to the worker in
// the request URL. The advertised tools are derived live from the
// Worker's Role.Tools: changing the Role updates every Worker holding
// it. There is no per-Worker grants table — capability is the Role's
// responsibility.
//
// Returning nil causes the SDK to respond 400 Bad Request.
func (s *Server) buildMCPServer(r *http.Request) *mcp.Server {
	workerID := orgchart.WorkerID(r.PathValue("id"))
	if workerID == "" {
		return nil
	}

	ctx := r.Context()
	orgID := OrgIDFromContext(ctx)
	if orgID == "" {
		s.logger.Info("mcp.missing_org_scope", "worker", workerID)
		return nil
	}
	worker, err := s.store.Workers.Get(ctx, orgID, workerID)
	if err != nil {
		s.logger.Info("mcp.unknown_worker", "worker", workerID, "err", err.Error())
		return nil
	}

	role, err := s.store.Roles.Get(ctx, orgID, worker.RoleID())
	if err != nil {
		s.logger.Info("mcp.role_lookup_failed", "worker", workerID, "role", worker.RoleID(), "err", err.Error())
		return nil
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "helix-org",
		Version: "0.1.0",
	}, nil)

	heldTools := make(map[tool.Name]bool, len(role.Tools))
	for _, toolName := range role.Tools {
		heldTools[toolName] = true
		t, err := s.registry.Get(toolName)
		if err != nil {
			// Role lists a tool the server doesn't know about. Skip
			// silently; removing it is the owner's job (update_role).
			s.logger.Info("mcp.unknown_tool_in_role", "worker", workerID, "role", role.ID, "tool", toolName)
			continue
		}
		registerToolForWorker(srv, t, worker, s.logger.With("worker", workerID, "tool", toolName))
	}

	if s.prompts != nil {
		for _, p := range s.prompts.All() {
			if req := p.RequiresTool(); req != "" && !heldTools[req] {
				continue
			}
			registerPromptForWorker(srv, p, s.logger.With("worker", workerID, "prompt", p.Name()))
		}
	}

	return srv
}

// registerToolForWorker binds a single tool onto the per-Worker MCP
// server. The handler closes over the caller so each invocation
// dispatches with the right Invocation without re-querying the store.
// Authorisation is by virtue of the tool appearing in the Worker's
// Role.Tools; there is no grant object to consult at call time.
func registerToolForWorker(srv *mcp.Server, t tool.Tool, caller orgchart.Worker, logger interface {
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

// registerPromptForWorker binds a single prompt onto the per-worker
// MCP server. The handler renders the prompt's template into seed
// messages; the LLM consumes those and drives the conversation,
// usually ending in a tool call (create_role, update_identity, …).
//
// Visibility is decided in buildMCPServer; by the time we get here the
// prompt is already in the worker's allowed set.
func registerPromptForWorker(srv *mcp.Server, p prompts.Prompt, logger interface {
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
