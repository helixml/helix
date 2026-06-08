package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	githubclient "github.com/helixml/helix/api/pkg/github"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/application/topology"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"

	"path/filepath"
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
	// DispatchManual enqueues an operator-driven activation for the
	// given Worker. Called by activateWorker after the synchronous
	// ensureProject step. activationID is the pre-allocated audit-row
	// ID; empty means the Spawner mints its own.
	DispatchManual(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID)
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

	// Topology reconciles activation/team Streams after a reporting
	// line is added or removed via the chart UI. Without it,
	// addWorkerParent / removeWorkerParent write the line but leave the
	// activation-stream observers and team-stream membership stale
	// (the reparent-desync bug). nil is a no-op.
	Topology *topology.Reconciler

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

	// GitHubIdentity is the richer resolver behind GitHubTokenResolver:
	// it reports whether the org's GitHub access is the installed Helix
	// App bot ("app") or a borrowed member OAuth token ("oauth"), plus the
	// installation coordinates. The repo picker uses Mode to choose the
	// right GitHub endpoint (installation repos vs the user's repos) — an
	// installation token cannot list /user/repos. nil → behave exactly as
	// before (OAuth-only). The struct is mirrored here (rather than
	// imported from api/pkg/server) to keep the org package free of a
	// dependency back onto the server package.
	GitHubIdentity func(ctx context.Context, orgID string) (GitHubIdentity, error)

	// GitHubInstallation reports whether the Helix App is installed for the
	// org and, if not, where to send the user to install it. Drives the New
	// Stream "Install Helix" gate. Wired in api/pkg/server (which has the
	// ServiceConnection store + the operator's app slug); nil → the gate
	// falls back to treating the org as not-installable.
	GitHubInstallation func(ctx context.Context, orgID string) (GitHubInstallationStatus, error)

	// GitHubAppRepos lists every repo the org's Helix App(s) can access,
	// aggregated across all installations (so a single app installed on
	// winderai AND helixml returns repos from both). isApp is false when the
	// org has no app (caller then falls back to the user's OAuth repos). Wired
	// in api/pkg/server.
	GitHubAppRepos func(ctx context.Context, orgID string) (repos []string, isApp bool, err error)

	// GitHubManifestStart builds the GitHub App Manifest flow for an org:
	// a Helix-authored manifest + an encrypted state + the GitHub POST URL,
	// which the frontend submits as a form so GitHub creates the app on the
	// user's behalf. Wired in api/pkg/server (needs the encryption key); nil
	// disables the "Create the Helix app" path.
	GitHubManifestStart func(ctx context.Context, orgID, githubOrg, origin string) (GitHubManifestStartResponse, error)

	// PublicServerURL is the operator-configured external base URL
	// (e.g. https://helix.example.com) that auto-installed GitHub
	// webhooks should POST back to. Falls back to localhost when
	// unset — the install-webhook handler refuses on localhost so
	// operators don't paste an unreachable URL into a real repo.
	PublicServerURL string

	// NewID and Now are seams for tests. Production wiring passes
	// system.GenerateID / time.Now.
	NewID func() string
	Now   func() time.Time
}

