package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix-org/domain"
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
	return mcp.NewStreamableHTTPHandler(s.buildMCPServer, &mcp.StreamableHTTPOptions{
		Stateless: true,
		Logger:    s.logger,
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

	for _, g := range grants {
		tool, err := s.registry.Get(g.ToolName)
		if err != nil {
			// A grant pointing at a tool we don't know about. Skip silently;
			// removing the grant is the owner's job.
			s.logger.Info("mcp.unknown_tool_grant", "worker", workerID, "tool", g.ToolName)
			continue
		}
		registerToolForWorker(srv, tool, worker, g, s.logger.With("worker", workerID, "tool", g.ToolName))
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
