package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/workers"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
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
	// DispatchManual enqueues an operator-driven activation for the
	// given Worker. Called by activateWorker after the synchronous
	// ensureProject step. activationID is the pre-allocated audit-row
	// ID; empty means the Spawner mints its own.
	DispatchManual(ctx context.Context, orgID string, workerID orgchart.WorkerID, activationID activation.ID)
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
// PublicURL / DBPath / EnvsDir are the operational state the settings
// page surfaces (today they come from CLI flags; the SaaS embedding
// leaves PublicURL empty).
type Deps struct {
	// Application services. The REST handlers are thin adapters over
	// these — constructed once at the composition root (and in the test
	// helper). The api package holds NO store.* repository, so the
	// compiler now forbids any handler reaching past a service into the
	// store (the Phase-D enforcement gate).
	Streams       *streams.Streams
	Roles         *roles.Roles
	Workers       *workers.Workers
	Subscriptions *subscriptions.Subscriptions
	Publishing    *publishing.Publishing
	Activations   *activations.Activations
	// Queries is the read facade for every projection the read handlers
	// render. One service spanning several repos (reads carry no
	// invariants to split on).
	Queries *queries.Queries

	Configs    *configregistry.Registry
	Hub        *wakebus.Bus
	Dispatcher Dispatcher

	// WorkerRuntime reads a Worker's runtime-state sidecar (project /
	// agent-app / session ids). A small port so the worker-detail and
	// activate handlers don't touch the store; implemented at the
	// composition root over runtimehelix.LoadState. nil → those fields
	// render empty.
	WorkerRuntime WorkerRuntime

	// SessionRestarter recreates a worker's desktop container — the
	// backend "restart the agent" primitive shared with the in-chat
	// restart button. nil → restartWorkerAgent falls back to a fresh
	// activation when the worker has a live session it can't restart.
	SessionRestarter SessionRestarter

	// GitHubInbound builds the inbound GitHub-webhook handler for an org
	// (the transport reads matching streams + appends events). Built at
	// the composition root so the api adapter never holds the store. nil
	// → POST /github/webhook returns 503.
	GitHubInbound func(orgID string) http.Handler

	PublicURL string
	DBPath    string

	// Tools is the same tools registry the MCP server exposes — used
	// by GET /tools so the chart UI's role-editor multi-select can
	// render the catalogue of available tools. nil = endpoint
	// returns an empty list (degrade gracefully on test wirings that
	// don't bother building a registry).
	Tools *mcptools.Registry

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
}

// WorkerRuntimeInfo is the subset of a Worker's runtime-state sidecar
// the REST adapter surfaces: the per-project deep-link ids and the
// current desktop session id.
type WorkerRuntimeInfo struct {
	ProjectID  string
	AgentAppID string
	SessionID  string
}

// WorkerRuntime resolves a Worker's runtime-state sidecar. Declared here
// (implemented at the composition root over runtimehelix.LoadState) so
// the worker-detail and activate handlers read project/agent/session ids
// without the api adapter touching the store.
type WorkerRuntime interface {
	State(ctx context.Context, orgID string, workerID orgchart.WorkerID) (WorkerRuntimeInfo, error)
}

// SessionRestarter recreates the desktop container backing a session —
// the single canonical "restart the agent" backend operation
// (StopDesktop → recreate → reset crashed prompts). The worker-page
// "Restart agent session" button routes through restartWorkerAgent into
// this port so it shares one implementation with the in-chat
// /sessions/{id}/restart-agent endpoint. Wired at the composition root
// over the in-proc helix client; nil → the handler falls back to a fresh
// activation.
type SessionRestarter interface {
	RestartSession(ctx context.Context, sessionID string) error
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
		{Pattern: "POST /workers/{id}/restart-agent", Handler: http.HandlerFunc(a.restartWorkerAgent)},
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
