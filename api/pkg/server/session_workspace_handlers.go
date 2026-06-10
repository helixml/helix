package server

// Workspace status + pre-fork commit helpers.
//
// When a user forks a session, the child boots a fresh container that
// clones the project repos at HEAD of the configured branch. ANY
// uncommitted edits or unpushed commits in the parent's container are
// invisible to the child — they live only on the parent's filesystem.
// Without intervention, a fork silently abandons the agent's in-flight
// work. The flow here is the safety net:
//
//   1. Frontend opens the fork-confirm modal → calls workspaceStatus
//      to find out which repos in the parent's container have
//      uncommitted/unpushed changes.
//   2. If anything's dirty, modal renders a "will commit & push N file
//      changes" panel; if everything's clean, modal proceeds normally.
//   3. On confirm, fork handler calls commitAndPushUncommittedRepos
//      BEFORE pausing the parent. If any push fails, the fork aborts
//      and the parent stays live so the user can fix git and retry.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// WorkspaceRepoStatus describes one repo's git state inside the
// session's desktop container. Returned as part of WorkspaceStatusResponse.
type WorkspaceRepoStatus struct {
	RepoID           string `json:"repo_id"`
	Name             string `json:"name"`
	Branch           string `json:"branch,omitempty"`
	UncommittedFiles int    `json:"uncommitted_files"`
	UnpushedCommits  int    `json:"unpushed_commits"`
	// Error is set when we couldn't determine the status (container
	// unreachable, path missing, git command failed). The repo is then
	// excluded from "dirty" totals — we don't refuse a fork because we
	// can't see one repo's state.
	Error string `json:"error,omitempty"`
}

// IsDirty returns true when there's at least one uncommitted file or
// unpushed commit on this repo.
func (r WorkspaceRepoStatus) IsDirty() bool {
	return r.UncommittedFiles > 0 || r.UnpushedCommits > 0
}

// WorkspaceStatusResponse is the shape returned by
// GET /api/v1/sessions/{id}/workspace-status.
type WorkspaceStatusResponse struct {
	SessionID   string                `json:"session_id"`
	Repos       []WorkspaceRepoStatus `json:"repos"`
	TotalDirty  int                   `json:"total_dirty"`
	IsDirty     bool                  `json:"is_dirty"`
	// ContainerReachable=false means we couldn't talk to the desktop
	// at all (e.g. it's been reaped). The frontend should treat this
	// as "unknown" and let the user decide whether to fork anyway.
	ContainerReachable bool `json:"container_reachable"`
}

// workspaceStatusGitScript is the bash one-liner we exec per repo. It's
// written defensively so a missing path returns a benign zero-result
// rather than failing the whole call.
const workspaceStatusGitScript = `cd %s 2>/dev/null || { echo '{"branch":"","uncommitted":0,"unpushed":0,"error":"path missing"}'; exit 0; }
uncommitted=$(git status --porcelain 2>/dev/null | wc -l | tr -d ' ')
branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
unpushed=$(git rev-list --count @{u}..HEAD 2>/dev/null || echo 0)
echo "{\"branch\":\"$branch\",\"uncommitted\":$uncommitted,\"unpushed\":$unpushed}"`

// workspaceStatus godoc
// @Summary  Check uncommitted / unpushed git state in a session's desktop container
// @Description Used by the fork-confirm modal so we can show "N files will be committed & pushed" or just proceed silently when the workspace is clean. Aborts gracefully on unreachable containers — the frontend treats that as "unknown".
// @Tags     sessions
// @Produce  json
// @Param    id   path   string  true  "Session ID"
// @Success  200  {object}  WorkspaceStatusResponse
// @Router   /api/v1/sessions/{id}/workspace-status [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) workspaceStatus(_ http.ResponseWriter, req *http.Request) (*WorkspaceStatusResponse, *system.HTTPError) {
	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		return nil, system.NewHTTPError400("session id required")
	}

	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("unauthenticated")
	}

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", sessionID))
	}
	if authErr := apiServer.authorizeUserToSession(ctx, user, session, types.ActionGet); authErr != nil {
		return nil, system.NewHTTPError403(authErr.Error())
	}

	resp := &WorkspaceStatusResponse{
		SessionID: sessionID,
		Repos:     []WorkspaceRepoStatus{},
	}

	projectID := session.ProjectID
	if projectID == "" {
		projectID = session.Metadata.ProjectID
	}
	if projectID == "" {
		// No project → no repos to check.
		resp.ContainerReachable = true
		return resp, nil
	}

	repos, err := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list repos: %v", err))
	}
	if len(repos) == 0 {
		resp.ContainerReachable = true
		return resp, nil
	}

	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	reachable := true

	for _, repo := range repos {
		out, execErr := apiServer.execInContainer(ctx, runnerID, []string{
			"bash", "-c", fmt.Sprintf(workspaceStatusGitScript, fmt.Sprintf("/home/retro/work/%s", repo.Name)),
		})
		status := WorkspaceRepoStatus{RepoID: repo.ID, Name: repo.Name}
		if execErr != nil {
			// Container unreachable counts once; subsequent repos
			// will produce the same error so don't spam the response
			// with N copies of "container not ready".
			reachable = false
			status.Error = execErr.Error()
			resp.Repos = append(resp.Repos, status)
			continue
		}
		var parsed struct {
			Branch      string `json:"branch"`
			Uncommitted int    `json:"uncommitted"`
			Unpushed    int    `json:"unpushed"`
			Error       string `json:"error,omitempty"`
		}
		if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); jsonErr != nil {
			status.Error = fmt.Sprintf("parse status: %v (raw: %q)", jsonErr, out)
			resp.Repos = append(resp.Repos, status)
			continue
		}
		status.Branch = parsed.Branch
		status.UncommittedFiles = parsed.Uncommitted
		status.UnpushedCommits = parsed.Unpushed
		if parsed.Error != "" {
			status.Error = parsed.Error
		}
		if status.IsDirty() {
			resp.TotalDirty += status.UncommittedFiles + status.UnpushedCommits
			resp.IsDirty = true
		}
		resp.Repos = append(resp.Repos, status)
	}

	resp.ContainerReachable = reachable
	return resp, nil
}

