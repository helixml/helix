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
//
// Both helpers reach the desktop via RevDial → /workspace/status and
// /workspace/commit-and-push (defined in api/pkg/desktop/workspace.go).
// We deliberately do NOT use the generic /exec endpoint because it's
// allowlist-restricted to a small set of safe commands — git plumbing
// runs through dedicated endpoints with structured request/response.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// resolveExpectedBranch returns the branch the fork's pre-commit step
// should target. Reads from the spec task's BranchName first (set when
// the task transitions to implementation), then falls back to the
// generated default (feature/NNNNNN-shortname) when BranchName is
// empty — the case where the user is forking from a session whose
// task is still in planning/spec_review and hasn't yet had the
// implementation transition compute the canonical name.
//
// Returns empty string when there's no spec task to resolve against.
func resolveExpectedBranch(specTask *types.SpecTask) string {
	if specTask == nil {
		return ""
	}
	if specTask.BranchName != "" {
		return specTask.BranchName
	}
	// Lazy fallback. GenerateFeatureBranchName needs at least a
	// TaskNumber (or BranchPrefix); without those it falls through to
	// an ID-based name which is still pushable.
	return services.GenerateFeatureBranchName(specTask)
}

// Wire types mirroring api/pkg/desktop/workspace.go. Duplicated
// rather than imported because the desktop package pulls in
// gstreamer/CGo via its video pipeline, which would balloon this
// package's build dependencies. Both sides need to stay in sync —
// the JSON field tags are the contract.

type desktopWorkspaceStatusResp struct {
	Repos []desktopWorkspaceRepoStatus `json:"repos"`
}

type desktopWorkspaceRepoStatus struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Branch           string `json:"branch,omitempty"`
	UncommittedFiles int    `json:"uncommitted_files"`
	UnpushedCommits  int    `json:"unpushed_commits"`
	Error            string `json:"error,omitempty"`
}

type desktopCommitReq struct {
	Message        string `json:"message"`
	ExpectedBranch string `json:"expected_branch,omitempty"`
}

type desktopCommitResp struct {
	Repos   []desktopCommitRepoResult `json:"repos"`
	Success bool                      `json:"success"`
}

type desktopCommitRepoResult struct {
	Name             string `json:"name"`
	Path             string `json:"path"`
	Action           string `json:"action"`
	UncommittedFiles int    `json:"uncommitted_files"`
	UnpushedCommits  int    `json:"unpushed_commits"`
	Error            string `json:"error,omitempty"`
	PushOutput       string `json:"push_output,omitempty"`
}

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
	SessionID  string                `json:"session_id"`
	Repos      []WorkspaceRepoStatus `json:"repos"`
	TotalDirty int                   `json:"total_dirty"`
	IsDirty    bool                  `json:"is_dirty"`
	// ContainerReachable=false means we couldn't talk to the desktop
	// at all (e.g. it's been reaped). The frontend should treat this
	// as "unknown" and let the user decide whether to fork anyway.
	ContainerReachable bool `json:"container_reachable"`

	// CanSaveChanges is false when there ARE dirty changes but the
	// fork's pre-commit safety net has nowhere viable to push them.
	// Concretely: the session has no spec task, or the spec task has
	// no branch name set, or the spec task's branch is a protected
	// branch (main / master) that the remote pre-receive hook will
	// reject. In any of those cases the frontend should refuse to
	// offer "Fork with auto-commit" — the user has to fix git state
	// manually (commit/push to a feature branch from the terminal)
	// before forking, OR explicitly abandon the changes.
	CanSaveChanges bool `json:"can_save_changes"`
	// CannotSaveReason is a human-readable explanation surfaced in
	// the blocking modal. Empty when CanSaveChanges is true.
	CannotSaveReason string `json:"cannot_save_reason,omitempty"`
	// ExpectedBranch is the branch the pre-fork commit will target,
	// resolved from the spec task. Empty for sessions without a
	// spec task. Exposed so the frontend can say "will commit to
	// <branch>" instead of just "will commit" — helps the user
	// understand what's about to happen.
	ExpectedBranch string `json:"expected_branch,omitempty"`
}

