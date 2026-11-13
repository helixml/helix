package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Route-level confinement for task-confined keys.
//
// A spec task created with confine_agent_key gets session keys confined to
// that task (enforceKeyTaskScope). The authorization helpers
// (enforceKeySessionScope, authorizeUserToSpecTask,
// denyTaskCreationToConfinedKey, denyProjectMutationToConfinedKey) are the
// enforcement; this guard runs before the handlers so that every route
// naming another task or session answers the same 404 as for an unknown id,
// whatever status its handler gives an authorization failure, and so that a
// handler that forgets its check still cannot reach outside the task.
//
// It refuses a task-confined key:
//
//   - any /api/v1/spec-tasks/{id} route, and any spec_task_id query
//     parameter, naming another task (404);
//   - any /api/v1/sessions/{id} or /api/v1/external-agents/{id} route, and
//     any session_id query parameter, naming a session outside its task
//     (404);
//   - clone groups, which span tasks (404);
//   - every task creation route (403);
//   - every helix-org route under /api/v1/orgs/{org}/ and the org MCP
//     (403): org Bots act across the organization on their own authority;
//   - every change to its project other than adding artifacts (403).

var errTaskConfinedKeyNotFound = errors.New("not found")

// taskConfinedKeyCreateRoutes are the routes that create spec tasks, or a
// project with tasks, by method and path. The clone route is matched apart,
// since it carries the source task's id.
var taskConfinedKeyCreateRoutes = map[string]string{
	"/api/v1/spec-tasks/from-prompt":            http.MethodPost,
	"/api/v1/projects":                          http.MethodPost,
	"/api/v1/projects/apply":                    http.MethodPut,
	"/api/v1/projects/quick-create":             http.MethodPost,
	"/api/v1/sample-projects/simple/fork":       http.MethodPost,
	"/api/v1/sample-projects/simple/fork-repos": http.MethodPost,
}

// taskConfinedKeySpecTaskCollectionRoutes are /api/v1/spec-tasks/<segment>
// routes whose segment is not a task id.
var taskConfinedKeySpecTaskCollectionRoutes = map[string]bool{
	"from-prompt":    true,
	"board-settings": true,
}

// taskConfinedKeyRefusal returns the status and error to answer with when a
// task-confined key may not make this request, or 0. Callers holding any
// other credential are unaffected.
func (auth *authMiddleware) taskConfinedKeyRefusal(user *types.User, r *http.Request) (int, error) {
	if user == nil || !user.TaskConfined {
		return 0, nil
	}
	status, err := auth.taskConfinedKeyCheck(user, r)
	if err != nil {
		log.Warn().
			Err(err).
			Str("key_spec_task_id", user.SpecTaskID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("Task-confined key denied")
	}
	return status, err
}

func (auth *authMiddleware) taskConfinedKeyCheck(user *types.User, r *http.Request) (int, error) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	query := r.URL.Query()

	if method, ok := taskConfinedKeyCreateRoutes[path]; ok && r.Method == method {
		return http.StatusForbidden, errTaskConfinedKeyCreate
	}

	if rest, ok := strings.CutPrefix(path, "/api/v1/spec-tasks/"); ok {
		taskID, sub, _ := strings.Cut(rest, "/")
		if !taskConfinedKeySpecTaskCollectionRoutes[taskID] {
			if err := enforceKeyTaskScope(user, taskID); err != nil {
				return http.StatusNotFound, errTaskConfinedKeyNotFound
			}
			if sub == "clone" && r.Method == http.MethodPost {
				return http.StatusForbidden, errTaskConfinedKeyCreate
			}
		}
	}
	if query.Has("spec_task_id") {
		if err := enforceKeyTaskScope(user, query.Get("spec_task_id")); err != nil {
			return http.StatusNotFound, errTaskConfinedKeyNotFound
		}
	}

	if strings.HasPrefix(path, "/api/v1/orgs/") || path == "/api/v1/mcp/helix-org" || strings.HasPrefix(path, "/api/v1/mcp/helix-org/") {
		return http.StatusForbidden, errTaskConfinedKeyOrg
	}

	if strings.HasPrefix(path, "/api/v1/clone-groups/") {
		return http.StatusNotFound, errTaskConfinedKeyNotFound
	}

	for _, prefix := range []string{"/api/v1/sessions/", "/api/v1/external-agents/"} {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok {
			continue
		}
		sessionID, _, _ := strings.Cut(rest, "/")
		if sessionID == "chat" || sessionID == "sync" {
			continue
		}
		if !auth.taskConfinedKeyReachesSession(r, user, sessionID) {
			return http.StatusNotFound, errTaskConfinedKeyNotFound
		}
	}
	if query.Has("session_id") && !auth.taskConfinedKeyReachesSession(r, user, query.Get("session_id")) {
		return http.StatusNotFound, errTaskConfinedKeyNotFound
	}

	if rest, ok := strings.CutPrefix(path, "/api/v1/projects/"); ok && !isSafeMethod(r.Method) {
		_, sub, _ := strings.Cut(rest, "/")
		if !(sub == "artifacts" && r.Method == http.MethodPost) {
			return http.StatusForbidden, errTaskConfinedKeyProject
		}
	}
	return 0, nil
}

// taskConfinedKeyReachesSession reports whether a task-confined key may name
// sessionID. An unknown session is left to the handler, which answers 404.
func (auth *authMiddleware) taskConfinedKeyReachesSession(r *http.Request, user *types.User, sessionID string) bool {
	if sessionID == "" || sessionID == user.SessionID {
		return true
	}
	session, err := auth.store.GetSession(r.Context(), sessionID)
	if errors.Is(err, store.ErrNotFound) {
		return true
	}
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("Failed to load session for task-confined key, refusing")
		return false
	}
	return enforceKeySessionScope(user, session) == nil
}
