package server

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/types"
)

// HelixOrgMCPBackend exposes the embedded helix-org MCP server through
// the Helix MCP gateway. The gateway sits behind Helix's standard auth
// chain, so by the time we get a request here the user has already
// been authenticated by api_key. Organization and Worker identity come only
// from the stored session named by that key, never from caller-controlled URL
// segments.
type HelixOrgMCPBackend struct {
	apiServer *HelixAPIServer
	botMCP    http.Handler
	scope     *helixOrgScope
	// mcpServer and store serve bot instances, whose tools are the
	// instance profile's tools rather than the bot's.
	mcpServer *helixorgserver.Server
	store     *helixorgstore.Store
}

// NewHelixOrgMCPBackend creates a backend that proxies to the in-process
// helix-org server handler.
func NewHelixOrgMCPBackend(apiServer *HelixAPIServer, orgHandlers *helixOrgHandlers) *HelixOrgMCPBackend {
	return &HelixOrgMCPBackend{
		apiServer: apiServer,
		botMCP:    orgHandlers.botMCP,
		scope:     orgHandlers.scope,
		mcpServer: orgHandlers.mcpServer,
		store:     orgHandlers.store,
	}
}

// ServeHTTP implements MCPBackend. The MCP gateway has already
// authenticated the request; this backend resolves the org and worker from the
// session-scoped key used by that worker's desktop.
func (b *HelixOrgMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	suffix := strings.Trim(mux.Vars(r)["path"], "/")
	if suffix != "" {
		http.Error(w, "helix-org MCP does not accept a suffix path", http.StatusBadRequest)
		return
	}
	if user == nil || user.TokenType != types.TokenTypeAPIKey || user.SessionID == "" {
		http.Error(w, "session-scoped API key required for helix-org MCP access", http.StatusForbidden)
		return
	}
	session, err := b.apiServer.Store.GetSession(r.Context(), user.SessionID)
	if err != nil || session == nil || session.Owner != user.ID || session.OrganizationID == "" || session.Metadata.OrgWorkerID == "" || user.OrganizationID != session.OrganizationID {
		http.Error(w, "session is not authorized for helix-org MCP access", http.StatusForbidden)
		return
	}
	if _, err := b.apiServer.authorizeOrgMember(r.Context(), user, session.OrganizationID); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if err := b.scope.ensureBootstrap(r.Context(), session.OrganizationID); err != nil {
		http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if profile := session.Metadata.BotInstance; profile != nil {
		b.serveInstance(w, r, user, session, *profile)
		return
	}

	workerID := session.Metadata.OrgWorkerID
	// Org, bot, and acting user all come from the authenticated session —
	// never from the URL or a client header — so the audit trail and any
	// hiring-user attribution reflect the real principal. The session id
	// lets org tools route follow-ups back to this session (e.g. a
	// secret-intake submission waking its requesting session).
	ctx := helixorgserver.WithOrgID(r.Context(), session.OrganizationID)
	ctx = helixorgserver.WithBotID(ctx, workerID)
	ctx = runtimehelix.WithUserID(ctx, user.ID)
	ctx = runtimehelix.WithSessionID(ctx, session.ID)

	log.Trace().
		Str("user_id", user.ID).
		Str("worker_id", workerID).
		Str("orig_path", r.URL.Path).
		Msg("helix-org MCP: serving bot MCP in-process")

	b.botMCP.ServeHTTP(w, r.WithContext(ctx))
}

// instanceCaller is the tool.Caller for a bot instance. It acts as its bot,
// so messages it posts and audit entries carry the bot's id.
type instanceCaller struct{ botID, orgID string }

func (c instanceCaller) ID() string             { return c.botID }
func (c instanceCaller) OrganizationID() string { return c.orgID }

// serveInstance serves the org tools a bot instance may call: its profile's
// tools that the bot itself still has, read live on every request. An
// instance with none gets 403 even though its config has no org server,
// because its session key can reach this endpoint directly.
func (b *HelixOrgMCPBackend) serveInstance(w http.ResponseWriter, r *http.Request, user *types.User, session *types.Session, profile types.BotInstanceProfile) {
	bot, err := b.store.Nodes.Get(r.Context(), session.OrganizationID, orgchart.NodeID(session.Metadata.OrgWorkerID))
	if err != nil {
		http.Error(w, "bot not found for instance", http.StatusForbidden)
		return
	}
	tools := make([]tool.Name, 0, len(profile.Tools))
	for _, name := range profile.Tools {
		if slices.Contains(bot.Tools, tool.Name(name)) {
			tools = append(tools, tool.Name(name))
		}
	}
	if len(tools) == 0 {
		http.Error(w, "this bot instance has no helix-org tools", http.StatusForbidden)
		return
	}
	ctx := helixorgserver.WithOrgID(r.Context(), session.OrganizationID)
	ctx = runtimehelix.WithUserID(ctx, user.ID)
	ctx = runtimehelix.WithSessionID(ctx, session.ID)
	if token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); token != "" && token != r.Header.Get("Authorization") {
		ctx = runtimehelix.WithBearerToken(ctx, token)
	}
	b.mcpServer.ServeMCPForCaller(w, r.WithContext(ctx), instanceCaller{botID: string(bot.ID), orgID: session.OrganizationID}, tools)
}