// GitHubIdentity is the resolved GitHub identity for an org. Mirrors
// server.OrgGitHubIdentity (kept local to avoid a dependency cycle).
type GitHubIdentity struct {
	Mode           string // "app" (installed Helix App bot) | "oauth" (legacy member token)
	Token          string
	AppID          int64
	InstallationID int64
	BaseURL        string
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
		{Pattern: "GET /overview", Handler: http.HandlerFunc(a.getOverview)},
		{Pattern: "GET /roles", Handler: http.HandlerFunc(a.listRoles)},
		{Pattern: "POST /roles", Handler: http.HandlerFunc(a.createRole)},
		{Pattern: "GET /roles/{id}", Handler: http.HandlerFunc(a.getRole)},
		{Pattern: "PUT /roles/{id}", Handler: http.HandlerFunc(a.updateRole)},
		{Pattern: "DELETE /roles/{id}", Handler: http.HandlerFunc(a.deleteRole)},
		{Pattern: "GET /workers", Handler: http.HandlerFunc(a.listWorkers)},
		{Pattern: "POST /workers", Handler: http.HandlerFunc(a.hireWorker)},
		{Pattern: "GET /workers/{id}", Handler: http.HandlerFunc(a.getWorker)},
		{Pattern: "DELETE /workers/{id}", Handler: http.HandlerFunc(a.fireWorker)},
		// Subscriptions are worker-anchored — the Worker Detail page
		// edits the worker's subscription set through these endpoints.
		{Pattern: "GET /workers/{id}/subscriptions", Handler: http.HandlerFunc(a.listWorkerSubscriptions)},
		{Pattern: "POST /workers/{id}/subscriptions", Handler: http.HandlerFunc(a.subscribeWorker)},
		{Pattern: "DELETE /workers/{id}/subscriptions/{stream_id}", Handler: http.HandlerFunc(a.unsubscribeWorker)},
		{Pattern: "POST /workers/{id}/chat", Handler: http.HandlerFunc(a.ensureWorkerChat)},
		{Pattern: "POST /workers/{id}/activate", Handler: http.HandlerFunc(a.activateWorker)},
		{Pattern: "POST /workers/{id}/role", Handler: http.HandlerFunc(a.updateWorkerRole)},
		{Pattern: "POST /workers/{id}/identity", Handler: http.HandlerFunc(a.updateWorkerIdentity)},
		// Reporting lines are many-to-many — add/remove individual
		// manager edges rather than replacing a single parent.
		{Pattern: "POST /workers/{id}/parents", Handler: http.HandlerFunc(a.addWorkerParent)},
		{Pattern: "DELETE /workers/{id}/parents/{parent_id}", Handler: http.HandlerFunc(a.removeWorkerParent)},
		{Pattern: "GET /tools", Handler: http.HandlerFunc(a.listTools)},
		{Pattern: "GET /settings", Handler: http.HandlerFunc(a.listSettings)},
		{Pattern: "PUT /settings/{key}", Handler: http.HandlerFunc(a.setSetting)},
		{Pattern: "DELETE /settings/{key}", Handler: http.HandlerFunc(a.deleteSetting)},
		{Pattern: "GET /streams", Handler: http.HandlerFunc(a.listStreams)},
		{Pattern: "POST /streams", Handler: http.HandlerFunc(a.createStream)},
		{Pattern: "GET /streams/{id}", Handler: http.HandlerFunc(a.getStream)},
		{Pattern: "PUT /streams/{id}", Handler: http.HandlerFunc(a.updateStream)},
		{Pattern: "DELETE /streams/{id}", Handler: http.HandlerFunc(a.deleteStream)},
		{Pattern: "GET /streams/{id}/events", Handler: http.HandlerFunc(a.streamEventsSSE)},
		{Pattern: "POST /streams/{id}/publish", Handler: http.HandlerFunc(a.publishToStream)},
		// Inbound webhook for the GitHub transport. The transport
		// resolves orgID from the request context (set by the org
		// middleware) and reads transport.github from the org's
		// config registry on every delivery, so a single mounted
		// route serves every org without rebinding state.
		{Pattern: "POST /github/webhook", Handler: http.HandlerFunc(a.githubWebhook)},
		// GitHub helper endpoints. listGitHubRepos powers the
		// searchable repo dropdown in the New Stream dialog;
		// installGitHubWebhook calls the GitHub API on behalf of the
		// operator so the only thing the user has to choose is which
		// repo. Both rely on a working GitHubTokenResolver (i.e. the
		// operator has a connected GitHub OAuth).
		{Pattern: "GET /github/repos", Handler: http.HandlerFunc(a.listGitHubRepos)},
		{Pattern: "GET /github/app-installation", Handler: http.HandlerFunc(a.getGitHubAppInstallation)},
		{Pattern: "POST /github/app-manifest", Handler: http.HandlerFunc(a.startGitHubAppManifest)},
		{Pattern: "POST /streams/{id}/github/install-webhook", Handler: http.HandlerFunc(a.installGitHubWebhook)},
		{Pattern: "GET /streams/{id}/github/webhook-status", Handler: http.HandlerFunc(a.getGitHubWebhookStatus)},
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

// ---- Org overview -------------------------------------------------------

// getOverview returns the workers-grouped-by-role payload used by the
// React Overview page (replaces the old position-tree chart).
//
// @Summary Helix-org: get org overview
// @Description Returns roles + workers grouped by role for the helix-org React Overview page.
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.OrgOverview
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/overview [get]
func (a *apiHandler) getOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
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
	writeJSON(w, http.StatusOK, buildOverview(workers, roles))
}

// buildOverview groups workers by their RoleID.
func buildOverview(workers []orgchart.Worker, roles []orgchart.Role) OrgOverview {
	byRole := make(map[orgchart.RoleID][]WorkerBadge)
	for _, wk := range workers {
		rid := wk.RoleID()
		byRole[rid] = append(byRole[rid], WorkerBadge{ID: string(wk.ID()), Kind: string(wk.Kind())})
	}
	sortedRoles := append([]orgchart.Role(nil), roles...)
	sort.SliceStable(sortedRoles, func(i, j int) bool { return sortedRoles[i].ID < sortedRoles[j].ID })
	out := OrgOverview{
		Roles:  make([]RoleBadge, 0, len(sortedRoles)),
		Groups: make([]RoleGroup, 0, len(sortedRoles)),
	}
	for _, ro := range sortedRoles {
		out.Roles = append(out.Roles, RoleBadge{ID: string(ro.ID)})
		group := RoleGroup{RoleID: string(ro.ID), Workers: byRole[ro.ID]}
		sort.SliceStable(group.Workers, func(i, j int) bool { return group.Workers[i].ID < group.Workers[j].ID })
		out.Groups = append(out.Groups, group)
	}
	return out
}

