package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
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
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
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

// ProjectEnsurer provisions (or fast-paths) the per-Worker Helix
// project + agent app for a Worker. Mirrors
// runtimehelix.WorkerProject.Ensure. The chart UI's worker detail
// page calls POST /workers/{id}/chat which routes through this to
// guarantee an agent_app_id exists before redirecting to /agent/.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, orgID string, workerID orgchart.WorkerID) (projectID, agentAppID, repoID string, err error)
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

	// Tools is the same tools registry the MCP server exposes — used
	// by GET /tools so the chart UI's role-editor multi-select can
	// render the catalogue of available grants. nil = endpoint
	// returns an empty list (degrade gracefully on test wirings that
	// don't bother building a registry).
	Tools *tools.Registry

	// ProjectEnsurer provisions (or fast-paths) a per-Worker Helix
	// project + agent app so the worker detail page's "Start new
	// chat" button can land on /agent/{agent_app_id}. Bootstrap
	// doesn't run this — first activation does — so the owner worker
	// has no agent app until someone calls Ensure. The chart's
	// POST /workers/{id}/chat endpoint exposes the call. nil disables
	// the endpoint (returns 501).
	ProjectEnsurer ProjectEnsurer

	// Lifecycle owns the cross-cutting Fire cascade (Helix project +
	// app teardown, store cleanup, env-dir removal). nil disables
	// DELETE /workers/{id} (returns 501).
	Lifecycle *lifecycle.Service

	// GitHubTokenResolver is the production hook for "reinstate the
	// GitHub stream + reuse the existing GitHub integration for
	// Auth". When transport.github.token is empty, the github
	// transport calls this to look up a GitHub OAuth connection
	// owned by an org member and return its access token. nil
	// disables the fallback — the transport's Token() then returns
	// empty string and downstream consumers degrade accordingly.
	//
	// Signature mirrors githubtransport.TokenResolver to avoid a
	// dependency cycle; the wiring in api/pkg/server adapts the
	// helix OAuth manager into this shape.
	GitHubTokenResolver func(ctx context.Context, orgID string) (string, error)

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
		{Pattern: "DELETE /positions/{id}", Handler: http.HandlerFunc(a.deletePosition)},
		{Pattern: "GET /roles", Handler: http.HandlerFunc(a.listRoles)},
		{Pattern: "POST /roles", Handler: http.HandlerFunc(a.createRole)},
		{Pattern: "GET /roles/{id}", Handler: http.HandlerFunc(a.getRole)},
		{Pattern: "PUT /roles/{id}", Handler: http.HandlerFunc(a.updateRole)},
		{Pattern: "DELETE /roles/{id}", Handler: http.HandlerFunc(a.deleteRole)},
		{Pattern: "GET /workers", Handler: http.HandlerFunc(a.listWorkers)},
		{Pattern: "POST /workers", Handler: http.HandlerFunc(a.hireWorker)},
		{Pattern: "GET /workers/{id}", Handler: http.HandlerFunc(a.getWorker)},
		{Pattern: "DELETE /workers/{id}", Handler: http.HandlerFunc(a.fireWorker)},
		{Pattern: "POST /workers/{id}/chat", Handler: http.HandlerFunc(a.ensureWorkerChat)},
		{Pattern: "POST /workers/{id}/role", Handler: http.HandlerFunc(a.updateWorkerRole)},
		{Pattern: "POST /workers/{id}/identity", Handler: http.HandlerFunc(a.updateWorkerIdentity)},
		{Pattern: "GET /tools", Handler: http.HandlerFunc(a.listTools)},
		{Pattern: "GET /settings", Handler: http.HandlerFunc(a.listSettings)},
		{Pattern: "PUT /settings/{key}", Handler: http.HandlerFunc(a.setSetting)},
		{Pattern: "DELETE /settings/{key}", Handler: http.HandlerFunc(a.deleteSetting)},
		{Pattern: "GET /streams", Handler: http.HandlerFunc(a.listStreams)},
		{Pattern: "POST /streams", Handler: http.HandlerFunc(a.createStream)},
		{Pattern: "GET /streams/{id}", Handler: http.HandlerFunc(a.getStream)},
		{Pattern: "DELETE /streams/{id}", Handler: http.HandlerFunc(a.deleteStream)},
		{Pattern: "GET /streams/{id}/events", Handler: http.HandlerFunc(a.streamEventsSSE)},
		{Pattern: "POST /streams/{id}/publish", Handler: http.HandlerFunc(a.publishToStream)},
		// Inbound webhook for the GitHub transport. The transport
		// resolves orgID from the request context (set by the org
		// middleware) and reads transport.github from the org's
		// config registry on every delivery, so a single mounted
		// route serves every org without rebinding state.
		{Pattern: "POST /github/webhook", Handler: http.HandlerFunc(a.githubWebhook)},
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
	roles, err := a.deps.Store.Roles.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list roles: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, buildChart(positions, workers, roles))
}

