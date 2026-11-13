package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Authorization for desktop keys outside a project.
//
// A desktop container gets a Helix API key in its environment
// (types.DesktopAgentAPIEnvVars). Code in the container holds it, so it must
// not carry its owner's full authority. Keys minted for work in a project are
// confined to that project by the project-scope checks. Two kinds of key are
// minted outside any project, and both are confined here, by route, to what
// the container itself needs:
//
//   - a sandbox key (sandbox.Controller.ensureSandboxAPIToken): ApiKey.SessionID
//     is the sandbox id;
//   - a session key (services.GetOrCreateSessionAPIKey) for a session with no
//     project, such as a plain chat's desktop: ApiKey.SessionID is the
//     session id.
//
// Both may reach:
//
//   - the LLM proxy (a session key only with its session's own agent, when
//     the request names one, a sandbox key with no agent; neither may name a
//     RAG source or assistant);
//   - its own session plumbing under /api/v1/sessions/{id} (zed-config and
//     the agent startup reports), the user websocket for its id and the
//     desktop bridge's RevDial tunnel for its id;
//   - the desktop and visualization MCP backends, with session_id naming its
//     own id.
//
// A sandbox key may also read its own sandbox and use its files, commands,
// screenshot and terminal (/api/v1/organizations/{key org}/sandboxes/{id});
// it may not update or delete itself, and it never sees another sandbox. The
// session MCP backend is refused to it, because the sandbox is no session.
//
// A session key may also read its own session (GET on /api/v1/sessions/{id}
// and below, which includes its agent credentials), store refreshed agent
// credentials, open the agent sync websocket for its session, and use the
// session and external MCP backends for its session and the Helix tools MCP
// backend for its session's agent. Those backends scope themselves to the
// key's session. Kodit is refused: the session is not in a project, so it
// has no repositories of its own to search (resolveKoditRepoScope gives it
// an empty scope, which the backend refuses). On the
// LLM proxy it may not name a RAG source or assistant (rag_source_id,
// assistant_id), which are authorized by owner only.
//
// A helix-org worker's session key (Metadata.OrgWorkerID) is not confined
// here: the worker manages its organization through the REST API as its
// hiring user, by design.
//
// A project key (project_apikey_handlers.go) is not a desktop key, but it is
// confined here the same way, by route: it is bound to one project and no
// session, and is held outside Helix (a portal, a CI job). It may reach:
//
//   - the LLM proxy, where its usage is attributed to its project (it may
//     not name a RAG source or assistant; an agent it names is authorized
//     by the handler, which confines it to its project's agents);
//   - its own project under /api/v1/projects/{id}, except key management,
//     access grants, moving the project and deleting it;
//   - spec tasks, sessions by id and repositories by id, whose handlers
//     confine it to its project (authorizeUserToProject,
//     enforceKeySessionScope, enforceKeyRepositoryScope);
//   - GET /api/v1/status.
//
// FAIL CLOSED: anything not matched is denied and logged. A session key
// whose session cannot be loaded is confined as if it had no agent.

type desktopKeyKind int

const (
	desktopKeySandbox desktopKeyKind = iota + 1
	desktopKeySession
	projectKey
)

// desktopKey is a project-less key held by code in a desktop container, or
// a project key.
type desktopKey struct {
	kind desktopKeyKind
	// id is the sandbox, session or project the key is bound to.
	id string
	// orgID is the key's organization.
	orgID string
	// parentApp is the agent of a session key's session, "" if none.
	parentApp string
}

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

// isProjectlessSessionKey reports whether the caller holds an ordinary API
// key bound to a session and no project. Restricted key types (embed, bot
// instance, app) have their own confinement and are not matched.
func isProjectlessSessionKey(user *types.User) bool {
	if user == nil || user.TokenType != types.TokenTypeAPIKey {
		return false
	}
	if user.ProjectID != "" || user.SessionID == "" || user.AppID != "" || system.IsSandboxID(user.SessionID) {
		return false
	}
	return user.APIKeyType == types.APIkeytypeAPI || user.APIKeyType == ""
}

// isProjectKey reports whether the caller holds an ordinary API key bound to
// a project and no session or spec task: a user-managed project key.
func isProjectKey(user *types.User) bool {
	if user == nil || user.TokenType != types.TokenTypeAPIKey {
		return false
	}
	if user.ProjectID == "" || user.SessionID != "" || user.SpecTaskID != "" || user.AppID != "" {
		return false
	}
	return user.APIKeyType == types.APIkeytypeAPI || user.APIKeyType == ""
}