// dialDesktop opens a RevDial connection to the session's desktop
// container. Centralised so the runner-id format ("desktop-<sessionID>")
// lives in one place. Returns an error (not a panic) when connman
// isn't initialised — tests that wire HelixAPIServer without the
// full connman should still be able to exercise the fork path.
func (apiServer *HelixAPIServer) dialDesktop(ctx context.Context, sessionID string) (io.ReadWriteCloser, error) {
	if apiServer.connman == nil {
		return nil, fmt.Errorf("connection manager not initialised")
	}
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	conn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		return nil, fmt.Errorf("container not ready: %w", err)
	}
	return conn, nil
}

// callDesktopJSON sends an HTTP request to the desktop's RevDial-
// exposed port (9876) and decodes the JSON response into `out`.
// Used by both workspace-status and commit-and-push.
func callDesktopJSON(conn io.ReadWriteCloser, method, path string, body interface{}, out interface{}) error {
	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		reqBody = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, "http://localhost:9876"+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := req.Write(conn); err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("desktop returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("parse json response: %w (body: %q)", err, string(respBody))
		}
	}
	return nil
}

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

	// Single round-trip to the desktop — it walks its own
	// findAllWorkspaces() and returns the per-repo status. We map the
	// result back into the API-level shape (which includes RepoID from
	// the helix store, not just the on-disk directory name).
	conn, dialErr := apiServer.dialDesktop(ctx, sessionID)
	if dialErr != nil {
		resp.ContainerReachable = false
		log.Debug().Err(dialErr).Str("session_id", sessionID).Msg("workspace-status: container not reachable")
		return resp, nil
	}
	defer conn.Close()

	var desktopResp desktopWorkspaceStatusResp
	if err := callDesktopJSON(conn, http.MethodGet, "/workspace/status", nil, &desktopResp); err != nil {
		resp.ContainerReachable = false
		log.Warn().Err(err).Str("session_id", sessionID).Msg("workspace-status: desktop call failed")
		return resp, nil
	}

	resp.ContainerReachable = true

	// Try to attach repo IDs by joining on Name. If we have the helix
	// repo metadata we surface it; otherwise we just pass the on-disk
	// status through (still useful to the user).
	repos, _ := apiServer.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{ProjectID: projectID})
	idByName := make(map[string]string, len(repos))
	for _, r := range repos {
		if r != nil {
			idByName[r.Name] = r.ID
		}
	}

	for _, ws := range desktopResp.Repos {
		status := WorkspaceRepoStatus{
			RepoID:           idByName[ws.Name],
			Name:             ws.Name,
			Branch:           ws.Branch,
			UncommittedFiles: ws.UncommittedFiles,
			UnpushedCommits:  ws.UnpushedCommits,
			Error:            ws.Error,
		}
		if status.IsDirty() {
			resp.TotalDirty += status.UncommittedFiles + status.UnpushedCommits
			resp.IsDirty = true
		}
		resp.Repos = append(resp.Repos, status)
	}

	// Resolve the spec-task's branch_name so the frontend can show it
	// in the modal ("will commit to <branch>") AND so we can compute
	// whether the pre-fork safety net has a viable target. Falls back
	// to the generated default when the spec task hasn't yet computed
	// its canonical name (still in planning / spec_review).
	if session.Metadata.SpecTaskID != "" {
		if specTask, stErr := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID); stErr == nil && specTask != nil {
			resp.ExpectedBranch = resolveExpectedBranch(specTask)
		}
	}

	// Decide whether the pre-fork commit can actually save changes.
	// Default to "can save" — only flip false when there ARE dirty
	// changes and we can prove the safety net would fail.
	resp.CanSaveChanges = true
	if resp.IsDirty {
		switch {
		case resp.ExpectedBranch == "":
			resp.CanSaveChanges = false
			resp.CannotSaveReason = "This session isn't linked to a feature branch — the safety net can't decide where to commit your changes. Commit and push them from the desktop terminal first, then try switching agent."
		case resp.ExpectedBranch == "main" || resp.ExpectedBranch == "master":
			resp.CanSaveChanges = false
			resp.CannotSaveReason = fmt.Sprintf("The branch for this session is %q, which the repository protects from direct pushes. Commit your changes to a feature branch from the desktop terminal first, then try switching agent.", resp.ExpectedBranch)
		}
	}

	return resp, nil
}

