package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
)

// listSpecTaskProposals returns all proposals for a given spec task.
// @Summary List proposals for a spec task
// @Description List all agent proposals (PR / sub-task / mark-complete) for a given spec task
// @Tags spec-tasks
// @Param taskId path string true "SpecTask ID"
// @Param status query string false "Filter by status (pending|approved|rejected|failed)"
// @Success 200 {array} types.SpecTaskProposal
// @Router /api/v1/spec-tasks/{taskId}/proposals [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSpecTaskProposals(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	taskID := mux.Vars(r)["taskId"]
	if taskID == "" {
		http.Error(w, "taskId required", http.StatusBadRequest)
		return
	}

	task, err := s.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get spec task: %v", err), http.StatusNotFound)
		return
	}
	project, err := s.Store.GetProject(r.Context(), task.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get project: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.authorizeUserToProject(r.Context(), user, project, types.ActionGet); err != nil {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}

	filters := &types.SpecTaskProposalFilters{SpecTaskID: taskID}
	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = types.SpecTaskProposalStatus(status)
	}
	out, err := s.Store.ListSpecTaskProposals(r.Context(), filters)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list proposals: %v", err), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []*types.SpecTaskProposal{}
	}
	writeResponse(w, out, http.StatusOK)
}

// listProjectPendingProposals returns all pending proposals across a project.
// Used by the project-level board to render a "N pending proposals" badge.
// @Summary List pending proposals for a project
// @Description List pending proposals across all spec tasks in a project
// @Tags projects
// @Param projectId path string true "Project ID"
// @Success 200 {array} types.SpecTaskProposal
// @Router /api/v1/projects/{projectId}/proposals [get]
// @Security BearerAuth
func (s *HelixAPIServer) listProjectPendingProposals(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	projectID := mux.Vars(r)["projectId"]
	if projectID == "" {
		http.Error(w, "projectId required", http.StatusBadRequest)
		return
	}
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get project: %v", err), http.StatusNotFound)
		return
	}
	if err := s.authorizeUserToProject(r.Context(), user, project, types.ActionGet); err != nil {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}

	out, err := s.Store.ListSpecTaskProposals(r.Context(), &types.SpecTaskProposalFilters{
		ProjectID: projectID,
		Status:    types.ProposalStatusPending,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list proposals: %v", err), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []*types.SpecTaskProposal{}
	}
	writeResponse(w, out, http.StatusOK)
}

// decideSpecTaskProposal applies a user decision to a pending proposal: approve
// (with optional payload edits) or reject (with optional comment). On approve,
// dispatches by Kind to actually execute the action. On any path, sends a
// follow-up message to the agent's session via the standard prompt-template
// channel so the agent learns the outcome.
//
// @Summary Decide on a spec task proposal
// @Description Approve or reject an agent's proposal; on approve, the action is executed and the agent is notified
// @Tags spec-tasks
// @Param proposalId path string true "Proposal ID"
// @Param request body types.ProposalDecisionRequest true "Decision payload"
// @Success 200 {object} types.ProposalDecisionResponse
// @Router /api/v1/proposals/{proposalId}/decide [post]
// @Security BearerAuth
func (s *HelixAPIServer) decideSpecTaskProposal(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	proposalID := mux.Vars(r)["proposalId"]
	if proposalID == "" {
		http.Error(w, "proposalId required", http.StatusBadRequest)
		return
	}

	// Detach so DB writes complete even if the client disconnects mid-dispatch.
	ctx, cancel := detachContext(r.Context(), 120*time.Second)
	defer cancel()

	proposal, err := s.Store.GetSpecTaskProposal(ctx, proposalID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get proposal: %v", err), http.StatusNotFound)
		return
	}
	if proposal.Status != types.ProposalStatusPending {
		http.Error(w, fmt.Sprintf("proposal already decided (status=%s)", proposal.Status), http.StatusConflict)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, proposal.SpecTaskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get spec task: %v", err), http.StatusInternalServerError)
		return
	}
	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get project: %v", err), http.StatusInternalServerError)
		return
	}
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionUpdate); err != nil {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}

	var req types.ProposalDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if req.Decision != "approve" && req.Decision != "reject" {
		http.Error(w, "decision must be 'approve' or 'reject'", http.StatusBadRequest)
		return
	}

	// Apply user edits before dispatching so downstream sees the final values.
	if len(req.EditedPayload) > 0 {
		applyEditedPayload(proposal, req.EditedPayload)
		proposal.EditedPayload = req.EditedPayload
	}

	now := time.Now()
	proposal.DecidedBy = user.ID
	proposal.DecidedAt = &now
	proposal.DecisionComment = req.Comment

	if req.Decision == "reject" {
		proposal.Status = types.ProposalStatusRejected
	} else {
		proposal.Status = types.ProposalStatusApproved
		// Dispatch the action. On execution failure we mark the proposal as
		// failed (not approved) so the user can see it didn't actually happen.
		if dispatchErr := s.dispatchProposalApproval(ctx, proposal, task, user.ID); dispatchErr != nil {
			proposal.Status = types.ProposalStatusFailed
			proposal.ResultError = dispatchErr.Error()
			log.Error().
				Err(dispatchErr).
				Str("proposal_id", proposal.ID).
				Str("kind", string(proposal.Kind)).
				Msg("Proposal dispatch failed")
		}
	}

	if err := s.Store.UpdateSpecTaskProposal(ctx, proposal); err != nil {
		http.Error(w, fmt.Sprintf("failed to update proposal: %v", err), http.StatusInternalServerError)
		return
	}

	// Fire-and-forget agent notification via the standard prompt-template channel.
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.sendProposalDecisionToAgent(context.Background(), task, proposal, user); err != nil {
			log.Warn().Err(err).Str("proposal_id", proposal.ID).Msg("Failed to deliver proposal decision message to agent")
		}
	}()

	writeResponse(w, &types.ProposalDecisionResponse{Proposal: proposal}, http.StatusOK)
}

