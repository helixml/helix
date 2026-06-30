package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/workers"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

// listWorkers returns every Worker row.
//
// @Summary Helix-org: list workers
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.WorkerDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers [get]
func (a *apiHandler) listWorkers(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	workers, err := a.deps.Queries.ListWorkers(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list workers: %w", err))
		return
	}
	// One List call builds the report → managers index so each worker's
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
	// Resolve each worker's tools via Role.Tools. Cache by role so a
	// org with many workers in the same role only pays for the
	// lookup once.
	roleCache := map[orgchart.BotID][]string{}
	out := make([]WorkerDTO, 0, len(workers))
	for _, wk := range workers {
		rid := wk.RoleID()
		tools, ok := roleCache[rid]
		if !ok {
			tools = nil
			if role, err := a.deps.Queries.GetRole(ctx, orgID, rid); err == nil {
				tools = make([]string, 0, len(role.Tools))
				for _, t := range role.Tools {
					tools = append(tools, string(t))
				}
			}
			roleCache[rid] = tools
		}
		out = append(out, workerDTO(wk, tools, managersByReport[wk.ID()]))
	}
	writeJSON(w, http.StatusOK, out)
}

// hireWorker creates a Worker through the same code path the MCP
// hire_worker tool drives. The caller identity is fixed to the
// embedded owner ("w-owner") — the React chart UX is owner-driven
// in the alpha; per-user hires arrive when multi-tenant lands.
//
// @Summary Helix-org: hire worker
// @Description Create a Worker in the given Position. Wraps the hire_worker MCP tool so REST + chat hires share semantics (env dir, transcript, hire dispatch).
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.HireWorkerRequest true "Hire request"
// @Success 201 {object} api.HireWorkerResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers [post]
func (a *apiHandler) hireWorker(w http.ResponseWriter, r *http.Request) {
	if a.deps.Lifecycle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("hire is not wired in this deployment"))
		return
	}
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req HireWorkerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.RoleID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("role_id is required"))
		return
	}
	if strings.TrimSpace(req.Kind) == "" {
		writeError(w, http.StatusBadRequest, errors.New("kind is required"))
		return
	}
	if strings.TrimSpace(req.IdentityContent) == "" {
		writeError(w, http.StatusBadRequest, errors.New("identity_content is required"))
		return
	}

	// REST and chat-driven hires share lifecycle.Hire — one
	// implementation, no synthetic Invocation, no owner lookup. Worker
	// mutations run as the org service identity; the picking user's id
	// (when present) is read off ctx by Hire's hire-hook.
	res, err := a.deps.Lifecycle.Hire(ctx, orgID, lifecycle.HireParams{
		ID:              strings.TrimSpace(req.ID),
		RoleID:          orgchart.BotID(strings.TrimSpace(req.RoleID)),
		ParentID:        orgchart.BotID(strings.TrimSpace(req.ParentID)),
		Kind:            orgchart.WorkerKind(strings.TrimSpace(req.Kind)),
		IdentityContent: req.IdentityContent,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, HireWorkerResponse{ID: string(res.WorkerID), ActivationID: string(res.ActivationID)})
}

// fireWorker tears down a Worker via the lifecycle service. Returns
// 409 if the target is the owner.
//
// @Summary Helix-org: fire worker
// @Description Delete a Worker. Cascades: stops sessions, deletes the Helix project + agent app, clears runtime state, deletes subscriptions + env dir + env row, then the worker row. Activations are preserved as audit.
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id} [delete]
func (a *apiHandler) fireWorker(w http.ResponseWriter, r *http.Request) {
	if a.deps.Lifecycle == nil {
		writeError(w, http.StatusNotImplemented, errors.New("fire is not wired in this deployment"))
		return
	}
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	switch err := a.deps.Lifecycle.Fire(r.Context(), orgID, id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, errStatus(err), err)
	}
}

// workerDTO converts a orgchart.Worker to its wire form. tools may be
// nil — callers that want to surface the Role's tools pass the sorted list.
// parentIDs are the managers this Worker reports to (from the reporting
// lines); nil for a top-level Worker.
func workerDTO(wk orgchart.Worker, tools []string, parentIDs []string) WorkerDTO {
	return WorkerDTO{
		ID:              string(wk.ID()),
		Kind:            string(wk.Kind()),
		RoleID:          string(wk.RoleID()),
		ParentIDs:       parentIDs,
		IdentityContent: wk.IdentityContent(),
		OrganizationID:  wk.OrganizationID(),
		Tools:           tools,
	}
}

// managerIDs returns the ids of the managers the given worker reports
// to, as strings, for embedding in a WorkerDTO. Returns nil on any
// store error — the reporting graph is best-effort context, never a
// reason to fail the whole worker read.
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