// confinedDesktopKey returns the confinement for a caller holding a
// project-less desktop key or a project key, or nil when the caller is not
// confined here.
func (auth *authMiddleware) confinedDesktopKey(ctx context.Context, user *types.User) *desktopKey {
	if isProjectKey(user) {
		return &desktopKey{kind: projectKey, id: user.ProjectID, orgID: user.OrganizationID}
	}
	if sandboxID := sandboxSessionKeyID(user); sandboxID != "" {
		return &desktopKey{kind: desktopKeySandbox, id: sandboxID, orgID: user.OrganizationID}
	}
	if !isProjectlessSessionKey(user) {
		return nil
	}
	key := &desktopKey{kind: desktopKeySession, id: user.SessionID, orgID: user.OrganizationID}
	session, err := auth.store.GetSession(ctx, user.SessionID)
	if err != nil || session == nil {
		log.Warn().Err(err).Str("session_id", user.SessionID).Msg("Session key's session not found, confining it")
		return key
	}
	if session.Metadata.OrgWorkerID != "" {
		return nil
	}
	key.parentApp = session.ParentApp
	return key
}

// desktopKeyRefusal returns the error to answer with when a project-less
// desktop key may not make this request, or nil. Callers holding any other
// credential are unaffected.
func (auth *authMiddleware) desktopKeyRefusal(user *types.User, r *http.Request) error {
	key := auth.confinedDesktopKey(r.Context(), user)
	if key == nil || key.allows(r) {
		return nil
	}
	log.Warn().
		Str("key_session_id", key.id).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("Desktop key denied")
	switch key.kind {
	case desktopKeySandbox:
		return ErrSandboxKeyNotAllowed
	case projectKey:
		return ErrProjectKeyNotAllowed
	}
	return ErrSessionKeyNotAllowed
}

// desktopKeyLLMPaths are the LLM proxy routes an agent in the desktop calls.
var desktopKeyLLMPaths = map[string]string{
	"/v1/chat/completions": http.MethodPost,
	"/v1/responses":        http.MethodPost,
	"/v1/messages":         http.MethodPost,
	"/v1/embeddings":       http.MethodPost,
	"/v1/models":           http.MethodGet,
	"/api/v1/v1/models":    http.MethodGet,
}

// desktopKeyMCPBackends are the MCP gateway backends that act only on the
// session named by the request's session_id, per key kind.
var desktopKeyMCPBackends = map[desktopKeyKind]map[string]bool{
	desktopKeySandbox: {
		"desktop":                     true,
		types.MCPBackendVisualization: true,
	},
	desktopKeySession: {
		"desktop":                     true,
		types.MCPBackendVisualization: true,
		// Its tools read sessions through authorizeUserToSession, which
		// confines the key to its own session.
		"session": true,
		// Kodit is not listed: a session outside any project has no
		// repositories of its own (resolveKoditRepoScope gives it an empty
		// scope, which KoditMCPBackend refuses).
	},
}

// sessionKeyOwnSessionWrites are the writes a session key may make under
// /api/v1/sessions/{id}: the agent plumbing the settings-sync daemon calls.
// Everything else on the session (chat, lifecycle, sharing) stays with
// people.
var sessionKeyOwnSessionWrites = append([]struct{ method, suffix string }{
	{http.MethodPut, "/claude-credentials"},
	{http.MethodPut, "/codex-credentials"},
}, botInstanceSessionPaths...)

// sandboxKeyOwnSandboxRoutes are the sub-routes of
// /organizations/{org}/sandboxes/{id} a sandbox may call on itself, by method.
// The sandbox row itself is read-only to its own key: PATCH and DELETE stay
// with people.
var sandboxKeyOwnSandboxRoutes = []struct {
	prefix  string
	methods []string
}{
	{"", []string{http.MethodGet}},
	{"/files", []string{http.MethodGet, http.MethodPut, http.MethodDelete}},
	{"/commands", []string{http.MethodGet, http.MethodPost}},
	{"/screenshot", []string{http.MethodGet}},
	{"/terminal", []string{http.MethodGet}},
}

