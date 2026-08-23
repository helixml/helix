package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/gorilla/mux"

	githubskill "github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/org/application/activations"
	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/application/attachments"
	"github.com/helixml/helix/api/pkg/org/application/chartlayout"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/application/helixevents"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/messages"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/application/processing"
	"github.com/helixml/helix/api/pkg/org/application/processors"
	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	orgsandboxes "github.com/helixml/helix/api/pkg/org/application/sandboxes"
	"github.com/helixml/helix/api/pkg/org/application/slackrouting"
	"github.com/helixml/helix/api/pkg/org/application/triggers"
	"github.com/helixml/helix/api/pkg/org/application/workersecrets"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
	"github.com/helixml/helix/api/pkg/org/infrastructure/agentdelivery"
	"github.com/helixml/helix/api/pkg/org/infrastructure/assetssh"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/infrastructure/streamcron"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	gitlabtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/gitlab"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
	"github.com/helixml/helix/api/pkg/org/infrastructure/wakebus"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/server/helixorg"
	slackcore "github.com/helixml/helix/api/pkg/serviceconnection/slack"

	"github.com/helixml/helix/api/pkg/org/application/cutover"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/services"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// helixOrgHandlers bundles the JSON HTTP surface helix-org exposes:
// the JSON-RPC MCP / webhook / org-graph / settings / topics endpoints
// mounted under /api/v1/orgs/{org}/. The React UI at
// /orgs/:org_id/helix-org/* consumes those endpoints.
type helixOrgHandlers struct {
	api http.Handler
	// mcpServer is the org MCP server behind `api`. Held directly so the
	// spec-task MCP backend can serve a project-scoped caller through the
	// same registry and audit path (ServeMCPForCaller).
	mcpServer *helixorgserver.Server
	scope     *helixOrgScope
	store     *helixorgstore.Store
	lifecycle *lifecycle.Service
	// seeder creates the membership-driven human nodes + the per-org Chief
	// of Staff bot. Copied onto HelixAPIServer by mountHelixOrg so the
	// org-lifecycle handlers (org create, membership add/remove) can drive it.
	seeder *orgGraphSeeder
	// streamCron is the in-process scheduler that fires events on
	// KindCron Triggers. The server's run loop calls Start on it in a
	// goroutine so it runs for the lifetime of the API process.
	streamCron        *streamcron.Scheduler
	githubDeliveryRun func(ctx context.Context)
	// publicGitHubWebhook is the inbound /github/webhook handler
	// mounted on the INSECURE router. GitHub deliveries carry no
	// helix session cookie or API key — they authenticate via the
	// per-org HMAC `webhook_secret` checked inside the github
	// transport. The path is /api/v1/orgs/{org}/github/webhook and
	// the handler resolves {org} from mux.Vars before dispatching.
	// Fans out to every github topic whose (repo, events) matches
	// the delivery — multi-topic behaviour.
	publicGitHubWebhook http.Handler
	// publicGitHubWebhookForStream is the per-topic variant. Path:
	// /api/v1/orgs/{org}/topics/{topic_id}/github/webhook —
	// deliveries to this URL are pinned to exactly one topic so
	// operators get a 1:1 mapping between GitHub webhooks and helix
	// Triggers. The Trigger's own (repo, events) config still applies
	// so cross-repo or non-whitelisted-event deliveries drop with
	// 204 (no GitHub retries).
	publicGitHubWebhookForStream  http.Handler
	publicGitLabWebhookForTrigger http.Handler
	// publicSlackEvents is the global inbound Slack Events API handler
	// mounted on the INSECURE router at /api/v1/slack/events. Slack
	// deliveries authenticate via the global app's signing-secret HMAC
	// (checked inside the handler), and team_id routes each delivery to
	// the owning org. One handler serves every org install.
	publicSlackEvents http.Handler
	// slackSocketRun runs the Socket Mode ingress for the lifetime of
	// ctx, when the global app is configured for it. Started in a
	// goroutine from the run loop, like streamCron.
	slackSocketRun func(ctx context.Context)
	// slackTopics auto-creates/removes the per-workspace Slack Trigger when
	// a workspace is connected/disconnected. An org primitive owned by
	// this subsystem, not the core server.
	slackTopics *slackWorkspaceTopics
	// slackAutoRouter creates the per-workspace auto-router on connect and
	// keeps its per-Worker routes in sync (composition over the processors
	// service + slackrouting reconciler). nil when org/Slack is disabled.
	slackAutoRouter *slackAutoRouter
	// slackSocket reconciles live Socket Mode connections against the
	// configured socket-mode apps; Kicked by the admin service-connection
	// handlers when a slack_app changes so it applies without a restart.
	slackSocket *slacktransport.SocketManager
	// publicGitHubManifestCallback receives GitHub's browser redirect after
	// the App Manifest flow creates the app (path
	// /api/v1/orgs/{org}/github/app-manifest/callback). Insecure mount: it's
	// a top-level navigation from github.com authenticated by the encrypted
	// ?state=, not the helix session. Exchanges the code, stores the app,
	// then redirects to the install page. The installation id is reconciled
	// later via GET /app/installations (no Setup-URL redirect needed).
	publicGitHubManifestCallback http.Handler
	assetSSHProxyRun             func(ctx context.Context) error
}

// initHelixOrgHandler builds the in-process helix-org HTTP handler;
// mounted at /api/v1/orgs/{org}/.
//
// Storage: the org-graph rows land in the same Postgres database
// helix uses for its primary state — no separate connection pool,
// no FILESTORE_TYPE=fs requirement. The helixStore must expose a
// *gorm.DB accessor (helix's PostgresStore does); otherwise this
// returns an error.
//
// Per-Worker state lives in the Worker's Helix project (a git repo +
// agent app) and on the repo's helix-specs branch — there is no
// API-host workspace directory.
//
// Returns nil (and logs) if the embedded org cannot be initialised for
// this deployment — callers must treat that as "don't mount".
//
// Requires a non-nil cfg.APIServer: the embedded helix-org module talks
// to Helix's project / git / app / session surfaces via an in-process
// adapter (helix_org_inproc.go) that needs the live *HelixAPIServer.
// Wirings without an APIServer (e.g. test harnesses) return (nil, nil)
// — the module simply isn't mounted.
// orgWorkerRuntime adapts runtimehelix.LoadState into the api package's
// WorkerRuntime port, so the REST worker-detail / activate handlers read
// the project / agent-app / session ids without the api adapter touching
// the store. sessions is the Helix session store used to resolve whether
// the bot's desktop sandbox is online (agent_status for the chart).
type orgWorkerRuntime struct {
	st       *helixorgstore.Store
	sessions interface {
		GetSession(ctx context.Context, id string) (*types.Session, error)
		GetApp(ctx context.Context, id string) (*types.App, error)
	}
}

func (o orgWorkerRuntime) State(ctx context.Context, orgID string, workerID orgchart.NodeID) (helixorgapi.BotRuntimeInfo, error) {
	s, err := runtimehelix.LoadState(ctx, o.st, orgID, workerID)
	if err != nil {
		return helixorgapi.BotRuntimeInfo{}, err
	}
	info := helixorgapi.BotRuntimeInfo{
		ProjectID:   s.ProjectID,
		AgentID:     s.AgentID,
		SessionID:   s.SessionID,
		AgentStatus: "stopped",
	}
	if s.AgentID != "" && o.sessions != nil {
		if app, err := o.sessions.GetApp(ctx, s.AgentID); err == nil && app != nil && len(app.Config.Helix.Assistants) > 0 {
			assistant := app.Config.Helix.Assistants[0]
			info.Runtime = string(assistant.CodeAgentRuntime)
			info.Model = assistant.Model
		}
	}
	// Resolve sandbox online-ness from the session metadata the desktop
	// stack already maintains (external_agent_status). Missing session
	// or lookup failure keeps the default "stopped".
	if s.SessionID != "" && o.sessions != nil {
		if sess, err := o.sessions.GetSession(ctx, s.SessionID); err == nil && sess != nil {
			if sess.Metadata.ExternalAgentStatus == "running" {
				info.AgentStatus = "running"
			}
		}
	}
	return info, nil
}

// SessionID adapts orgWorkerRuntime to activations.SessionResolver so the
// manual-activate use case can populate the response's session id without
// the activations service touching the store.
func (o orgWorkerRuntime) SessionID(ctx context.Context, orgID string, workerID orgchart.NodeID) (string, error) {
	s, err := runtimehelix.LoadState(ctx, o.st, orgID, workerID)
	if err != nil {
		return "", err
	}
	return s.SessionID, nil
}

// botSessionResetter implements helixorgapi.BotSessionResetter: it fully
// removes a bot's current session so restartBotAgent's follow-up Activate
// provisions a genuinely fresh one. Composed of two existing high-level
// ops on the in-proc client (stop desktop + delete session) plus clearing
// the persisted session pointer in the org runtime-state store — no
// container/workspace internals.
type botSessionResetter struct {
	client *inProcHelixClient
	st     *helixorgstore.Store
}

// StopDesktop stops the external-agent container for a session without
// deleting the session row (bot-detail / chart "Stop" control).
func (r botSessionResetter) StopDesktop(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := r.client.StopExternalAgent(ctx, sessionID); err != nil {
		return fmt.Errorf("stop desktop %s: %w", sessionID, err)
	}
	return nil
}