// getWorker returns a Worker + the role/position it fills.
//
// @Summary Helix-org: get worker detail
// @Tags HelixOrg
// @Produce json
// @Param id path string true "Worker ID"
// @Success 200 {object} api.WorkerDetailDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id} [get]
func (a *apiHandler) getWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	wk, err := a.deps.Queries.GetWorker(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}

	// Tools are derived from the Worker's Role.Tools.
	var (
		toolNames []string
		roDTO     *RoleDTO
	)
	if rid := wk.RoleID(); rid != "" {
		ro, err := a.deps.Queries.GetRole(ctx, orgID, rid)
		if err == nil {
			rd := roleDTO(ro)
			roDTO = &rd
			toolNames = make([]string, 0, len(ro.Tools))
			for _, t := range ro.Tools {
				toolNames = append(toolNames, string(t))
			}
			sort.Strings(toolNames)
		}
	}

	detail := WorkerDetailDTO{Worker: workerDTO(wk, toolNames, a.managerIDs(ctx, orgID, id))}
	detail.Role = roDTO
	// Populate the agent app id + project id from the helix-runtime
	// sidecar so the chart UI can deep-link "chat with worker" to the
	// per-project Human Desktop session. Missing state = the worker
	// hasn't activated yet; we leave the fields empty and the UI
	// shows a disabled button.
	if a.deps.WorkerRuntime != nil {
		if info, err := a.deps.WorkerRuntime.State(ctx, orgID, id); err == nil {
			detail.AgentAppID = info.AgentAppID
			detail.ProjectID = info.ProjectID
		}
	}
	writeJSON(w, http.StatusOK, detail)
}

// ensureWorkerChat provisions (or fast-paths) the Worker's per-Worker
// Helix project + agent app, then returns the agent_app_id so the
// chart UI can deep-link to /agent/<app_id>. The owner worker has no
// agent app on bootstrap (the spawner provisions one only when an AI
// worker is activated); calling this on first chart visit gets us a
// chat-able app for the human owner without bootstrap-time changes.
//
// Idempotent — WorkerProject.Ensure fast-paths when the project
// already exists.
//
// @Summary Helix-org: provision a per-worker chat app
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 200 {object} api.WorkerChatDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/chat [post]
func (a *apiHandler) ensureWorkerChat(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	if _, err := a.deps.Queries.GetWorker(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	projectID, agentAppID, _, err := a.deps.ProjectEnsurer.Ensure(ctx, orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("ensure worker chat: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, WorkerChatDTO{AgentAppID: agentAppID, ProjectID: projectID})
}

// activateWorker manually triggers an activation for a Worker. The
// worker page's "Start Desktop" button hits this endpoint instead of
// /sessions/{id}/resume so the full activation pipeline runs:
// ensureProject → AttachHelixOrgMCP → ensureSession → Helix spins
// up the desktop container as part of the session start. This is the
// path that guarantees the helix-org MCP entry is present on the
// agent app when Zed reads /sessions/{id}/zed-config inside the
// container — plain resume reads but doesn't write Config.Helix, so
// it can't fix an MCP-clobbered agent app on its own.
//
// Synchronous up to ensureProject so the response carries the
// project + agent_app IDs the UI needs to navigate the user to the
// desktop. The session-start work runs async on the per-Worker
// queue inside the dispatcher.
//
// @Summary Helix-org: manually trigger a worker activation
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 202 {object} api.WorkerActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/activate [post]
func (a *apiHandler) activateWorker(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	// Confirm the Worker exists for a clean 404 before the activate
	// command runs its project/dispatch side effects.
	if _, err := a.deps.Queries.GetWorker(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	// The activate command (ensure project + MCP attach → read session →
	// pre-allocate audit row → enqueue on the per-Worker queue) is owned
	// by the activations service; the handler just maps the result.
	res, err := a.deps.Activations.Activate(ctx, orgID, id)
	if err != nil {
		if errors.Is(err, activations.ErrActivateUnavailable) {
			writeError(w, http.StatusNotImplemented, err)
			return
		}
		writeError(w, errStatus(err), err)
		return
	}
	writeJSON(w, http.StatusAccepted, WorkerActivateDTO{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		AgentAppID:   res.AgentAppID,
		SessionID:    res.SessionID,
	})
}

// restartWorkerAgent recreates the worker's desktop container from
// scratch — the worker-page "Restart agent session" button. Unlike
// activateWorker (which continues the existing session via SendMessage
// and so cannot recover a stuck container), this resolves the worker's
// current session and delegates to the shared backend restart primitive
// (StopDesktop → recreate → reset crashed prompts). If the worker has no
// live session yet, it falls back to a normal activation so first-time
// start still works.
//
// @Summary Helix-org: restart a worker's agent session (recreate desktop container)
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 202 {object} api.WorkerActivateDTO
// @Failure 404 {object} api.ErrorResponse
// @Failure 500 {object} api.ErrorResponse
// @Failure 501 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/restart-agent [post]
func (a *apiHandler) restartWorkerAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	// Confirm the Worker exists for a clean 404 before any side effects.
	if _, err := a.deps.Queries.GetWorker(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}

	// Resolve the worker's current desktop session. Empty means the
	// worker has never activated — there's no container to recreate, so
	// fall through to a normal activation (which starts a fresh one).
	var sessionID string
	if a.deps.WorkerRuntime != nil {
		if info, err := a.deps.WorkerRuntime.State(ctx, orgID, id); err == nil {
			sessionID = info.SessionID
		}
	}

	if sessionID != "" && a.deps.SessionRestarter != nil {
		if err := a.deps.SessionRestarter.RestartSession(ctx, sessionID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("restart worker %s session: %w", id, err))
			return
		}
		writeJSON(w, http.StatusAccepted, WorkerActivateDTO{SessionID: sessionID})
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
	writeJSON(w, http.StatusAccepted, WorkerActivateDTO{
		ActivationID: string(res.ActivationID),
		ProjectID:    res.ProjectID,
		AgentAppID:   res.AgentAppID,
		SessionID:    res.SessionID,
	})
}

// updateWorkerIdentity rewrites a Worker's IdentityContent. The
// Spawner projects the new content into the Worker's identity.md on
// the next activation.
//
// @Summary Helix-org: update worker identity
// @Tags HelixOrg
// @Accept json
// @Param id path string true "Worker ID"
// @Param payload body api.UpdateWorkerIdentityRequest true "New identity content"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/identity [post]
func (a *apiHandler) updateWorkerIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req UpdateWorkerIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.deps.Workers.UpdateIdentity(ctx, orgID, id, req.Identity); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update worker %s: %w", id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// addWorkerParent adds a reporting line: the Worker at {id} now also
// reports to the manager in the body. Reporting is many-to-many, so
// this is additive — a Worker can report to several managers. The
// chart UI calls it when an accountability edge is drawn between two
// Worker nodes.
//
// Validation:
//   - the manager must reference a Worker that exists in the org
//   - the manager must not already be a descendant of {id}, which
//     would close a reporting cycle (the graph is a DAG)
//
// Idempotent: re-adding an existing line returns 204.
//
// @Summary Helix-org: add a worker reporting line (manager)
// @Tags HelixOrg
// @Accept json
// @Param id path string true "Worker ID (the report)"
// @Param payload body api.AddWorkerParentRequest true "Manager worker id"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/parents [post]
func (a *apiHandler) addWorkerParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req AddWorkerParentRequest
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
	switch err := a.deps.Workers.AddParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, workers.ErrReportingLinesUnavailable):
		writeError(w, http.StatusNotImplemented, err)
	case errors.Is(err, workers.ErrReportingCycle):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, errStatus(err), err)
	}
}

