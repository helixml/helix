package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/prompts"
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
	// tools (and anything they call into via helixclient) can use
	// helixclient.BearerFromContext to discover the caller's
	// identity. In the embedded SaaS this is the picking user's
	// own api_key; tools like hire_worker persist it onto the new
	// Worker so subsequent activations run as the same user. In
	// standalone helix-org the request carries no Authorization
	// header and this is a no-op.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if auth := r.Header.Get("Authorization"); auth != "" {
			if token := strings.TrimPrefix(auth, "Bearer "); token != auth && token != "" {
				ctx = helixclient.WithBearerToken(ctx, token)
			}
		}
		// Embedding hosts (e.g. the SaaS alpha) forward the calling
		// user's stable identifier in this header so tools like
		// hire_worker can persist it onto a Worker's runtime state
		// — letting the Spawner mint a fresh per-user api_key at
		// activation time instead of stashing a token at rest.
		if uid := strings.TrimSpace(r.Header.Get("X-Helix-Org-User-Id")); uid != "" {
			ctx = helixclient.WithUserID(ctx, uid)
		}
		if ctx != r.Context() {
			r = r.WithContext(ctx)
		}
		inner.ServeHTTP(w, r)
	})
}

// buildMCPServer assembles a fresh *mcp.Server tailored to the worker in
// the request URL. Tools are filtered by the worker's grants — the LLM
// only ever sees what the owner authorised — and each tool handler
// closes over the grant so scope and enforcement mode are bound at
// registration time.
//
// Returning nil causes the SDK to respond 400 Bad Request.
func (s *Server) buildMCPServer(r *http.Request) *mcp.Server {
	workerID := domain.WorkerID(r.PathValue("id"))
	if workerID == "" {
		return nil
	}

	ctx := r.Context()
	worker, err := s.store.Workers.Get(ctx, workerID)
	if err != nil {
		s.logger.Info("mcp.unknown_worker", "worker", workerID, "err", err.Error())
		return nil
	}

	grants, err := s.store.Grants.ListByWorker(ctx, workerID)
	if err != nil {
		s.logger.Info("mcp.grants_lookup_failed", "worker", workerID, "err", err.Error())
		return nil
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "helix-org",
		Version: "0.1.0",
	}, nil)

	heldTools := make(map[domain.ToolName]bool, len(grants))
	for _, g := range grants {
		heldTools[g.ToolName] = true
		tool, err := s.registry.Get(g.ToolName)
		if err != nil {
			// A grant pointing at a tool we don't know about. Skip silently;
			// removing the grant is the owner's job.
			s.logger.Info("mcp.unknown_tool_grant", "worker", workerID, "tool", g.ToolName)
			continue
		}
		registerToolForWorker(srv, tool, worker, g, s.logger.With("worker", workerID, "tool", g.ToolName))
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

// registerToolForWorker binds a single granted tool onto the per-worker
// MCP server. The handler closes over caller and grant so each call
// dispatches with the right Invocation without re-querying the store.
// The grant is what authorises the call; there's nothing else on it
// the tool needs at invocation time.
func registerToolForWorker(srv *mcp.Server, tool domain.Tool, caller domain.Worker, _ domain.ToolGrant, logger interface {
	Info(msg string, args ...any)
}) {
	srv.AddTool(&mcp.Tool{
		Name:        string(tool.Name()),
		Description: tool.Description(),
		InputSchema: tool.InputSchema(),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.Params.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		result, err := tool.Invoke(ctx, domain.Invocation{
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