// buildChart walks positions + workers into the tree the chart
// renders. Exported so it can be reused by future in-process
// consumers (e.g. an MCP tool surfacing the same shape) without going
// through HTTP.
func buildChart(positions []orgchart.Position, workers []orgchart.Worker, roles []orgchart.Role) Chart {
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
	// Roles list — every role visible to the org, sorted by ID,
	// surfaced so the React chart can render empty role groups (a
	// role with no positions yet still appears as a group ready to
	// receive its first position).
	sortedRoles := append([]orgchart.Role(nil), roles...)
	sort.SliceStable(sortedRoles, func(i, j int) bool { return sortedRoles[i].ID < sortedRoles[j].ID })
	out.Roles = make([]RoleBadge, 0, len(sortedRoles))
	for _, ro := range sortedRoles {
		out.Roles = append(out.Roles, RoleBadge{ID: string(ro.ID)})
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

// listTools returns the catalogue of available MCP tools the org
// can grant to its roles. Powers the role editor's multi-select.
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
	// Populate the agent app id + project id from the helix-runtime
	// sidecar so the chart UI can deep-link "chat with worker" to the
	// per-project Human Desktop session. Missing state = the worker
	// hasn't activated yet; we leave the fields empty and the UI
	// shows a disabled button.
	if state, err := runtimehelix.LoadState(ctx, a.deps.Store, orgID, id); err == nil {
		detail.AgentAppID = state.AgentAppID
		detail.ProjectID = state.ProjectID
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	if _, err := a.deps.Store.Workers.Get(ctx, orgID, id); err != nil {
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

// createStream creates a new Stream. Mirrors the MCP create_stream
// tool — same Transport shape, same "id auto-falls-back-to-s-<uuid>"
// behaviour. CreatedBy is set to the embedded owner worker so REST
// + chat creations are attributable.
//
// @Summary Helix-org: create a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param payload body api.CreateStreamRequest true "Stream spec"
// @Success 201 {object} api.StreamDTO
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams [post]
func (a *apiHandler) createStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var req CreateStreamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	id := streaming.StreamID(strings.TrimSpace(req.ID))
	if id == "" {
		if a.deps.NewID == nil {
			writeError(w, http.StatusInternalServerError, errors.New("NewID not wired"))
			return
		}
		id = streaming.StreamID("s-" + a.deps.NewID())
	}
	tr := transport.Transport{}
	if req.Transport != nil {
		tr.Kind = transport.Kind(req.Transport.Kind)
		if len(req.Transport.Config) > 0 {
			raw, err := json.Marshal(req.Transport.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("encode transport config: %w", err))
				return
			}
			tr.Config = raw
		}
	}
	now := time.Now().UTC()
	if a.deps.Now != nil {
		now = a.deps.Now()
	}
	owner := orgchart.WorkerID(a.deps.Owner)
	s, err := streaming.NewStream(id, req.Name, req.Description, owner, now, tr, orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Streams.Create(ctx, s); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("create stream: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, StreamDTO{
		ID:          string(s.ID),
		Name:        s.Name,
		Description: s.Description,
		Kind:        string(s.Transport.Kind),
		CreatedBy:   string(s.CreatedBy),
		CreatedAt:   s.CreatedAt.Format(time.RFC3339),
	})
}

// getStream returns a single stream + its current subscribers and
// recent events. Powers the stream detail page.
//
// @Summary Helix-org: get a stream
// @Tags HelixOrg
// @Produce json
// @Param id path string true "Stream ID"
// @Success 200 {object} api.StreamDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [get]
func (a *apiHandler) getStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.StreamID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	s, err := a.deps.Store.Streams.Get(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", id, err))
		return
	}
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
	if subs, err := a.deps.Store.Subscriptions.ListForStream(ctx, orgID, s.ID); err == nil {
		for _, sub := range subs {
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Store.Events.ListForStream(ctx, orgID, s.ID, 50); err == nil {
		for _, ev := range events {
			dto.RecentEvents = append(dto.RecentEvents, eventCard(ev))
		}
	}
	writeJSON(w, http.StatusOK, dto)
}

// deleteStream removes a stream row. Subscriptions and events are
// NOT cascade-deleted in this iteration — the caller is expected to
// drain them first via unsubscribe / publish flows. Empty stream
// rows are idempotent (404 → 404, no error).
//
// @Summary Helix-org: delete a stream
// @Tags HelixOrg
// @Param id path string true "Stream ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [delete]
func (a *apiHandler) deleteStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id := streaming.StreamID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream id is required"))
		return
	}
	if err := a.deps.Store.Streams.Delete(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete stream: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// githubDispatcher adapts the api.Dispatcher into the
// github.Dispatcher interface. The two are structurally identical;
// the adapter exists only so the github package can keep its own
// Dispatcher type without importing api (would create a cycle).
type githubDispatcher struct{ inner Dispatcher }

func (d githubDispatcher) Dispatch(ctx context.Context, ev streaming.Event) {
	if d.inner == nil {
		return
	}
	d.inner.Dispatch(ctx, ev)
}

// githubWebhook is the per-request dispatcher for POST /github/webhook.
// Builds a github.Transport bound to the request's orgID (resolved
// from the org middleware's context) and hands the request off to
// its HandleInbound. Building per-request keeps the route stateless
// — a single mounted handler serves every org.
//
// @Summary Helix-org: inbound GitHub webhook
// @Tags HelixOrg
// @Param payload body object true "Raw GitHub webhook delivery"
// @Success 204 "Delivery accepted and fanned out"
// @Success 200 "Delivery accepted but no matching streams"
// @Failure 401 {object} api.ErrorResponse "Bad or missing X-Hub-Signature-256"
// @Failure 503 {object} api.ErrorResponse "transport.github not configured"
// @Router /api/v1/orgs/{org}/github/webhook [post]
func (a *apiHandler) githubWebhook(w http.ResponseWriter, r *http.Request) {
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	t := githubtransport.New(
		orgID,
		a.deps.Configs,
		a.deps.Store,
		a.deps.Hub,
		githubDispatcher{inner: a.deps.Dispatcher},
		slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})),
	)
	if a.deps.GitHubTokenResolver != nil {
		t = t.WithTokenResolver(githubtransport.TokenResolver(a.deps.GitHubTokenResolver))
	}
	t.HandleInbound().ServeHTTP(w, r)
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