// commitAndPushUncommittedRepos runs `git add -A && git commit && git
// push` per dirty repo in the parent's container. Called from
// forkSession when the request has auto_commit_uncommitted=true.
// Returns an error if ANY push fails — the fork should abort rather
// than leave half-pushed-state-then-paused.
func (apiServer *HelixAPIServer) commitAndPushUncommittedRepos(
	ctx context.Context,
	parentSessionID string,
	projectID string,
	commitMessage string,
) error {
	if projectID == "" {
		return nil
	}
	repos, err := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
	if err != nil {
		return fmt.Errorf("list repos: %w", err)
	}
	if len(repos) == 0 {
		return nil
	}

	runnerID := fmt.Sprintf("desktop-%s", parentSessionID)
	var failures []string

	for _, repo := range repos {
		path := fmt.Sprintf("/home/retro/work/%s", repo.Name)
		// One bash invocation per repo, with early-exit if path missing
		// or if nothing's dirty. Captures combined stdout+stderr so
		// hook output (commit-msg, pre-push) surfaces on failure.
		script := fmt.Sprintf(`set -e
cd %s 2>/dev/null || { echo "SKIP path missing"; exit 0; }
dirty=$(git status --porcelain 2>/dev/null)
unpushed=$(git rev-list --count @{u}..HEAD 2>/dev/null || echo 0)
if [ -z "$dirty" ] && [ "$unpushed" = "0" ]; then
  echo "CLEAN"
  exit 0
fi
if [ -n "$dirty" ]; then
  git add -A
  git -c commit.gpgsign=false commit -m %s
fi
git push origin HEAD 2>&1
echo "OK"`, path, shellEscape(commitMessage))

		out, execErr := apiServer.execInContainer(ctx, runnerID, []string{"bash", "-c", script})
		out = strings.TrimSpace(out)
		if execErr != nil {
			failures = append(failures, fmt.Sprintf("repo %s: %v (output: %s)", repo.Name, execErr, out))
			log.Warn().Err(execErr).Str("repo", repo.Name).Str("session_id", parentSessionID).Str("output", out).Msg("fork: commit+push failed")
			continue
		}
		// Bash succeeded but git push could still have reported errors
		// to stdout. Detect by looking for typical failure phrases.
		if strings.Contains(out, "rejected") || strings.Contains(out, "failed to push") || strings.Contains(out, "error:") {
			failures = append(failures, fmt.Sprintf("repo %s: %s", repo.Name, out))
			log.Warn().Str("repo", repo.Name).Str("session_id", parentSessionID).Str("output", out).Msg("fork: git push reported error")
			continue
		}
		log.Info().Str("repo", repo.Name).Str("session_id", parentSessionID).Str("output", out).Msg("fork: pre-fork commit+push complete")
	}

	if len(failures) > 0 {
		return fmt.Errorf("pre-fork commit+push failed: %s", strings.Join(failures, "; "))
	}
	return nil
}

// shellEscape wraps a string in single quotes for safe use inside a
// bash double-quoted script. Single-quoted strings have NO escape
// processing in bash so the only thing we need to handle is embedded
// single quotes (replace ' with '\'').
func shellEscape(s string) string {
	var b bytes.Buffer
	b.WriteByte('\'')
	for _, r := range s {
		if r == '\'' {
			b.WriteString(`'\''`)
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}
