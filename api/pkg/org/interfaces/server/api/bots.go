package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
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
	"github.com/helixml/helix/api/pkg/system"
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
	bs, err := a.listLiveBots(ctx, orgID)
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
			if a.deps.Lifecycle.Deleting(orgID, l.ManagerID) {
				continue
			}
			managersByReport[l.ReportID] = append(managersByReport[l.ReportID], string(l.ManagerID))
		}
	}
	editable := a.editableBots(ctx, orgID, bs)
	out := make([]BotDTO, 0, len(bs))
	for _, b := range bs {
		dto := botDTO(b, managersByReport[b.ID])
		dto.CanEdit = editable[b.ID]
		a.applyCanonicalAgentProfile(ctx, b, &dto, "list org bots")
		dto.Status = "stopped"
		if a.deps.BotRuntime != nil {
			info, err := a.deps.BotRuntime.State(ctx, orgID, b.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Errorf("get bot %s runtime state: %w", b.ID, err))
				return
			}
			if info.Status != "" {
				dto.Status = info.Status
			}
			dto.AgentWorkState = info.AgentWorkState
			dto.RestartRequired = info.RestartRequired
			dto.ProjectID = info.ProjectID
			dto.SessionID = info.SessionID
			dto.AgentRuntime = info.Runtime
			dto.AgentModel = info.Model
			applySandboxInfo(&dto, info)
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
	// A standard Bot receives the complete worker set plus any explicitly
	// requested additions. A manager receives that set plus the organization
	// control-plane mutations used to hire and manage other Nodes.
	tools := mcptools.MergeDefaultBotTools(requestedTools)
	if req.Owner {
		tools = mcptools.OwnerBotTools()
	}
	if parentID := strings.TrimSpace(req.ParentID); parentID != "" && !a.authorizeBotEdit(w, r, orgID, orgchart.NodeID(parentID), "reports") {
		return
	}
	deferActivation := a.deps.Configs != nil && !a.deps.Configs.IsDefaultAgentConfigured(ctx, orgID)
	// REST and chat-driven creates share lifecycle.Create — one
	// implementation.
	res, err := a.deps.Lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:      strings.TrimSpace(req.ID),
		Name:    strings.TrimSpace(req.Name),
		Content: req.Content,
		Tools:   tools,
		Grantor: restGrantor(ctx),

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
	if errors.Is(err, nodes.ErrToolGrantForbidden) {
		writeError(w, http.StatusForbidden, err)
		return
	}
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
	if a.deps.Lifecycle.Deleting(orgID, id) {
		writeError(w, http.StatusNotFound, fmt.Errorf("get bot %s: being deleted", id))
		return
	}
	b, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}

	dto := botDTO(b, a.managerIDs(ctx, orgID, id))
	dto.CanEdit = a.botEditAllowed(ctx, orgID, b)
	a.applyCanonicalAgentProfile(ctx, b, &dto, "get org bot")
	if a.deps.Activations != nil {
		dto.ActivationError, err = a.deps.Activations.LatestError(ctx, orgID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get bot %s activation status: %w", id, err))
			return
		}
		dto.ActivationError = displayActivationError(dto.ActivationError)
	}
	// Seeded nodes carry their built-in prompt so the UI can offer a
	// reset to it; operator-created nodes have none and the field stays
	// empty, which is the UI's signal to hide the affordance.
	if def, ok := seedprompts.Default(id); ok {
		dto.DefaultInstructions = def
	}
	dto.Status = "stopped"
	// Populate the agent app id + project id from the helix-runtime
	// sidecar so the chart UI can deep-link "chat with bot" to the
	// per-project Human Desktop session. Status drives the green/grey
	// presence control on the bot detail page.
	if a.deps.BotRuntime != nil {
		info, err := a.deps.BotRuntime.State(ctx, orgID, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("get bot %s runtime state: %w", id, err))
			return
		}
		agentID := b.AgentID
		if agentID == "" {
			agentID = info.AgentID
		}
		detail := BotDetailDTO{Bot: dto, LegacyAppID: agentID, ProjectID: info.ProjectID}
		if info.Status != "" {
			detail.Bot.Status = info.Status
		}
		detail.Bot.AgentWorkState = info.AgentWorkState
		detail.Bot.RestartRequired = info.RestartRequired
		detail.Bot.ProjectID = info.ProjectID
		detail.Bot.SessionID = info.SessionID
		detail.Bot.AgentRuntime = info.Runtime
		detail.Bot.AgentModel = info.Model
		applySandboxInfo(&detail.Bot, info)
		writeJSON(w, http.StatusOK, detail)
		return
	}
	writeJSON(w, http.StatusOK, BotDetailDTO{Bot: dto, LegacyAppID: b.AgentID})
}

