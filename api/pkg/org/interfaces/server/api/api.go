package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/chartlayout"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/topics"
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
// a client posts an event into a topic. Defined here (rather than
// imported from server.go's sibling) to keep the import edge
// one-directional — server/api is below server, not next to it.
type Dispatcher interface {
	Dispatch(ctx context.Context, ev streaming.Event)
	// DispatchManual enqueues an operator-driven activation for the
	// given Bot. Called by activateBot after the synchronous
	// ensureProject step. activationID is the pre-allocated audit-row
	// ID; empty means the Spawner mints its own.
	DispatchManual(ctx context.Context, orgID string, botID orgchart.BotID, activationID activation.ID)
}

// ProjectEnsurer provisions (or fast-paths) the per-Bot Helix project +
// agent app for a Bot. Mirrors runtimehelix.BotProject.Ensure. The
// chart UI's bot detail page calls POST /bots/{id}/chat which routes
// through this to guarantee an agent_app_id exists before redirecting to
// /agent/.
type ProjectEnsurer interface {
	Ensure(ctx context.Context, orgID string, botID orgchart.BotID) (projectID, agentAppID, repoID string, err error)
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
	Topics *topics.Topics
	// Bots is the merged role+worker mutation service: content/tools
	// updates (PATCH /bots/{id}) and reporting-line edges
	// (AddParent/RemoveParent). Creation/deletion go through Lifecycle.
	Bots          *bots.Bots
	Subscriptions *subscriptions.Subscriptions
	Publishing    *publishing.Publishing
	Activations   *activations.Activations
	// Processors owns the processor CRUD + preview use cases. nil →
	// the /processors routes return 503 (test wirings that skip it).
	Processors *processors.Processors
	// ChartLayout owns free-placed canvas coordinates for the org chart
	// UI. nil → /chart/positions routes return 503.
	ChartLayout *chartlayout.Service
	// Queries is the read facade for every projection the read handlers
	// render. One service spanning several repos (reads carry no
	// invariants to split on).
	Queries *queries.Queries

	Configs    *configregistry.Registry
	Hub        *wakebus.Bus
	Dispatcher Dispatcher

	// BotRuntime reads a Bot's runtime-state sidecar (project /
	// agent-app / session ids). A small port so the bot-detail and
	// activate handlers don't touch the store; implemented at the
	// composition root over runtimehelix.LoadState. nil → those fields
	// render empty.
	BotRuntime BotRuntime

	// BotSessionResetter fully tears down a bot's current session (stops
	// the desktop, deletes the session row, clears the persisted session
	// pointer) so restartBotAgent's subsequent Activate provisions a
	// brand-new session on a fresh desktop with newly added MCP services.
	// nil → restartBotAgent skips the reset and just re-activates.
	BotSessionResetter BotSessionResetter
	// BotDesktopStopper stops the desktop only (session + transcript kept).
	// nil → stopBotAgent returns 501.
	BotDesktopStopper BotDesktopStopper

	// GitHubInbound builds the inbound GitHub-webhook handler for an org
	// (the transport reads matching topics + appends events). Built at
	// the composition root so the api adapter never holds the store. nil
	// → POST /github/webhook returns 503.
	GitHubInbound func(orgID string) http.Handler

	PublicURL string
	DBPath    string

	// Tools is the same tools registry the MCP server exposes — used
	// by GET /tools so the chart UI's bot-editor multi-select can
	// render the catalogue of available tools. nil = endpoint
	// returns an empty list (degrade gracefully on test wirings that
	// don't bother building a registry).
	Tools *mcptools.Registry

	// ProjectEnsurer provisions (or fast-paths) a per-Bot Helix
	// project + agent app so the bot detail page's "Start new
	// chat" button can land on /agent/{agent_app_id}. Bootstrap
	// doesn't run this — first activation does. The chart's
	// POST /bots/{id}/chat endpoint exposes the call. nil disables
	// the endpoint (returns 501).
	ProjectEnsurer ProjectEnsurer

	// Lifecycle owns the cross-cutting Create + Delete cascades (bot
	// row, reporting lines, Helix project + app teardown, store
	// cleanup, topology reconcile). nil disables POST /bots and
	// DELETE /bots/{id} (returns 501).
	Lifecycle *lifecycle.Service

	// GitHubTokenResolver is the production hook for "reinstate the
	// GitHub topic + reuse the existing GitHub integration for
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
	// Topic "Install Helix" gate. Wired in api/pkg/server (which has the
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

// BotRuntimeInfo is the subset of a Bot's runtime-state sidecar the
// REST adapter surfaces: the per-project deep-link ids, the current
// desktop session id, and whether that sandbox is online.
type BotRuntimeInfo struct {
	ProjectID  string
	AgentAppID string
	SessionID  string
	// AgentStatus is "running" when the bot's exploratory-session
	// desktop is online (external_agent_status == running), else
	// "stopped". Empty when the status could not be resolved.
	AgentStatus string
}

// BotRuntime resolves a Bot's runtime-state sidecar. Declared here
// (implemented at the composition root over runtimehelix.LoadState) so
// the bot-detail and activate handlers read project/agent/session ids
// without the api adapter touching the store.
type BotRuntime interface {
	State(ctx context.Context, orgID string, botID orgchart.BotID) (BotRuntimeInfo, error)
}

// BotSessionResetter fully removes a bot's current session so the next
// activation is genuinely fresh. It stops the desktop, deletes the
// session row (an exploratory session is a project singleton that
// StartExternalAgentSession would otherwise reuse), and clears the
// persisted session pointer. The bot-page "Restart agent session" button
// calls this and then Activates, giving a brand-new session, desktop and
// thread with the bot's current tools / MCP services. Wired at the
// composition root over the in-proc helix client; nil → restartBotAgent
// skips the reset and just re-activates the existing session.
type BotSessionResetter interface {
	ResetSession(ctx context.Context, orgID string, botID orgchart.BotID, sessionID string) error
}

// BotDesktopStopper stops a bot's desktop container without deleting the
// session row (so the transcript survives and the next start can resume).
// nil → stopBotAgent returns 501.
type BotDesktopStopper interface {
	StopDesktop(ctx context.Context, sessionID string) error
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
		// Bots are the single org-chart aggregate (the merged Role +
		// Worker). Create/Delete go through the lifecycle cascade;
		// content/tools edits and reporting-line edges go through the
		// bots service.
		{Pattern: "GET /bots", Handler: http.HandlerFunc(a.listBots)},
		{Pattern: "POST /bots", Handler: http.HandlerFunc(a.createBot)},
		{Pattern: "GET /bots/{id}", Handler: http.HandlerFunc(a.getBot)},
		{Pattern: "PATCH /bots/{id}", Handler: http.HandlerFunc(a.updateBot)},
		{Pattern: "DELETE /bots/{id}", Handler: http.HandlerFunc(a.deleteBot)},
		// Subscriptions are bot-anchored — the Bot Detail page edits the
		// bot's subscription set through these endpoints.
		{Pattern: "GET /bots/{id}/subscriptions", Handler: http.HandlerFunc(a.listBotSubscriptions)},
		{Pattern: "POST /bots/{id}/subscriptions", Handler: http.HandlerFunc(a.subscribeBot)},
		{Pattern: "DELETE /bots/{id}/subscriptions/{topic_id}", Handler: http.HandlerFunc(a.unsubscribeBot)},
		{Pattern: "POST /bots/{id}/chat", Handler: http.HandlerFunc(a.ensureBotChat)},
		{Pattern: "POST /bots/{id}/activate", Handler: http.HandlerFunc(a.activateBot)},
		{Pattern: "POST /bots/{id}/stop-agent", Handler: http.HandlerFunc(a.stopBotAgent)},
		{Pattern: "POST /bots/{id}/restart-agent", Handler: http.HandlerFunc(a.restartBotAgent)},
		// Reporting lines are many-to-many — add/remove individual
		// manager edges rather than replacing a single parent.
		{Pattern: "POST /bots/{id}/parents", Handler: http.HandlerFunc(a.addBotParent)},
		{Pattern: "DELETE /bots/{id}/parents/{parent_id}", Handler: http.HandlerFunc(a.removeBotParent)},
		{Pattern: "GET /tools", Handler: http.HandlerFunc(a.listTools)},
		{Pattern: "GET /settings", Handler: http.HandlerFunc(a.listSettings)},
		{Pattern: "PUT /settings/{key}", Handler: http.HandlerFunc(a.setSetting)},
		{Pattern: "DELETE /settings/{key}", Handler: http.HandlerFunc(a.deleteSetting)},
		{Pattern: "GET /topics", Handler: http.HandlerFunc(a.listTopics)},
		{Pattern: "POST /topics", Handler: http.HandlerFunc(a.createTopic)},
		{Pattern: "GET /topics/{id}", Handler: http.HandlerFunc(a.getTopic)},
		{Pattern: "PUT /topics/{id}", Handler: http.HandlerFunc(a.updateTopic)},
		{Pattern: "DELETE /topics/{id}", Handler: http.HandlerFunc(a.deleteTopic)},
		{Pattern: "GET /topics/{id}/events", Handler: http.HandlerFunc(a.topicEventsSSE)},
		{Pattern: "GET /topics/{id}/messages", Handler: http.HandlerFunc(a.listTopicMessages)},
		{Pattern: "POST /topics/{id}/publish", Handler: http.HandlerFunc(a.publishToTopic)},
		// Processors — JSON:API CRUD.
		{Pattern: "GET /processors", Handler: http.HandlerFunc(a.listProcessors)},
		{Pattern: "POST /processors", Handler: http.HandlerFunc(a.createProcessor)},
		{Pattern: "GET /processors/{id}", Handler: http.HandlerFunc(a.getProcessor)},
		{Pattern: "PUT /processors/{id}", Handler: http.HandlerFunc(a.updateProcessor)},
		{Pattern: "DELETE /processors/{id}", Handler: http.HandlerFunc(a.deleteProcessor)},
		// Chart free-placed layout (bots / topics / processors).
		{Pattern: "GET /chart/positions", Handler: http.HandlerFunc(a.getChartPositions)},
		{Pattern: "PUT /chart/positions", Handler: http.HandlerFunc(a.putChartPositions)},
		{Pattern: "DELETE /chart/positions", Handler: http.HandlerFunc(a.deleteChartPositions)},
		// Inbound webhook for the GitHub transport. The transport
		// resolves orgID from the request context (set by the org
		// middleware) and reads transport.github from the org's
		// config registry on every delivery, so a single mounted
		// route serves every org without rebinding state.
		{Pattern: "POST /github/webhook", Handler: http.HandlerFunc(a.githubWebhook)},
		// GitHub helper endpoints. listGitHubRepos powers the
		// searchable repo dropdown in the New Topic dialog;
		// installGitHubWebhook calls the GitHub API on behalf of the
		// operator so the only thing the user has to choose is which
		// repo. Both rely on a working GitHubTokenResolver (i.e. the
		// operator has a connected GitHub OAuth).
		{Pattern: "GET /github/repos", Handler: http.HandlerFunc(a.listGitHubRepos)},
		{Pattern: "GET /github/app-installation", Handler: http.HandlerFunc(a.getGitHubAppInstallation)},
		{Pattern: "POST /github/app-manifest", Handler: http.HandlerFunc(a.startGitHubAppManifest)},
		{Pattern: "POST /topics/{id}/github/install-webhook", Handler: http.HandlerFunc(a.installGitHubWebhook)},
		{Pattern: "GET /topics/{id}/github/webhook-status", Handler: http.HandlerFunc(a.getGitHubWebhookStatus)},
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
	case errors.Is(err, store.ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
