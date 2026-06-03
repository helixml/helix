package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
)

// resolveOrgID returns the orgID stashed on ctx by the helix-org
// middleware (withHelixOrgScope in api/pkg/server). Empty orgID
// means no scope was set — handlers respond 400 and bail rather
// than silently scoping to "".
func resolveOrgID(r *http.Request) (string, error) {
	if orgID := helixorgserver.OrgIDFromContext(r.Context()); orgID != "" {
		return orgID, nil
	}
	return "", errors.New("helix-org scope missing — request did not pass through /orgs/{org} middleware")
}

// Dispatcher is the dispatcher port the publish handler invokes when
// a client posts an event into a stream. Defined here (rather than
// imported from server.go's sibling) to keep the import edge
// one-directional — server/api is below server, not next to it.
type Dispatcher interface {
	Dispatch(ctx context.Context, ev streaming.Event)
}

// Deps is the JSON API's wiring.
//
// Owner is the WorkerID hardcoded as "w-owner"; plumbed through so
// publish attribution stays consistent with the React publish form.
//
// PublicURL / DBPath / EnvsDir are the operational state the settings
// page surfaces (today they come from CLI flags; the SaaS embedding
// leaves PublicURL empty).
type Deps struct {
	Store      *store.Store
	Configs    *configregistry.Registry
	Hub        *streamhub.Hub
	Dispatcher Dispatcher

	Owner     string
	PublicURL string
	DBPath    string
	EnvsDir   string

	// HireWorker is the constructed hire tool, shared with the MCP
	// registry. The REST POST /workers handler builds a synthetic
	// Invocation around the owner Worker and dispatches through this
	// same path so REST hires and chat-driven hires produce identical
	// store state. nil disables POST /workers (returns 501).
	HireWorker *tools.HireWorker

	// Lifecycle owns the cross-cutting Fire cascade (Helix project +
	// app teardown, store cleanup, env-dir removal). nil disables
	// DELETE /workers/{id} (returns 501).
	Lifecycle *lifecycle.Service

	// NewID and Now are seams for tests. Production wiring passes
	// system.GenerateID / time.Now.
	NewID func() string
	Now   func() time.Time
}

// Route pairs a net/http ServeMux pattern with the handler that
// serves it — the same shape api/pkg/org/server.Route uses so the
// JSON routes can be passed straight into Server.Handler(extras...).
type Route struct {
	Pattern string
	Handler http.Handler
}

// Routes returns every JSON route this package registers. Pass the
// slice into helixorgserver.Server.Handler as extras so the routes
// land on the same mux as MCP/webhooks (and pick up the same
// request-logging middleware).
//
// Patterns are flat (no /api/v1/orgs/{org}/ prefix) because the
// host strips that prefix via stripOrgScopedPrefix before dispatching.
func Routes(deps Deps) []Route {
	a := &apiHandler{deps: deps}
	return []Route{
		{Pattern: "GET /chart", Handler: http.HandlerFunc(a.getChart)},
		{Pattern: "GET /positions", Handler: http.HandlerFunc(a.listPositions)},
		{Pattern: "POST /positions", Handler: http.HandlerFunc(a.createPosition)},
		{Pattern: "GET /positions/{id}", Handler: http.HandlerFunc(a.getPosition)},
		{Pattern: "PUT /positions/{id}", Handler: http.HandlerFunc(a.updatePosition)},
		{Pattern: "GET /roles", Handler: http.HandlerFunc(a.listRoles)},
		{Pattern: "POST /roles", Handler: http.HandlerFunc(a.createRole)},
		{Pattern: "GET /roles/{id}", Handler: http.HandlerFunc(a.getRole)},
		{Pattern: "PUT /roles/{id}", Handler: http.HandlerFunc(a.updateRole)},
		{Pattern: "GET /workers", Handler: http.HandlerFunc(a.listWorkers)},
		{Pattern: "POST /workers", Handler: http.HandlerFunc(a.hireWorker)},
		{Pattern: "GET /workers/{id}", Handler: http.HandlerFunc(a.getWorker)},
		{Pattern: "DELETE /workers/{id}", Handler: http.HandlerFunc(a.fireWorker)},
		{Pattern: "POST /workers/{id}/role", Handler: http.HandlerFunc(a.updateWorkerRole)},
		{Pattern: "POST /workers/{id}/identity", Handler: http.HandlerFunc(a.updateWorkerIdentity)},
		{Pattern: "GET /settings", Handler: http.HandlerFunc(a.listSettings)},
		{Pattern: "PUT /settings/{key}", Handler: http.HandlerFunc(a.setSetting)},
		{Pattern: "DELETE /settings/{key}", Handler: http.HandlerFunc(a.deleteSetting)},
		{Pattern: "GET /streams", Handler: http.HandlerFunc(a.listStreams)},
		{Pattern: "GET /streams/{id}/events", Handler: http.HandlerFunc(a.streamEventsSSE)},
		{Pattern: "POST /streams/{id}/publish", Handler: http.HandlerFunc(a.publishToStream)},
	}
}

