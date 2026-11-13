package server

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
)

// Repository writes by agent-bound keys over REST.
//
// authorizeUserToRepository checks the key owner's rights and that the
// repository is attached to the key's project. A repository can be attached to
// several projects, so that check alone lets an agent in project A write the
// branches project B checks out and runs. Git pushes are confined by the
// pre-receive hook to services.GitHTTPServer.AgentPushBranches; the REST routes
// that write files or refs apply the same list here:
//
//   - PUT /contents writes Branch;
//   - POST /branches creates BranchName;
//   - POST /pull-requests pushes SourceBranch upstream;
//   - PATCH /pulls/{n} edits a PR whose head the agent owns.
//
// On helix-specs a REST write is narrower than a push: the agent may write
// only its own task's design directory. An agent-bound key without a spec task
// (sandbox, exploratory desktop, project, app, embed, bot keys) has no list and
// writes nothing through REST.
//
// Repository administration (settings, delete, sync with the remote, kodit
// rescan, merging, access grants) is refused to every agent-bound key by
// agentKeyRepositoryRefusal, in the auth middleware.

var (
	errAgentKeyRepositoryAdmin = errors.New("agent keys may not administer, sync or merge repositories")
	errAgentKeyNoSpecTask      = errors.New("this key has no spec task, so it may not write repositories through the API")
)

// holdsAgentKey reports whether the caller authenticated with an agent-bound
// API key. Helix's own in-process callers carry no token type and are not
// matched.
func holdsAgentKey(user *types.User) bool {
	return user != nil && user.TokenType == types.TokenTypeAPIKey && !user.IsHumanCredential()
}

// authorizeAgentRepositoryWrite refuses an agent-bound key a write to branch of
// repoID (and, for a file write, filePath on it) outside the branches its spec
// task may push. Any other caller is unaffected. The returned error carries
// its HTTP status (httpStatusError).
func (apiServer *HelixAPIServer) authorizeAgentRepositoryWrite(ctx context.Context, user *types.User, repoID, branch, filePath string) error {
	if !holdsAgentKey(user) {
		return nil
	}
	refuse := func(reason string) error {
		log.Warn().
			Str("user_id", user.ID).
			Str("key_spec_task_id", user.SpecTaskID).
			Str("key_project_id", user.ProjectID).
			Str("repo_id", repoID).
			Str("branch", branch).
			Str("path", filePath).
			Str("reason", reason).
			Msg("Agent key repository write refused")
		return statusErr(http.StatusForbidden, "%s", reason)
	}
	if user.SpecTaskID == "" {
		return refuse(errAgentKeyNoSpecTask.Error())
	}
	allowed, err := apiServer.gitHTTPServer.AgentPushBranches(ctx, user.SpecTaskID, repoID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", user.SpecTaskID).Str("repo_id", repoID).Msg("Failed to resolve agent push branches")
		if errors.Is(err, services.ErrAgentSpecTaskNotFound) {
			return refuse(services.AgentPushBranchesMessage(err))
		}
		return statusErr(http.StatusServiceUnavailable, "%s", services.AgentPushBranchesMessage(err))
	}
	if !slices.Contains(allowed, branch) {
		return refuse("this key may write only the branches: " + strings.Join(allowed, ", "))
	}
	if branch != services.SpecsBranchName || filePath == "" {
		return nil
	}
	task, err := apiServer.Store.GetSpecTask(ctx, user.SpecTaskID)
	if err != nil {
		return refuse(services.AgentPushBranchesMessage(services.ErrAgentSpecTaskNotFound))
	}
	if task.DesignDocPath == "" {
		return refuse("this key's task has no design directory on " + services.SpecsBranchName)
	}
	dir := "design/tasks/" + task.DesignDocPath + "/"
	if !strings.HasPrefix(filePath, dir) {
		return refuse("on " + services.SpecsBranchName + " this key may write only under " + dir)
	}
	return nil
}

// writeHTTPStatusError answers with the status an httpStatusError carries, or
// 500.
func writeHTTPStatusError(w http.ResponseWriter, err error) {
	var se *httpStatusError
	if errors.As(err, &se) {
		http.Error(w, se.message, se.status)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// agentKeyRepositoryRefusal applies agentKeyRepositoryRouteAllows to an
// agent-bound key's request for /api/v1/git/repositories/{id}/..., or returns
// nil.
func agentKeyRepositoryRefusal(user *types.User, r *http.Request) error {
	if !holdsAgentKey(user) {
		return nil
	}
	rest, ok := strings.CutPrefix(strings.TrimSuffix(r.URL.Path, "/"), "/api/v1/git/repositories/")
	if !ok {
		return nil
	}
	id, sub, _ := strings.Cut(rest, "/")
	if id == "" || id == "from-template" || agentKeyRepositoryRouteAllows(sub, r.Method) {
		return nil
	}
	log.Warn().
		Str("user_id", user.ID).
		Str("key_spec_task_id", user.SpecTaskID).
		Str("key_project_id", user.ProjectID).
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("Agent key repository administration refused")
	return errAgentKeyRepositoryAdmin
}

// agentKeyRepositoryRouteAllows applies to "/git/repositories/{id}/<sub>". An
// agent-bound key gets reads, file writes and the pull-request workflow; the
// handlers confine the writes to its branches (authorizeAgentRepositoryWrite).
// Changing, deleting, re-indexing or syncing the repository with its remote,
// merging and managing access stay with people.
func agentKeyRepositoryRouteAllows(sub, method string) bool {
	if sub == "access-grants" || strings.HasPrefix(sub, "access-grants/") {
		return false
	}
	if method == http.MethodGet || method == http.MethodHead {
		return true
	}
	parts := strings.Split(sub, "/")
	switch method {
	case http.MethodPut:
		return sub == "contents"
	case http.MethodPost:
		if sub == "branches" || sub == "pull-requests" {
			return true
		}
		return len(parts) == 3 && parts[0] == "pulls" && parts[2] == "request-rebase"
	case http.MethodPatch:
		return len(parts) == 2 && parts[0] == "pulls"
	}
	return false
}