func (r botSessionResetter) ResetSession(ctx context.Context, orgID string, botID orgchart.NodeID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	// Stop the desktop first — deleting the session row alone leaves the
	// container running. Best-effort: the container may already be gone
	// (paused/crashed), and we never resume it, so a stop error must not
	// block the teardown (mirrors restartSessionContainer's StopDesktop).
	if err := r.client.StopExternalAgent(ctx, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("reset bot session: stop desktop failed (continuing — may already be down)")
	}
	// Delete the session row. An exploratory session is a project singleton
	// that StartExternalAgentSession would otherwise reuse, so removing it
	// is what makes the follow-up activation mint a brand-new session.
	if err := r.client.DeleteSession(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session %s: %w", sessionID, err)
	}
	// Clear the persisted pointer so the spawner's ensureSession starts a
	// fresh session instead of trying to ClearSession the deleted one.
	if err := runtimehelix.SaveSession(ctx, r.st, orgID, botID, ""); err != nil {
		return fmt.Errorf("clear session pointer for bot %s: %w", botID, err)
	}
	return nil
}

// orgServices bundles the application services the REST adapter (and the
// per-Worker MCP server) consume. Assembled once by buildOrgServices at
// the composition root — the "Module struct holds the assembled
// services" shape from design §5.4.
type orgServices struct {
	Nodes       *nodes.Nodes
	Triggers    *triggers.Service
	Messages    *messages.Messages
	Attachments *attachments.Service
	Publishing  *publishing.Publishing
	Queries     *queries.Queries
	Activations *activations.Activations
	Processors  *processors.Processors
}

// buildOrgServices constructs every org application service from the
// store + collaborators. One place owns the wiring so the apiDeps
// literal reads as a list of pre-built services, not seven inline
// constructors. deps carries the clock / id-gen / topology / hire-hook
// seams (a mcptools.Deps is already assembled by the caller).
func buildOrgServices(st *helixorgstore.Store, deps *mcptools.Config, bc *wakebus.Bus, dispatcher *dispatch.Dispatcher, provisioners map[transport.Kind]trigger.Inbound, knownTools func() map[tool.Name]bool) orgServices {
	// KnownTools reads the registry lazily: the registry is built after
	// these services, and Reconcile only runs per-org on the first
	// request, by which time it is fully populated.
	botsSvc := nodes.New(nodes.Deps{
		Nodes: st.Nodes, Lines: st.ReportingLines, Reconciler: deps.Reconciler,
		Now: deps.Now, NewID: deps.NewID,
		BaseTools: mcptools.BaseReadTools, KnownTools: knownTools,
		OnToolsChanged: deps.ToolChangeNotifier,
	})
	svc := orgServices{
		Nodes: botsSvc,
		Triggers: triggers.New(triggers.Deps{
			Triggers: st.Triggers, Attachments: st.WorkerAttachments, Events: st.Events,
			Now: deps.Now, NewID: deps.NewID, Provisioners: provisioners,
		}),
		Messages:    messages.New(messages.Deps{Triggers: st.Triggers, Events: st.Events, Notifier: bc}),
		Attachments: attachments.New(attachments.Deps{Store: st, Now: deps.Now, NewID: deps.NewID}),
		Processors: processors.New(processors.Deps{
			Processors: st.Processors, Triggers: st.Triggers, Attachments: st.WorkerAttachments, Now: deps.Now, NewID: deps.NewID,
		}),
		Publishing: publishing.New(publishing.Deps{Triggers: st.Triggers, Events: st.Events, Hub: bc, Router: dispatcher, Now: deps.Now, NewID: deps.NewID}),
		Queries: queries.New(queries.Deps{
			Nodes: st.Nodes, ReportingLines: st.ReportingLines, Triggers: st.Triggers,
			Attachments: st.WorkerAttachments, Processors: st.Processors, Events: st.Events, Activations: st.Activations,
		}),
		// Activations is built at the composition root (not here) because
		// the Activate use case needs the project ensurer + dispatcher +
		// session resolver, which aren't available in this builder.
	}
	deps.Publishing = svc.Publishing
	deps.Triggers = svc.Triggers
	deps.Attachments = svc.Attachments
	return svc
}

// registerHelixOrgRoutes brings up the helix-org subsystem and registers
// its entire HTTP surface: the public GitHub/Slack webhooks + OAuth
// callbacks on the insecure router, the org-scoped Slack endpoints and
// the /orgs/{org}/ catch-all on the auth router, the org MCP backend,
// plus the long-lived stream-cron and Socket Mode goroutines. Every
// org-shaped route + lifecycle hook lives here.
func (s *HelixAPIServer) registerHelixOrgRoutes(ctx context.Context, insecureRouter, authRouter *mux.Router) error {
	orgHandlers, err := initHelixOrgHandler(ctx, helixOrgConfig{
		LocalFSPath: s.Cfg.FileStore.LocalFSPath,
		APIServer:   s,
	}, s.Store)
	if err != nil {
		return fmt.Errorf("initialise helix-org: %w", err)
	}
	if orgHandlers == nil {
		return nil
	}
	// Hold the subsystem handle (the Slack handlers reach the per-workspace
	// Trigger reconciler through it) and register the post-mutation hook so
	// the generic service-connection handlers can stay helix-org-agnostic
	// while a slack_app change still reconciles Socket Mode / cascades.
	s.helixOrg = orgHandlers
	s.orgSeeder = orgHandlers.seeder
	s.onServiceConnectionChange = s.reactToServiceConnectionChange

	// Stream-cron scheduler runs for the lifetime of ctx
	// (ListenAndServe's). Logs its own errors; one bad fire can't kill
	// the loop because fire() has panic recovery.
	if orgHandlers.streamCron != nil {
		go func() {
			if err := orgHandlers.streamCron.Start(ctx); err != nil {
				log.Error().Err(err).Msg("streamcron scheduler exited with error")
			}
		}()
	}
	if orgHandlers.githubDeliveryRun != nil {
		go orgHandlers.githubDeliveryRun(ctx)
	}
	if orgHandlers.assetSSHProxyRun != nil {
		go func() {
			if err := orgHandlers.assetSSHProxyRun(ctx); err != nil {
				log.Error().Err(err).Msg("asset SSH proxy exited with error")
			}
		}()
	}
	// /api/v1/orgs/{org}/github/webhook — public, GitHub deliveries
	// authenticate via HMAC of the per-org webhook_secret. Registered on
	// the INSECURE router so the helix session-cookie / api-key auth
	// doesn't 401 inbound deliveries. Must be registered BEFORE the
	// authRouter PathPrefix("/orgs/{org}/") so this exact path wins.
	if orgHandlers.publicGitHubWebhook != nil {
		insecureRouter.
			Handle("/orgs/{org}/github/webhook", orgHandlers.publicGitHubWebhook).
			Methods(http.MethodPost)
	}
	// Per-stream variant — operators paste this URL into a GitHub repo's
	// webhook config when they want a 1:1 mapping between a GitHub webhook
	// and a helix stream. Insecure mount: GitHub deliveries authenticate
	// via HMAC over the body, not a helix session.
	if orgHandlers.publicGitHubWebhookForStream != nil {
		insecureRouter.
			Handle("/orgs/{org}/topics/{topic_id}/github/webhook", orgHandlers.publicGitHubWebhookForStream).
			Methods(http.MethodPost)
	}
	if orgHandlers.publicGitLabWebhookForTrigger != nil {
		insecureRouter.
			Handle("/orgs/{org}/topics/{topic_id}/gitlab/webhook", orgHandlers.publicGitLabWebhookForTrigger).
			Methods(http.MethodPost)
	}
	// GitHub App Manifest flow callbacks — top-level browser navigations
	// from github.com (GET), so they must be on the insecure router (no
	// session cookie / API key). The conversion callback authenticates
	// via the encrypted ?state=. Registered before the /orgs/{org}/
	// prefix so these exact paths win the match.
	if orgHandlers.publicGitHubManifestCallback != nil {
		insecureRouter.
			Handle("/orgs/{org}/github/app-manifest/callback", orgHandlers.publicGitHubManifestCallback).
			Methods(http.MethodGet)
	}
	// /api/v1/slack/events — single global inbound Slack Events API
	// endpoint. Insecure mount: Slack deliveries carry no helix session;
	// the handler verifies the global app's signing-secret HMAC and
	// routes by team_id. One endpoint serves every org install.
	if orgHandlers.publicSlackEvents != nil {
		insecureRouter.
			Handle("/slack/events", orgHandlers.publicSlackEvents).
			Methods(http.MethodPost)
	}
	// /api/v1/slack/oauth/callback — top-level browser redirect from
	// slack.com after the admin approves the install. Insecure (no
	// session cookie); authenticated by the encrypted ?state= carrying
	// the org id.
	insecureRouter.
		HandleFunc("/slack/oauth/callback", s.slackOAuthCallback).
		Methods(http.MethodGet)
	// Socket Mode ingress — long-lived, only active when the global app
	// is configured for it. Started like streamCron.
	if orgHandlers.slackSocketRun != nil {
		go orgHandlers.slackSocketRun(ctx)
	}

	// Org-scoped Slack endpoints. Registered BEFORE the /orgs/{org}/
	// catch-all so these exact paths win the match. Each handler does its
	// own lookupOrg + org-membership authorisation (strict multi-tenancy),
	// so they don't need the org-scope middleware.
	authRouter.HandleFunc("/orgs/{org}/slack/apps", s.listOrgSlackApps).Methods(http.MethodGet)
	authRouter.HandleFunc("/orgs/{org}/slack/oauth/start", s.slackOAuthStart).Methods(http.MethodGet)
	authRouter.HandleFunc("/orgs/{org}/slack/workspaces", s.listSlackWorkspaces).Methods(http.MethodGet)
	authRouter.HandleFunc("/orgs/{org}/slack/workspaces", s.connectSlackWorkspace).Methods(http.MethodPost)
	authRouter.HandleFunc("/orgs/{org}/slack/workspaces/{id}", s.deleteSlackWorkspace).Methods(http.MethodDelete)

	s.registerHelixOrgAuthenticatedRoutes(authRouter, orgHandlers)

	// Expose helix-org's owner MCP through the standard Helix MCP gateway.
	// Backend identifies tenants by URL prefix
	// (/api/v1/mcp/helix-org/{org}/...) — the gateway already auth-checks
	// the api_key via authRouter; the per-org backend layer resolves orgID
	// from the request before dispatching to the handler.
	s.mcpGateway.RegisterBackend("helix-org", NewHelixOrgMCPBackend(s, orgHandlers))
	// Spec tasks get their own project-scoped slice of the same tool registry.
	s.mcpGateway.RegisterBackend("helix-tasks", NewSpecTaskMCPBackend(s, orgHandlers))
	return nil
}