// Handler returns a standalone net/http.Handler with every JSON
// route mounted. Used by tests; production wiring uses Routes() and
// merges into the org server's existing mux.
func Handler(deps Deps) http.Handler {
	mux := http.NewServeMux()
	for _, rt := range Routes(deps) {
		mux.Handle(rt.Pattern, rt.Handler)
	}
	return mux
}

type apiHandler struct {
	deps Deps
}

// ---- Org chart ----------------------------------------------------------

// getChart returns the org chart tree.
//
// @Summary Helix-org: get org chart
// @Description Returns the positions+workers tree rendered by the helix-org React UI
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.Chart
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/chart [get]
func (a *apiHandler) getChart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	positions, err := a.deps.Store.Positions.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list positions: %w", err))
		return
	}
	workers, err := a.deps.Store.Workers.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list workers: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, buildChart(positions, workers))
}

// buildChart walks positions + workers into the tree the chart
// renders. Exported so it can be reused by future in-process
// consumers (e.g. an MCP tool surfacing the same shape) without going
// through HTTP.
func buildChart(positions []orgchart.Position, workers []orgchart.Worker) Chart {
	byPos := make(map[orgchart.PositionID][]orgchart.Worker)
	for _, w := range workers {
		if pid := w.Position(); pid != "" {
			byPos[pid] = append(byPos[pid], w)
		}
	}
	idx := make(map[orgchart.PositionID]orgchart.Position, len(positions))
	for _, p := range positions {
		idx[p.ID] = p
	}
	// Sort positions so the resulting tree is deterministic and
	// friendly to React diffing.
	sorted := append([]orgchart.Position(nil), positions...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	childrenOf := make(map[orgchart.PositionID][]orgchart.Position)
	var roots []orgchart.Position
	for _, p := range sorted {
		if p.ParentID == nil {
			roots = append(roots, p)
			continue
		}
		if _, ok := idx[*p.ParentID]; ok {
			childrenOf[*p.ParentID] = append(childrenOf[*p.ParentID], p)
		} else {
			// Orphan — parent not in this snapshot; treat as root so the
			// chart still surfaces the node rather than dropping it.
			roots = append(roots, p)
		}
	}

	var build func(p orgchart.Position) ChartNode
	build = func(p orgchart.Position) ChartNode {
		n := ChartNode{
			PositionID: string(p.ID),
			RoleID:     string(p.RoleID),
		}
		if p.ParentID != nil {
			n.ParentID = string(*p.ParentID)
		}
		for _, wk := range byPos[p.ID] {
			n.Workers = append(n.Workers, WorkerBadge{
				ID:   string(wk.ID()),
				Kind: string(wk.Kind()),
			})
		}
		sort.SliceStable(n.Workers, func(i, j int) bool { return n.Workers[i].ID < n.Workers[j].ID })
		for _, c := range childrenOf[p.ID] {
			n.Children = append(n.Children, build(c))
		}
		return n
	}
	out := Chart{Roots: make([]ChartNode, 0, len(roots))}
	for _, r := range roots {
		out.Roots = append(out.Roots, build(r))
	}
	return out
}

// ---- Positions / Roles / Workers ----------------------------------------

// listPositions returns every Position row.
//
// @Summary Helix-org: list positions
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.PositionDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/positions [get]
func (a *apiHandler) listPositions(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	positions, err := a.deps.Store.Positions.List(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list positions: %w", err))
		return
	}
	out := make([]PositionDTO, 0, len(positions))
	for _, p := range positions {
		out = append(out, positionDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func positionDTO(p orgchart.Position) PositionDTO {
	dto := PositionDTO{ID: string(p.ID), RoleID: string(p.RoleID)}
	if p.ParentID != nil {
		dto.ParentID = string(*p.ParentID)
	}
	return dto
}

// listRoles returns every Role row.
//
// @Summary Helix-org: list roles
// @Tags HelixOrg
// @Produce json
// @Success 200 {array} api.RoleDTO
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/roles [get]
func (a *apiHandler) listRoles(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	roles, err := a.deps.Store.Roles.List(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list roles: %w", err))
		return
	}
	out := make([]RoleDTO, 0, len(roles))
	for _, ro := range roles {
		out = append(out, roleDTO(ro))
	}
	writeJSON(w, http.StatusOK, out)
}

func roleDTO(r orgchart.Role) RoleDTO {
	dto := RoleDTO{ID: string(r.ID), Content: r.Content}
	if !r.CreatedAt.IsZero() {
		dto.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if !r.UpdatedAt.IsZero() {
		dto.UpdatedAt = r.UpdatedAt.Format(time.RFC3339)
	}
	for _, t := range r.Tools {
		dto.Tools = append(dto.Tools, string(t))
	}
	for _, s := range r.Streams {
		dto.Streams = append(dto.Streams, string(s))
	}
	return dto
}

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
	workers, err := a.deps.Store.Workers.List(r.Context(), orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list workers: %w", err))
		return
	}
	out := make([]WorkerDTO, 0, len(workers))
	for _, wk := range workers {
		out = append(out, workerDTO(wk, nil))
	}
	writeJSON(w, http.StatusOK, out)
}

// hireWorker creates a Worker through the same code path the MCP
// hire_worker tool drives. The caller identity is fixed to the
// embedded owner ("w-owner") — the React chart UX is owner-driven
// in the alpha; per-user hires arrive when multi-tenant lands.
//
// @Summary Helix-org: hire worker
// @Description Create a Worker in the given Position. Wraps the hire_worker MCP tool so REST + chat hires share semantics (env dir, activation stream, hire dispatch).
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
	if a.deps.HireWorker == nil {
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
	if strings.TrimSpace(req.PositionID) == "" {
		writeError(w, http.StatusBadRequest, errors.New("position_id is required"))
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

	owner, err := a.deps.Store.Workers.Get(ctx, orgID, orgchart.WorkerID(a.deps.Owner))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("load owner %s: %w", a.deps.Owner, err))
		return
	}

	// hire_worker reads its args off tool.Invocation.Args using the
	// same JSON shape MCP delivers — we marshal HireWorkerRequest into
	// the wire form so there is exactly one parser.
	type wireGrant struct {
		ToolName string `json:"toolName"`
	}
	type wireArgs struct {
		ID              string      `json:"id,omitempty"`
		PositionID      string      `json:"positionId"`
		Kind            string      `json:"kind"`
		IdentityContent string      `json:"identityContent"`
		Grants          []wireGrant `json:"grants,omitempty"`
	}
	wargs := wireArgs{
		ID:              strings.TrimSpace(req.ID),
		PositionID:      strings.TrimSpace(req.PositionID),
		Kind:            strings.TrimSpace(req.Kind),
		IdentityContent: req.IdentityContent,
	}
	for _, g := range req.Grants {
		if name := strings.TrimSpace(g.ToolName); name != "" {
			wargs.Grants = append(wargs.Grants, wireGrant{ToolName: name})
		}
	}
	argsJSON, err := json.Marshal(wargs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("marshal hire args: %w", err))
		return
	}

	resp, err := a.deps.HireWorker.Invoke(ctx, tool.Invocation{Caller: owner, Args: argsJSON})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var out HireWorkerResponse
	if err := json.Unmarshal(resp, &out); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("decode hire response: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// fireWorker tears down a Worker via the lifecycle service. Returns
// 409 if the target is the owner.
//
// @Summary Helix-org: fire worker
// @Description Delete a Worker. Cascades: stops sessions, deletes the Helix project + agent app, clears runtime state, deletes subscriptions + grants + env dir + env row, then the worker row. Activations are preserved as audit.
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	switch err := a.deps.Lifecycle.Fire(r.Context(), orgID, id); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, lifecycle.ErrOwnerProtected):
		writeError(w, http.StatusConflict, err)
	default:
		writeError(w, errStatus(err), err)
	}
}

// workerDTO converts a orgchart.Worker to its wire form. tools may be
// nil — callers populating per-worker grants pass the sorted list.
func workerDTO(wk orgchart.Worker, tools []string) WorkerDTO {
	return WorkerDTO{
		ID:              string(wk.ID()),
		Kind:            string(wk.Kind()),
		PositionID:      string(wk.Position()),
		IdentityContent: wk.IdentityContent(),
		OrganizationID:  wk.OrganizationID(),
		Tools:           tools,
	}
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	wk, err := a.deps.Store.Workers.Get(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	grants, err := a.deps.Store.Grants.ListByWorker(ctx, orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list grants: %w", err))
		return
	}
	tools := make([]string, 0, len(grants))
	for _, g := range grants {
		tools = append(tools, string(g.ToolName))
	}
	sort.Strings(tools)

	detail := WorkerDetailDTO{Worker: workerDTO(wk, tools)}
	if pid := wk.Position(); pid != "" {
		pos, err := a.deps.Store.Positions.Get(ctx, orgID, pid)
		if err == nil {
			pd := positionDTO(pos)
			detail.Position = &pd
			ro, err := a.deps.Store.Roles.Get(ctx, orgID, pos.RoleID)
			if err == nil {
				rd := roleDTO(ro)
				detail.Role = &rd
			}
		}
	}
	writeJSON(w, http.StatusOK, detail)
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req UpdateWorkerIdentityRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	existing, err := a.deps.Store.Workers.Get(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	if err := a.deps.Store.Workers.Update(ctx, existing.WithIdentityContent(req.Identity)); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update worker: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req UpdateWorkerRoleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wk, err := a.deps.Store.Workers.Get(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	pid := wk.Position()
	if pid == "" {
		writeError(w, http.StatusConflict, errors.New("worker has no position"))
		return
	}
	pos, err := a.deps.Store.Positions.Get(ctx, orgID, pid)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get position %s: %w", pid, err))
		return
	}
	existing, err := a.deps.Store.Roles.Get(ctx, orgID, pos.RoleID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get role %s: %w", pos.RoleID, err))
		return
	}
	existing.Content = req.Content
	if err := a.deps.Store.Roles.Update(ctx, existing); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update role: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Settings -----------------------------------------------------------

// listSettings returns the registry's spec list + current redacted values.
//
// @Summary Helix-org: list settings
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.SettingsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings [get]
func (a *apiHandler) listSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	resp := SettingsResponse{
		Owner:     a.deps.Owner,
		PublicURL: a.deps.PublicURL,
		DBPath:    a.deps.DBPath,
		EnvsDir:   a.deps.EnvsDir,
	}
	if a.deps.Configs != nil {
		specs := a.deps.Configs.Specs()
		resp.Specs = make([]SettingsSpecDTO, 0, len(specs))
		for _, sp := range specs {
			resp.Specs = append(resp.Specs, settingsSpecDTO(ctx, orgID, a.deps.Configs, a.deps.Store, sp))
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// settingsSpecDTO resolves the current redacted value + the
// "configured" bool surfaced on each settings row. Lives outside the
// handler so a future "GET /settings/{key}" can reuse it.
func settingsSpecDTO(ctx context.Context, orgID string, reg *configregistry.Registry, st *store.Store, sp configregistry.Spec) SettingsSpecDTO {
	row := SettingsSpecDTO{
		Key:         sp.Key,
		Type:        string(sp.Type),
		Required:    sp.Required,
		Description: sp.Description,
	}
	// "Configured" means the configs row exists (not "has a value via
	// default").
	if st != nil && st.Configs != nil {
		if _, err := st.Configs.Get(ctx, orgID, sp.Key); err == nil {
			row.Configured = true
		}
	}
	// GetRedacted falls back to the default when no row is set; an
	// error means "not configured and no default" — render empty.
	if v, err := reg.GetRedacted(ctx, orgID, sp.Key); err == nil {
		row.Value = v
	}
	return row
}

// setSetting writes a config row for the given key.
//
// @Summary Helix-org: set a setting
// @Tags HelixOrg
// @Accept json
// @Param key path string true "Setting key"
// @Param payload body api.SetSettingRequest true "Setting value (raw JSON per spec type)"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings/{key} [put]
func (a *apiHandler) setSetting(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("key is required"))
		return
	}
	var req SetSettingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Configs.Set(r.Context(), orgID, key, req.Value, orgchart.WorkerID(a.deps.Owner)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteSetting removes the config row for the given key, falling back to defaults.
//
// @Summary Helix-org: delete a setting
// @Tags HelixOrg
// @Param key path string true "Setting key"
// @Success 204
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/settings/{key} [delete]
func (a *apiHandler) deleteSetting(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if key == "" {
		writeError(w, http.StatusBadRequest, errors.New("key is required"))
		return
	}
	if err := a.deps.Configs.Delete(r.Context(), orgID, key); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Streams ------------------------------------------------------------

// listStreams returns every stream + a unified recent-events firehose.
//
// @Summary Helix-org: list streams
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.StreamsResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams [get]
func (a *apiHandler) listStreams(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	streams, err := a.deps.Store.Streams.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list streams: %w", err))
		return
	}
	sort.SliceStable(streams, func(i, j int) bool { return streams[i].CreatedAt.Before(streams[j].CreatedAt) })

	resp := StreamsResponse{Streams: make([]StreamDTO, 0, len(streams))}
	for _, s := range streams {
		dto := StreamDTO{
			ID:          string(s.ID),
			Name:        s.Name,
			Description: s.Description,
			Kind:        string(s.Transport.Kind),
			CreatedBy:   string(s.CreatedBy),
			CreatedAt:   s.CreatedAt.Format(time.RFC3339),
		}
		dto.CanPublish = s.Transport.Kind != transport.KindGitHub
		if !dto.CanPublish {
			dto.DisableReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
		}
		subs, err := a.deps.Store.Subscriptions.ListForStream(ctx, orgID, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions for %s: %w", s.ID, err))
			return
		}
		for _, sub := range subs {
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
		events, err := a.deps.Store.Events.ListForStream(ctx, orgID, s.ID, 50)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list events for %s: %w", s.ID, err))
			return
		}
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
		resp.Streams = append(resp.Streams, dto)
	}

	recent, err := a.deps.Store.Events.ListAll(ctx, orgID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list all events: %w", err))
		return
	}
	for _, ev := range recent {
		resp.Recent = append(resp.Recent, eventCard(ev))
	}
	writeJSON(w, http.StatusOK, resp)
}

// eventCard converts a streaming.Event into its wire shape, expanding the
// canonical Message envelope when the body parses.
func eventCard(ev streaming.Event) EventCard {
	card := EventCard{
		ID:        string(ev.ID),
		StreamID:  string(ev.StreamID),
		Source:    string(ev.Source),
		CreatedAt: ev.CreatedAt.Format(time.RFC3339),
		Body:      ev.Body,
	}
	if msg, err := ev.Message(); err == nil {
		card.HasMessage = true
		card.From = msg.From
		card.Subject = msg.Subject
		card.MessageBody = msg.Body
		if len(msg.To) > 0 {
			card.To = strings.Join(msg.To, ", ")
		}
	}
	return card
}

// streamEventsSSE pushes EventCard JSON arrays on every Hub.Notify.
//
// Each SSE `data:` line is a JSON array of recent events (cap 50,
// newest first). Frontends replace their event list on every message
// — simpler than diffing partial updates.
//
// @Summary Helix-org: SSE stream of events for one stream
// @Tags HelixOrg
// @Produce text/event-stream
// @Param id path string true "Stream ID"
// @Success 200 {string} string "SSE: event: message / data: [EventCard,...]"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/events [get]
func (a *apiHandler) streamEventsSSE(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	if a.deps.Hub == nil {
		writeError(w, http.StatusServiceUnavailable, errors.New("broadcaster not configured"))
		return
	}
	streamID := r.PathValue("id")
	if streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	wake := a.deps.Hub.Subscribe([]streaming.StreamID{streaming.StreamID(streamID)})
	defer a.deps.Hub.Unsubscribe([]streaming.StreamID{streaming.StreamID(streamID)}, wake)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	emit := func() error {
		events, err := a.deps.Store.Events.ListForStream(r.Context(), orgID, streaming.StreamID(streamID), 50)
		if err != nil {
			return err
		}
		cards := make([]EventCard, 0, len(events))
		for _, ev := range events {
			cards = append(cards, eventCard(ev))
		}
		payload, err := json.Marshal(cards)
		if err != nil {
			return err
		}
		// SSE data lines must not embed raw newlines; JSON marshal of
		// a slice never produces newlines.
		_, _ = fmt.Fprint(w, "event: message\n")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
		flusher.Flush()
		return nil
	}

	if err := emit(); err != nil {
		return
	}

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-wake:
			if err := emit(); err != nil {
				return
			}
		case <-ping.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// publishToStream appends a Message event attributed to the owner
// and fans it out to subscribers via the dispatcher. Consumes JSON
// and returns the new event's ID.
//
// @Summary Helix-org: publish a message to a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Stream ID"
// @Param payload body api.PublishRequest true "Message body+optional subject/to"
// @Success 201 {object} api.PublishResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 409 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/publish [post]
func (a *apiHandler) publishToStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	streamID := streaming.StreamID(r.PathValue("id"))
	if streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	if a.deps.NewID == nil || a.deps.Now == nil {
		writeError(w, http.StatusInternalServerError, errors.New("api not configured for publish (missing NewID/Now)"))
		return
	}
	var req PublishRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		writeError(w, http.StatusBadRequest, errors.New("body is required"))
		return
	}
	st, err := a.deps.Store.Streams.Get(ctx, orgID, streamID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", streamID, err))
		return
	}
	if st.Transport.Kind == transport.KindGitHub {
		writeError(w, http.StatusConflict, errors.New("github transport is inbound only"))
		return
	}
	owner := orgchart.WorkerID(a.deps.Owner)
	msg := streaming.Message{
		From:    string(owner),
		To:      req.To,
		Subject: strings.TrimSpace(req.Subject),
		Body:    req.Body,
	}
	ev, err := streaming.NewMessageEvent(
		streaming.EventID("e-"+a.deps.NewID()),
		streamID,
		owner,
		msg,
		a.deps.Now(),
		orgID,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Events.Append(ctx, ev); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("append event: %w", err))
		return
	}
	if a.deps.Hub != nil {
		a.deps.Hub.Notify(streamID)
	}
	if a.deps.Dispatcher != nil {
		a.deps.Dispatcher.Dispatch(ctx, ev)
	}
	writeJSON(w, http.StatusCreated, PublishResponse{EventID: string(ev.ID)})
}

// ---- helpers ------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
}

func decodeJSON(r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	return nil
}

// errStatus maps store sentinel errors to HTTP codes. Unknown errors
// fall through to 500.
func errStatus(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