// commitAndPushUncommittedRepos delegates to the desktop's
// /workspace/commit-and-push endpoint which walks its own workspaces
// and runs `git add -A && git commit && git push` per dirty repo.
// Returns an error if ANY repo's commit or push fails — the fork
// should abort rather than leave half-pushed-state-then-paused.
//
// expectedBranch (when non-empty) is passed through so the desktop
// can switch to the right branch before committing. Spec-task
// containers default to `main` after clone and depend on the agent
// to `git checkout <feature-branch>` later; if the user dirties
// files before that checkout happens, a naive push targets `main`
// and gets rejected by the pre-receive hook. Telling the desktop
// the expected branch up-front lets it recover.
//
// If the container isn't reachable, this is a no-op (we can't see
// what's there to push, but we also can't pause an unreachable
// container's worth of work — best effort is "proceed silently").
func (apiServer *HelixAPIServer) commitAndPushUncommittedRepos(
	ctx context.Context,
	parentSessionID string,
	commitMessage string,
	expectedBranch string,
) error {
	conn, dialErr := apiServer.dialDesktop(ctx, parentSessionID)
	if dialErr != nil {
		log.Debug().Err(dialErr).Str("session_id", parentSessionID).Msg("pre-fork commit: container not reachable, skipping")
		return nil
	}
	defer conn.Close()

	var desktopResp desktopCommitResp
	if err := callDesktopJSON(conn, http.MethodPost, "/workspace/commit-and-push",
		desktopCommitReq{Message: commitMessage, ExpectedBranch: expectedBranch}, &desktopResp); err != nil {
		// Older container images don't have this endpoint yet (rolled
		// out with the fork safety-net change; needs build-ubuntu and
		// a new session to take effect). Treat 404 as "feature not
		// available on this desktop" and skip the pre-fork commit
		// rather than blocking the fork entirely. The user can
		// always commit manually before forking on an old container.
		if isDesktopEndpointMissing(err) {
			log.Warn().Err(err).Str("session_id", parentSessionID).
				Msg("pre-fork commit: desktop image predates /workspace/commit-and-push; skipping")
			return nil
		}
		return fmt.Errorf("commit-and-push call: %w", err)
	}

	if desktopResp.Success {
		for _, r := range desktopResp.Repos {
			if r.Action != "clean" {
				log.Info().Str("repo", r.Name).Str("action", r.Action).Int("files", r.UncommittedFiles).
					Int("commits", r.UnpushedCommits).Str("session_id", parentSessionID).
					Msg("fork: pre-fork commit+push complete")
			}
		}
		return nil
	}

	// At least one repo failed — collect the errors for a single
	// user-facing message.
	var failures []string
	for _, r := range desktopResp.Repos {
		if r.Error != "" {
			failures = append(failures, fmt.Sprintf("repo %s: %s", r.Name, r.Error))
		}
	}
	if len(failures) == 0 {
		failures = append(failures, "unknown desktop error")
	}
	return fmt.Errorf("pre-fork commit+push failed: %s", joinErrors(failures))
}

// isDesktopEndpointMissing detects the "HTTP 404 page not found"
// reply older sandbox images return for unknown routes — needed so
// the fork flow doesn't break for users still on a desktop image
// that predates /workspace/commit-and-push.
func isDesktopEndpointMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return (containsCaseInsensitive(msg, "404") && containsCaseInsensitive(msg, "page not found"))
}

func containsCaseInsensitive(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	if len(haystack) < len(needle) {
		return false
	}
	// Cheap ASCII case-insensitive contains — both strings are short
	// runtime-internal labels, no need for proper Unicode folding.
	hl := bytes.ToLower([]byte(haystack))
	nl := bytes.ToLower([]byte(needle))
	return bytes.Contains(hl, nl)
}

// joinErrors flattens N error strings into one with "; " separators,
// quietly truncating any individual string longer than ~500 chars so
// the surface message doesn't blow up the snackbar.
func joinErrors(errs []string) string {
	const perItemMax = 500
	var b bytes.Buffer
	for i, s := range errs {
		if i > 0 {
			b.WriteString("; ")
		}
		if len(s) > perItemMax {
			s = s[:perItemMax] + "…[truncated]"
		}
		b.WriteString(s)
	}
	return b.String()
}
