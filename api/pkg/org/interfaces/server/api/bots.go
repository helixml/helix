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
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/eventsource"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/seedprompts"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ---- Nodes ---------------------------------------------------------------

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
	managersByReport := map[orgchart.NodeID][]string{}
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
		dto := botDTO(b, managersByReport[b.ID])
		a.applyCanonicalAgentProfile(ctx, b, &dto, "list org bots")
		dto.Status = "stopped"
		if a.deps.BotRuntime != nil {
			if info, err := a.deps.BotRuntime.State(ctx, orgID, b.ID); err == nil {
				if info.Status != "" {
					dto.Status = info.Status
				}
				dto.RestartRequired = info.RestartRequired
				dto.ProjectID = info.ProjectID
				dto.SessionID = info.SessionID
				dto.AgentRuntime = info.Runtime
				dto.AgentModel = info.Model
				applySandboxInfo(&dto, info)
			}
		}
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, out)
}

// createBot creates a Bot through the same lifecycle path the MCP
// create_bot tool drives (bot row + base-tool union, initial reporting
// line, topology reconcile, create-activation dispatch).
//
// @Summary Helix-org: create a bot
// @Description Create a Bot. Wraps the lifecycle Create so REST + chat creates share semantics (base-tool union, reporting line, transcript channel, create dispatch).
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
	requestedTools := toToolNames(req.Tools)
	if (req.Owner || mcptools.HasNonDefaultBotTool(requestedTools)) && !helixorgserver.CanManageOrganization(ctx) {
		writeError(w, http.StatusForbidden, errors.New("only organization owners and administrators can grant organization-management tools"))
		return
	}
	// A standard Bot receives the complete worker set plus any explicitly
	// requested additions. A manager receives that set plus the organization
	// control-plane mutations used to hire and manage other Nodes.
	tools := mcptools.MergeDefaultBotTools(requestedTools)
	if req.Owner {
		tools = mcptools.OwnerBotTools()
	}
	deferActivation := a.deps.Configs != nil && !a.deps.Configs.IsDefaultAgentConfigured(ctx, orgID)
	// REST and chat-driven creates share lifecycle.Create — one
	// implementation.
	res, err := a.deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:      strings.TrimSpace(req.ID),
		Name:    strings.TrimSpace(req.Name),
		Content: req.Content,
		Tools:   tools,

		Sources:         toTriggerSources(req.Triggers),
		ParentID:        orgchart.NodeID(strings.TrimSpace(req.ParentID)),
		PreserveContext: req.PreserveContext,
		SandboxRuntime:  string(req.SandboxRuntime),
		SandboxVCPUs:    sandboxVCPUs(req.SandboxResourceOverrides),
		DeferActivation: deferActivation,
		AgentConfig: lifecycle.AgentConfig{
			CodeAgentRuntime:        req.CodeAgentRuntime,
			CodeAgentCredentialType: req.CodeAgentCredentialType,
			Provider:                strings.TrimSpace(req.Provider),
			Model:                   strings.TrimSpace(req.Model),
			ReasoningEffort:         strings.TrimSpace(req.ReasoningEffort),
		},
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, CreateBotResponse{ID: string(res.Node.ID), ActivationID: string(res.ActivationID)})
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
	id := orgchart.NodeID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	b, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}

	dto := botDTO(b, a.managerIDs(ctx, orgID, id))
	a.applyCanonicalAgentProfile(ctx, b, &dto, "get org bot")
	// Seeded nodes carry their built-in prompt so the UI can offer a
	// reset to it; operator-created nodes have none and the field stays
	// empty, which is the UI's signal to hide the affordance.
	if def, ok := seedprompts.Default(id); ok {
		dto.DefaultInstructions = def
	}
	dto.Status = "stopped"
	// Populate the agent app id + project id from the helix-runtime
	// sidecar so the chart UI can deep-link "chat with bot" to the
	// per-project Human Desktop session. Missing state = the bot
	// hasn't activated yet; we leave the fields empty and the UI
	// shows a disabled button. Status drives the green/grey
	// presence control on the bot detail page.
	if a.deps.BotRuntime != nil {
		if info, err := a.deps.BotRuntime.State(ctx, orgID, id); err == nil {
			agentID := b.AgentID
			if agentID == "" {
				agentID = info.AgentID
			}
			detail := BotDetailDTO{Bot: dto, LegacyAppID: agentID, ProjectID: info.ProjectID}
			if info.Status != "" {
				detail.Bot.Status = info.Status
			}
			detail.Bot.RestartRequired = info.RestartRequired
			detail.Bot.ProjectID = info.ProjectID
			detail.Bot.SessionID = info.SessionID
			detail.Bot.AgentRuntime = info.Runtime
			detail.Bot.AgentModel = info.Model
			applySandboxInfo(&detail.Bot, info)
			writeJSON(w, http.StatusOK, detail)
			return
		}
	}
	writeJSON(w, http.StatusOK, BotDetailDTO{Bot: dto, LegacyAppID: b.AgentID})
}