// ---- Roles / Workers ----------------------------------------------------

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
	ctx := r.Context()
	workers, err := a.deps.Store.Workers.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list workers: %w", err))
		return
	}
	// One List call builds the report → managers index so each worker's
	// parent_ids don't cost a query.
	managersByReport := map[orgchart.WorkerID][]string{}
	if a.deps.Store.ReportingLines != nil {
		lines, err := a.deps.Store.ReportingLines.List(ctx, orgID)
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
	roleCache := map[orgchart.RoleID][]string{}
	out := make([]WorkerDTO, 0, len(workers))
	for _, wk := range workers {
		rid := wk.RoleID()
		tools, ok := roleCache[rid]
		if !ok {
			tools = nil
			if role, err := a.deps.Store.Roles.Get(ctx, orgID, rid); err == nil {
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

	owner, err := a.deps.Store.Workers.Get(ctx, orgID, orgchart.WorkerID(a.deps.Owner))
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("load owner %s: %w", a.deps.Owner, err))
		return
	}

	// hire_worker reads its args off tool.Invocation.Args using the
	// same JSON shape MCP delivers — we marshal HireWorkerRequest into
	// the wire form so there is exactly one parser.
	type wireArgs struct {
		ID              string `json:"id,omitempty"`
		RoleID          string `json:"roleId"`
		ParentID        string `json:"parentId,omitempty"`
		Kind            string `json:"kind"`
		IdentityContent string `json:"identityContent"`
	}
	wargs := wireArgs{
		ID:              strings.TrimSpace(req.ID),
		RoleID:          strings.TrimSpace(req.RoleID),
		ParentID:        strings.TrimSpace(req.ParentID),
		Kind:            strings.TrimSpace(req.Kind),
		IdentityContent: req.IdentityContent,
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
func (a *apiHandler) managerIDs(ctx context.Context, orgID string, id orgchart.WorkerID) []string {
	if a.deps.Store.ReportingLines == nil {
		return nil
	}
	managers, err := a.deps.Store.ReportingLines.ListManagers(ctx, orgID, id)
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

	// Tools are derived from the Worker's Role.Tools.
	var (
		toolNames []string
		roDTO     *RoleDTO
	)
	if rid := wk.RoleID(); rid != "" {
		ro, err := a.deps.Store.Roles.Get(ctx, orgID, rid)
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
	if a.deps.ProjectEnsurer == nil {
		writeError(w, http.StatusNotImplemented, errors.New("project ensurer not wired"))
		return
	}
	if a.deps.Dispatcher == nil {
		writeError(w, http.StatusNotImplemented, errors.New("dispatcher not wired"))
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
	// Every identity in helix-org (human or AI, owner or hired) has a
	// chat agent and a desktop; activation runs the same pipeline for
	// all of them. No kind gating — the UI calls this for any worker
	// the operator clicks Restart Desktop on.

	// 1. Synchronously ensure the project + MCP attach. Side effect:
	// dynamicProjectApplier.Ensure re-attaches the helix-org MCP on
	// the agent app, which is the immediate user-visible fix the
	// operator clicked Start Desktop for.
	projectID, agentAppID, _, err := a.deps.ProjectEnsurer.Ensure(ctx, orgID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("ensure project for %s: %w", id, err))
		return
	}

	// 2. Look up the persisted session id (may be empty if this is
	// the first activation). The UI uses it to navigate straight to
	// the desktop viewer; on first activation, it'll wait for the
	// next state refresh.
	var sessionID string
	if state, err := runtimehelix.LoadState(ctx, a.deps.Store, orgID, id); err == nil {
		sessionID = state.SessionID
	}

	// 3. Pre-allocate the audit row so the response can carry the
	// activation_id synchronously. Mirrors hire_worker's pattern —
	// the Spawner picks the row up (matched by Trigger.ActivationID)
	// and Completes it when the activation finishes, rather than
	// minting a sibling.
	var activationID activation.ID
	if a.deps.Store.Activations != nil && a.deps.NewID != nil && a.deps.Now != nil {
		activationID = activation.ID("a-" + a.deps.NewID())
		act, err := activation.New(
			activationID,
			id,
			[]activation.Trigger{{Kind: activation.TriggerManual}},
			a.deps.Now(),
			orgID,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("build manual activation: %w", err))
			return
		}
		if err := a.deps.Store.Activations.Create(ctx, act); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("persist manual activation: %w", err))
			return
		}
	}

	// 4. Enqueue. The dispatcher's per-Worker queue coalesces with
	// any in-flight activation, so a double-click on Start Desktop
	// folds into a single follow-up rather than two parallel runs.
	envPath := ""
	if a.deps.EnvsDir != "" {
		envPath = filepath.Join(a.deps.EnvsDir, string(id))
	}
	a.deps.Dispatcher.DispatchManual(ctx, orgID, id, envPath, activationID)

	writeJSON(w, http.StatusAccepted, WorkerActivateDTO{
		ActivationID: string(activationID),
		ProjectID:    projectID,
		AgentAppID:   agentAppID,
		SessionID:    sessionID,
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
	id := orgchart.WorkerID(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req AddWorkerParentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	managerID := orgchart.WorkerID(strings.TrimSpace(req.ParentID))
	if managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("parent_id is required"))
		return
	}
	if a.deps.Store.ReportingLines == nil {
		writeError(w, http.StatusNotImplemented, errors.New("reporting lines not wired"))
		return
	}
	// Both endpoints must exist before we wire them.
	if _, err := a.deps.Store.Workers.Get(ctx, orgID, id); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", id, err))
		return
	}
	if _, err := a.deps.Store.Workers.Get(ctx, orgID, managerID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get manager %s: %w", managerID, err))
		return
	}

	line, err := orgchart.NewReportingLine(orgID, managerID, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Cycle guard over the DAG: if {id} is already an ancestor of the
	// proposed manager, adding manager → {id} closes a loop. Walk up
	// from the manager via all reporting lines (BFS) and reject if we
	// reach {id}.
	lines, err := a.deps.Store.ReportingLines.List(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list reporting lines: %w", err))
		return
	}
	managersOf := map[orgchart.WorkerID][]orgchart.WorkerID{}
	for _, l := range lines {
		managersOf[l.ReportID] = append(managersOf[l.ReportID], l.ManagerID)
	}
	seen := map[orgchart.WorkerID]bool{}
	queue := []orgchart.WorkerID{managerID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == id {
			writeError(w, http.StatusConflict, fmt.Errorf("making %s report to %s would create a reporting cycle", id, managerID))
			return
		}
		if seen[cur] {
			continue
		}
		seen[cur] = true
		queue = append(queue, managersOf[cur]...)
	}

	if err := a.deps.Store.ReportingLines.Add(ctx, line); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("add reporting line: %w", err))
		return
	}

	// Settle the streams the new edge implies: subscribe the manager to
	// the report's activation stream and add the report to the manager's
	// team stream (creating it on the manager's first report). Pass both
	// endpoints so the manager's team stream is in the reconcile scope.
	if a.deps.Topology != nil {
		if err := a.deps.Topology.Reconcile(ctx, orgID, id, managerID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("reconcile topology: %w", err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
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
	id := orgchart.WorkerID(r.PathValue("id"))
	managerID := orgchart.WorkerID(r.PathValue("parent_id"))
	if id == "" || managerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id and parent_id are required"))
		return
	}
	if a.deps.Store.ReportingLines == nil {
		writeError(w, http.StatusNotImplemented, errors.New("reporting lines not wired"))
		return
	}
	if err := a.deps.Store.ReportingLines.Remove(ctx, orgID, id, managerID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("remove reporting line %s→%s: %w", id, managerID, err))
		return
	}

	// Settle the streams the dropped edge implies: unsubscribe the
	// ex-manager from the report's activation stream and remove the
	// report from the ex-manager's team stream (tearing it down if that
	// was the manager's last report). Pass both endpoints — the
	// ex-manager is no longer in ListManagers(id), so it must be named
	// explicitly to fall in the reconcile scope.
	if a.deps.Topology != nil {
		if err := a.deps.Topology.Reconcile(ctx, orgID, id, managerID); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("reconcile topology: %w", err))
			return
		}
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
	rid := wk.RoleID()
	if rid == "" {
		writeError(w, http.StatusConflict, errors.New("worker has no role"))
		return
	}
	existing, err := a.deps.Store.Roles.Get(ctx, orgID, rid)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get role %s: %w", rid, err))
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
			dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
		}
		if cfg, err := transportConfigMap(s.Transport); err == nil {
			dto.Config = cfg
		}
		subs, err := a.deps.Store.Subscriptions.ListForStream(ctx, orgID, s.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions for %s: %w", s.ID, err))
			return
		}
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Streams page renders them as chips.
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
		dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
	}
	if cfg, err := transportConfigMap(s.Transport); err == nil {
		dto.Config = cfg
	}
	if subs, err := a.deps.Store.Subscriptions.ListForStream(ctx, orgID, s.ID); err == nil {
		for _, sub := range subs {
			// Subscriptions are worker-anchored — return worker ids
			// directly. The Streams page renders them as chips.
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

// resolveEffectivePublicURL returns the base URL helix uses for
// github webhook payload URLs, in the same priority order as
// installGitHubWebhook: `streams.public_url` org config wins,
// SERVER_URL (env) is the fallback. Returns "" when neither is
// set. Surfaced in StreamDTO so the detail page can evaluate the
// loopback warning without re-implementing the priority logic.
func (a *apiHandler) resolveEffectivePublicURL(ctx context.Context, orgID string) string {
	envURL := strings.TrimSpace(a.deps.PublicServerURL)
	if a.deps.Configs != nil {
		if override, err := a.deps.Configs.GetString(ctx, orgID, "streams.public_url"); err == nil {
			if v := strings.TrimSpace(override); v != "" {
				return v
			}
		}
	}
	return envURL
}

// transportConfigMap unmarshals a Transport.Config raw JSON blob
// into a typed map for the StreamDTO `config` field. Returns an
// empty map when the transport has no config (local kind).
func transportConfigMap(t transport.Transport) (map[string]interface{}, error) {
	if len(t.Config) == 0 {
		return nil, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal(t.Config, &out); err != nil {
		return nil, fmt.Errorf("decode transport config: %w", err)
	}
	return out, nil
}

// updateStream rewrites the mutable subset of a stream — name,
// description, and (optionally) transport kind + config. Returns
// the post-update StreamDTO so the UI can replace its cached row
// without a follow-up GET. Composite key (id, orgID) is enforced
// by the repo; cross-org id-guessing returns 404.
//
// @Summary Helix-org: update a stream
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Param id path string true "Stream ID"
// @Param payload body api.UpdateStreamRequest true "Stream patch"
// @Success 200 {object} api.StreamDTO
// @Failure 400 {object} api.ErrorResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id} [put]
func (a *apiHandler) updateStream(w http.ResponseWriter, r *http.Request) {
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
	var req UpdateStreamRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	existing, err := a.deps.Store.Streams.Get(ctx, orgID, id)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", id, err))
		return
	}
	// Start from the existing transport; replace fully when the
	// caller supplies `transport` with both kind and config; replace
	// only the config when only the config is supplied (typical
	// "tweak the github repo or events whitelist" flow).
	tr := existing.Transport
	if req.Transport != nil {
		if k := strings.TrimSpace(req.Transport.Kind); k != "" {
			tr.Kind = transport.Kind(k)
		}
		if req.Transport.Config != nil {
			raw, err := json.Marshal(req.Transport.Config)
			if err != nil {
				writeError(w, http.StatusBadRequest, fmt.Errorf("encode transport config: %w", err))
				return
			}
			tr.Config = raw
		}
	}
	updated, err := streaming.NewStream(
		existing.ID, req.Name, req.Description,
		existing.CreatedBy, existing.CreatedAt, tr, existing.OrganizationID,
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Streams.Update(ctx, updated); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update stream: %w", err))
		return
	}
	// Reuse getStream's response shape — including subscribers,
	// recent events, and the parsed config map — so the UI just
	// swaps its cached row.
	dto := StreamDTO{
		ID:          string(updated.ID),
		Name:        updated.Name,
		Description: updated.Description,
		Kind:        string(updated.Transport.Kind),
		CreatedBy:   string(updated.CreatedBy),
		CreatedAt:   updated.CreatedAt.Format(time.RFC3339),
	}
	dto.CanPublish = updated.Transport.Kind != transport.KindGitHub
	if !dto.CanPublish {
		dto.DisableReason = "github transport is inbound only — act on the repo with `gh` from the worker's environment"
		dto.EffectivePublicURL = a.resolveEffectivePublicURL(ctx, orgID)
	}
	if cfg, err := transportConfigMap(updated.Transport); err == nil {
		dto.Config = cfg
	}
	if subs, err := a.deps.Store.Subscriptions.ListForStream(ctx, orgID, updated.ID); err == nil {
		for _, sub := range subs {
			dto.Subscribers = append(dto.Subscribers, string(sub.WorkerID))
		}
	}
	if events, err := a.deps.Store.Events.ListForStream(ctx, orgID, updated.ID, 50); err == nil {
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

// listWorkerSubscriptions returns the worker's current subscription
// set. Drives the Worker detail page's Subscriptions panel.
//
// @Summary Helix-org: list a worker's subscriptions
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Success 200 {object} api.WorkerSubscriptionsResponse
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions [get]
func (a *apiHandler) listWorkerSubscriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	if wid == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	if _, err := a.deps.Store.Workers.Get(ctx, orgID, wid); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", wid, err))
		return
	}
	subs, err := a.deps.Store.Subscriptions.ListForWorker(ctx, orgID, wid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("list subscriptions: %w", err))
		return
	}
	resp := WorkerSubscriptionsResponse{WorkerID: string(wid), Subscriptions: make([]WorkerSubscriptionDTO, 0, len(subs))}
	for _, sub := range subs {
		resp.Subscriptions = append(resp.Subscriptions, WorkerSubscriptionDTO{
			StreamID:  string(sub.StreamID),
			CreatedAt: sub.CreatedAt.Format(time.RFC3339),
		})
	}
	writeJSON(w, http.StatusOK, resp)
}

// subscribeWorker adds a subscription on the given worker to the
// stream in the request body. Idempotent — re-subscribing returns
// 200 with the existing row's metadata.
//
// @Summary Helix-org: subscribe a worker to a stream
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Param payload body api.SubscribeWorkerRequest true "stream to subscribe to"
// @Success 200 {object} api.WorkerSubscriptionDTO
// @Success 201 {object} api.WorkerSubscriptionDTO
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions [post]
func (a *apiHandler) subscribeWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	if wid == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id is required"))
		return
	}
	var req SubscribeWorkerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	streamID := streaming.StreamID(strings.TrimSpace(req.StreamID))
	if streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream_id is required"))
		return
	}
	if _, err := a.deps.Store.Workers.Get(ctx, orgID, wid); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get worker %s: %w", wid, err))
		return
	}
	if _, err := a.deps.Store.Streams.Get(ctx, orgID, streamID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", streamID, err))
		return
	}
	if existing, err := a.deps.Store.Subscriptions.Find(ctx, orgID, wid, streamID); err == nil {
		writeJSON(w, http.StatusOK, WorkerSubscriptionDTO{
			StreamID:  string(existing.StreamID),
			CreatedAt: existing.CreatedAt.Format(time.RFC3339),
		})
		return
	}
	now := time.Now().UTC()
	if a.deps.Now != nil {
		now = a.deps.Now()
	}
	sub, err := streaming.NewSubscription(string(wid), streamID, now, orgID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.deps.Store.Subscriptions.Create(ctx, sub); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("create subscription: %w", err))
		return
	}
	writeJSON(w, http.StatusCreated, WorkerSubscriptionDTO{
		StreamID:  string(sub.StreamID),
		CreatedAt: sub.CreatedAt.Format(time.RFC3339),
	})
}