func (key *desktopKey) allows(r *http.Request) bool {
	path := strings.TrimSuffix(r.URL.Path, "/")
	query := r.URL.Query()

	if method, ok := desktopKeyLLMPaths[path]; ok {
		if r.Method != method && r.Method != http.MethodOptions {
			return false
		}
		// A RAG source or assistant is authorized by owner only
		// (controller.ChatCompletion): a desktop key may not name one.
		if query.Has("rag_source_id") || query.Has("assistant_id") {
			return false
		}
		if key.kind == projectKey {
			return true
		}
		// An agent brings its owner's knowledge, RAG and API tools. The
		// handlers pin a session key's calls to its session's agent when
		// it has one; without one the key may not pick another. A sandbox
		// has no agent, so its key may name none.
		appID := query.Get("app_id")
		if key.kind == desktopKeySession {
			return appID == "" || appID == key.parentApp
		}
		return appID == ""
	}

	if key.kind == projectKey {
		return key.projectRouteAllows(path, r.Method)
	}

	if key.kind == desktopKeySandbox && key.orgID != "" {
		if rest, ok := strings.CutPrefix(path, "/api/v1/organizations/"+key.orgID+"/sandboxes/"+key.id); ok {
			return ownSandboxRouteAllows(rest, r.Method)
		}
	}
	if rest, ok := strings.CutPrefix(path, "/api/v1/sessions/"+key.id); ok {
		return key.ownSessionRouteAllows(rest, r.Method)
	}

	switch path {
	case "/api/v1/ws/user":
		return r.Method == http.MethodGet && query.Get("session_id") == key.id
	case "/api/v1/external-agents/sync":
		return key.kind == desktopKeySession && r.Method == http.MethodGet && query.Get("session_id") == key.id
	case "/api/v1/revdial":
		if runnerID := query.Get("runnerid"); runnerID != "" {
			return runnerID == "desktop-"+key.id
		}
		// A data connection answers a dial the API issued to this desktop's
		// control connection; its dialer id is an unguessable token.
		return query.Get("revdial.dialer") != ""
	}

	if server, ok := strings.CutPrefix(path, "/api/v1/mcp/"); ok {
		return key.mcpAllows(server, query.Get("session_id"), query.Get("app_id"))
	}
	return false
}

// ownSessionRouteAllows applies to "/api/v1/sessions/{id}<rest>" with the
// key's own id. rest is "" or starts with "/"; anything else is another id
// sharing the prefix.
func (key *desktopKey) ownSessionRouteAllows(rest, method string) bool {
	if rest != "" && !strings.HasPrefix(rest, "/") {
		return false
	}
	writes := botInstanceSessionPaths
	if key.kind == desktopKeySession {
		// The session's own record and content are its desktop's to read.
		if method == http.MethodGet || method == http.MethodHead {
			return true
		}
		writes = sessionKeyOwnSessionWrites
	}
	for _, route := range writes {
		if rest == route.suffix && method == route.method {
			return true
		}
	}
	return false
}

func (key *desktopKey) mcpAllows(server, sessionID, appID string) bool {
	backend, _, _ := strings.Cut(server, "/")
	if desktopKeyMCPBackends[key.kind][backend] {
		return sessionID == key.id
	}
	if key.kind != desktopKeySession {
		return false
	}
	switch backend {
	case "external":
		// The proxy serves the MCP servers of the key's own session
		// (ExternalMCPBackend uses the key's SessionID, never the URL).
		return sessionID == "" || sessionID == key.id
	case "helix":
		// The Helix tools of the session's own agent.
		return key.parentApp != "" && appID == key.parentApp && (sessionID == "" || sessionID == key.id)
	}
	return false
}

func ownSandboxRouteAllows(rest, method string) bool {
	for _, route := range sandboxKeyOwnSandboxRoutes {
		if route.prefix == "" {
			if rest != "" {
				continue
			}
		} else if rest != route.prefix && !strings.HasPrefix(rest, route.prefix+"/") {
			continue
		}
		for _, m := range route.methods {
			if method == m {
				return true
			}
		}
		return false
	}
	return false
}

// projectKeyRefusedProjectRoutes are the sub-routes of /projects/{id} a
// project key may not call on its own project: managing keys and access, and
// moving the project to another organization, belong to a person.
var projectKeyRefusedProjectRoutes = []string{"/api_keys", "/access-grants", "/move"}

// projectRouteAllows applies to a project key on any route but the LLM
// proxy.
func (key *desktopKey) projectRouteAllows(path, method string) bool {
	if rest, ok := strings.CutPrefix(path, "/api/v1/projects/"+key.id); ok {
		if rest == "" {
			return method != http.MethodDelete
		}
		if !strings.HasPrefix(rest, "/") {
			return false
		}
		for _, refused := range projectKeyRefusedProjectRoutes {
			if rest == refused || strings.HasPrefix(rest, refused+"/") {
				return false
			}
		}
		return true
	}
	if path == "/api/v1/status" {
		return method == http.MethodGet
	}
	if path == "/api/v1/spec-tasks" || strings.HasPrefix(path, "/api/v1/spec-tasks/") {
		return true
	}
	for _, prefix := range []string{"/api/v1/sessions/", "/api/v1/git/repositories/"} {
		if id, ok := strings.CutPrefix(path, prefix); ok {
			id, _, _ = strings.Cut(id, "/")
			// Ids only: /sessions/chat and /git/repositories/from-template
			// create objects outside the project.
			return id != "" && id != "chat" && id != "from-template"
		}
	}
	return false
}
