package server

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
)

func (apiServer *HelixAPIServer) authorizeWorkspaceReviewRequest(w http.ResponseWriter, req *http.Request) (*types.Session, bool) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return nil, false
	}
	sessionID := mux.Vars(req)["sessionID"]
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return nil, false
	}
	if err := apiServer.authorizeUserToSession(req.Context(), user, session, types.ActionGet); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return nil, false
	}
	return session, true
}

func (apiServer *HelixAPIServer) callSessionDesktopJSON(ctx context.Context, sessionID, method, path string, body, out interface{}) error {
	conn, err := apiServer.dialDesktop(ctx, sessionID)
	if err != nil {
		return err
	}
	defer conn.Close()
	// ctx bounds Dial on its own; applyDesktopDeadline is what bounds the
	// request/response that follows. Without it a ctx timeout here is
	// cosmetic and callers block for as long as the desktop stays silent.
	applyDesktopDeadline(ctx, conn)
	return callDesktopJSON(conn, method, path, body, out)
}

// getWorkspaceReview godoc
// @Summary Get workspace review sources
// @Description Returns coherent all-task, branch, and working-tree patches for a connected external-agent workspace.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param workspace query string false "Workspace name"
// @Param base query string false "Base ref (default main)"
// @Param ignore_whitespace query bool false "Ignore whitespace-only changes"
// @Success 200 {object} types.WorkspaceReviewResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/workspace-review [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getWorkspaceReview(w http.ResponseWriter, req *http.Request) {
	session, ok := apiServer.authorizeWorkspaceReviewRequest(w, req)
	if !ok {
		return
	}
	var response types.WorkspaceReviewResponse
	path := "/workspace/review"
	if req.URL.RawQuery != "" {
		path += "?" + req.URL.RawQuery
	}
	if err := apiServer.callSessionDesktopJSON(req.Context(), session.ID, http.MethodGet, path, nil, &response); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// getWorkspaceFiles godoc
// @Summary List workspace files
// @Description Returns a bounded flat list of tracked and non-ignored untracked workspace entries.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param workspace query string false "Workspace name"
// @Success 200 {object} types.WorkspaceFilesResponse
// @Router /api/v1/external-agents/{sessionID}/workspace-files [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getWorkspaceFiles(w http.ResponseWriter, req *http.Request) {
	apiServer.proxyAuthorizedWorkspaceGET(w, req, "/workspace/files", &types.WorkspaceFilesResponse{})
}

// getWorkspaceFile godoc
// @Summary Read a workspace file
// @Description Reads a bounded UTF-8 source file after real-path containment checks.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param workspace query string false "Workspace name"
// @Param path query string true "Repository-relative file path"
// @Success 200 {object} types.WorkspaceFileResponse
// @Router /api/v1/external-agents/{sessionID}/workspace-file [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getWorkspaceFile(w http.ResponseWriter, req *http.Request) {
	apiServer.proxyAuthorizedWorkspaceGET(w, req, "/workspace/file", &types.WorkspaceFileResponse{})
}

// getWorkspaceSkills godoc
// @Summary List workspace skills
// @Description Returns agent skills installed for the connected external-agent workspace and sandbox user.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param workspace query string false "Workspace name"
// @Success 200 {object} types.WorkspaceSkillsResponse
// @Router /api/v1/external-agents/{sessionID}/workspace-skills [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getWorkspaceSkills(w http.ResponseWriter, req *http.Request) {
	apiServer.proxyAuthorizedWorkspaceGET(w, req, "/workspace/skills", &types.WorkspaceSkillsResponse{})
}

func (apiServer *HelixAPIServer) proxyAuthorizedWorkspaceGET(w http.ResponseWriter, req *http.Request, desktopPath string, response interface{}) {
	session, ok := apiServer.authorizeWorkspaceReviewRequest(w, req)
	if !ok {
		return
	}
	if req.URL.RawQuery != "" {
		desktopPath += "?" + req.URL.RawQuery
	}
	if err := apiServer.callSessionDesktopJSON(req.Context(), session.ID, http.MethodGet, desktopPath, nil, response); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

// getWorkspaceTurnReview godoc
// @Summary Get the immutable diff for one interaction
// @Description Resolves hidden checkpoint refs only from the stored interaction receipt.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param interactionID path string true "Interaction ID"
// @Param ignore_whitespace query bool false "Ignore whitespace-only changes"
// @Success 200 {object} types.WorkspaceReviewSource
// @Router /api/v1/external-agents/{sessionID}/workspace-review/turn/{interactionID} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getWorkspaceTurnReview(w http.ResponseWriter, req *http.Request) {
	session, ok := apiServer.authorizeWorkspaceReviewRequest(w, req)
	if !ok {
		return
	}
	interactionID := mux.Vars(req)["interactionID"]
	interaction, err := apiServer.Store.GetInteraction(req.Context(), interactionID)
	if err != nil || interaction == nil || interaction.SessionID != session.ID {
		http.Error(w, "interaction not found", http.StatusNotFound)
		return
	}
	changes := interaction.CodeChanges
	if changes == nil || changes.Status != types.CodeChangesStatusReady || changes.BeforeRef == "" || changes.AfterRef == "" {
		http.Error(w, "turn diff is unavailable", http.StatusNotFound)
		return
	}
	ignoreWhitespace := req.URL.Query().Get("ignore_whitespace") == "true"
	var checkpoint types.WorkspaceCheckpointDiffResponse
	err = apiServer.callSessionDesktopJSON(req.Context(), session.ID, http.MethodPost, "/workspace/checkpoints/diff", types.WorkspaceCheckpointDiffRequest{
		Workspace: changes.Workspace, BeforeRef: changes.BeforeRef, AfterRef: changes.AfterRef,
		IgnoreWhitespace: ignoreWhitespace,
	}, &checkpoint)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, types.WorkspaceReviewSource{
		ID: "turn:" + interactionID, Title: "Turn changes", Patch: checkpoint.Patch,
		PatchHash: checkpoint.PatchHash, Truncated: checkpoint.Truncated, Files: checkpoint.Files,
		TotalAdditions: checkpoint.TotalAdditions, TotalDeletions: checkpoint.TotalDeletions,
	})
}