// unsubscribeWorker drops the (worker, stream) subscription row.
//
// @Summary Helix-org: unsubscribe a worker from a stream
// @Tags HelixOrg
// @Param id path string true "Worker ID"
// @Param stream_id path string true "Stream ID"
// @Success 204
// @Failure 404 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/workers/{id}/subscriptions/{stream_id} [delete]
func (a *apiHandler) unsubscribeWorker(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	wid := orgchart.WorkerID(r.PathValue("id"))
	streamID := streaming.StreamID(r.PathValue("stream_id"))
	if wid == "" || streamID == "" {
		writeError(w, http.StatusBadRequest, errors.New("worker id and stream id are required"))
		return
	}
	if err := a.deps.Store.Subscriptions.Delete(ctx, orgID, wid, streamID); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("delete subscription: %w", err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
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

// ---- GitHub helper endpoints -------------------------------------------

// GitHubRepoDTO is one entry in the searchable repo dropdown the
// New Stream dialog shows when transport=github is picked. Kept
// intentionally narrow: just the canonical `owner/name` and an
// optional flag so the UI can dim non-admin repos (you can't
// install a webhook without admin rights).
type GitHubRepoDTO struct {
	FullName string `json:"full_name"`
	Private  bool   `json:"private,omitempty"`
}

// GitHubInstallationStatus is the resolved install state for an org plus
// the URL to start an install when it isn't installed yet.
type GitHubInstallationStatus struct {
	// AppExists is true when a Helix GitHub App has been created/registered
	// for this org (a github_app ServiceConnection exists), even if not yet
	// installed on any repo. Drives the gate's create-vs-install branch.
	AppExists bool `json:"app_exists"`
	// Installed is true when the Helix GitHub App has an installation for
	// this org (a github_app ServiceConnection with an installation id).
	Installed bool `json:"installed"`
	// InstallURL is where the New Stream gate sends the user to install the
	// app (https://github.com/apps/<slug>/installations/new). Populated from
	// the created app's slug, or from GITHUB_APP_SLUG for a pre-existing app.
	InstallURL string `json:"install_url,omitempty"`
	// ManageURL is the app's developer-settings page on GitHub
	// (github.com/organizations/<owner>/settings/apps/<slug>) — where you edit
	// permissions, repos, and delete the app. Empty when the owner is unknown
	// (e.g. a BYO app configured without it).
	ManageURL string `json:"manage_url,omitempty"`
}

// GitHubManifestStartResponse is what the frontend needs to POST a GitHub App
// manifest on the user's behalf: the GitHub URL to POST to, the manifest JSON
// to submit as the "manifest" form field, and the CSRF state.
type GitHubManifestStartResponse struct {
	PostURL  string `json:"post_url"`
	Manifest string `json:"manifest"`
	State    string `json:"state"`
}

// getGitHubAppInstallation backs the New Stream "Install Helix" gate: it
// reports whether the org has the Helix App installed and, if not, the URL
// to install it. The user's own GitHub identity is used only for that
// install step; everything afterwards (repo listing, worker git/gh) runs as
// the bot.
//
// @Summary Helix-org: GitHub App install status for the org
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.GitHubInstallationStatus
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/app-installation [get]
func (a *apiHandler) getGitHubAppInstallation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if a.deps.GitHubInstallation == nil {
		// No installation resolver wired — report not-installed with no URL
		// so the gate degrades to "ask your admin" rather than 500ing.
		writeJSON(w, http.StatusOK, GitHubInstallationStatus{})
		return
	}
	status, err := a.deps.GitHubInstallation(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github app installation: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// startGitHubAppManifest builds the GitHub App Manifest flow so the frontend
// can create the Helix app on the user's behalf (org-owned). Body:
// { "github_org": "acme", "origin": "https://helix.example.com" }.
//
// @Summary Helix-org: start the GitHub App manifest (create) flow
// @Tags HelixOrg
// @Accept json
// @Produce json
// @Success 200 {object} api.GitHubManifestStartResponse
// @Failure 412 {object} api.ErrorResponse "manifest flow not wired"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/app-manifest [post]
func (a *apiHandler) startGitHubAppManifest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if a.deps.GitHubManifestStart == nil {
		writeError(w, http.StatusPreconditionFailed, errors.New("GitHub App manifest flow is not enabled on this deployment"))
		return
	}
	var body struct {
		GitHubOrg string `json:"github_org"`
		Origin    string `json:"origin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	resp, err := a.deps.GitHubManifestStart(ctx, orgID, body.GitHubOrg, body.Origin)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// GitHubReposResponse is the body of GET /github/repos.
type GitHubReposResponse struct {
	Repos []GitHubRepoDTO `json:"repos"`
	// Source identifies which token paid for this list — useful
	// when debugging "I can't see repo X" reports.
	Source string `json:"source,omitempty"`
}

// listGitHubRepos returns every repo the connected GitHub OAuth
// token can see, sorted alphabetically. Drives the searchable
// dropdown so operators don't have to remember the exact
// `owner/name` they want to wire up.
//
// @Summary Helix-org: list GitHub repos accessible to the org's connected token
// @Tags HelixOrg
// @Produce json
// @Success 200 {object} api.GitHubReposResponse
// @Failure 412 {object} api.ErrorResponse "no GitHub token configured"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/github/repos [get]
func (a *apiHandler) listGitHubRepos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID, err := resolveOrgID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// App mode: aggregate repos across every installation of the org's Helix
	// App(s) — so one app installed on winderai + helixml returns both. The
	// installation token can't call /user/repos, so this lists
	// /installation/repositories per installation server-side.
	if a.deps.GitHubAppRepos != nil {
		names, isApp, err := a.deps.GitHubAppRepos(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusBadGateway, fmt.Errorf("list github app repos: %w", err))
			return
		}
		if isApp {
			out := GitHubReposResponse{Repos: make([]GitHubRepoDTO, 0, len(names)), Source: "app"}
			for _, n := range names {
				out.Repos = append(out.Repos, GitHubRepoDTO{FullName: n})
			}
			writeJSON(w, http.StatusOK, out)
			return
		}
	}

	// Legacy OAuth fallback: list the connecting user's repos.
	var token string
	switch {
	case a.deps.GitHubIdentity != nil:
		id, err := a.deps.GitHubIdentity(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github identity: %w", err))
			return
		}
		token = id.Token
	case a.deps.GitHubTokenResolver != nil:
		t, err := a.deps.GitHubTokenResolver(ctx, orgID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github token: %w", err))
			return
		}
		token = t
	default:
		writeError(w, http.StatusPreconditionFailed, errors.New("no GitHub identity wired; install the Helix GitHub App or connect a GitHub account"))
		return
	}
	if token == "" {
		writeError(w, http.StatusPreconditionFailed, errors.New("Helix GitHub App not installed for this org and no GitHub OAuth connection found"))
		return
	}
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("build github client: %w", err))
		return
	}
	names, err := client.LoadRepos()
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("list github repos: %w", err))
		return
	}
	out := GitHubReposResponse{Repos: make([]GitHubRepoDTO, 0, len(names)), Source: "oauth"}
	for _, n := range names {
		out.Repos = append(out.Repos, GitHubRepoDTO{FullName: n})
	}
	writeJSON(w, http.StatusOK, out)
}

// InstallGitHubWebhookResponse is the body of POST
// /streams/{id}/github/install-webhook.
type InstallGitHubWebhookResponse struct {
	WebhookID      int64  `json:"webhook_id"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	PayloadURL     string `json:"payload_url"`
	// Warning is a non-fatal message about the just-installed
	// webhook — e.g. "SERVER_URL is a loopback address so GitHub's
	// servers can't actually deliver to this URL". The webhook IS
	// installed on GitHub; the warning just tells the operator
	// what needs fixing on their side for deliveries to flow.
	Warning string `json:"warning,omitempty"`
}

// GitHubWebhookStatusResponse is the body of GET
// /streams/{id}/github/webhook-status. It reports the LIVE state of the
// stream's repo webhook as seen on GitHub (not the stored config), so the
// detail page can show a link when the hook really exists and a re-install
// button when it doesn't — and self-correct stale stored ids.
type GitHubWebhookStatusResponse struct {
	// State is one of:
	//   "installed" — a webhook for this stream's payload URL exists on the repo
	//   "missing"   — GitHub was reachable and has no such webhook (needs install)
	//   "unknown"   — couldn't determine (no repo / no public URL / no creds /
	//                 GitHub error); see Detail. The UI falls back to stored state.
	State          string `json:"state"`
	WebhookID      int64  `json:"webhook_id,omitempty"`
	WebhookHTMLURL string `json:"webhook_html_url,omitempty"`
	Active         bool   `json:"active,omitempty"`
	PayloadURL     string `json:"payload_url,omitempty"`
	// Detail explains a "unknown" state (and is empty otherwise).
	Detail string `json:"detail,omitempty"`
}

// installGitHubWebhook calls the GitHub REST API on behalf of the
// operator to register a webhook on the stream's repo pointing at
// helix's per-stream payload URL. Idempotent: if a webhook with
// the same URL already exists on the repo, we adopt it (no
// double-install). Persists the resulting webhook id + edit-page
// URL on the stream's transport config so the detail page can
// deep-link out.
//
// Pre-conditions:
//   - transport=github stream
//   - GitHubTokenResolver returns a non-empty token
//   - transport.github.webhook_secret configured on the org; if
//     missing, helix auto-generates one and persists it (the
//     operator never has to copy it manually).
//   - PublicServerURL set to a non-localhost origin (refused
//     otherwise — GitHub's servers can't reach localhost).
//
// @Summary Helix-org: auto-install the webhook for a github stream
// @Tags HelixOrg
// @Param id path string true "Stream ID"
// @Produce json
// @Success 200 {object} api.InstallGitHubWebhookResponse
// @Failure 400 {object} api.ErrorResponse
// @Failure 412 {object} api.ErrorResponse "pre-conditions not met"
// @Failure 502 {object} api.ErrorResponse "GitHub API call failed"
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/github/install-webhook [post]
func (a *apiHandler) installGitHubWebhook(w http.ResponseWriter, r *http.Request) {
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
	// SERVER_URL reachability is checked for a warning, but does
	// NOT block the install — operators want to set the webhook up
	// even on a local dev machine and fix the URL once their
	// cloudflared/ngrok/reverse-proxy is wired. We collect the
	// warning here and return it in the response so the UI can
	// surface it next to the success.
	// Resolve the public base URL for the webhook payload URL:
	//   1. `streams.public_url` org config (UI-editable) wins, so
	//      an admin can fix a loopback SERVER_URL via the
	//      Settings page without touching .env.
	//   2. Else fall back to PublicServerURL (SERVER_URL env on
	//      the helix host).
	// Empty result is a hard refusal — without a URL we'd register
	// a relative path with GitHub which the API either rejects or
	// silently fails on. A loopback URL is different: well-formed,
	// just unreachable, so we install + warn so the operator can
	// fix it later.
	publicURL := strings.TrimSpace(a.deps.PublicServerURL)
	if a.deps.Configs != nil {
		if override, err := a.deps.Configs.GetString(ctx, orgID, "streams.public_url"); err == nil && strings.TrimSpace(override) != "" {
			publicURL = strings.TrimSpace(override)
		}
	}
	if publicURL == "" {
		writeError(w, http.StatusPreconditionFailed, errors.New("no public URL configured for helix. Set `streams.public_url` on the helix-org Settings page (or SERVER_URL in helix's .env), then re-install the webhook."))
		return
	}
	// GitHub's webhook API refuses to register hooks pointed at
	// loopback addresses with `422 url is not supported because
	// it isn't reachable over the public Internet`. There's no
	// way to override that on GitHub's end, so we pre-flight
	// the check here and return a clear, actionable 412 instead.
	// Operators can fix it from the helix-org Settings page by
	// setting `streams.public_url` (no .env edit needed).
	if u, err := url.Parse(publicURL); err == nil {
		host := strings.ToLower(u.Hostname())
		if host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" {
			writeError(w, http.StatusPreconditionFailed, fmt.Errorf("public URL %q is a loopback address — GitHub refuses to install webhooks pointed at unreachable hosts. Set `streams.public_url` on the helix-org Settings page to a publicly reachable hostname (cloudflared / ngrok / reverse proxy), or update SERVER_URL in helix's .env and restart the api container", publicURL))
			return
		}
	}
	s, err := a.deps.Store.Streams.Get(ctx, orgID, streamID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", streamID, err))
		return
	}
	if s.Transport.Kind != transport.KindGitHub {
		writeError(w, http.StatusBadRequest, fmt.Errorf("stream %s is not a github transport (kind=%s)", streamID, s.Transport.Kind))
		return
	}
	cfg, err := s.Transport.GitHubConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse github config: %w", err))
		return
	}
	if cfg.Repo == "" {
		writeError(w, http.StatusBadRequest, errors.New("stream's github config has no repo set; edit the stream first"))
		return
	}
	if len(cfg.Events) == 0 {
		cfg.Events = []string{"*"}
	}
	if a.deps.GitHubTokenResolver == nil {
		writeError(w, http.StatusPreconditionFailed, errors.New("no GitHubTokenResolver wired"))
		return
	}
	token, err := a.deps.GitHubTokenResolver(ctx, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("resolve github token: %w", err))
		return
	}
	if token == "" {
		writeError(w, http.StatusPreconditionFailed, errors.New("no GitHub credentials for this org: install the Helix GitHub App (preferred) or connect GitHub OAuth on the Connected Services page"))
		return
	}
	secret, err := ensureGitHubWebhookSecret(ctx, a.deps.Configs, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("ensure webhook secret: %w", err))
		return
	}
	repoParts := strings.SplitN(cfg.Repo, "/", 2)
	if len(repoParts) != 2 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("malformed repo %q", cfg.Repo))
		return
	}
	owner, repoName := repoParts[0], repoParts[1]
	payloadURL := strings.TrimRight(publicURL, "/") +
		"/api/v1/orgs/" + url.PathEscape(orgID) +
		"/streams/" + url.PathEscape(string(streamID)) + "/github/webhook"
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("build github client: %w", err))
		return
	}
	hook, err := client.UpsertWebhook(owner, repoName, "web", payloadURL, cfg.Events, secret)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("create github webhook: %w", err))
		return
	}
	htmlURL := githubclient.WebhookSettingsURL(owner, repoName, hook.ID)
	cfg.WebhookID = hook.ID
	cfg.WebhookHTMLURL = htmlURL
	cfgRaw, err := json.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("re-marshal config: %w", err))
		return
	}
	s.Transport.Config = cfgRaw
	if err := a.deps.Store.Streams.Update(ctx, s); err != nil {
		writeError(w, errStatus(err), fmt.Errorf("update stream after webhook install: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, InstallGitHubWebhookResponse{
		WebhookID:      hook.ID,
		WebhookHTMLURL: htmlURL,
		PayloadURL:     payloadURL,
	})
}

