package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// listSpecTaskPRProposals godoc
// @Summary List a spec task's pull request proposals
// @Description Every pull request a spec task opens starts as an agent proposal awaiting user approval.
// @Tags spec-tasks
// @Produce json
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {array} types.SpecTaskPRProposal
// @Router /api/v1/spec-tasks/{spec_task_id}/pr-proposals [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSpecTaskPRProposals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	task, ok := s.authorizedSpecTaskForProposals(w, r, types.ActionGet)
	if !ok {
		return
	}
	proposals, err := s.Store.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{SpecTaskID: task.ID})
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	if proposals == nil {
		proposals = []*types.SpecTaskPRProposal{}
	}
	writeResponse(w, proposals, http.StatusOK)
}

// decideSpecTaskPRProposal godoc
// @Summary Approve or reject a pull request proposal
// @Description Approving grants the agent push rights to the proposal's head branch and opens the pull request as soon as the branch has commits beyond the base. Edited fields override the agent's proposal. Rejecting withdraws push rights.
// @Tags spec-tasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "SpecTask ID"
// @Param proposal_id path string true "Proposal ID"
// @Param request body types.PRProposalDecisionRequest true "Decision"
// @Success 200 {object} types.SpecTaskPRProposal
// @Router /api/v1/spec-tasks/{spec_task_id}/pr-proposals/{proposal_id}/decide [post]
// @Security BearerAuth
func (s *HelixAPIServer) decideSpecTaskPRProposal(w http.ResponseWriter, r *http.Request) {
	task, ok := s.authorizedSpecTaskForProposals(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	var req types.PRProposalDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrResponse(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Opening the PR pushes upstream and calls the provider; finish even if the
	// client goes away.
	ctx, cancel := detachContext(r.Context(), 2*time.Minute)
	defer cancel()

	proposalID := mux.Vars(r)["proposal_id"]
	existing, err := s.Store.GetSpecTaskPRProposal(ctx, proposalID)
	if err != nil || existing.SpecTaskID != task.ID {
		writeErrResponse(w, fmt.Errorf("proposal %s not found", proposalID), http.StatusNotFound)
		return
	}

	proposal, err := s.prProposals.Decide(ctx, getRequestUser(r), proposalID, &req)
	if err != nil {
		var oauthErr *services.OAuthRequiredError
		switch {
		case errors.As(err, &oauthErr):
			writeResponse(w, map[string]interface{}{
				"error":         "oauth_required",
				"message":       oauthErr.Error(),
				"provider_type": oauthErr.ProviderType,
			}, http.StatusUnprocessableEntity)
		case errors.Is(err, services.ErrPRProposalInvalid):
			writeErrResponse(w, err, http.StatusBadRequest)
		case errors.Is(err, services.ErrPRProposalConflict):
			writeErrResponse(w, err, http.StatusConflict)
		case errors.Is(err, store.ErrNotFound):
			writeErrResponse(w, err, http.StatusNotFound)
		default:
			writeErrResponse(w, err, http.StatusInternalServerError)
		}
		return
	}
	writeResponse(w, proposal, http.StatusOK)
}

func (s *HelixAPIServer) authorizedSpecTaskForProposals(w http.ResponseWriter, r *http.Request, action types.Action) (*types.SpecTask, bool) {
	ctx := r.Context()
	task, err := s.Store.GetSpecTask(ctx, mux.Vars(r)["spec_task_id"])
	if err != nil {
		writeErrResponse(w, fmt.Errorf("spec task not found"), http.StatusNotFound)
		return nil, false
	}
	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		writeErrResponse(w, fmt.Errorf("failed to get project: %w", err), http.StatusInternalServerError)
		return nil, false
	}
	if err := s.authorizeUserToProject(ctx, getRequestUser(r), project, action); err != nil {
		writeErrResponse(w, fmt.Errorf("not authorized"), http.StatusForbidden)
		return nil, false
	}
	return task, true
}
