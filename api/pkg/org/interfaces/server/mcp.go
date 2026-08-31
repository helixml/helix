package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orgruntime "github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
)

// botCaller adapts an orgchart.Node's identity to the tool.Caller
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
	botID := orgchart.NodeID(r.PathValue("id"))
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

	return s.assembleMCPServer(string(botID), botCaller{id: string(bot.ID), orgID: bot.OrganizationID}, bot.Tools)
}

// assembleMCPServer builds the MCP server exposing exactly `tools` to
// `caller`. Split out of buildMCPServer so an embedding host can serve the
// same surface for a caller that is not a Bot (see ServeMCPForCaller) without
// a second copy of the registration, audit, and prompt-gating logic.
func (s *Server) assembleMCPServer(label string, caller tool.Caller, tools []tool.Name) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{
		Name:    "helix-org",
		Version: "0.1.0",
	}, nil)

	granted := make(map[tool.Name]bool, len(tools))
	for _, toolName := range tools {
		granted[toolName] = true
		t, err := s.registry.Get(toolName)
		if err != nil {
			// Caller lists a tool the server doesn't know about. Skip
			// silently; removing it is the owner's job (PATCH /bots/{id}).
			s.logger.Info("mcp.unknown_tool_on_bot", "bot", label, "tool", toolName)
			continue
		}
		s.registerToolForBot(srv, t, caller, s.logger.With("bot", label, "tool", toolName))
	}

	if s.prompts != nil {
		for _, p := range s.prompts.All() {
			if req := p.RequiresTool(); req != "" && !granted[req] {
				continue
			}
			registerPromptForBot(srv, p, s.logger.With("bot", label, "prompt", p.Name()))
		}
	}

	return srv
}

// ServeMCPForCaller serves one MCP request for a caller the org graph knows
// nothing about — today, a spec task's coding agent authenticated by its
// session-scoped api key. The embedding host has already authenticated the
// request and resolved the tool list; this is the same transport, registry,
// and audit path Bots use, only with the identity supplied instead of loaded.
func (s *Server) ServeMCPForCaller(w http.ResponseWriter, r *http.Request, caller tool.Caller, tools []tool.Name) {
	mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.assembleMCPServer(caller.ID(), caller, tools)
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		Logger:                     s.logger,
		DisableLocalhostProtection: true,
	}).ServeHTTP(w, r)
}

// registerToolForBot binds a single tool onto the per-Bot MCP server.
// The handler closes over the caller so each invocation dispatches with
// the right Invocation without re-querying the store. Authorisation is
// by virtue of the tool appearing in the Bot's Tools; there is no
// separate tool record to consult at call time.
func (s *Server) registerToolForBot(srv *mcp.Server, t tool.Tool, caller tool.Caller, logger interface {
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
		auditEntry := s.newMCPAuditEntry(ctx, caller, string(t.Name()), args)
		started := time.Now()
		result, err := t.Invoke(ctx, tool.Invocation{
			Caller: caller,
			Args:   args,
		})
		if err != nil {
			auditEntry.Status = orgaudit.StatusFailed
			auditEntry.Metadata.Error = err.Error()
			auditEntry.Metadata.DurationMS = time.Since(started).Milliseconds()
			s.recordAudit(ctx, auditEntry)
			logger.Info("mcp.tool_error", "err", err.Error())
			out := &mcp.CallToolResult{}
			out.SetError(err)
			return out, nil
		}
		auditEntry.Status = orgaudit.StatusSucceeded
		auditEntry.Metadata.DurationMS = time.Since(started).Milliseconds()
		s.recordAudit(ctx, auditEntry)
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(result)}},
		}, nil
	})
}

// actorTypeFor labels an audit entry by the kind of caller. Anything that does
// not declare itself is a Bot — recording a spec task as one would make the org
// audit log lie about who acted.
func actorTypeFor(caller tool.Caller) orgaudit.ActorType {
	if self, ok := caller.(orgaudit.SelfDescribingActor); ok {
		return self.AuditActorType()
	}
	return orgaudit.ActorBot
}

type mcpAuditArgs struct {
	ProjectID string `json:"project_id"`
	Asset     string `json:"asset"`
	AssetID   string `json:"asset_id"`
}

func auditActorID(caller tool.Caller) string {
	if self, ok := caller.(orgaudit.SelfDescribingActorID); ok {
		if id := self.AuditActorID(); id != "" {
			return id
		}
	}
	return caller.ID()
}

func (s *Server) newMCPAuditEntry(ctx context.Context, caller tool.Caller, action string, args json.RawMessage) orgaudit.Entry {
	entry := orgaudit.Entry{
		OrganizationID: caller.OrganizationID(),
		UserID:         runtimehelix.UserIDFromContext(ctx),
		ActorID:        auditActorID(caller),
		ActorType:      actorTypeFor(caller),
		EventType:      orgaudit.EventMCPCall,
		Action:         action,
		Status:         orgaudit.StatusAttempted,
		Metadata: orgaudit.Metadata{
			Arguments: orgaudit.RedactArguments(args),
		},
	}
	var parsed mcpAuditArgs
	if err := json.Unmarshal(args, &parsed); err == nil {
		entry.ProjectID = parsed.ProjectID
		entry.Metadata.AssetRef = parsed.Asset
		if entry.Metadata.AssetRef == "" {
			entry.Metadata.AssetRef = parsed.AssetID
		}
	}
	if entry.ProjectID == "" {
		if p, ok := orgruntime.ProjectPrincipalFromContext(ctx); ok {
			entry.ProjectID = p.ProjectID
		}
	}
	if entry.ProjectID == "" && s.projects != nil {
		projectID, err := s.projects(ctx, entry.OrganizationID, entry.ActorID)
		if err != nil {
			s.logger.Info("mcp.audit_project_error", "bot", entry.ActorID, "err", err.Error())
		} else {
			entry.ProjectID = projectID
		}
	}
	if entry.Metadata.AssetRef != "" && s.assets != nil {
		value, err := s.assets.Get(ctx, entry.OrganizationID, asset.ID(entry.Metadata.AssetRef))
		if err != nil {
			value, err = s.assets.GetByName(ctx, entry.OrganizationID, entry.Metadata.AssetRef)
		}
		if err == nil {
			entry.AssetID = string(value.ID)
		}
	}
	return entry
}

func (s *Server) recordAudit(ctx context.Context, entry orgaudit.Entry) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Record(ctx, entry); err != nil {
		s.logger.Error("mcp.audit_write_error", "event_type", entry.EventType, "action", entry.Action, "err", err.Error())
	}
}

// registerPromptForBot binds a single prompt onto the per-bot MCP
// server. The handler renders the prompt's template into seed messages;
// the LLM consumes those and drives the conversation, usually ending in
// a tool call (create_bot, create_trigger, …).
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

// ToolInfo is the name + description pair the tool-picker UI renders.
type ToolInfo struct {
	Name        string
	Description string
}

// ToolCatalogue returns the registered tools matching names, in the order
// given. Unknown names are skipped so a catalogue constant can name a tool a
// given deployment did not register.
func (s *Server) ToolCatalogue(names []tool.Name) []ToolInfo {
	out := make([]ToolInfo, 0, len(names))
	for _, name := range names {
		t, err := s.registry.Get(name)
		if err != nil {
			continue
		}
		out = append(out, ToolInfo{Name: string(t.Name()), Description: t.Description()})
	}
	return out
}