func (s *HelixAPIServer) registerHelixOrgAuthenticatedRoutes(authRouter *mux.Router, orgHandlers *helixOrgHandlers) {
	identityOnly := s.withHelixOrgIdentity(stripOrgScopedPrefix(orgHandlers.api))
	authRouter.Handle("/orgs/{org}/settings", identityOnly).Methods(http.MethodGet)
	authRouter.Handle("/orgs/{org}/settings/{key}", identityOnly).Methods(http.MethodPut, http.MethodDelete)
	authRouter.Handle("/orgs/{org}/github/app-installation", identityOnly).Methods(http.MethodGet)
	authRouter.Handle("/orgs/{org}/github/app-manifest", identityOnly).Methods(http.MethodPost)

	// All remaining per-tenant org-graph routes bootstrap the graph before dispatch.
	authRouter.PathPrefix("/orgs/{org}/").Handler(
		s.withHelixOrgScope(orgHandlers.scope,
			stripOrgScopedPrefix(orgHandlers.api),
		),
	)
}

func initHelixOrgHandler(ctx context.Context, cfg helixOrgConfig, helixStore helixstore.Store) (*helixOrgHandlers, error) {
	if cfg.APIServer == nil {
		log.Warn().Msg("helix-org disabled: no HelixAPIServer threaded into helixOrgConfig")
		return nil, nil
	}

	// Working directory root. LocalFSPath = the SaaS persistent
	// volume mount when fs is enabled; os.TempDir() when not.
	// Container restarts wipe TempDir contents, but the per-Worker
	// envs are placeholders only (per-Worker state lives in Helix
	// projects), so a fresh directory after restart is acceptable.
	root := cfg.LocalFSPath
	if root == "" {
		root = os.TempDir()
	}
	orgRoot := filepath.Join(root, "helix-org")
	if err := os.MkdirAll(orgRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create helix-org dir %q: %w", orgRoot, err)
	}

	// Open the org store against helix's Postgres connection. The
	// helixStore must expose a *gorm.DB accessor — there is no
	// dialect fallback any more.
	st, err := openOrgStore(helixStore)
	if err != nil {
		return nil, err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Convert the retired Topic model into Triggers, Processor inputs and
	// Worker attachments. Runs once per boot before anything reads the
	// org graph, and is repeat-safe: on a converged deployment (or a
	// clean install) it reads three empty tables and returns. A failure
	// here is fatal — booting on half-converted data would route events
	// to the wrong Workers.
	if _, err := cutover.Convert(ctx, cutover.Deps{Store: st, Logger: logger}); err != nil {
		return nil, fmt.Errorf("convert retired topic model: %w", err)
	}

	// Bootstrap is lazy: withHelixOrgScope calls
	// helixOrgScope.ensureBootstrap(ctx, orgID) on first request for
	// each org, materialising the owner Worker + structural grants
	// then. Bootstrap rows carry org_id and the FK to
	// organizations(id) reaps them on org delete.

	// Wake-only topic notifier. Backed by the host API server's
	// pubsub.PubSub (the canonical Helix NATS instance) — the
	// wakebus package is a thin facade preserving the typed
	// streaming.StreamID API the helix-org call sites used when this was the
	// in-process broadcast.Hub.
	bc := wakebus.New(cfg.APIServer.pubsub)
	deps := mcptools.DefaultDeps(st)
	deps.Hub = bc

	// Operational config registry — chat backend creds, model
	// selection, etc. Backed by the same Postgres rows so settings
	// survive restarts. Surfaced via Organization Settings at
	// /orgs/:org_id/general (backed by
	// /api/v1/orgs/{org}/settings). Constructed before the spawner
	// so the spawner can read chat.app_id at activation
	// time.
	configReg := configregistry.New(st.Configs)
	helixorg.RegisterConfigSpecs(configReg)

	// The Helix service api_key is per-org and provisioned lazily by
	// helixOrgScope.ensureBootstrap on the first request for an org.
	// See helix_org_middleware.go.

	// In-process adapter satisfying runtimehelix.ProjectService,
	// runtimehelix.SpawnerClient, and chat.ChatBridgeClient — the three
	// surfaces every Worker's per-project flow needs (project / git /
	// app on apply; chat session create/output/stop on activation and
	// owner-chat). The adapter calls HelixAPIServer's handler methods
	// directly; no HTTP loopback.
	//
	// Request-scoped calls run as the authenticated user. Background calls
	// resolve the owner of their organization in the adapter.
	inProcClient := NewInProcHelixClient(cfg.APIServer, configReg)
	deps.AgentContentUpdater = inProcClient
	deps.AgentProfileReader = inProcClient
	deps.ToolChangeNotifier = cfg.APIServer.publishAgentToolChange

	// Wire the helix-runtime HireHook so hire_worker persists the
	// hiring user's identifier onto the new Worker's runtime state.
	// Replaces the direct runtimehelix.SaveHiringUser call hire_worker
	// used to make.
	deps.HireHook = &runtimehelix.Hire{Store: st}

	// ProjectConfig backs the get_worker_project +
	// configure_worker_project MCP tools — owner-only read/patch
	// of a Worker's helix project config (startup script today,
	// skills/guidelines later). Reuses the in-proc client for the
	// underlying Helix project read/write.
	projectConfig, err := runtimehelix.NewProjectConfig(st, inProcClient)
	if err != nil {
		return nil, fmt.Errorf("init project config: %w", err)
	}
	deps.ProjectConfig = projectConfig

	// SpecTasks backs the spec-task MCP tools — a Worker managing the
	// spec tasks in its own Helix project. The helix store satisfies the
	// read/write port directly; specTaskWorkflow wraps the canonical
	// SpecDrivenTaskService (ApproveSpecs) + the server's PR-creation
	// method so the approve / open-PR verbs reuse the exact REST code.
	specTasks, err := runtimehelix.NewSpecTasks(st, helixStore, specTaskWorkflow{apiServer: cfg.APIServer})
	if err != nil {
		return nil, fmt.Errorf("init spec tasks: %w", err)
	}
	deps.SpecTasks = specTasks
	deps.ProjectAccess = specTasks

	// Projects backs the project-discovery MCP tools (list_projects,
	// get_project) — org-scoped reads so an org-wide PM Bot can find the
	// projects it manages. The helix store satisfies the port directly.
	projectsPort, err := runtimehelix.NewProjects(helixStore)
	if err != nil {
		return nil, fmt.Errorf("init projects: %w", err)
	}
	deps.Projects = projectsPort

	// Sandboxes backs the Chief of Staff's standalone sandbox discovery and
	// CRUD tools. It delegates to the same controller as the REST/UI surface,
	// while the adapter derives the human owner and hard-scopes every row to
	// the caller's organization.
	sandboxesPort, err := runtimehelix.NewSandboxes(st, cfg.APIServer.sandboxController, helixStore)
	if err != nil {
		return nil, fmt.Errorf("init sandboxes: %w", err)
	}
	deps.Sandboxes = sandboxesPort
	sandboxAccess := orgsandboxes.New(sandboxesPort, deps.Queries)

	// Repositories backs list_repositories / attach_repository /
	// detach_repository — org git repos attached to Bot projects so
	// sandboxes can clone the code. Chief of Staff gets these by default.
	reposPort, err := runtimehelix.NewRepositories(st, helixStore)
	if err != nil {
		return nil, fmt.Errorf("init repositories: %w", err)
	}
	deps.Repositories = reposPort

	// Project applier — shared infra for owner-chat and Worker
	// activations. Applies every Worker's project with the same
	// `worker.runtime` (default `claude_code`) and the same MCP
	// wiring (auth-gated gateway URL with the service api_key in
	// headers).
	//
	// The wrapper re-resolves `worker.*` and `helix.*` from the config
	// registry on every Ensure call, so live changes via the settings
	// page take effect on the next activation — no API restart needed.
	projectApplier := &dynamicProjectApplier{
		cfg:        configReg,
		projectSvc: inProcClient,
		Store:      st,
		helixStore: helixStore,
		logger:     logger,
	}

	// Wire helix-org's production Spawner. The owner is a Worker, so
	// helix-org/server/chat.HelixBridge reuses the same applier; both
	// drive per-Worker projects through the same default settings.
	// inProcClient satisfies both SpawnerClient AND ProjectService —
	// passing it as the latter wires the spawner's *internal* fast-
	// path ensureProject so it can verify per-Worker projects without
	// a nil-deref. (Reproducer: hire AI worker via chart → click the
	// chip → API panics at project.go:156 inside the cached spawner's
	// ensureProject before this argument existed.)
	// gitHubTokenResolver resolves a current GitHub OAuth access token
	// for an org by walking the org's members + their oauth_connections
	// (see helix_org_github.go). Drives the github topic transport's
	// outbound `Token()` lookup. Worker credentials resolve independently
	// through explicit Worker secret bindings.
	oauthResolver := helixorg.NewGitHubOAuthResolver(cfg.APIServer.oauthManager, helixStore)
	// identityResolver prefers the installed Helix App bot over a borrowed
	// member OAuth token: if the org has a github_app ServiceConnection it
	// mints a short-lived installation token (decrypting the stored PEM with
	// the server encryption key), else it falls back to oauthResolver.
	// github.MintInstallationCredential is the production minter — it
	// returns both the token and the server-reported expiry.
	identityResolver := helixorg.NewOrgGitHubIdentityResolver(
		cfg.APIServer.getEncryptionKey,
		helixStore,
		oauthResolver,
		func(ctx context.Context, appID, installationID int64, pem, baseURL string) (helixorg.MintedInstallation, error) {
			cred, err := githubskill.MintInstallationCredential(ctx, appID, installationID, pem, baseURL)
			if err != nil {
				return helixorg.MintedInstallation{}, err
			}
			return helixorg.MintedInstallation{Token: cred.Token, ExpiresAt: cred.ExpiresAt}, nil
		},
	)
	// gitHubTokenResolver is the bot-preferring token projection used by
	// the outbound github topic transport and the webhook-install code
	// path. Returns the App installation token when one exists, else the
	// legacy member OAuth token — so once an org installs the Helix App,
	// its agents act as the bot rather than a human. (Worker shell-tool
	// credentials no longer flow through this projection; they resolve from
	// the explicitly bound Connected Account.)
	gitHubTokenResolver := func(ctx context.Context, orgID string) (string, error) {
		id, err := identityResolver(ctx, orgID)
		if err != nil {
			return "", err
		}
		return id.Token, nil
	}

	// Transcript mirror — process-wide singleton shared by the spawner
	// (Ensure), bootstrap (EnsureAll), and lifecycle.Fire (Stop).
	mirror := runtimehelix.NewMirror(context.Background(), runtimehelix.MirrorConfig{
		PubSub:      cfg.APIServer.pubsub,
		Snapshotter: runtimehelix.NoopSessionPreamble{},
		Client:      inProcClient,
		ExploratorySession: func(ctx context.Context, projectID string) (string, error) {
			sess, err := helixStore.GetProjectExploratorySession(ctx, projectID)
			if err != nil || sess == nil {
				return "", err
			}
			return sess.ID, nil
		},
		Store:  st,
		Hub:    bc,
		NewID:  deps.NewID,
		Now:    deps.Now,
		Logger: logger,
	})

	spawnerFn := lazyHelixOrgSpawner(spawnerDeps{
		Cfg:           configReg,
		HelixStore:    helixStore,
		SpawnerClient: inProcClient,
		ProjectSvc:    inProcClient,
		OrgStore:      st,
		Hub:           bc,
		PubSub:        cfg.APIServer.pubsub,
		Logger:        logger,
		Applier:       projectApplier,
		Mirror:        mirror,
		NewID:         deps.NewID,
		Now:           deps.Now,
	})
	dispatcher := dispatch.New(st, spawnerFn, logger)
	var agentDelivery lifecycle.AgentDeliveryLifecycle
	if provider, ok := cfg.APIServer.pubsub.(pubsub.DurablePubSub); ok {
		durableQueue, err := agentdelivery.New(ctx, provider, activation.Spawn(spawnerFn), logger)
		if err != nil {
			return nil, fmt.Errorf("init durable agent delivery: %w", err)
		}
		if err := durableQueue.Start(); err != nil {
			return nil, fmt.Errorf("start durable agent delivery: %w", err)
		}
		dispatcher.RegisterActivationQueue(durableQueue)
		agentDelivery = durableQueue
	}
	// slackWS resolves the encrypted workspace install, both for inbound
	// team-id resolution and for Worker secret bindings (a Worker replying
	// to Slack fetches that token with get_secret and calls Slack's API
	// itself).
	slackWS := newSlackWorkspaces(helixStore, cfg.APIServer.getEncryptionKey)
	secretResolver := workersecrets.Resolver{
		ValidateHelixSecret: func(ctx context.Context, b workersecret.Binding) error {
			return projectConfig.ValidateWorkerProjectSecret(ctx, b.OrganizationID, b.WorkerID, b.SecretID)
		},
		HelixSecret: func(ctx context.Context, b workersecret.Binding) (workersecret.Resolved, error) {
			value, err := projectConfig.ResolveWorkerProjectSecret(ctx, b.OrganizationID, b.WorkerID, b.SecretID)
			return workersecret.Resolved{Descriptor: workersecret.Descriptor{Usage: b.Usage, ContentType: b.ContentType, SuggestedFilename: b.SuggestedFilename, Available: err == nil}, Value: value}, err
		},
		ValidateConnectedAccount: func(ctx context.Context, b workersecret.Binding) error {
			conn, err := helixStore.GetServiceConnection(ctx, b.AccountID)
			if errors.Is(err, helixstore.ErrNotFound) {
				return fmt.Errorf("connected account is unavailable")
			}
			if err != nil {
				return fmt.Errorf("load connected account: %w", err)
			}
			if conn.OrganizationID != b.OrganizationID {
				return fmt.Errorf("connected account is unavailable")
			}
			switch b.ExportKey {
			case "github_app/installation_token":
				if conn.Type != types.ServiceConnectionTypeGitHubApp {
					return fmt.Errorf("connected account export is unavailable")
				}
			case "slack_workspace/bot_token":
				if conn.Type != types.ServiceConnectionTypeSlackWorkspace {
					return fmt.Errorf("connected account export is unavailable")
				}
			default:
				return fmt.Errorf("connected account export %q is not approved", b.ExportKey)
			}
			return nil
		},
		ConnectedAccount: func(ctx context.Context, b workersecret.Binding) (workersecret.Resolved, error) {
			if b.ExportKey == "github_app/installation_token" {
				conn, err := helixStore.GetServiceConnection(ctx, b.AccountID)
				if errors.Is(err, helixstore.ErrNotFound) {
					return workersecret.Resolved{}, fmt.Errorf("connected account is unavailable")
				}
				if err != nil {
					return workersecret.Resolved{}, fmt.Errorf("load connected account: %w", err)
				}
				if conn.OrganizationID != b.OrganizationID || conn.Type != types.ServiceConnectionTypeGitHubApp {
					return workersecret.Resolved{}, fmt.Errorf("connected account is unavailable")
				}
				key, err := cfg.APIServer.getEncryptionKey()
				if err != nil {
					return workersecret.Resolved{}, err
				}
				pem, err := crypto.DecryptAES256GCM(conn.GitHubPrivateKey, key)
				if err != nil {
					return workersecret.Resolved{}, fmt.Errorf("decrypt GitHub App credential: %w", err)
				}
				credential, err := githubskill.MintInstallationCredential(ctx, conn.GitHubAppID, conn.GitHubInstallationID, string(pem), conn.BaseURL)
				if err != nil {
					return workersecret.Resolved{}, fmt.Errorf("mint GitHub installation token: %w", err)
				}
				descriptor := workersecret.Descriptor{Available: true}
				if !credential.ExpiresAt.IsZero() {
					expiresAt := credential.ExpiresAt.UTC()
					descriptor.ExpiresAt = &expiresAt
				}
				return workersecret.Resolved{
					Descriptor: descriptor,
					Value:      credential.Token,
				}, nil
			}
			if b.ExportKey == "slack_workspace/bot_token" {
				conn, err := helixStore.GetServiceConnection(ctx, b.AccountID)
				if errors.Is(err, helixstore.ErrNotFound) {
					return workersecret.Resolved{}, fmt.Errorf("connected account is unavailable")
				}
				if err != nil {
					return workersecret.Resolved{}, fmt.Errorf("load connected account: %w", err)
				}
				if conn.OrganizationID != b.OrganizationID || conn.Type != types.ServiceConnectionTypeSlackWorkspace {
					return workersecret.Resolved{}, fmt.Errorf("connected account is unavailable")
				}
				ws, err := slackWS.resolveForOrg(ctx, b.OrganizationID, conn.SlackTeamID)
				if err != nil {
					return workersecret.Resolved{}, err
				}
				if ws.BotToken == "" {
					return workersecret.Resolved{}, fmt.Errorf("connected account is unavailable")
				}
				return workersecret.Resolved{Descriptor: workersecret.Descriptor{Usage: "use as a Slack Bearer token", Available: true, ResourceID: conn.SlackTeamID}, Value: ws.BotToken}, nil
			}
			return workersecret.Resolved{}, fmt.Errorf("connected account export %q is not approved", b.ExportKey)
		},
	}
	catalog := func(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.AvailableSource, error) {
		secrets, err := projectConfig.ListWorkerProjectSecretRecords(ctx, orgID, workerID)
		if err != nil && !errors.Is(err, runtime.ErrProjectConfigUnsupported) {
			return nil, err
		}
		out := make([]workersecret.AvailableSource, 0, len(secrets))
		for _, secret := range secrets {
			out = append(out, workersecret.AvailableSource{Group: "Helix Secrets", Label: secret.Name, SourceKind: workersecret.SourceHelixSecret, SecretID: secret.ID, ProposedName: secret.Name, Usage: "export " + secret.Name})
		}
		connections, err := helixStore.ListServiceConnections(ctx, orgID)
		if err != nil {
			return nil, err
		}
		for _, conn := range connections {
			switch conn.Type {
			case types.ServiceConnectionTypeGitHubApp:
				out = append(out, workersecret.AvailableSource{Group: "Connected Accounts", Label: conn.Name, SourceKind: workersecret.SourceConnectedAccount, AccountID: conn.ID, ExportKey: "github_app/installation_token", ProposedName: "GH_TOKEN", Usage: "export GH_TOKEN"})
			case types.ServiceConnectionTypeSlackWorkspace:
				out = append(out, workersecret.AvailableSource{
					Group: "Connected Accounts", Label: conn.SlackTeamName,
					SourceKind: workersecret.SourceConnectedAccount, AccountID: conn.ID,
					ExportKey: "slack_workspace/bot_token", ProposedName: "SLACK_BOT_TOKEN",
					Usage: "use as a Slack Bearer token", ResourceID: conn.SlackTeamID,
				})
			}
		}
		return out, nil
	}
	workerSecrets, err := workersecrets.New(st.WorkerSecretBindings, st.Nodes, secretResolver, deps.Now, func(_ context.Context, orgID string, _ orgchart.NodeID, name, _ string, result workersecret.Resolved, err error) {
		if err == nil {
			cfg.APIServer.recordCredential(orgID, name, result.Value)
		}
	}, catalog)
	if err != nil {
		return nil, fmt.Errorf("init worker secrets: %w", err)
	}
	deps.WorkerSecrets = workerSecrets
	deps.Dispatcher = dispatcher

	githubDeliveryReconciler := githubtransport.NewDeliveryReconciler(
		st,
		githubtransport.TokenResolver(gitHubTokenResolver),
		cfg.APIServer.Cfg.GitHub.APIBaseURL(),
		logger,
	)

	// Prompts registry — drives slash-command typeahead in the chat
	// composer (/help, /role, /worker, …) and surfaces the same set as
	// MCP prompts on each per-Worker MCP server. Without this the chat
	// bridge sends `/help` as a literal user message to the LLM, which
	// has no idea what it means; with it, expandSlashCommand replaces
	// the token with the rendered prompt body before sending.
	promptReg := prompts.NewRegistry()
	if err := prompts.RegisterBuiltins(promptReg); err != nil {
		return nil, fmt.Errorf("register helix-org prompts: %w", err)
	}

	// The MCP registry (reg) and orgServer are built later, after lifecycleSvc
	// is assembled, so MCP create_bot shares the same reconciler-complete
	// lifecycle as REST. See `deps.Lifecycle = lifecycleSvc` below.

	// JSON handlers consumed by the React pages at
	// /orgs/:org_id/helix-org/*. They mount under
	// /api/v1/orgs/{org}/ via the orgServer's extras list. REST hire and
	// chat-driven hire both call the same workers.Hire service (wired
	// into apiDeps.Workers below) — one implementation, no drift.

	// Delete cascades Helix-side Agent teardown, archives its runtime-owned
	// project, preserves repositories, and performs full org-store cleanup. The Helix
	// runtime port is satisfied by the same in-process adapter every
	// other Helix call goes through.
	lifecycleSvc := &lifecycle.Service{
		Store:         st,
		Helix:         inProcClient,
		Agents:        inProcClient,
		Logger:        logger,
		AgentDelivery: agentDelivery,
		// Node-scoped reconcilers: the single topology reconciler (one owner
		// of activation/team Topic lifecycle across create, reparent, and delete).
		NodeReconcilers: []lifecycle.NodeReconciler{deps.Reconciler},
		Mirror:          mirror, // Delete stops the deleted bot's subscription
		// Hire collaborators (the create half of the lifecycle). REST POST
		// /workers and the MCP hire_worker tool both drive Hire through
		// this service, so the hire semantics live in one place.
		Dispatcher: dispatcher,
		HireHook:   deps.HireHook,
		Now:        deps.Now,
		NewID:      deps.NewID,
	}

	// GitHub-App integration (install-status gate + repo picker) — owned
	// by the helixorg.GitHubIntegration adapter rather than inline closures here.
	gitHubInt := helixorg.NewGitHubIntegration(helixStore, cfg.APIServer.getEncryptionKey, cfg.APIServer.Cfg.GitHub.AppSlug, cfg.APIServer.Cfg.GitHub.WebURL())

	// Inbound-webhook provisioners, keyed by transport Kind. Each
	// transport that needs external registration plugs in here; the
	// trigger service dispatches on the Trigger's Kind. The github API
	// specifics live in the github transport infra package, not the
	// application layer.
	var gitLabWebhookManager gitlabtransport.WebhookManager
	if manager, ok := cfg.APIServer.gitRepositoryService.(gitlabtransport.WebhookManager); ok {
		gitLabWebhookManager = manager
	}
	inboundProvisioners := map[transport.Kind]trigger.Inbound{
		transport.KindGitHub: githubtransport.NewWebhookProvisioner(
			configReg,
			githubtransport.TokenResolver(gitHubTokenResolver),
			cfg.APIServer.Cfg.WebServer.URL,
		),
		transport.KindGitLab: gitlabtransport.NewWebhookProvisioner(
			configReg,
			gitLabWebhookManager,
			cfg.APIServer.Cfg.WebServer.URL,
		),
		// Slack has no per-Trigger install: a Slack Trigger is
		// workspace-scoped and receives every channel the bot is
		// /invite'd into. It is auto-created with the workspace
		// (slackWorkspaceTriggerID), so there's no provisioner to
		// register.
	}

	// Application services shared by the REST adapter. Built once here
	// (the composition root) from the store + collaborators; the api
	// package holds these services, never the store (Phase-D seam).
	// The MCP registry is the authority on which tool names exist. It is
	// built below (it needs the assembled services), so the catalogue is
	// read through a closure — by the time any per-org reconcile runs,
	// RegisterBuiltins has populated it.
	reg := mcptools.NewRegistry()
	knownTools := func() map[tool.Name]bool {
		all := reg.List()
		out := make(map[tool.Name]bool, len(all))
		for _, t := range all {
			out[t.Name()] = true
		}
		return out
	}
	svc := buildOrgServices(st, &deps, bc, dispatcher, inboundProvisioners, knownTools)

	// Auto-manage one Slack Trigger per connected workspace.
	slackTopics := &slackWorkspaceTopics{triggers: svc.Triggers, logger: logger}

	// streamCron drives KindCron Triggers through the shared publish use
	// case — append → notify → route — so cron-driven activations look
	// identical to every other event downstream. Started in a goroutine
	// from registerRoutes once we have the long-lived ctx.
	streamCronScheduler, err := streamcron.New(st, svc.Publishing, deps.NewID, deps.Now)
	if err != nil {
		return nil, fmt.Errorf("init streamcron scheduler: %w", err)
	}
	// Create (the lifecycle's create half) delegates the bot-row creation to
	// the bots service so the base-tool union + id minting are shared with
	// the REST/MCP update path.
	lifecycleSvc.Nodes = svc.Nodes
	// Create attaches the new bot to its initial sources via the shared
	// attachment use case (same as attach_worker) — one implementation.
	lifecycleSvc.Attacher = svc.Attachments

	// The helixevents reconciler owns the org's single "Helix events"
	// Trigger — created on org bootstrap and ensured defensively by the
	// attention publisher for brand-new orgs. Shared by the publisher
	// (below) and the bootstrap path (via the org scope) so both agree on
	// its identity.
	helixEventsReconciler := helixevents.New(helixevents.Deps{
		Triggers: st.Triggers,
		Now:      deps.Now,
		Logger:   logger,
	})

	// Wire the spec-task attention-event sink: each AttentionEvent the
	// Helix UI shows is also published onto the org's single Helix events
	// Trigger, so attached Workers are started via the normal route path.
	// Routing to individual bots is done by filter processors over that
	// one Trigger (keyed on domain / event_type / project_id).
	cfg.APIServer.attentionService.SetEventSink(&attentionTopicPublisher{
		reconciler: helixEventsReconciler,
		publisher:  svc.Publishing,
	})

	// Slack inbound: one shared ingest serves both ingress sources. It
	// resolves a delivery's team_id to the owning org (a slack_workspace
	// ServiceConnection), then publishes onto matching KindSlack Triggers —
	// the attachment + processor/filter layer routes to Workers.
	slackIngest := slacktransport.NewIngest(slackWS, st, svc.Publishing, logger)
	// REST Events API source — one global signed webhook for every org.
	publicSlackEvents := slackcore.EventsAPIHandler(cfg.APIServer.slackSigningSecrets, slackIngest.OnEvent, logger)
	// Socket Mode source — a manager reconciles live connections against
	// the configured socket-mode apps on an interval (and on Kick from the
	// create/delete handlers), so installing or editing a socket app takes
	// effect with no server restart. Single-replica: a multi-replica
	// deployment would need a cross-replica owner lock to hold the one
	// socket, which isn't wired today.
	slackSocket := cfg.APIServer.newSlackSocketManager(slackIngest, logger)
	slackSocketRun := func(ctx context.Context) {
		slackSocket.Run(ctx, slackSocketReconcileInterval)
	}

	// Processor execution: the runner re-publishes each processor's
	// output through svc.Publishing, so it is wired after buildOrgServices
	// (which builds Publishing) and registered late on the dispatcher,
	// exactly like the outbound emitters above.
	processorRunner := processing.New(st.Processors, svc.Publishing, logger)
	dispatcher.RegisterProcessorRunner(processorRunner)
	threadFollower := slackrouting.NewThreadFollower(slackrouting.ThreadFollowerDeps{
		Events:    st.DomainEvents,
		Publisher: svc.Publishing,
		NewID:     deps.NewID,
		Now:       deps.Now,
		Logger:    logger,
	})
	processorRunner.RegisterPostRouter(threadFollower)

	// Slack auto-router: a second reconciler (composition over the processors
	// service) maintains one route per AI Worker on each Automated Slack
	// router. Wired into hire/fire via the lifecycle service, and invoked on
	// workspace-connect via slackAutoRouter below.
	slackRouteReconciler := slackrouting.New(slackrouting.Deps{
		Nodes:       st.Nodes,
		Attachments: svc.Attachments,
		Processors:  svc.Processors,
		Now:         deps.Now,
		Logger:      logger,
	})
	lifecycleSvc.OrgReconcilers = append(lifecycleSvc.OrgReconcilers, slackRouteReconciler)

	// Share the one lifecycleSvc with the MCP tools so create_bot runs the
	// same OrgReconcilers (Slack auto-router) as REST POST /bots.
	deps.Lifecycle = lifecycleSvc
	deps.HumanDelivery = humanInbox{
		store:           helixStore,
		slackWorkspaces: slackWS,
		threadFollower:  threadFollower,
		ensureSlackRouter: func(ctx context.Context, orgID string, routerID processor.ProcessorID, workerID string) error {
			router, err := svc.Processors.Get(ctx, orgID, routerID)
			if err != nil {
				return err
			}
			return validateSlackReplyRouter(router, strings.TrimPrefix(routerID, "p-slack-router-"), workerID)
		},
	}

	// Activations owns start/stop/restart for REST and MCP. Built before
	// RegisterBuiltins so start_bot / stop_bot / restart_bot share the same
	// service instance as POST /bots/{id}/activate|stop-agent|restart-agent.
	sessionResetter := botSessionResetter{client: inProcClient, st: st}
	workerRuntime := orgWorkerRuntime{st: st, sessions: helixStore}
	svc.Activations = activations.New(activations.Deps{
		Repo:       st.Activations,
		Now:        deps.Now,
		NewID:      deps.NewID,
		Ensurer:    projectApplier,
		Dispatcher: dispatcher,
		Sessions:   workerRuntime,
		Stopper:    sessionResetter,
		Resetter:   sessionResetter,
	})
	deps.Activations = svc.Activations
	// Share the processors service with MCP tools so create_processor uses
	// the same auto-provision + cycle-check path as REST /processors.
	deps.Processors = svc.Processors

	encryptionKey, err := cfg.APIServer.getEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("load asset encryption key: %w", err)
	}
	assetsSvc, err := assetapp.New(assetapp.Deps{
		Assets: st.Assets,
		Links:  st.AssetLinks,
		Nodes:  st.Nodes,
		GenerateKey: func() (string, string, error) {
			return crypto.GenerateSSHKeyPair("ed25519")
		},
		Encrypt: func(plaintext []byte) (string, error) {
			return crypto.EncryptAES256GCM(plaintext, encryptionKey)
		},
		Now:            deps.Now,
		NewID:          deps.NewID,
		OnToolsChanged: deps.ToolChangeNotifier,
	})
	if err != nil {
		return nil, fmt.Errorf("init assets service: %w", err)
	}
	assetSSH, err := assetssh.New(assetsSvc, func(ciphertext string) ([]byte, error) {
		return crypto.DecryptAES256GCM(ciphertext, encryptionKey)
	}, deps.NewID)
	if err != nil {
		return nil, fmt.Errorf("init asset SSH client: %w", err)
	}
	assetSSHIssuer, err := assetssh.NewIssuer(assetsSvc, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init asset SSH certificate issuer: %w", err)
	}
	assetSSHProxy, err := assetssh.NewProxy(assetsSvc, assetSSH, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init asset SSH proxy: %w", err)
	}
	sandboxSSHIssuer, err := assetssh.NewSandboxIssuer(sandboxAccess, encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("init sandbox SSH certificate issuer: %w", err)
	}
	assetSSHProxy.WithSandboxes(sandboxAccess)
	orgAudit := services.NewOrgAuditLogService(helixStore)
	auditProjects := func(ctx context.Context, orgID, actorID string) (string, error) {
		state, err := runtimehelix.LoadState(ctx, st, orgID, orgchart.NodeID(actorID))
		if err != nil {
			return "", err
		}
		return state.ProjectID, nil
	}
	assetSSH.WithAudit(orgAudit, auditProjects)
	assetSSHProxy.WithAudit(orgAudit, auditProjects)
	deps.Assets = assetsSvc
	deps.AssetSSH = assetSSH
	deps.AssetSSHIssuer = assetSSHIssuer
	deps.SandboxSSHIssuer = sandboxSSHIssuer
	deps.AssetSSHProxyAddress = cfg.APIServer.Cfg.WebServer.AssetSSHProxyAddress
	deps.AssetHealth = assetSSH.Health

	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		return nil, fmt.Errorf("register helix-org builtins: %w", err)
	}
	orgServer := helixorgserver.NewFromStore(st, reg, bc, dispatcher, logger).
		WithPrompts(promptReg).
		WithAudit(orgAudit, auditProjects)

	slackAutoRouter := &slackAutoRouter{procs: svc.Processors, routes: slackRouteReconciler, logger: logger}
	apiDeps := helixorgapi.Deps{
		Assets:        assetsSvc,
		Triggers:      svc.Triggers,
		Messages:      svc.Messages,
		Nodes:         svc.Nodes,
		Attachments:   svc.Attachments,
		Publishing:    svc.Publishing,
		Queries:       svc.Queries,
		Activations:   svc.Activations,
		Processors:    svc.Processors,
		ChartLayout:   chartlayout.New(chartlayout.Deps{Positions: st.ChartPositions, Now: deps.Now}),
		WorkerSecrets: workerSecrets,
		AuthorizeHumanContact: func(ctx context.Context, orgID, humanUserID string) error {
			callerID := runtimehelix.UserIDFromContext(ctx)
			if callerID == "" {
				return fmt.Errorf("authenticated user is missing")
			}
			if callerID == humanUserID {
				return nil
			}
			if _, err := cfg.APIServer.authorizeOrgOwner(ctx, &types.User{ID: callerID}, orgID); err != nil {
				return fmt.Errorf("only the person or an organization owner can update human contact details: %w", err)
			}
			return nil
		},
		BotRuntime: workerRuntime,
		// Kept on apiDeps so legacy/tests that poke the ports directly still
		// work; REST stop/restart now go through Activations.
		BotSessionResetter: sessionResetter,
		BotDesktopStopper:  sessionResetter,
		// GitHubInbound builds the inbound github transport per org — it
		// reads matching topics + appends events, so it holds the store
		// here in the composition root rather than in the api adapter.
		GitHubInbound: func(orgID string) http.Handler {
			t := githubtransport.New(orgID, configReg, st, svc.Publishing, logger)
			if gitHubTokenResolver != nil {
				t = t.WithTokenResolver(githubtransport.TokenResolver(gitHubTokenResolver))
			}
			return t.HandleInbound()
		},
		Configs:             configReg,
		Hub:                 bc,
		Dispatcher:          dispatcher,
		DBPath:              orgRoot,
		Lifecycle:           lifecycleSvc,
		AgentUpdater:        inProcClient,
		AgentReader:         inProcClient,
		AgentDefaultApplier: inProcClient,
		Tools:               reg,
		ProjectEnsurer:      projectApplier,
		// Production: the github topic transport's Token() falls
		// back to whatever GitHub OAuth connection the org members
		// have already authorised, so operators don't have to paste a
		// PAT into transport.github. The resolver lives in
		// helix_org_github.go.
		GitHubTokenResolver: gitHubTokenResolver,
		// GitHubIdentity lets the repo picker tell app mode from oauth mode
		// so it lists the installation's repos (not /user/repos) when the
		// bot is installed. Adapts the server-side resolver into the org
		// package's mirror struct.
		GitHubIdentity: func(ctx context.Context, orgID string) (helixorgapi.GitHubIdentity, error) {
			id, err := identityResolver(ctx, orgID)
			if err != nil {
				return helixorgapi.GitHubIdentity{}, err
			}
			return helixorgapi.GitHubIdentity{
				Mode:           id.Mode,
				Token:          id.Token,
				AppID:          id.AppID,
				InstallationID: id.InstallationID,
				BaseURL:        id.BaseURL,
			}, nil
		},
		// GitHubInstallation backs the New Topic "Install Helix" gate;
		// GitHubAppRepos backs the repo picker. Both are owned by the
		// helixorg.GitHubIntegration adapter (helixorg/github.go) — this
		// composition root just constructs it and passes method values.
		GitHubInstallation: gitHubInt.InstallationStatus,
		GitHubAppRepos:     gitHubInt.AppRepos,
		// GitHubManifestStart builds the "create the Helix app" manifest flow.
		GitHubManifestStart: helixorg.NewGitHubManifestStart(cfg.APIServer.getEncryptionKey, cfg.APIServer.Cfg.GitHub.WebURL()),
		// PublicServerURL is the externally-reachable base URL the
		// auto-installed GitHub webhook should POST back to. Helix's
		// SERVER_URL env var is the canonical place it lives.
		PublicServerURL: cfg.APIServer.Cfg.WebServer.URL,
		AssetHealth: func(ctx context.Context, orgID, id string) helixorgapi.AssetHealthDTO {
			health := assetSSH.Health(ctx, orgID, id)
			return helixorgapi.AssetHealthDTO{
				TCPReachable: health.TCPReachable,
				SSHReachable: health.SSHReachable,
				LatencyMS:    health.LatencyMS,
				Error:        health.Error,
				CheckedAt:    health.CheckedAt,
			}
		},
	}
	apiRoutes := helixorgapi.Routes(apiDeps)
	extras := make([]helixorgserver.Route, 0, len(apiRoutes))
	for _, rt := range apiRoutes {
		extras = append(extras, helixorgserver.Route{Pattern: rt.Pattern, Handler: rt.Handler})
	}

	log.Info().
		Str("root", orgRoot).
		Int("json_api_routes", len(extras)).
		Msg("helix-org mounted at /api/v1/orgs/{org}/helix-org/")
	scope := newHelixOrgScope(configReg, st, helixStore, mirror, slackRouteReconciler, helixEventsReconciler)
	scope.botTools = svc.Nodes
	scope.botRepair = func(ctx context.Context, orgID, _ string) error {
		if err := removeLegacyHelixOrgMCPs(ctx, orgID, st, inProcClient); err != nil {
			return err
		}
		return repairNeverActivatedBots(ctx, orgID, st, dispatcher, configReg, inProcClient)
	}

	// Public github webhook handler — mounted on the insecure router
	// because GitHub deliveries authenticate via HMAC, not the helix
	// session/api-key layer. Per-request: resolve {org} from mux
	// vars → orgID → build the github.Transport → dispatch.
	ghLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Reuse the bot-preferring projection so the public webhook's outbound
	// actions act as the installed App when there is one.
	tokenResolver := gitHubTokenResolver
	publisher := svc.Publishing
	publicGitHubWebhook := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orgSlugOrID := mux.Vars(r)["org"]
		if orgSlugOrID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		org, err := cfg.APIServer.lookupOrg(r.Context(), orgSlugOrID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := scope.ensureBootstrap(r.Context(), org.ID); err != nil {
			http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
			return
		}
		t := githubtransport.New(org.ID, configReg, st, publisher, ghLogger)
		if tokenResolver != nil {
			t = t.WithTokenResolver(githubtransport.TokenResolver(tokenResolver))
		}
		t.HandleInbound().ServeHTTP(w, r)
	})

	// Per-Trigger public github webhook handler. Same auth model as
	// the org-level handler (HMAC over body); routes deliveries to
	// the single Trigger named in the path so operators can hand
	// GitHub a Trigger-specific URL.
	publicGitHubWebhookForStream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		orgSlugOrID := vars["org"]
		triggerID := vars["trigger_id"]
		if orgSlugOrID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		if triggerID == "" {
			http.Error(w, "missing trigger_id", http.StatusBadRequest)
			return
		}
		org, err := cfg.APIServer.lookupOrg(r.Context(), orgSlugOrID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := scope.ensureBootstrap(r.Context(), org.ID); err != nil {
			http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
			return
		}
		t := githubtransport.New(org.ID, configReg, st, publisher, ghLogger)
		if tokenResolver != nil {
			t = t.WithTokenResolver(githubtransport.TokenResolver(tokenResolver))
		}
		t.HandleInboundForTrigger(triggerID).ServeHTTP(w, r)
	})

	publicGitLabWebhookForTrigger := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		orgSlugOrID := vars["org"]
		triggerID := vars["trigger_id"]
		if orgSlugOrID == "" || triggerID == "" {
			http.Error(w, "missing org or trigger_id", http.StatusBadRequest)
			return
		}
		org, err := cfg.APIServer.lookupOrg(r.Context(), orgSlugOrID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if err := scope.ensureBootstrap(r.Context(), org.ID); err != nil {
			http.Error(w, "bootstrap: "+err.Error(), http.StatusInternalServerError)
			return
		}
		gitlabtransport.New(org.ID, configReg, st, publisher, ghLogger).
			HandleInboundForTrigger(triggerID).ServeHTTP(w, r)
	})

	// GitHub App Manifest flow callbacks. Insecure mounts (top-level
	// navigations from github.com): the conversion callback is authenticated
	// by the encrypted ?state=; the setup callback only records a non-secret
	// installation id onto the org's app.
	publicGitHubManifestCallback := helixorg.NewGitHubManifestCallbackHandler(
		cfg.APIServer.getEncryptionKey, helixStore, deps.NewID,
		cfg.APIServer.Cfg.GitHub.WebURL(), cfg.APIServer.Cfg.GitHub.APIBaseURL(),
	)

	// Membership-driven human-node + Chief of Staff seeder. Reuses the same
	// lifecycle (CoS runs) and bots (human nodes never run) services the REST
	// create path uses; botStore backs idempotency checks.
	seeder := &orgGraphSeeder{lifecycle: lifecycleSvc, bots: svc.Nodes, botStore: st.Nodes}
	// Bootstrap-time reconcile: converge human nodes against org membership,
	// and re-seed / tool-backfill Chief of Staff (idempotent — unions any
	// new OwnerBotTools entries onto existing CoS). Runs once per org per
	// process via ensureBootstrap.
	scope.humanReconcile = func(ctx context.Context, orgID string) error {
		members, err := listOrgMemberUsers(ctx, helixStore, orgID)
		if err != nil {
			return err
		}
		if err := seeder.ReconcileHumans(ctx, orgID, members); err != nil {
			return err
		}
		if err := seeder.SeedChiefOfStaff(ctx, orgID); err != nil {
			return err
		}
		return lifecycleSvc.ReconcileAgentLinks(ctx, orgID)
	}

	return &helixOrgHandlers{
		api:                           orgServer.Handler(extras...),
		mcpServer:                     orgServer,
		scope:                         scope,
		store:                         st,
		lifecycle:                     lifecycleSvc,
		seeder:                        seeder,
		streamCron:                    streamCronScheduler,
		githubDeliveryRun:             githubDeliveryReconciler.Run,
		publicGitHubWebhook:           publicGitHubWebhook,
		publicGitHubWebhookForStream:  publicGitHubWebhookForStream,
		publicGitLabWebhookForTrigger: publicGitLabWebhookForTrigger,
		publicGitHubManifestCallback:  publicGitHubManifestCallback,
		publicSlackEvents:             publicSlackEvents,
		slackSocketRun:                slackSocketRun,
		slackTopics:                   slackTopics,
		slackAutoRouter:               slackAutoRouter,
		slackSocket:                   slackSocket,
		assetSSHProxyRun: func(ctx context.Context) error {
			return assetSSHProxy.Serve(ctx, cfg.APIServer.Cfg.WebServer.AssetSSHProxyListen)
		},
	}, nil
}

