package server

import (
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Authorization for sandbox keys.
//
// A desktop sandbox created through the sandboxes API gets a key bound to the
// sandbox (ApiKey.SessionID is the sandbox id; see
// sandbox.Controller.ensureSandboxAPIToken). Code in the container holds it,
// so it must not carry its owner's full authority. A sandbox in a project is
// confined to that project by the project-scope checks. A sandbox outside any
// project is confined here, by route, to what the container itself needs:
//
//   - the LLM proxy;
//   - its own sandbox and session plumbing (/api/v1/sandboxes/{id}/...,
//     /api/v1/sessions/{id}/..., the user websocket for its id);
//   - the desktop bridge's RevDial tunnel for its id;
//   - the session-bound MCP backends (session, desktop, visualization) for its
//     id.
//
// It has no repositories of its own, so git is refused (GitHTTPServer).
// FAIL CLOSED: anything not matched is denied and logged.

// sandboxSessionKeyID returns the sandbox a non-project sandbox key is bound
// to, or "" when the caller holds no such key.
func sandboxSessionKeyID(user *types.User) string {
	if user == nil || user.TokenType != types.TokenTypeAPIKey || user.ProjectID != "" {
		return ""
	}
	if !system.IsSandboxID(user.SessionID) {
		return ""
	}
	return user.SessionID
}

// sandboxKeyLLMPaths are the LLM proxy routes an agent in the sandbox calls.
var sandboxKeyLLMPaths = map[string]string{
	"/v1/chat/completions": http.MethodPost,
	"/v1/responses":        http.MethodPost,
	"/v1/messages":         http.MethodPost,
	"/v1/embeddings":       http.MethodPost,
	"/v1/models":           http.MethodGet,
	"/api/v1/v1/models":    http.MethodGet,
}

// sandboxKeyMCPBackends are the MCP gateway backends that act only on the
// session (here: the sandbox) named by the request.
var sandboxKeyMCPBackends = map[string]bool{
	"session":                     true,
	"desktop":                     true,
	types.MCPBackendVisualization: true,
}

// sandboxKeyAllows reports whether a non-project sandbox key may make this
// request. Callers holding any other credential are unaffected.
func sandboxKeyAllows(user *types.User, r *http.Request) bool {
	sandboxID := sandboxSessionKeyID(user)
	if sandboxID == "" {
		return true
	}
	if sandboxKeyAllowsPath(sandboxID, r) {
		return true
	}
	log.Warn().
		Str("sandbox_id", sandboxID).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("Sandbox key denied")
	return false
}

func sandboxKeyAllowsPath(sandboxID string, r *http.Request) bool {
	path := strings.TrimSuffix(r.URL.Path, "/")
	query := r.URL.Query()

	if method, ok := sandboxKeyLLMPaths[path]; ok {
		return r.Method == method || r.Method == http.MethodOptions
	}

	for _, prefix := range []string{"/api/v1/sandboxes/", "/api/v1/sessions/"} {
		if rest, ok := strings.CutPrefix(path, prefix+sandboxID); ok {
			return rest == "" || strings.HasPrefix(rest, "/")
		}
	}

	switch path {
	case "/api/v1/ws/user":
		return r.Method == http.MethodGet && query.Get("session_id") == sandboxID
	case "/api/v1/revdial":
		if runnerID := query.Get("runnerid"); runnerID != "" {
			return runnerID == "desktop-"+sandboxID
		}
		// A data connection answers a dial the API issued to this sandbox's
		// control connection; its dialer id is an unguessable token.
		return query.Get("revdial.dialer") != ""
	}

	if server, ok := strings.CutPrefix(path, "/api/v1/mcp/"); ok {
		backend, _, _ := strings.Cut(server, "/")
		querySessionID := query.Get("session_id")
		return sandboxKeyMCPBackends[backend] && (querySessionID == "" || querySessionID == sandboxID)
	}
	return false
}
