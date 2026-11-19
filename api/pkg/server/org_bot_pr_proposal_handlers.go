package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// orgBotRef identifies the org bot a request is about, once authorized.
type orgBotRef struct {
	OrgID string
	BotID orgchart.NodeID
}

// OrgBotPRAutoApproveRequest turns auto-approval of a bot's PR proposals on or off.
type OrgBotPRAutoApproveRequest struct {
	Enabled bool `json:"enabled"`
}

// OrgBotKeepAliveRequest turns a bot's Keep Alive on or off.
type OrgBotKeepAliveRequest struct {
	KeepAlive bool `json:"keep_alive"`
}

// listOrgBotPRProposals godoc
// @Summary List an org bot's pull request proposals
// @Description An org bot opens pull requests (e.g. to its own playbook) only through proposals a user approves.
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or name"
// @Param id path string true "Bot ID"
// @Success 200 {array} types.SpecTaskPRProposal
// @Router /api/v1/orgs/{org}/bots/{id}/pr-proposals [get]
// @Security BearerAuth
func (s *HelixAPIServer) listOrgBotPRProposals(w http.ResponseWriter, r *http.Request) {
	bot, ok := s.authorizedOrgBotForProposals(w, r, types.ActionGet)
	if !ok {
		return
	}
	proposals, err := s.prProposals.ListForOrgBot(r.Context(), bot.OrgID, string(bot.BotID))
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	if proposals == nil {
		proposals = []*types.SpecTaskPRProposal{}
	}
	writeResponse(w, proposals, http.StatusOK)
}

// decideOrgBotPRProposal godoc
// @Summary Approve or reject an org bot's pull request proposal
// @Description Approving opens the pull request as soon as the branch has commits beyond the base. Edited fields override the bot's proposal. A bot's own key cannot decide.
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization ID or name"
// @Param id path string true "Bot ID"
// @Param proposal_id path string true "Proposal ID"
// @Param request body types.PRProposalDecisionRequest true "Decision"
// @Success 200 {object} types.SpecTaskPRProposal
// @Router /api/v1/orgs/{org}/bots/{id}/pr-proposals/{proposal_id}/decide [post]
// @Security BearerAuth
func (s *HelixAPIServer) decideOrgBotPRProposal(w http.ResponseWriter, r *http.Request) {
	if isAgentScopedKey(getRequestUser(r)) {
		writeErrResponse(w, errAgentKeyDecision, http.StatusForbidden)
		return
	}
	bot, ok := s.authorizedOrgBotForProposals(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	var req types.PRProposalDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrResponse(w, jsonDecodeHTTPError(err), http.StatusBadRequest)
		return
	}
	// Opening the PR pushes upstream and calls the provider; finish even if
	// the client goes away.
	ctx, cancel := detachContext(r.Context(), 2*time.Minute)
	defer cancel()

	proposalID := mux.Vars(r)["proposal_id"]
	existing, err := s.Store.GetSpecTaskPRProposal(ctx, proposalID)
	if err != nil || existing.OrgBotID != string(bot.BotID) || existing.OrganizationID != bot.OrgID {
		writeErrResponse(w, fmt.Errorf("proposal %s not found", proposalID), http.StatusNotFound)
		return
	}
	proposal, err := s.prProposals.Decide(ctx, getRequestUser(r), proposalID, &req)
	if err != nil {
		writePRProposalError(w, err)
		return
	}
	writeResponse(w, proposal, http.StatusOK)
}

// setOrgBotPRAutoApprove godoc
// @Summary Turn auto-approval of an org bot's pull request proposals on or off
// @Description When on, the bot's proposals are approved as the user who turned it on. A bot's own key cannot change it.
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization ID or name"
// @Param id path string true "Bot ID"
// @Param request body OrgBotPRAutoApproveRequest true "Setting"
// @Success 204
// @Router /api/v1/orgs/{org}/bots/{id}/pr-auto-approve [put]
// @Security BearerAuth
func (s *HelixAPIServer) setOrgBotPRAutoApprove(w http.ResponseWriter, r *http.Request) {
	if isAgentScopedKey(getRequestUser(r)) {
		writeErrResponse(w, errAgentKeyDecision, http.StatusForbidden)
		return
	}
	bot, ok := s.authorizedOrgBotForProposals(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	var req OrgBotPRAutoApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrResponse(w, jsonDecodeHTTPError(err), http.StatusBadRequest)
		return
	}
	if err := s.prProposals.SetOrgBotAutoApprove(r.Context(), bot.OrgID, string(bot.BotID), req.Enabled, getRequestUser(r).ID); err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// setOrgBotKeepAlive godoc
// @Summary Turn an org bot's Keep Alive on or off
// @Description When on, the bot's desktop is not shut down when idle. A bot's own key cannot change it.
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization ID or name"
// @Param id path string true "Bot ID"
// @Param request body OrgBotKeepAliveRequest true "Setting"
// @Success 204
// @Router /api/v1/orgs/{org}/bots/{id}/keep-alive [put]
// @Security BearerAuth
func (s *HelixAPIServer) setOrgBotKeepAlive(w http.ResponseWriter, r *http.Request) {
	if isAgentScopedKey(getRequestUser(r)) {
		writeErrResponse(w, errAgentKeyDecision, http.StatusForbidden)
		return
	}
	bot, ok := s.authorizedOrgBotForProposals(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	var req OrgBotKeepAliveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrResponse(w, jsonDecodeHTTPError(err), http.StatusBadRequest)
		return
	}
	if err := s.helixOrg.store.Nodes.UpdateKeepAlive(r.Context(), bot.OrgID, bot.BotID, req.KeepAlive, time.Now()); err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// authorizedOrgBotForProposals resolves {org} and {id} to an org bot and
// authorizes the caller against that bot's project, as spec-task proposals
// authorize against the task's project. Every failure to find or reach the bot
// is the same 404, so another tenant's bots cannot be probed.
func (s *HelixAPIServer) authorizedOrgBotForProposals(w http.ResponseWriter, r *http.Request, action types.Action) (orgBotRef, bool) {
	ctx := r.Context()
	notFound := func() (orgBotRef, bool) {
		writeErrResponse(w, errors.New("bot not found"), http.StatusNotFound)
		return orgBotRef{}, false
	}
	org, err := s.lookupOrg(ctx, mux.Vars(r)["org"])
	if err != nil || org == nil {
		return notFound()
	}
	if _, err := s.authorizeOrgMember(ctx, getRequestUser(r), org.ID); err != nil {
		return notFound()
	}
	if s.prProposals == nil || s.helixOrg == nil || s.helixOrg.store == nil {
		writeErrResponse(w, errors.New("org bots are not available on this server"), http.StatusNotImplemented)
		return orgBotRef{}, false
	}
	botID := orgchart.NodeID(mux.Vars(r)["id"])
	if _, err := s.helixOrg.store.Nodes.Get(ctx, org.ID, botID); err != nil {
		return notFound()
	}
	state, err := runtimehelix.LoadState(ctx, s.helixOrg.store, org.ID, botID)
	if err != nil || state.ProjectID == "" {
		return notFound()
	}
	project, err := s.Store.GetProject(ctx, state.ProjectID)
	if err != nil || project.OrganizationID != org.ID {
		return notFound()
	}
	if err := s.authorizeUserToProject(ctx, getRequestUser(r), project, action); err != nil {
		return notFound()
	}
	return orgBotRef{OrgID: org.ID, BotID: botID}, true
}

func writePRProposalError(w http.ResponseWriter, err error) {
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
}