// getGitHubWebhookStatus reports the LIVE state of a github stream's repo
// webhook by listing the repo's hooks (as the bot) and matching this stream's
// payload URL. Read-only: it never creates, edits, or persists. Returns
// state="unknown" (rather than an HTTP error) for every "can't tell" case so
// the detail page can degrade to stored config instead of showing a scary
// failure for an ordinarily-fine condition (e.g. no public URL configured yet).
//
// @Summary Helix-org: live webhook status for a github stream
// @Tags HelixOrg
// @Param id path string true "Stream ID"
// @Produce json
// @Success 200 {object} api.GitHubWebhookStatusResponse
// @Failure 400 {object} api.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/orgs/{org}/streams/{id}/github/webhook-status [get]
func (a *apiHandler) getGitHubWebhookStatus(w http.ResponseWriter, r *http.Request) {
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
	s, err := a.deps.Store.Streams.Get(ctx, orgID, streamID)
	if err != nil {
		writeError(w, errStatus(err), fmt.Errorf("get stream %s: %w", streamID, err))
		return
	}
	if s.Transport.Kind != transport.KindGitHub {
		writeError(w, http.StatusBadRequest, fmt.Errorf("stream %s is not a github transport (kind=%s)", streamID, s.Transport.Kind))
		return
	}
	cfg, err := s.Transport.GitHubConfig()
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parse github config: %w", err))
		return
	}

	unknown := func(detail string) {
		writeJSON(w, http.StatusOK, GitHubWebhookStatusResponse{State: "unknown", Detail: detail})
	}
	if cfg.Repo == "" {
		unknown("stream has no repo set")
		return
	}
	repoParts := strings.SplitN(cfg.Repo, "/", 2)
	if len(repoParts) != 2 {
		unknown(fmt.Sprintf("malformed repo %q", cfg.Repo))
		return
	}
	owner, repoName := repoParts[0], repoParts[1]

	// Same payload-URL resolution as install (streams.public_url override on top
	// of SERVER_URL). Without a public URL we can't compute the URL GitHub would
	// hold, so we can't match a hook — report unknown.
	publicURL := strings.TrimSpace(a.deps.PublicServerURL)
	if a.deps.Configs != nil {
		if override, err := a.deps.Configs.GetString(ctx, orgID, "streams.public_url"); err == nil && strings.TrimSpace(override) != "" {
			publicURL = strings.TrimSpace(override)
		}
	}
	if publicURL == "" {
		unknown("no public URL configured for helix")
		return
	}
	payloadURL := strings.TrimRight(publicURL, "/") +
		"/api/v1/orgs/" + url.PathEscape(orgID) +
		"/streams/" + url.PathEscape(string(streamID)) + "/github/webhook"

	if a.deps.GitHubTokenResolver == nil {
		unknown("no GitHubTokenResolver wired")
		return
	}
	token, err := a.deps.GitHubTokenResolver(ctx, orgID)
	if err != nil {
		unknown(fmt.Sprintf("resolve github token: %v", err))
		return
	}
	if token == "" {
		unknown("no GitHub credentials for this org (install the Helix GitHub App or connect GitHub OAuth)")
		return
	}
	client, err := githubclient.NewGithubClient(githubclient.ClientOptions{Ctx: ctx, Token: token})
	if err != nil {
		unknown(fmt.Sprintf("build github client: %v", err))
		return
	}
	hook, found, err := client.FindWebhook(owner, repoName, payloadURL)
	if err != nil {
		// GitHub reachable-but-errored (e.g. missing repository_hooks
		// permission, 404 on a repo the bot can't see). Can't assert
		// "missing", so report unknown with the reason.
		unknown(fmt.Sprintf("list webhooks on %s: %v", cfg.Repo, err))
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, GitHubWebhookStatusResponse{State: "missing", PayloadURL: payloadURL})
		return
	}
	writeJSON(w, http.StatusOK, GitHubWebhookStatusResponse{
		State:          "installed",
		WebhookID:      hook.ID,
		WebhookHTMLURL: githubclient.WebhookSettingsURL(owner, repoName, hook.ID),
		Active:         hook.Active,
		PayloadURL:     payloadURL,
	})
}

// ensureGitHubWebhookSecret reads the org's transport.github
// webhook_secret. If it's unset, generates a 32-byte random hex
// secret and persists it so future webhook installs (and the
// HMAC verifier on inbound deliveries) use the same value. This
// removes the manual "go to Settings, paste a secret" step from
// the operator's flow.
func ensureGitHubWebhookSecret(ctx context.Context, reg *configregistry.Registry, orgID string) (string, error) {
	if reg == nil {
		return "", errors.New("config registry not wired")
	}
	var cfg struct {
		Token         string `json:"token,omitempty"`
		WebhookSecret string `json:"webhook_secret,omitempty"`
	}
	_ = reg.GetObject(ctx, orgID, "transport.github", &cfg)
	if cfg.WebhookSecret != "" {
		return cfg.WebhookSecret, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	cfg.WebhookSecret = hex.EncodeToString(buf)
	out, err := json.Marshal(cfg)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}
	// Persist as the system owner — webhook-secret bootstrap is
	// helix self-care, not an operator-attributed change.
	if err := reg.Set(ctx, orgID, "transport.github", string(out), orgchart.WorkerID("w-owner")); err != nil {
		return "", fmt.Errorf("persist secret: %w", err)
	}
	return cfg.WebhookSecret, nil
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