// dispatchProposalApproval executes the action implied by an approved proposal,
// keyed by Kind. Mutates the task as needed (e.g. setting Status=Done for a
// confirmed mark-complete proposal). Mutates proposal.Result* fields with
// outcome metadata.
func (s *HelixAPIServer) dispatchProposalApproval(ctx context.Context, proposal *types.SpecTaskProposal, task *types.SpecTask, userID string) error {
	switch proposal.Kind {
	case types.ProposalKindPullRequest:
		return s.dispatchPRProposal(ctx, proposal, task, userID)
	case types.ProposalKindSpecTask:
		return s.dispatchSpecTaskProposal(ctx, proposal, task, userID)
	case types.ProposalKindMarkComplete:
		return s.dispatchMarkCompleteProposal(ctx, proposal, task, userID)
	default:
		return fmt.Errorf("unknown proposal kind: %s", proposal.Kind)
	}
}

// dispatchPRProposal pushes the proposed branch and opens a PR. Honours
// edited payload values (head_branch, base_branch, title, body) over agent
// originals.
func (s *HelixAPIServer) dispatchPRProposal(ctx context.Context, proposal *types.SpecTaskProposal, task *types.SpecTask, userID string) error {
	repoID := proposal.PRRepositoryID
	if repoID == "" {
		project, err := s.Store.GetProject(ctx, task.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to get project for default repo: %w", err)
		}
		repoID = project.DefaultRepoID
	}
	if repoID == "" {
		return fmt.Errorf("no repository_id and project has no default repository")
	}
	repo, err := s.Store.GetGitRepository(ctx, repoID)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}
	if repo.ExternalURL == "" {
		return fmt.Errorf("repository has no external URL configured for PR creation")
	}

	headBranch := proposal.PRHeadBranch
	if headBranch == "" {
		headBranch = task.BranchName
	}
	if headBranch == "" {
		return fmt.Errorf("no head_branch supplied and task has no BranchName")
	}
	baseBranch := proposal.PRBaseBranch
	if baseBranch == "" {
		baseBranch = repo.DefaultBranch
	}

	title := proposal.PRTitle
	body := proposal.PRBody
	if title == "" {
		title = task.Name
	}
	if body == "" {
		body = task.Description
	}

	// Push the branch (acting as the approving user so OAuth credentials apply).
	if err := s.gitRepositoryService.WithRepoLock(repo.ID, func() error {
		return s.gitRepositoryService.PushBranchToRemote(ctx, repo.ID, headBranch, false, userID)
	}); err != nil {
		return fmt.Errorf("failed to push branch %s: %w", headBranch, err)
	}

	prID, err := s.gitRepositoryService.CreatePullRequest(ctx, repo.ID, title, body, headBranch, baseBranch, userID)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	// Look up the freshly created PR for URL / number.
	repoPR := types.RepoPR{
		RepositoryID:   repo.ID,
		RepositoryName: repo.Name,
		PRID:           prID,
		PRState:        string(types.PullRequestStateOpen),
	}
	prs, listErr := s.gitRepositoryService.ListPullRequests(ctx, repo.ID)
	if listErr == nil {
		for _, pr := range prs {
			if pr.ID == prID {
				repoPR.PRNumber = pr.Number
				repoPR.PRURL = pr.URL
				repoPR.PRState = string(pr.State)
				break
			}
		}
	}
	proposal.ResultPRID = prID
	proposal.ResultPRURL = repoPR.PRURL
	proposal.PRHeadBranch = headBranch // record final values used
	proposal.PRBaseBranch = baseBranch

	// Append (or replace existing entry for the same repo) on the task.
	merged := false
	for i := range task.RepoPullRequests {
		if task.RepoPullRequests[i].RepositoryID == repo.ID && task.RepoPullRequests[i].PRID == prID {
			task.RepoPullRequests[i] = repoPR
			merged = true
			break
		}
	}
	if !merged {
		task.RepoPullRequests = append(task.RepoPullRequests, repoPR)
	}
	task.UpdatedAt = time.Now()
	if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
		return fmt.Errorf("failed to update task with new PR: %w", err)
	}
	return nil
}

