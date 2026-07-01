package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// ---- Bots ---------------------------------------------------------------

// listBots returns every Bot row, each with its tools and the managers
// it reports to.
//
// @Summary Helix-org: list bots
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.BotDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots [get]
func (a *apiHandler) listBots(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	bs, err := a.deps.Queries.ListBots(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list bots: %w", err))
		return
	}
	// One List call builds the report → managers index so each bot's
	// parent_ids don't cost a query.
	managersByReport := map[orgchart.BotID][]string{}
	if a.deps.Queries.ReportingLinesWired() {
		lines, err := a.deps.Queries.ListReportingLines(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list reporting lines: %w", err))
			return
		}
		for _, l := range lines {
			managersByReport[l.ReportID] = append(managersByReport[l.ReportID], string(l.ManagerID))
		}
	}
	out := make([]BotDTO, 0, len(bs))
	for _, b := range bs {
		out = append(out, botDTO(b, managersByReport[b.ID]))
	}
	writeJSON(w, http.StatusOK, out)
}

// createBot creates a Bot through the same lifecycle path the MCP
// create_bot tool drives (bot row + base-tool union, initial reporting
// line, topology reconcile, create-activation dispatch).
//
// @Summary Helix-org: create a bot
// @Description Create a Bot. Wraps the lifecycle Create so REST + chat creates share semantics (base-tool union, reporting line, transcript topics, create dispatch).
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param org path string true "Organization slug or id"
// @Param payload body api.CreateBotRequest true "Bot spec"
// @Success 201 {object} api.CreateBotResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots [post]
func (a *apiHandler) createBot(w http.ResponseWriter, r *http.Request) {
	if a.deps.Lifecycle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("create is not wired in this deployment"))
		return
	}
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateBotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, errors.New("content is required"))
		return
	}
	// REST and chat-driven creates share lifecycle.Create — one
	// implementation. The base read tools are unioned by the bots
	// service so a "New Bot" dialog with no tools picker still gets a
	// usable MCP surface.
	res, err := a.deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:              strings.TrimSpace(req.ID),
		Content:         req.Content,
		Tools:           toToolNames(req.Tools),
		Topics:          toTopicIDs(req.Topics),
		ParentID:        orgchart.BotID(strings.TrimSpace(req.ParentID)),
		PreserveContext: req.PreserveContext,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, CreateBotResponse{ID: string(res.Bot.ID), ActivationID: string(res.ActivationID)})
}

// getBot returns one Bot + the surrounding runtime context.
//
// @Summary Helix-org: get bot detail
// @Tags HelixOrg
// @Produce json
// @Param id path string true "Bot ID"
// @Success 200 {object} api.BotDetailDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id} [get]
func (a *apiHandler) getBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	b, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}

	detail := BotDetailDTO{Bot: botDTO(b, a.managerIDs(ctx, orgID, id))}
	// Populate the agent app id + project id from the helix-runtime
	// sidecar so the chart UI can deep-link "chat with bot" to the
	// per-project Human Desktop session. Missing state = the bot
	// hasn't activated yet; we leave the fields empty and the UI
	// shows a disabled button.
	if a.deps.BotRuntime != nil {
		if info, err := a.deps.BotRuntime.State(ctx, orgID, id); err == nil {
			detail.AgentAppID = info.AgentAppID
			detail.ProjectID = info.ProjectID
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

// updateBot rewrites a Bot's content / tools / topics. A nil field is
// left unchanged (content-only edit preserves Tools/Topics).
//
// @Summary Helix-org: update a bot
// @Tags HelixOrg
// @Accept json
// @Param org path string true "Organization slug or id"
// @Param id path string true "Bot ID"
// @Param payload body api.UpdateBotRequest true "Patch fields"
// @Success 200 {object} api.BotDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id} [patch]
func (a *apiHandler) updateBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	var req UpdateBotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var toolsPatch *[]tool.Name
	if req.Tools != nil {
		t := toToolNames(req.Tools)
		toolsPatch = &t
	}
	var topicsPatch *[]streaming.TopicID
	if req.Topics != nil {
		s := toTopicIDs(req.Topics)
		topicsPatch = &s
	}
	updated, err := a.deps.Bots.Update(ctx, orgID, id, bots.UpdateParams{
		Content:         req.Content,
		Tools:           toolsPatch,
		Topics:          topicsPatch,
		PreserveContext: req.PreserveContext,
	})
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update bot: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, botDTO(updated, a.managerIDs(ctx, orgID, id)))
}

