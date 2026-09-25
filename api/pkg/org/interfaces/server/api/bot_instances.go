package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/instances"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

// BotInstanceDTO is one instance of a Bot.
type BotInstanceDTO struct {
	SessionID      string               `json:"session_id"`
	BotID          string               `json:"bot_id"`
	Name           string               `json:"name"`
	SandboxRuntime types.SandboxRuntime `json:"sandbox_runtime"`
	// SandboxStatus is the sandbox's external agent status: "" (stopped),
	// "starting", "running", "restarting", "terminated_idle" …
	SandboxStatus string `json:"sandbox_status,omitempty"`
	Owner         string `json:"owner"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// CreateBotInstanceRequest is the body of POST /bots/{id}/instances.
type CreateBotInstanceRequest struct {
	Name string `json:"name,omitempty"`
	// SandboxRuntime overrides the Bot's instance profile runtime:
	// "headless-ubuntu" or "ubuntu-desktop".
	SandboxRuntime types.SandboxRuntime `json:"sandbox_runtime,omitempty"`
	// Message is queued as the instance's first turn.
	Message string `json:"message,omitempty"`
}

func botInstanceDTO(session *types.Session) BotInstanceDTO {
	return BotInstanceDTO{
		SessionID:      session.ID,
		BotID:          session.Metadata.OrgWorkerID,
		Name:           session.Name,
		SandboxRuntime: session.Metadata.SandboxRuntime,
		SandboxStatus:  session.Metadata.ExternalAgentStatus,
		Owner:          session.Owner,
		CreatedAt:      session.Created.Format(time.RFC3339),
		UpdatedAt:      session.Updated.Format(time.RFC3339),
	}
}

// botInstancesPort returns the port or writes 501 when it isn't wired.
func (a *apiHandler) botInstancesPort(w http.ResponseWriter) (instances.Manager, bool) {
	if a.deps.BotInstances == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bot instances are not wired in this deployment"))
		return nil, false
	}
	return a.deps.BotInstances, true
}

// listBotInstances lists a Bot's instances, newest first.
//
// @Summary Helix-org: list a bot's instances
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 200 {array} api.BotInstanceDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/instances [get]
func (a *apiHandler) listBotInstances(w http.ResponseWriter, r *http.Request) {
	manager, ok := a.botInstancesPort(w)
	if !ok {
		return
	}
	orgID, botID, ok := a.botPath(w, r)
	if !ok {
		return
	}
	sessions, err := manager.List(r.Context(), orgID, botID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("list instances of bot %s: %w", botID, err))
		return
	}
	out := make([]BotInstanceDTO, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, botInstanceDTO(session))
	}
	writeJSON(w, http.StatusOK, out)
}

// createBotInstance starts a new instance of a Bot. The sandbox starts
// asynchronously; the response carries the new session.
//
// @Summary Helix-org: create a bot instance
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Param request body api.CreateBotInstanceRequest true "Instance"
// @Success 201 {object} api.BotInstanceDTO
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/instances [post]
func (a *apiHandler) createBotInstance(w http.ResponseWriter, r *http.Request) {
	manager, ok := a.botInstancesPort(w)
	if !ok {
		return
	}
	orgID, botID, ok := a.botPath(w, r)
	if !ok {
		return
	}
	var req CreateBotInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	session, err := manager.Create(r.Context(), orgID, botID, instances.Params(req))
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("create instance of bot %s: %w", botID, err))
		return
	}
	writeJSON(w, http.StatusCreated, botInstanceDTO(session))
}

// deleteBotInstance deletes an instance: its sandbox, its workspace and its
// session. The Bot and its other instances are untouched.
//
// @Summary Helix-org: delete a bot instance
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Param session_id path string true "Instance session ID"
// @Success 204
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/instances/{session_id} [delete]
func (a *apiHandler) deleteBotInstance(w http.ResponseWriter, r *http.Request) {
	manager, ok := a.botInstancesPort(w)
	if !ok {
		return
	}
	orgID, botID, ok := a.botPath(w, r)
	if !ok {
		return
	}
	sessionID := r.PathValue("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, errors.New("session id is required"))
		return
	}
	if err := manager.Delete(r.Context(), orgID, botID, sessionID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete instance %s of bot %s: %w", sessionID, botID, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// botPath resolves the org and bot from the request path and confirms the
// Bot exists, writing the error response when it can't.
func (a *apiHandler) botPath(w http.ResponseWriter, r *http.Request) (string, orgchart.NodeID, bool) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return "", "", false
	}
	botID := orgchart.NodeID(r.PathValue("id"))
	if botID == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return "", "", false
	}
	if _, err := a.deps.Queries.GetBot(r.Context(), orgID, botID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", botID, err))
		return "", "", false
	}
	return orgID, botID, true
}