// helixOrgConfig is enough of the surrounding config to bring up the
// embedded org. LocalFSPath roots the per-Worker working-directory
// tree (falls back to os.TempDir() when empty). APIServer=nil
// disables helix-org entirely.
type helixOrgConfig struct {
	LocalFSPath string
	APIServer   *HelixAPIServer
}

// dynamicProjectApplier is a chat.ProjectEnsurer that re-reads
// `worker.*` and `helix.*` from the config registry on every Ensure
// call. Building the underlying runtimehelix.WorkerProject at API
// startup and reusing it freezes `worker.runtime`/`credentials`/
// `provider`/`model` at boot time. These values provision new apps;
// existing apps own their runtime configuration after creation.
//
// Store is exposed directly because helix_org_chat.go needs it to
// load/save the per-Worker session pointer on the same row the
// spawner uses (helix-org's WorkerRuntimeState).
type dynamicProjectApplier struct {
	cfg        *configregistry.Registry
	projectSvc runtimehelix.ProjectService
	Store      *helixorgstore.Store
	// helixStore is the main Helix store, used to validate the service API
	// key and resolve the org's display name for the `<Bot> @ <Org>` label.
	helixStore helixstore.Store
	logger     *slog.Logger
}

// Ensure satisfies chat.ProjectEnsurer. Builds a fresh
// runtimehelix.WorkerProject from the current registry state and
// delegates. WorkerProject.Ensure is itself idempotent — first call
// applies, subsequent calls fast-path on the existing project.
//
// After Ensure succeeds, re-attaches the helix-org MCP entry on the
// per-Worker agent app. ApplyProject (called inside WorkerProject.Ensure)
// wholesale-replaces Config.Helix on update, so any MCPs we attached
func (d *dynamicProjectApplier) Ensure(ctx context.Context, orgID string, workerID orgchart.NodeID) (projectID, agentAppID, repoID string, err error) {
	applier, err := buildHelixOrgProjectApplier(ctx, orgID, d.cfg, d.projectSvc, d.Store, d.logger)
	if err != nil {
		return "", "", "", err
	}
	applier.OrgDisplayName = orgDisplayName(ctx, d.helixStore, orgID)
	projectID, agentAppID, repoID, err = applier.Ensure(ctx, orgID, workerID)
	if err != nil {
		return "", "", "", err
	}
	return projectID, agentAppID, repoID, nil
}