// updateBot rewrites a Bot's content / tools. A nil field is
// left unchanged (a content-only edit preserves Tools).
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
	id := orgchart.NodeID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	var req UpdateBotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot for update: %w", err))
		return
	}
	var toolsPatch *[]tool.Name
	if req.Tools != nil {
		t := toToolNames(req.Tools)
		if (mcptools.HasNonDefaultBotTool(t) || mcptools.HasNonDefaultBotTool(existing.Tools)) && !helixorgserver.CanManageOrganization(ctx) {
			writeError(w, http.StatusForbidden, errors.New("only organization owners and administrators can modify organization-management tools"))
			return
		}
		toolsPatch = &t
	}
	namePatch := req.Name
	contentPatch := req.Content
	configPatch := AgentConfigPatch{
		CodeAgentRuntime:        req.CodeAgentRuntime,
		CodeAgentCredentialType: req.CodeAgentCredentialType,
		Provider:                req.Provider,
		Model:                   req.Model,
		ReasoningEffort:         req.ReasoningEffort,
	}
	canonicalChange := !configPatch.Empty() || namePatch != nil || contentPatch != nil
	if existing.AgentID != "" && canonicalChange && a.deps.AgentUpdater == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("canonical agent updater is not available"))
		return
	}
	var sandboxRuntimePatch *string
	if req.SandboxRuntime != nil {
		runtime := string(*req.SandboxRuntime)
		sandboxRuntimePatch = &runtime
	}
	var sandboxVCPUsPatch *int
	if req.SandboxResourceOverrides != nil {
		vcpus := req.SandboxResourceOverrides.VCPUs
		sandboxVCPUsPatch = &vcpus
	}
	projectIDsPatch := stringSlicePatch(req.ProjectIDs)
	updated := existing
	nodeChange := namePatch != nil || contentPatch != nil || toolsPatch != nil || projectIDsPatch != nil ||
		req.PreserveContext != nil || sandboxRuntimePatch != nil || sandboxVCPUsPatch != nil
	if nodeChange {
		updated, err = a.deps.Nodes.Update(ctx, orgID, id, nodes.UpdateParams{
			Name:            namePatch,
			Content:         contentPatch,
			Tools:           toolsPatch,
			ProjectIDs:      projectIDsPatch,
			PreserveContext: req.PreserveContext,
			SandboxRuntime:  sandboxRuntimePatch,
			SandboxVCPUs:    sandboxVCPUsPatch,
		})
		if err != nil {
			writeError(w, errStatus(err), fmt.Errorf("update bot: %w", err))
			return
		}
	}
	if updated.AgentID != "" && canonicalChange {
		if err := a.deps.AgentUpdater.UpdateAgent(ctx, updated.AgentID, configPatch, namePatch, contentPatch); err != nil {
			if nodeChange {
				rollback := nodes.UpdateParams{}
				if namePatch != nil {
					rollback.Name = &existing.Name
				}
				if contentPatch != nil {
					rollback.Content = &existing.Content
				}
				if toolsPatch != nil {
					tools := append([]tool.Name(nil), existing.Tools...)
					rollback.Tools = &tools
				}
				if projectIDsPatch != nil {
					projectIDs := append([]string(nil), existing.ProjectIDs...)
					rollback.ProjectIDs = &projectIDs
				}
				if req.PreserveContext != nil {
					preserveContext := existing.PreserveContext
					rollback.PreserveContext = &preserveContext
				}
				if sandboxRuntimePatch != nil {
					sandboxRuntime := existing.SandboxRuntime
					rollback.SandboxRuntime = &sandboxRuntime
				}
				if sandboxVCPUsPatch != nil {
					sandboxVCPUs := existing.SandboxVCPUs
					rollback.SandboxVCPUs = &sandboxVCPUs
				}
				_, rollbackErr := a.deps.Nodes.Update(ctx, orgID, id, rollback)
				if rollbackErr != nil {
					writeError(w, http.StatusInternalServerError, fmt.Errorf("update Bot App: %v; rollback Bot: %w", err, rollbackErr))
					return
				}
			}
			writeError(w, errStatus(err), fmt.Errorf("update Bot App: %w", err))
			return
		}
	}
	// The write already succeeded, so an unreadable App must not turn
	// into a 500 the client reads as "the update failed".
	dto := botDTO(updated, a.managerIDs(ctx, orgID, id))
	a.applyCanonicalAgentProfile(ctx, updated, &dto, "update org bot")
	writeJSON(w, http.StatusOK, dto)
}