// removeWorkerParent drops one reporting line: the Worker at {id} no
// longer reports to {parent_id}. The chart UI calls it when an
// accountability edge is deleted. Returns 404 when no such line exists.
//
// @Summary Helix-org: remove a worker reporting line (manager)
// @Tags HelixOrg
// @Param id path string true "Worker ID (the report)"
// @Param parent_id path string true "Manager worker id"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/parents/{parent_id} [delete]
func (a *apiHandler) removeWorkerParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	managerID := orgchart.BotID(r.PathValue("parent_id"))
	if id == "" || managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id and parent_id are required"))
		return
	}
	// The service drops the line and reconciles the Topics the dropped
	// edge implies (unsubscribe ex-manager from the report's activation
	// topic, remove report from the ex-manager's team topic).
	switch err := a.deps.Workers.RemoveParent(ctx, orgID, id, managerID); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, workers.ErrReportingLinesUnavailable):
		writeError(w, http.StatusNotImplemented, err)
	default:
		writeError(w, errStatus(err), err)
	}
}

// updateWorkerRole rewrites the role.md of the Role the Worker's
// Position references. Keyed by Worker so the React client can
// `POST /workers/{id}/role` from the worker-detail page without first
// resolving Position → Role.
//
// Returns 409 if the Worker has no Position (unassigned) — there is
// no role to update.
//
// @Summary Helix-org: update worker role
// @Tags HelixOrg
// @Accept json
// @Param id path string true "Worker ID"
// @Param payload body api.UpdateWorkerRoleRequest true "New role content"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/role [post]
func (a *apiHandler) updateWorkerRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := orgchart.BotID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req UpdateWorkerRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if _, err := a.deps.Workers.UpdateRole(ctx, orgID, id, req.Content); err != nil {
		if errors.Is(err, workers.ErrWorkerHasNoRole) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, errStatus(err), fmt.Errorf("update worker role %s: %w", id, err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