// dispatchSpecTaskProposal creates a new SpecTask in the same project as the
// parent, with ParentTaskID linking it back. Reuses the shared
// services.CreateSpecTaskFromProposal helper.
func (s *HelixAPIServer) dispatchSpecTaskProposal(ctx context.Context, proposal *types.SpecTaskProposal, parent *types.SpecTask, userID string) error {
	newTask, err := services.CreateSpecTaskFromProposal(ctx, s.Store, services.CreateSpecTaskRequest{
		ProjectID:      parent.ProjectID,
		UserID:         userID,
		Name:           proposal.TaskName,
		Description:    proposal.TaskDescription,
		Type:           proposal.TaskType,
		Priority:       proposal.TaskPriority,
		OriginalPrompt: proposal.TaskOriginalPrompt,
		ParentTaskID:   parent.ID,
	})
	if err != nil {
		return err
	}
	proposal.ResultTaskID = newTask.ID
	return nil
}

// dispatchMarkCompleteProposal moves the parent task to Done. The user must
// have approved (Mark Done); rejection (Send Back) leaves status unchanged
// and is handled in the caller before dispatch is invoked.
func (s *HelixAPIServer) dispatchMarkCompleteProposal(ctx context.Context, proposal *types.SpecTaskProposal, task *types.SpecTask, userID string) error {
	now := time.Now()
	task.Status = types.TaskStatusDone
	task.StatusUpdatedAt = &now
	task.CompletedAt = &now
	task.UpdatedAt = now
	if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
		return fmt.Errorf("failed to mark task done: %w", err)
	}
	_ = userID // accepted for signature symmetry; not needed here
	return nil
}

// sendProposalDecisionToAgent renders the appropriate decision prompt template
// and delivers it as a user-turn message in the agent's session. Same channel
// the design-review comment / approval / revision flows already use.
func (s *HelixAPIServer) sendProposalDecisionToAgent(ctx context.Context, task *types.SpecTask, proposal *types.SpecTaskProposal, decidedBy *types.User) error {
	identity := decidedBy.Email
	if identity == "" {
		identity = decidedBy.ID
	}
	message, err := services.BuildProposalDecisionPrompt(proposal, identity)
	if err != nil {
		return fmt.Errorf("failed to build decision prompt: %w", err)
	}
	if _, _, err := s.sendMessageToSpecTaskAgent(ctx, task, message, ""); err != nil {
		return fmt.Errorf("failed to send decision message to agent: %w", err)
	}
	return nil
}

// applyEditedPayload merges the user's payload edits onto the proposal in-place.
// Only fields relevant to the proposal Kind are read; unknown keys are ignored.
func applyEditedPayload(proposal *types.SpecTaskProposal, raw []byte) {
	var edits map[string]any
	if err := json.Unmarshal(raw, &edits); err != nil {
		log.Warn().Err(err).Str("proposal_id", proposal.ID).Msg("Failed to parse edited_payload — ignoring")
		return
	}
	if v, ok := edits["pr_repository_id"].(string); ok {
		proposal.PRRepositoryID = v
	}
	if v, ok := edits["pr_head_branch"].(string); ok {
		proposal.PRHeadBranch = v
	}
	if v, ok := edits["pr_base_branch"].(string); ok {
		proposal.PRBaseBranch = v
	}
	if v, ok := edits["pr_title"].(string); ok {
		proposal.PRTitle = v
	}
	if v, ok := edits["pr_body"].(string); ok {
		proposal.PRBody = v
	}
	if v, ok := edits["task_name"].(string); ok {
		proposal.TaskName = v
	}
	if v, ok := edits["task_description"].(string); ok {
		proposal.TaskDescription = v
	}
	if v, ok := edits["task_type"].(string); ok {
		proposal.TaskType = v
	}
	if v, ok := edits["task_priority"].(string); ok {
		proposal.TaskPriority = types.SpecTaskPriority(v)
	}
}