// deleteBot tears down a Bot via the lifecycle service. Cascades the
// Helix app, runtime state, attachments, reporting lines, then the bot
// row. Its runtime-owned project is archived and repositories are preserved.
//
// @Summary Helix-org: delete a bot
// @Description Delete a Bot. Cascades: archives its runtime-owned project, detaches and deletes the Helix agent app, clears runtime state, drops attachments + reporting lines, then the bot row. Repositories and activations are preserved.
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
	id := orgchart.NodeID(r.PathValue("id"))
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
	id := orgchart.NodeID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	var req AddBotParentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	managerID := orgchart.NodeID(strings.TrimSpace(req.ParentID))
	if managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("parent_id is required"))
		return
	}
	// The service validates both endpoints, guards the DAG against
	// cycles, wires the line, and reconciles the transcript/team channels
	// the new edge implies — one place, shared invariants.
	switch err := a.deps.Nodes.AddParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, nodes.ErrReportingLinesUnavailable):
		writeError(w, http.StatusNotImplemented, err)
	case errors.Is(err, nodes.ErrReportingCycle):
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
	id := orgchart.NodeID(r.PathValue("id"))
	managerID := orgchart.NodeID(r.PathValue("parent_id"))
	if id == "" || managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id and parent_id are required"))
		return
	}
	// The service drops the line and reconciles the channels the dropped
	// edge implies (unsubscribe ex-manager from the report's activation
	// channel, remove report from the ex-manager's team chat).
	switch err := a.deps.Nodes.RemoveParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, nodes.ErrReportingLinesUnavailable):
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
	id := orgchart.NodeID(r.PathValue("id"))
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
	writeJSON(w, http.StatusOK, BotChatDTO{LegacyAppID: agentAppID, ProjectID: projectID})
}

// activateBot manually triggers an activation for a Bot. The bot
// page's "Start Desktop" button hits this so the full activation
// pipeline runs: ensureProject → ensureSession →
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
	id := orgchart.NodeID(r.PathValue("id"))
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
		LegacyAppID:  res.AgentID,
		SessionID:    res.SessionID,
	})
}