// buildHelixOrgProjectApplier constructs the WorkerProject that
// both the chat bridge (owner-chat) and the spawner (AI Worker
// activations) drive. Single source of truth for the embedded
// SaaS's default agent configuration from the config registry,
// subscription credentials.
//
// Called per Ensure by dynamicProjectApplier so registry edits
// (agent.default and legacy worker.* keys)
// take effect immediately. The struct it returns is cheap to build
// and short-lived — one apply call, then discarded.
//
// orgDisplayName resolves an org's human label for the `<Bot> @ <Org>`
// project name. Prefers DisplayName, falls back to the slug Name, then
// to "" (which makes WorkerProject use a bare bot label). Best-effort:
// a lookup failure yields "" rather than breaking activation.
func orgDisplayName(ctx context.Context, s helixstore.Store, orgID string) string {
	if s == nil || orgID == "" {
		return ""
	}
	org, err := s.GetOrganization(ctx, &helixstore.GetOrganizationQuery{ID: orgID})
	if err != nil || org == nil {
		return ""
	}
	if org.DisplayName != "" {
		return org.DisplayName
	}
	return org.Name
}

func buildHelixOrgProjectApplier(
	ctx context.Context,
	orgID string,
	cfg *configregistry.Registry,
	projectSvc runtimehelix.ProjectService,
	orgStore *helixorgstore.Store,
	logger *slog.Logger,
) (*runtimehelix.WorkerProject, error) {
	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, cfg)
	return &runtimehelix.WorkerProject{
		Service:     projectSvc,
		Store:       orgStore,
		OrgID:       orgID,
		Runtime:     runtime,
		Credentials: credentials,
		Provider:    provider,
		Model:       model,
		Logger:      logger,
	}, nil
}