// deleteBot tears down a Bot via the lifecycle service. Cascades the
// Helix project + app, runtime state, subscriptions, reporting lines,
// then the bot row.
//
// @Summary Helix-org: delete a bot
// @Description Delete a Bot. Cascades: stops sessions, deletes the Helix project + agent app, clears runtime state, drops subscriptions + reporting lines, then the bot row. Activations are preserved as audit.
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id} [delete]
func (a *apiHandler) deleteBot(w http.ResponseWriter, r *http.Request) {
	if a.deps.Lifecycle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("delete is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	switch err := a.deps.Lifecycle.Delete(r.Context(), orgID, id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, errStatus(err), err)
	}
}

// addBotParent adds a reporting line: the Bot at {id} now also reports
// to the manager in the body. Reporting is many-to-many, so this is
// additive — a Bot can report to several managers. The chart UI calls
// it when an accountability edge is drawn between two Bot nodes.
//
// Validation:
//   - the manager must reference a Bot that exists in the org
//   - the manager must not already be a descendant of {id}, which
//     would close a reporting cycle (the graph is a DAG)
//
// Idempotent: re-adding an existing line returns 204.
//
// @Summary Helix-org: add a bot reporting line (manager)
// @Tags HelixOrg
// @Accept json
// @Param id path string true "Bot ID (the report)"
// @Param payload body api.AddBotParentRequest true "Manager bot id"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/parents [post]
func (a *apiHandler) addBotParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	var req AddBotParentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	managerID := orgchart.BotID(strings.TrimSpace(req.ParentID))
	if managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("parent_id is required"))
		return
	}
	// The service validates both endpoints, guards the DAG against
	// cycles, wires the line, and reconciles the activation/team Topics
	// the new edge implies — one place, shared invariants.
	switch err := a.deps.Bots.AddParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, bots.ErrReportingLinesUnavailable):
		writeError(w, http.StatusNotImplemented, err)
	case errors.Is(err, bots.ErrReportingCycle):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, errStatus(err), err)
	}
}

// removeBotParent drops one reporting line: the Bot at {id} no longer
// reports to {parent_id}. The chart UI calls it when an accountability
// edge is deleted. Returns 404 when no such line exists.
//
// @Summary Helix-org: remove a bot reporting line (manager)
// @Tags HelixOrg
// @Param id path string true "Bot ID (the report)"
// @Param parent_id path string true "Manager bot id"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/parents/{parent_id} [delete]
func (a *apiHandler) removeBotParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	managerID := orgchart.BotID(r.PathValue("parent_id"))
	if id == "" || managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id and parent_id are required"))
		return
	}
	// The service drops the line and reconciles the Topics the dropped
	// edge implies (unsubscribe ex-manager from the report's activation
	// topic, remove report from the ex-manager's team topic).
	switch err := a.deps.Bots.RemoveParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, bots.ErrReportingLinesUnavailable):
		writeError(w, http.StatusNotImplemented, err)
	default:
		writeError(w, errStatus(err), err)
	}
}