func displayActivationError(err string) string {
	if strings.Contains(err, "activation prompt ") && strings.Contains(err, " was deleted") {
		return "The previous task was cancelled before the bot could run. Start the bot, then send it again."
	}
	if strings.Contains(err, "agent turn aborted") && system.Has5xxStatusCode(err) {
		return "The AI model was temporarily unavailable, so this task did not run. The bot will try again automatically when the model is available."
	}
	return err
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
// @Failure 403 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
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
	if !a.authorizeLoadedBotEdit(w, r, orgID, existing, "configuration") {
		return
	}
	if req.ProjectIDs != nil && !a.authorizeBotProjects(w, r, orgID, existing, req.ProjectIDs) {
		return
	}
	var toolsPatch *[]tool.Name
	if req.Tools != nil {
		t := toToolNames(req.Tools)
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
	grantor := restGrantor(ctx)
	updated := existing
	nodeChange := namePatch != nil || contentPatch != nil || toolsPatch != nil || projectIDsPatch != nil ||
		req.PreserveContext != nil || sandboxRuntimePatch != nil || sandboxVCPUsPatch != nil || req.InstanceProfile != nil
	if nodeChange {
		updated, err = a.deps.Nodes.Update(ctx, orgID, id, nodes.UpdateParams{
			Name:            namePatch,
			Content:         contentPatch,
			Tools:           toolsPatch,
			ProjectIDs:      projectIDsPatch,
			PreserveContext: req.PreserveContext,
			SandboxRuntime:  sandboxRuntimePatch,
			SandboxVCPUs:    sandboxVCPUsPatch,
			InstanceProfile: req.InstanceProfile,
			Grantor:         grantor,
		})
		if err != nil {
			writeError(w, errStatus(err), fmt.Errorf("update bot: %w", err))
			return
		}
	}
	if updated.AgentID != "" && canonicalChange {
		if err := a.deps.AgentUpdater.UpdateAgent(ctx, updated.AgentID, configPatch, namePatch, contentPatch); err != nil {
			if nodeChange {
				rollback := nodes.UpdateParams{Grantor: grantor}
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
				if req.InstanceProfile != nil {
					instanceProfile := existing.EffectiveInstanceProfile()
					rollback.InstanceProfile = &instanceProfile
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
	if req.InstanceProfile != nil && a.deps.BotInstances != nil {
		if err := a.deps.BotInstances.SyncProfile(ctx, orgID, id); err != nil {
			writeError(w, errStatus(err), fmt.Errorf("instance profile saved, but applying it to instances failed (retry to re-apply): %w", err))
			return
		}
	}
	// The write already succeeded, so an unreadable App must not turn
	// into a 500 the client reads as "the update failed".
	dto := botDTO(updated, a.managerIDs(ctx, orgID, id))
	dto.CanEdit = true
	a.applyCanonicalAgentProfile(ctx, updated, &dto, "update org bot")
	writeJSON(w, http.StatusOK, dto)
}

// deleteBot starts tearing down a Bot via the lifecycle service and returns
// 202 without waiting: destroying its sandboxes can take minutes. The bot is
// hidden from reads and refuses edits until the teardown finishes. Cascades
// the Helix app, runtime state, attachments, reporting lines, then the bot
// row. Its runtime-owned project is archived and repositories are preserved.
//
// @Summary Helix-org: delete a bot
// @Description Delete a Bot in the background; returns 202 once the teardown has started. Cascades: archives its runtime-owned project, detaches and deletes the Helix agent app, clears runtime state, drops attachments + reporting lines, then the bot row. Repositories and activations are preserved.
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 202
// @Failure 404 {object} api.ErrorResponse
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
	if !a.authorizeBotEditAccess(w, r, orgID, id, "deletion") {
		return
	}
	if err := a.deps.Lifecycle.StartDelete(r.Context(), orgID, id); err != nil {
		writeError(w, errStatus(err), err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
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
	if !a.authorizeBotEdit(w, r, orgID, id, "reporting lines") || !a.authorizeBotEdit(w, r, orgID, managerID, "reports") {
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
	if !a.authorizeBotEdit(w, r, orgID, id, "reporting lines") || !a.authorizeBotEdit(w, r, orgID, managerID, "reports") {
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
// @Failure 409 {object} api.ErrorResponse
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
	if a.refuseDeletingBot(w, orgID, id) {
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
// @Failure 409 {object} api.ErrorResponse
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
	if a.refuseDeletingBot(w, orgID, id) {
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
	bot, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	if !a.authorizeLoadedBotEdit(w, r, orgID, bot, "sandbox") {
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
	bot, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	if !a.authorizeLoadedBotEdit(w, r, orgID, bot, "sandbox") {
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

// applyBotConfig recreates the Bot's existing container with its latest
// persisted launch config while preserving its session and healthy ACP thread.
//
// @Summary Helix-org: apply Bot config to its running sandbox
// @Tags HelixOrg
// @Param id path string true "Bot ID"
// @Success 202
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/bots/{id}/apply-config [post]
func (a *apiHandler) applyBotConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if a.deps.BotConfigApplier == nil {
		writeError(w, http.StatusNotImplemented, errors.New("bot config apply is not wired in this deployment"))
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
	bot, err := a.deps.Queries.GetBot(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get bot %s: %w", id, err))
		return
	}
	if !a.authorizeLoadedBotEdit(w, r, orgID, bot, "sandbox") {
		return
	}
	if err := a.deps.BotConfigApplier.ApplyConfig(ctx, orgID, id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("apply bot %s config: %w", id, err))
		return
	}
	w.WriteHeader(http.StatusAccepted)
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
		if a.deps.Lifecycle.Deleting(orgID, m) {
			continue
		}
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
		PortraitIndex:   b.PortraitIndex,
		PreserveContext: b.PreserveContext,
		SandboxRuntime:  types.SandboxRuntime(b.SandboxRuntime),
		InstanceProfile: b.EffectiveInstanceProfile(),
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

// restGrantor is the tool-grant authority of a REST caller: organization
// owners and administrators have no ceiling; other members may change only
// the standard worker set (nodes.AuthorizeTools).
// botEditAllowed reports whether the caller may change bot: organization
// owners and platform admins, or a member who may update the Bot's agent
// app. Everything a Bot does, and where, is decided by its configuration,
// reporting lines, attachments, secrets, asset links and instances, so all
// of those are edits of the agent. A Bot with no agent app (hired before
// agents were linked, and not yet reconciled) has nothing a member could be
// authorized against, so only owners and admins may edit it.
func (a *apiHandler) botEditAllowed(ctx context.Context, orgID string, bot orgchart.Node) bool {
	if helixorgserver.CanManageOrganization(ctx) {
		return true
	}
	if a.deps.AgentEditAuthorizer == nil {
		return false
	}
	agentID := a.botAgentID(ctx, orgID, bot)
	return agentID != "" && a.deps.AgentEditAuthorizer.AuthorizeAgentUpdate(ctx, agentID) == nil
}

// botAgentID is the agent app a member's edit right is checked against:
// the Bot's own link, else the runtime's pointer for a Bot whose link has
// not been reconciled yet.
func (a *apiHandler) botAgentID(ctx context.Context, orgID string, bot orgchart.Node) string {
	if bot.AgentID != "" || a.deps.BotRuntime == nil {
		return bot.AgentID
	}
	if info, err := a.deps.BotRuntime.State(ctx, orgID, bot.ID); err == nil {
		return info.AgentID
	}
	return ""
}

// editableBots reports, for every bot in bots, whether the caller may change
// it — botEditAllowed for a whole list. Owners and admins may change all of
// them without any lookup; for a member the caller is resolved once through
// AgentEditBatchAuthorizer when the authorizer offers it, else per bot.
func (a *apiHandler) editableBots(ctx context.Context, orgID string, bots []orgchart.Node) map[orgchart.NodeID]bool {
	out := make(map[orgchart.NodeID]bool, len(bots))
	if helixorgserver.CanManageOrganization(ctx) {
		for _, b := range bots {
			out[b.ID] = true
		}
		return out
	}
	if a.deps.AgentEditAuthorizer == nil {
		return out
	}
	batch, ok := a.deps.AgentEditAuthorizer.(AgentEditBatchAuthorizer)
	if !ok {
		for _, b := range bots {
			out[b.ID] = a.botEditAllowed(ctx, orgID, b)
		}
		return out
	}
	agentByBot := make(map[orgchart.NodeID]string, len(bots))
	appIDs := make([]string, 0, len(bots))
	for _, b := range bots {
		if id := a.botAgentID(ctx, orgID, b); id != "" {
			agentByBot[b.ID] = id
			appIDs = append(appIDs, id)
		}
	}
	if len(appIDs) == 0 {
		return out
	}
	editable, err := batch.EditableAgents(ctx, appIDs)
	if err != nil {
		// Fail closed: a read error must not grant edit controls.
		return out
	}
	for botID, appID := range agentByBot {
		out[botID] = editable[appID]
	}
	return out
}

// authorizeBotEdit applies botEditAllowed to a request, loading the bot.
// Handlers that already hold the bot use authorizeLoadedBotEdit. `what`
// names the part being changed for the error message. It writes the error
// response and returns false when the caller may not.
func (a *apiHandler) authorizeBotEdit(w http.ResponseWriter, r *http.Request, orgID string, botID orgchart.NodeID, what string) bool {
	return a.authorizeBotEditAccess(w, r, orgID, botID, what) && !a.refuseDeletingBot(w, orgID, botID)
}

// authorizeBotEditAccess is authorizeBotEdit without the in-flight delete
// check, for the delete handler, where repeating a delete is a no-op.
func (a *apiHandler) authorizeBotEditAccess(w http.ResponseWriter, r *http.Request, orgID string, botID orgchart.NodeID, what string) bool {
	ctx := r.Context()
	if helixorgserver.CanManageOrganization(ctx) {
		return true
	}
	if a.deps.Queries == nil {
		writeAPIError(w, http.StatusForbidden, "forbidden", botEditDenied(what))
		return false
	}
	bot, err := a.deps.Queries.GetBot(ctx, orgID, botID)
	if err != nil {
		writeAPIError(w, errStatus(err), "not_found", "Bot not found")
		return false
	}
	if !a.botEditAllowed(ctx, orgID, bot) {
		writeAPIError(w, http.StatusForbidden, "forbidden", botEditDenied(what))
		return false
	}
	return true
}

// authorizeLoadedBotEdit is authorizeBotEdit for a bot the handler has
// already loaded, so the row is not read twice per mutation.
func (a *apiHandler) authorizeLoadedBotEdit(w http.ResponseWriter, r *http.Request, orgID string, bot orgchart.Node, what string) bool {
	if !a.botEditAllowed(r.Context(), orgID, bot) {
		writeAPIError(w, http.StatusForbidden, "forbidden", botEditDenied(what))
		return false
	}
	return !a.refuseDeletingBot(w, orgID, bot.ID)
}

// refuseDeletingBot writes 409 when a background delete of the bot is in
// flight, so nothing re-provisions or edits a bot that is being torn down.
func (a *apiHandler) refuseDeletingBot(w http.ResponseWriter, orgID string, botID orgchart.NodeID) bool {
	if !a.deps.Lifecycle.Deleting(orgID, botID) {
		return false
	}
	writeAPIError(w, http.StatusConflict, "conflict", "Bot is being deleted")
	return true
}

// listLiveBots lists the org's bots minus those being deleted.
func (a *apiHandler) listLiveBots(ctx context.Context, orgID string) ([]orgchart.Node, error) {
	bs, err := a.deps.Queries.ListBots(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(bs, func(b orgchart.Node) bool {
		return a.deps.Lifecycle.Deleting(orgID, b.ID)
	}), nil
}

func botEditDenied(what string) string {
	return "Only organization owners, administrators, or people who can edit this bot's agent can change its " + what
}

// authorizeBotProjects refuses project ids being added to a Bot's allowlist
// that the Bot's hiring user may not access. Ids already on the Bot are not
// re-checked, so a later revocation does not block unrelated edits.
func (a *apiHandler) authorizeBotProjects(w http.ResponseWriter, r *http.Request, orgID string, bot orgchart.Node, requested []string) bool {
	if a.deps.BotProjectAuthorizer == nil {
		return true
	}
	for _, projectID := range requested {
		projectID = strings.TrimSpace(projectID)
		if projectID == "" || slices.Contains(bot.ProjectIDs, projectID) {
			continue
		}
		if err := a.deps.BotProjectAuthorizer.AuthorizeBotProject(r.Context(), orgID, bot.ID, projectID); err != nil {
			writeAPIError(w, http.StatusForbidden, "forbidden", fmt.Sprintf("This bot cannot be given project %s: %v", projectID, err))
			return false
		}
	}
	return true
}

func restGrantor(ctx context.Context) nodes.Grantor {
	return nodes.Grantor{Admin: helixorgserver.CanManageOrganization(ctx)}
}