// resolveWorkerAgentConfig reads the atomic default agent config (or legacy
// worker.* keys) and normalises
// them into the (runtime, credentials, provider, model) tuple that
// matches Helix's per-agent UI:
//
//   - claude_code/codex_cli + subscription → no provider/model (CLI authenticates via OAuth)
//   - claude_code + api_key       → provider+model required, inference via Helix's anthropic provider
//   - zed_agent (or other)        → provider+model required, always Helix-routed (credentials forced to "api_key")
//
// We coerce silly combinations (e.g. zed_agent + subscription) to the
// only mode that actually works for that runtime, mirroring Helix's
// per-agent validator.
func resolveWorkerAgentConfig(ctx context.Context, orgID string, cfg *configregistry.Registry) (runtime, credentials, provider, model string) {
	agent, _ := cfg.GetDefaultAgentConfig(ctx, orgID)
	runtime = string(agent.CodeAgentRuntime)
	if runtime == "" {
		runtime = "claude_code"
	}
	credentials = string(agent.CodeAgentCredentialType)
	if credentials == "" {
		credentials = "subscription"
	}
	if !workerRuntimeSupportsSubscription(runtime) {
		credentials = "api_key"
	}
	if credentials == "api_key" {
		provider = agent.Provider
		model = agent.Model
	} else if runtime == "codex_cli" {
		model = agent.Model
	}
	return runtime, credentials, provider, model
}