// ensureBotChat provisions (or fast-paths) the Bot's per-Bot Helix
// project + agent app, then returns the agent_app_id so the chart UI
// can deep-link to /agent/<app_id>.
//
// Idempotent — BotProject.Ensure fast-paths when the project already
// exists.
//
// @Summary Helix-org: provision a per-bot chat app
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 200 {object} api.BotChatDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/chat [post]
func (a *apiHandler) ensureBotChat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.deps.ProjectEnsurer == nil {
		writeError(w, http.StatusNotImplemented, errors.New("project ensurer not wired"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	if _, err := a.deps.Queries.GetBot(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	projectID, agentAppID, _, err := a.deps.ProjectEnsurer.Ensure(ctx, orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("ensure bot chat: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, BotChatDTO{AgentAppID: agentAppID, ProjectID: projectID})
}

// activateBot manually triggers an activation for a Bot. The bot
// page's "Start Desktop" button hits this so the full activation
// pipeline runs: ensureProject → AttachHelixOrgMCP → ensureSession →
// Helix spins up the desktop container as part of session start.
//
// Synchronous up to ensureProject so the response carries the project +
// agent_app IDs the UI needs. The session-start work runs async on the
// per-Bot queue inside the dispatcher.
//
// @Summary Helix-org: manually trigger a bot activation
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 202 {object} api.BotActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/activate [post]
func (a *apiHandler) activateBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.deps.Activations == nil {
		writeError(w, http.StatusNotImplemented, errors.New("activate is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	// Confirm the Bot exists for a clean 404 before the activate
	// command runs its project/dispatch side effects.
	if _, err := a.deps.Queries.GetBot(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	res, err := a.deps.Activations.Activate(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, activations.ErrActivateUnavailable) {
			writeError(w, http.StatusNotImplemented, err)
			return
		}
		writeError(w, errStatus(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, BotActivateDTO{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		AgentAppID:   res.AgentAppID,
		SessionID:    res.SessionID,
	})
}

// restartBotAgent recreates the bot's desktop container from scratch —
// the bot-page "Restart agent session" button. Unlike activateBot
// (which continues the existing session and so cannot recover a stuck
// container), this resolves the bot's current session and delegates to
// the shared backend restart primitive (StopDesktop → recreate → reset
// crashed prompts). If the bot has no live session yet, it falls back
// to a normal activation so first-time start still works.
//
// @Summary Helix-org: restart a bot's agent session (recreate desktop container)
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 202 {object} api.BotActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/restart-agent [post]
func (a *apiHandler) restartBotAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	// Confirm the Bot exists for a clean 404 before any side effects.
	if _, err := a.deps.Queries.GetBot(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}

	// Resolve the bot's current desktop session. Empty means the bot has
	// never activated — there's no container to recreate, so fall through
	// to a normal activation (which starts a fresh one).
	var sessionID string
	if a.deps.BotRuntime != nil {
		if info, err := a.deps.BotRuntime.State(ctx, orgID, id); err == nil {
			sessionID = info.SessionID
		}
	}

	if sessionID != "" && a.deps.SessionRestarter != nil {
		if err := a.deps.SessionRestarter.RestartSession(ctx, sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("restart bot %s session: %w", id, err))
			return
		}
		writeJSON(w, http.StatusAccepted, BotActivateDTO{SessionID: sessionID})
		return
	}

	// No live session (or restarter unwired): fall back to a normal
	// activation, which provisions the project and starts a fresh session.
	if a.deps.Activations == nil {
		writeError(w, http.StatusNotImplemented, errors.New("restart is not wired in this deployment"))
		return
	}
	res, err := a.deps.Activations.Activate(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, activations.ErrActivateUnavailable) {
			writeError(w, http.StatusNotImplemented, err)
			return
		}
		writeError(w, errStatus(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, BotActivateDTO{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		AgentAppID:   res.AgentAppID,
		SessionID:    res.SessionID,
	})
}

// ---- helpers ------------------------------------------------------------

// managerIDs returns the ids of the managers the given bot reports to,
// as strings, for embedding in a BotDTO. Returns nil on any store error
// — the reporting graph is best-effort context, never a reason to fail
// the whole bot read.
func (a *apiHandler) managerIDs(ctx context.Context, orgID string, id orgchart.BotID) []string {
	if !a.deps.Queries.ReportingLinesWired() {
		return nil
	}
	managers, err := a.deps.Queries.ListManagers(ctx, orgID, id)
	if err != nil || len(managers) == 0 {
		return nil
	}
	out := make([]string, 0, len(managers))
	for _, m := range managers {
		out = append(out, string(m))
	}
	return out
}

// botDTO converts an orgchart.Bot to its wire form. parentIDs are the
// managers this Bot reports to (from the reporting lines); nil for a
// top-level Bot.
func botDTO(b orgchart.Bot, parentIDs []string) BotDTO {
	dto := BotDTO{
		ID:              string(b.ID),
		Content:         b.Content,
		ParentIDs:       parentIDs,
		OrganizationID:  b.OrganizationID,
		PreserveContext: b.PreserveContext,
	}
	if !b.CreatedAt.IsZero() {
		dto.CreatedAt = b.CreatedAt.Format(time.RFC3339)
	}
	if !b.UpdatedAt.IsZero() {
		dto.UpdatedAt = b.UpdatedAt.Format(time.RFC3339)
	}
	tools := make([]string, 0, len(b.Tools))
	for _, t := range b.Tools {
		tools = append(tools, string(t))
	}
	sort.Strings(tools)
	dto.Tools = tools
	for _, s := range b.Topics {
		dto.Topics = append(dto.Topics, string(s))
	}
	return dto
}

func toToolNames(in []string) []tool.Name {
	if len(in) == 0 {
		return nil
	}
	out := make([]tool.Name, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, tool.Name(t))
		}
	}
	return out
}

func toTopicIDs(in []string) []streaming.TopicID {
	if len(in) == 0 {
		return nil
	}
	out := make([]streaming.TopicID, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, streaming.TopicID(t))
		}
	}
	return out
}

// listTools returns the catalogue of available MCP tools that can be
// listed on a Bot. Powers the bot editor's multi-select.
//
// @Summary Helix-org: list available MCP tools
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.ToolDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/tools [get]
func (a *apiHandler) listTools(w http.ResponseWriter, r *http.Request) {
	out := make([]ToolDTO, 0)
	if a.deps.Tools != nil {
		for _, t := range a.deps.Tools.List() {
			out = append(out, ToolDTO{
				Name:        string(t.Name()),
				Description: t.Description(),
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