// stopBot stops the bot's desktop sandbox without deleting the
// session (transcript stays). The chart / bot-detail "Stop" control hits
// this. No-op (204) when there is no session or the desktop is already down.
// Delegates to activations.Stop — same path as the MCP stop_bot tool.
//
// @Summary Helix-org: stop a bot's desktop
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/stop [post]
func (a *apiHandler) stopBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.deps.Activations == nil {
		writeError(w, http.StatusNotImplemented, errors.New("stop is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.NodeID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	if _, err := a.deps.Queries.GetBot(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	if _, err := a.deps.Activations.Stop(ctx, orgID, id); err != nil {
		if errors.Is(err, activations.ErrStopUnavailable) {
			writeError(w, http.StatusNotImplemented, err)
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Errorf("stop bot %s desktop: %w", id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// restartBot gives the bot a genuinely fresh session. Delegates to activations.Restart (reset
// session then Activate) — same path as the MCP restart_bot tool.
//
// @Summary Helix-org: restart a bot (fresh session + desktop)
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 202 {object} api.BotActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/restart [post]
func (a *apiHandler) restartBot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.deps.Activations == nil {
		writeError(w, http.StatusNotImplemented, errors.New("restart is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.NodeID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("bot id is required"))
		return
	}
	if _, err := a.deps.Queries.GetBot(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	res, err := a.deps.Activations.Restart(ctx, orgID, id)
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
		LegacyAppID:  res.AgentID,
		SessionID:    res.SessionID,
	})
}

// ---- helpers ------------------------------------------------------------

// managerIDs returns the ids of the managers the given bot reports to,
// as strings, for embedding in a BotDTO. Returns nil on any store error
// — the reporting graph is best-effort context, never a reason to fail
// the whole bot read.
func (a *apiHandler) managerIDs(ctx context.Context, orgID string, id orgchart.NodeID) []string {
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

// botDTO converts an orgchart.Node to its wire form. parentIDs are the
// managers this Bot reports to (from the reporting lines); nil for a
// top-level Bot.
func botDTO(b orgchart.Node, parentIDs []string) BotDTO {
	dto := BotDTO{
		ID:              string(b.ID),
		LegacyAppID:     b.AgentID,
		Name:            b.Name,
		Content:         b.Content,
		ProjectIDs:      b.ProjectIDs,
		ParentIDs:       parentIDs,
		OrganizationID:  b.OrganizationID,
		PreserveContext: b.PreserveContext,
		SandboxRuntime:  types.SandboxRuntime(b.SandboxRuntime),
	}
	if b.SandboxVCPUs > 0 {
		dto.SandboxResourceOverrides = &types.SandboxResourceOverrides{VCPUs: b.SandboxVCPUs, MemoryMB: b.SandboxMemoryMB}
	}
	// The Bot's own execution config is the base profile. It is what the
	// DTO carries once the legacy App link is gone, and what the handlers
	// serve when that App can't be read.
	if b.CodeAgentConfig != nil {
		dto.CodeAgentRuntime = b.CodeAgentConfig.Runtime
		dto.CodeAgentCredentialType = b.CodeAgentConfig.CredentialType
		dto.Provider = b.CodeAgentConfig.ProviderRef
		dto.Model = b.CodeAgentConfig.Model
		dto.ReasoningEffort = b.CodeAgentConfig.ReasoningEffort
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
	return dto
}

// applyCanonicalAgentProfile overlays the legacy App profile onto dto
// while a Bot still has one. A read failure is not fatal: botDTO has
// already filled dto from the Bot's own name, instructions and
// CodeAgentConfig, which is the source of truth after the cutover, so
// every handler serves that rather than failing the whole request over
// one unreadable App.
func (a *apiHandler) applyCanonicalAgentProfile(ctx context.Context, bot orgchart.Node, dto *BotDTO, op string) {
	if err := a.canonicalAgentProfile(ctx, bot, dto); err != nil {
		log.Warn().Err(err).Str("org", bot.OrganizationID).Str("bot", string(bot.ID)).
			Msg(op + ": using Bot-owned profile because the legacy App profile is unavailable")
	}
}

func (a *apiHandler) canonicalAgentProfile(ctx context.Context, bot orgchart.Node, dto *BotDTO) error {
	if dto == nil || bot.AgentID == "" || a.deps.AgentReader == nil {
		return nil
	}
	profile, err := a.deps.AgentReader.ReadAgent(ctx, bot.AgentID)
	if err != nil {
		return fmt.Errorf("read canonical agent %s for bot %s: %w", bot.AgentID, bot.ID, err)
	}
	dto.Name = profile.Name
	dto.Content = profile.Instructions
	dto.CodeAgentRuntime = profile.CodeAgentRuntime
	dto.CodeAgentCredentialType = profile.CodeAgentCredentialType
	dto.Provider = profile.Provider
	dto.Model = profile.Model
	dto.ReasoningEffort = profile.ReasoningEffort
	return nil
}

func stringSlicePatch(in []string) *[]string {
	if in == nil {
		return nil
	}
	return &in
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

// toTriggerSources turns the create request's Trigger ids into terminal
// source references. Attaching to a Processor branch at creation goes
// through the attachment endpoints, not this shorthand.
func toTriggerSources(in []string) []eventsource.SourceRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]eventsource.SourceRef, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, eventsource.Trigger(t))
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

// sandboxVCPUs reads the vCPU count out of an optional resource override;
// memory is never accepted independently (the preset ladder fixes it).
func sandboxVCPUs(overrides *types.SandboxResourceOverrides) int {
	if overrides == nil {
		return 0
	}
	return overrides.VCPUs
}

// applySandboxInfo copies the runtime sidecar's resolved sandbox view onto
// the wire DTO. Shared by the list and detail handlers so the two never
// disagree about which fields carry the effective launch config.
func applySandboxInfo(dto *BotDTO, info BotRuntimeInfo) {
	dto.EffectiveSandboxRuntime = info.EffectiveSandboxRuntime
	dto.EffectiveSandboxResourceOverrides = info.EffectiveSandboxResources
	dto.SandboxID = info.SandboxID
	dto.SandboxStatus = info.SandboxStatus
	dto.SandboxStatusMessage = info.SandboxStatusMessage
}