func workerRuntimeSupportsSubscription(runtime string) bool {
	return runtime == "claude_code" || runtime == "codex_cli"
}

func removeLegacyHelixOrgMCPs(ctx context.Context, orgID string, st *helixorgstore.Store, projectSvc runtimehelix.ProjectService) error {
	// Stamp orgID on the context so the inproc client can resolve identity
	// when it calls GetAppConfig/UpdateAppConfig. Without this, the inproc
	// client finds neither user nor org on the context and returns
	// "no user or organization on context". lazyHelixOrgSpawner in this file
	// uses the same pattern (see WithOrgID call near line 1503).
	ctx = helixorgserver.WithOrgID(ctx, orgID)
	workers, err := st.Nodes.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("list bots for legacy MCP cleanup: %w", err)
	}
	var cleanupErrors []error
	for _, worker := range workers {
		if worker.AgentID == "" {
			continue
		}
		if err := runtimehelix.RemoveHelixOrgMCP(ctx, projectSvc, worker.AgentID); err != nil {
			if errors.Is(err, helixstore.ErrNotFound) {
				continue
			}
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove legacy helix-org MCP from bot %s: %w", worker.ID, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

// buildHelixOrgSpawnerConfig assembles the SpawnerConfig for
// helix-org's production zed_external Spawner. The embedded SaaS
// runs Workers on the `claude_code` runtime with subscription
// credentials — the in-sandbox Claude Code CLI authenticates
// Anthropic via the operator's own OAuth, so we don't pass
// Provider/Model and the Helix-side anthropic proxy doesn't need an
// BearerForUser resolves the hiring user's id (persisted on the
// Worker's runtime state by hire_worker) to a current api_key at
// activation time. This is how every per-Worker Helix project +
// session winds up owned by the human who hired the Worker — their
// Claude subscription, their desktop quota, their audit trail —
// without helix-org ever holding a token at rest.
// spawnerDeps groups the process-wide collaborators the helix-org
// Spawner needs into one options struct — the alternative was ~13
// positional params on both buildHelixOrgSpawnerConfig and
// lazyHelixOrgSpawner (design §5.4). Populate the exported fields at the
// call site so the wiring reads as names, not a positional wall.
//
// SpawnerClient and ProjectService are the same in-proc adapter in
// production but kept separate so a future split (remote spawner, local
// project service) doesn't churn the struct.
type spawnerDeps struct {
	Cfg           *configregistry.Registry
	HelixStore    helixstore.Store
	SpawnerClient runtimehelix.SpawnerClient
	// ProjectSvc lets the spawner's *internal* ensureProject fast-path
	// verify the Helix project exists without a nil-deref. Required.
	ProjectSvc runtimehelix.ProjectService
	OrgStore   *helixorgstore.Store
	Hub        *wakebus.Bus
	// PubSub is the host API's NATS pubsub; the per-activation bridge
	// calls SubscribeSessionUpdates on it. Required.
	PubSub  pubsub.PubSub
	Logger  *slog.Logger
	Applier *dynamicProjectApplier // used by lazyHelixOrgSpawner only
	Mirror  *runtimehelix.Mirror   // process-wide singleton
	NewID   func() string
	Now     func() time.Time
}

func buildHelixOrgSpawnerConfig(ctx context.Context, orgID string, d spawnerDeps) (runtimehelix.SpawnerConfig, error) {
	if d.PubSub == nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix-org spawner: PubSub is required")
	}
	if d.ProjectSvc == nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix-org spawner: ProjectService is required")
	}
	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, d.Cfg)
	specsMandate, _ := d.Cfg.GetString(ctx, orgID, "worker.specs_mandate")
	return runtimehelix.SpawnerConfig{
		Client:         d.SpawnerClient,
		ProjectService: d.ProjectSvc,
		OrgID:          orgID,
		OrgDisplayName: orgDisplayName(ctx, d.HelixStore, orgID),
		Runtime:        runtime,
		Credentials:    credentials,
		Provider:       provider,
		Model:          model,
		SpecsMandate:   specsMandate,
		Store:          d.OrgStore,
		Hub:            d.Hub,
		PubSub:         d.PubSub,
		Snapshotter:    runtimehelix.NoopSessionPreamble{},
		Logger:         d.Logger,
		NewID:          d.NewID,
		Now:            d.Now,
		BearerForUser: func(ctx context.Context, userID string) (string, error) {
			return helixorg.NewHelixAPIKeys(d.HelixStore, d.Cfg).User(ctx, userID)
		},
	}, nil
}

