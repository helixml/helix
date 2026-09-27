package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/types"
)

// Authorization for bot instance keys.
//
// A bot instance (types.SessionRoleOrgBotInstance) works for untrusted end
// users and reads untrusted pages, so any instruction it follows may come from
// an attacker. Its sandbox key (types.APIkeytypeBotInstance) therefore reaches
// only what the sandbox itself needs, bound to the one session recorded on
// the key:
//
//   - its own session plumbing: Zed config, startup/config reports, the user
//     websocket, the agent sync websocket and the desktop bridge's RevDial;
//   - the LLM proxy;
//   - the MCP servers its instance profile keeps;
//   - read-only git on its project's repositories (enforced by the git server,
//     see GitHTTPServer.botInstanceGitAllows).
//
// FAIL CLOSED. Anything not matched is denied, and every denial is logged so a
// missing entry shows up as a 403 with its path rather than a silent failure.

// botInstanceSessionPaths are "/api/v1/sessions/{id}<suffix>" routes the
// sandbox calls for its own session.
var botInstanceSessionPaths = []struct{ method, suffix string }{
	{http.MethodGet, "/zed-config"},
	{http.MethodPost, "/zed-config/user"},
	{http.MethodPost, "/agent-startup-error"},
	{http.MethodPost, "/agent-config-applied"},
}

// botInstanceLLMPaths are the LLM proxy routes the harness and Zed call.
var botInstanceLLMPaths = map[string]string{
	"/v1/chat/completions": http.MethodPost,
	"/v1/responses":        http.MethodPost,
	"/v1/messages":         http.MethodPost,
	"/v1/models":           http.MethodGet,
}

// botInstanceMCPServers maps an MCP gateway backend to the instance profile
// server it serves. The helix-org backend has no entry: it checks the
// profile's tools itself and refuses an instance without any.
var botInstanceMCPServers = map[string]string{
	"session": types.InstanceMCPServerHelixSession,
	"desktop": types.InstanceMCPServerHelixDesktop,
	"kodit":   types.InstanceMCPServerKodit,
}

// botInstanceKeyAllows reports whether a bot instance key may make this
// request.
func (auth *authMiddleware) botInstanceKeyAllows(ctx context.Context, user *types.User, r *http.Request) bool {
	allowed := auth.botInstanceKeyAllowsPath(ctx, user, r)
	if !allowed {
		log.Warn().
			Str("session_id", user.SessionID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("Bot instance key denied")
	}
	return allowed
}

func (auth *authMiddleware) botInstanceKeyAllowsPath(ctx context.Context, user *types.User, r *http.Request) bool {
	sessionID := user.SessionID
	if sessionID == "" {
		return false
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	query := r.URL.Query()

	if method, ok := botInstanceLLMPaths[path]; ok {
		return r.Method == method || r.Method == http.MethodOptions
	}

	if rest, ok := strings.CutPrefix(path, "/api/v1/sessions/"+sessionID); ok {
		for _, route := range botInstanceSessionPaths {
			if rest == route.suffix && r.Method == route.method {
				return true
			}
		}
		return false
	}

	switch path {
	case "/api/v1/ws/user", "/api/v1/external-agents/sync":
		return r.Method == http.MethodGet && query.Get("session_id") == sessionID
	case "/api/v1/revdial":
		// The control connection registers the desktop bridge under this
		// session. A data connection answers a dial the API itself issued; its
		// dialer id is an unguessable token the API handed to that control
		// connection.
		if runnerID := query.Get("runnerid"); runnerID != "" {
			return runnerID == "desktop-"+sessionID
		}
		return query.Get("revdial.dialer") != ""
	}

	if server, ok := strings.CutPrefix(path, "/api/v1/mcp/"); ok {
		return auth.botInstanceMCPAllows(ctx, sessionID, server, query.Get("session_id"))
	}
	return false
}

// botInstanceMCPAllows lets an instance reach an MCP gateway backend only when
// its instance profile keeps that server.
func (auth *authMiddleware) botInstanceMCPAllows(ctx context.Context, sessionID, server, querySessionID string) bool {
	if querySessionID != "" && querySessionID != sessionID {
		return false
	}
	if server == "helix-org" {
		return true
	}
	session, err := auth.store.GetSession(ctx, sessionID)
	if err != nil || session.Metadata.BotInstance == nil {
		return false
	}
	profile := session.Metadata.BotInstance
	if name, ok := strings.CutPrefix(server, "external/"); ok {
		name, _, _ = strings.Cut(name, "/")
		return profileKeepsServer(profile, name)
	}
	backend, _, _ := strings.Cut(server, "/")
	name, ok := botInstanceMCPServers[backend]
	return ok && profileKeepsServer(profile, name)
}

// profileKeepsServer compares by the sanitized name the Zed config keys
// servers under, so a project MCP named "My CRM" matches "my-crm".
func profileKeepsServer(profile *types.BotInstanceProfile, name string) bool {
	for _, kept := range profile.MCPServers {
		if external_agent.SanitizeMCPName(kept) == external_agent.SanitizeMCPName(name) {
			return true
		}
	}
	return false
}