// lazyHelixOrgSpawner returns a runtime.Spawner that builds a fresh
// SpawnerConfig, scoped to the activating org, on every activation.
//
// It MUST NOT cache a single inner Spawner across orgs. SpawnerConfig
// carries tenant-specific identity in OrgID. A cached spawner freezes the
// first activating org's identity and replays it for every other org.
//
// Building per activation is cheap (a handful of config-registry
// reads) and ensures newly provisioned apps use current org defaults.
// Existing apps retain their own configuration. The one thing the old cache
// legitimately provided — a single process-wide inflight cap — is
// preserved by minting one shared semaphore here and injecting it into
// each per-activation config via SpawnerConfig.Sem.
//
// The dynamic applier still runs first to provision or fast-path the
// per-Worker project.
func lazyHelixOrgSpawner(d spawnerDeps) runtime.Spawner {
	// One inflight cap shared across every per-org spawner config.
	sem := make(chan struct{}, runtimehelix.DefaultMaxInflight)
	return func(ctx context.Context, orgID string, workerID orgchart.NodeID, triggers []activation.Trigger) error {
		ctx = helixorgserver.WithOrgID(ctx, orgID)
		// Apply (or fast-path) the per-Worker project with the current
		// worker.* settings before delegating.
		if d.Applier != nil {
			if _, _, _, err := d.Applier.Ensure(ctx, orgID, workerID); err != nil {
				return fmt.Errorf("helix-org spawner: pre-apply project for %s: %w", workerID, err)
			}
		}
		// Rebuild the SpawnerConfig for THIS org on every activation —
		// never reuse another org's config. The shared semaphore keeps
		// the global inflight cap intact.
		cfgVal, err := buildHelixOrgSpawnerConfig(ctx, orgID, d)
		if err != nil {
			return fmt.Errorf("helix-org spawner not configured: %w", err)
		}
		cfgVal.Mirror = d.Mirror // process-wide singleton; not per-org config
		cfgVal.Sem = sem
		log.Trace().
			Str("org_id", orgID).
			Str("worker_id", string(workerID)).
			Str("runtime", cfgVal.Runtime).
			Str("credentials", cfgVal.Credentials).
			Msg("helix-org spawner: per-org activation")
		return runtimehelix.Spawner(cfgVal)(ctx, orgID, workerID, triggers)
	}
}

// openOrgStore binds the org-graph repos against helix's existing
// Postgres connection. The helixStore must expose a *gorm.DB
// accessor (helix's PostgresStore does); there is no dialect
// fallback — helix-org now shares helix's database.
//
// The orgPostgresDB anonymous interface lets us pick up the
// (*PostgresStore).GormDB() accessor without leaking a hard
// dependency on the concrete type — a future store impl that
// exposes the same method works transparently.
func openOrgStore(helixStore helixstore.Store) (*helixorgstore.Store, error) {
	type orgPostgresDB interface {
		GormDB() *gorm.DB
	}
	accessor, ok := helixStore.(orgPostgresDB)
	if !ok {
		return nil, fmt.Errorf("helix-org requires a Postgres-backed helix store; got %T", helixStore)
	}
	// Production wiring: install the FK constraint that ties every
	// org_* table back to organizations(id) ON DELETE CASCADE.
	//
	// OpenWithDB only runs an idempotent AutoMigrate — org_* rows
	// (workers, roles, topics, runtime state, …) survive an API
	// restart. The composite-PK schema (id, org_id) is the only shape
	// in production. If a hand-written breaking migration ever becomes
	// necessary, write an explicit migration script — never drop the
	// tables on boot.
	st, err := orggorm.OpenWithDB(accessor.GormDB(), orggorm.Options{
		InstallOrganizationFK: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open helix-org gorm: %w", err)
	}
	return st, nil
}
