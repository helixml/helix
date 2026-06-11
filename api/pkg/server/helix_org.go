package server

import (
	"context"
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
	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/application/queries"
	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/application/streams"
	"github.com/helixml/helix/api/pkg/org/application/subscriptions"
	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/application/workers"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/credential"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/infrastructure/streamcron"
	githubtransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/github"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/pubsub"
	helixstore "github.com/helixml/helix/api/pkg/store"
)

// helixOrgHandlers bundles the JSON HTTP surface helix-org exposes:
// the JSON-RPC MCP / webhook / org-graph / settings / streams endpoints
// mounted under /api/v1/orgs/{org}/. The React UI at
// /orgs/:org_id/helix-org/* consumes those endpoints.
type helixOrgHandlers struct {
	api   http.Handler
	scope *helixOrgScope
	// streamCron is the in-process scheduler that fires events on
	// KindCron streams. The server's run loop calls Start on it in a
	// goroutine so it runs for the lifetime of the API process.
	streamCron *streamcron.Scheduler
	// publicGitHubWebhook is the inbound /github/webhook handler
	// mounted on the INSECURE router. GitHub deliveries carry no
	// helix session cookie or API key — they authenticate via the
	// per-org HMAC `webhook_secret` checked inside the github
	// transport. The path is /api/v1/orgs/{org}/github/webhook and
	// the handler resolves {org} from mux.Vars before dispatching.
	// Fans out to every github stream whose (repo, events) matches
	// the delivery — multi-stream behaviour.
	publicGitHubWebhook http.Handler
	// publicGitHubWebhookForStream is the per-stream variant. Path:
	// /api/v1/orgs/{org}/streams/{stream_id}/github/webhook —
	// deliveries to this URL are pinned to exactly one stream so
	// operators get a 1:1 mapping between GitHub webhooks and helix
	// streams. The stream's own (repo, events) config still applies
	// so cross-repo or non-whitelisted-event deliveries drop with
	// 204 (no GitHub retries).
	publicGitHubWebhookForStream http.Handler
	// publicGitHubManifestCallback receives GitHub's browser redirect after
	// the App Manifest flow creates the app (path
	// /api/v1/orgs/{org}/github/app-manifest/callback). Insecure mount: it's
	// a top-level navigation from github.com authenticated by the encrypted
	// ?state=, not the helix session. Exchanges the code, stores the app,
	// then redirects to the install page. The installation id is reconciled
	// later via GET /app/installations (no Setup-URL redirect needed).
	publicGitHubManifestCallback http.Handler
}

// alphaFeatureHelixOrg is the alpha-feature flag that gates the
// embedded helix-org surface. Granted per-user via:
//
//	UPDATE users SET alpha_features = array_append(alpha_features, 'helix-org')
//	WHERE email = '...';
const alphaFeatureHelixOrg = "helix-org"

// initHelixOrgHandler builds the in-process helix-org HTTP handler;
// mounted at /api/v1/orgs/{org}/, gated per-user by the `helix-org`
// alpha feature flag.
//
// Storage: the org-graph rows land in the same Postgres database
// helix uses for its primary state — no separate connection pool,
// no FILESTORE_TYPE=fs requirement. The helixStore must expose a
// *gorm.DB accessor (helix's PostgresStore does); otherwise this
// returns an error.
//
// Working directories: each Worker still has an envsDir entry for
// the Spawner's cwd, but the directory's contents are placeholder
// only — real per-Worker state lives in the Worker's Helix project
// (a git repo + agent app). When LocalFSPath is empty the envsDir
// goes under os.TempDir() so gcs/s3 deployments work too.
//
// Every gated user currently shares one owner Worker — see the design
// doc (design/2026-05-17-helix-org-saas-alpha.md) for the multi-tenant
// follow-up.
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
// the store.
type orgWorkerRuntime struct{ st *helixorgstore.Store }

func (o orgWorkerRuntime) State(ctx context.Context, orgID string, workerID orgchart.WorkerID) (helixorgapi.WorkerRuntimeInfo, error) {
	s, err := runtimehelix.LoadState(ctx, o.st, orgID, workerID)
	if err != nil {
		return helixorgapi.WorkerRuntimeInfo{}, err
	}
	return helixorgapi.WorkerRuntimeInfo{
		ProjectID:  s.ProjectID,
		AgentAppID: s.AgentAppID,
		SessionID:  s.SessionID,
	}, nil
}

// orgServices bundles the application services the REST adapter (and the
// per-Worker MCP server) consume. Assembled once by buildOrgServices at
// the composition root — the "Module struct holds the assembled
// services" shape from design §5.4.
type orgServices struct {
	Roles         *roles.Roles
	Streams       *streams.Streams
	Workers       *workers.Workers
	Subscriptions *subscriptions.Subscriptions
	Publishing    *publishing.Publishing
	Queries       *queries.Queries
	Activations   *activations.Activations
}

// buildOrgServices constructs every org application service from the
// store + collaborators. One place owns the wiring so the apiDeps
// literal reads as a list of pre-built services, not seven inline
// constructors. deps carries the clock / id-gen / topology / hire-hook
// seams (a tools.Deps is already assembled by the caller).
func buildOrgServices(st *helixorgstore.Store, deps tools.Deps, bc *streamhub.Hub, dispatcher *dispatch.Dispatcher, envsDir string) orgServices {
	rolesSvc := roles.New(roles.Deps{Roles: st.Roles, Now: deps.Now, NewID: deps.NewID, BaseTools: tools.BaseReadTools})
	return orgServices{
		Roles:   rolesSvc,
		Streams: streams.New(streams.Deps{Streams: st.Streams, Now: deps.Now, NewID: deps.NewID}),
		Workers: workers.New(workers.Deps{
			Workers: st.Workers, Roles: rolesSvc, Lines: st.ReportingLines, Topology: deps.Topology,
			Environments: st.Environments, Activations: st.Activations, Dispatcher: dispatcher,
			HireHook: deps.HireHook, EnvsDir: envsDir, Now: deps.Now, NewID: deps.NewID,
		}),
		Subscriptions: subscriptions.New(subscriptions.Deps{Subscriptions: st.Subscriptions, Streams: st.Streams, Workers: st.Workers, Now: deps.Now}),
		Publishing:    publishing.New(publishing.Deps{Streams: st.Streams, Events: st.Events, Hub: bc, Dispatcher: dispatcher, Now: deps.Now, NewID: deps.NewID}),
		Queries:       queries.New(queries.Deps{Roles: st.Roles, Workers: st.Workers, ReportingLines: st.ReportingLines, Streams: st.Streams, Subscriptions: st.Subscriptions, Events: st.Events}),
		Activations:   activations.New(activations.Deps{Repo: st.Activations, Now: deps.Now, NewID: deps.NewID}),
	}
}

func initHelixOrgHandler(cfg helixOrgConfig, helixStore helixstore.Store) (*helixOrgHandlers, error) {
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
	envsDir := filepath.Join(orgRoot, "envs")
	ownerEnvPath := filepath.Join(envsDir, "w-owner")
	if err := os.MkdirAll(ownerEnvPath, 0o750); err != nil {
		return nil, fmt.Errorf("create owner env %q: %w", ownerEnvPath, err)
	}

	// Open the org store against helix's Postgres connection. The
	// helixStore must expose a *gorm.DB accessor — there is no
	// dialect fallback any more.
	st, err := openOrgStore(helixStore)
	if err != nil {
		return nil, err
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Bootstrap is lazy: withHelixOrgScope calls
	// helixOrgScope.ensureBootstrap(ctx, orgID) on first request for
	// each org, materialising the owner Worker + structural grants
	// then. Bootstrap rows carry org_id and the FK to
	// organizations(id) reaps them on org delete.

	// Wake-only stream notifier. Backed by the host API server's
	// pubsub.PubSub (the canonical Helix NATS instance) — the
	// streamhub package is a thin facade preserving the typed
	// streaming.StreamID API the helix-org call sites used when this was the
	// in-process broadcast.Hub.
	bc := streamhub.New(cfg.APIServer.pubsub)
	deps := tools.DefaultDeps(st)
	deps.Hub = bc
	deps.EnvsDir = envsDir

	// Operational config registry — chat backend creds, model
	// selection, etc. Backed by the same Postgres rows so settings
	// survive restarts. Surfaced via the React settings page at
	// /orgs/:org_id/helix-org/settings (backed by
	// /api/v1/orgs/{org}/settings). Constructed before the spawner
	// so the spawner can read chat.app_id / helix.url at activation
	// time.
	configReg := configregistry.New(st.Configs)
	registerHelixOrgConfigSpecs(configReg)

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
	// Returns nil only if there's no admin user available to use as the
	// service identity — the alpha doesn't make sense without one, so
	// we treat that as "feature disabled until an admin registers".
	inProcClient := buildInProcHelixClient(context.Background(), cfg.APIServer, helixStore)
	if inProcClient == nil {
		log.Warn().Msg("helix-org disabled: in-proc adapter unavailable (no admin user found — register one before enabling the helix-org alpha)")
		return nil, nil
	}

	// Build the single Workspace shared by the WorkerProject (for
	// first-apply file pushes — agent.md / role.md / identity.md)
	// and update_role / update_identity (which call MirrorFile to
	// re-push canonical content on demand). One place owns the
	// on-branch path layout. Held in a local and injected into the
	// project applier below — no package global.
	var orgWorkspace *runtimehelix.Workspace
	if cfg.GitRepositoryService != nil {
		gitWriter := cfg.GitRepositoryService.(runtimehelix.WorkspaceGit)
		orgWorkspace = runtimehelix.NewWorkspace(gitWriter, st, "helix-specs", "helix-org", "helix-org@helix.local")
		deps.Workspace = orgWorkspace
	}

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
		workspace:  orgWorkspace,
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
	// (see helix_org_github.go). Drives the github stream transport's
	// outbound `Token()` lookup; the worker-side mint path now flows
	// through the mint_credential MCP tool + CredentialProvider, not a
	// boot-time SecretInjector.
	oauthResolver := newGitHubOAuthResolver(cfg.APIServer.oauthManager, helixStore)
	// identityResolver prefers the installed Helix App bot over a borrowed
	// member OAuth token: if the org has a github_app ServiceConnection it
	// mints a short-lived installation token (decrypting the stored PEM with
	// the server encryption key), else it falls back to oauthResolver.
	// github.MintInstallationCredential is the production minter — it
	// returns both the token and the server-reported expiry, which
	// mint_credential surfaces to agents.
	identityResolver := newOrgGitHubIdentityResolver(
		cfg.APIServer.getEncryptionKey,
		helixStore,
		oauthResolver,
		func(ctx context.Context, appID, installationID int64, pem, baseURL string) (MintedInstallation, error) {
			cred, err := githubskill.MintInstallationCredential(ctx, appID, installationID, pem, baseURL)
			if err != nil {
				return MintedInstallation{}, err
			}
			return MintedInstallation{Token: cred.Token, ExpiresAt: cred.ExpiresAt}, nil
		},
	)
	// gitHubTokenResolver is the bot-preferring token projection used by
	// the outbound github stream transport and the webhook-install code
	// path. Returns the App installation token when one exists, else the
	// legacy member OAuth token — so once an org installs the Helix App,
	// its agents act as the bot rather than a human. (Worker shell-tool
	// credentials no longer flow through this projection; they go through
	// the per-org CredentialProvider wired into mint_credential below.)
	gitHubTokenResolver := func(ctx context.Context, orgID string) (string, error) {
		id, err := identityResolver(ctx, orgID)
		if err != nil {
			return "", err
		}
		return id.Token, nil
	}

	// credentialProviders backs the mint_credential MCP tool — the
	// single surface every Worker uses to obtain an org-scoped
	// external-provider credential on demand. Adding a new provider
	// (Slack, …) is a new file under
	// infrastructure/transports/<name>/credential_provider.go plus
	// one entry here — no edits to mint_credential.go.
	deps.CredentialProviders = map[string]credential.Provider{
		"github": githubtransport.NewCredentialProvider(
			func(ctx context.Context, orgID string) (githubtransport.Identity, error) {
				id, err := identityResolver(ctx, orgID)
				if err != nil {
					return githubtransport.Identity{}, err
				}
				return githubtransport.Identity{Token: id.Token, ExpiresAt: id.ExpiresAt}, nil
			},
		),
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
	deps.Dispatcher = dispatcher

	// streamCron drives KindCron streams. Same call sequence as the
	// publish MCP tool — Events.Append → Hub.Notify → Dispatcher.Dispatch
	// — so cron-driven activations look identical to publish-driven
	// activations downstream. Started in a goroutine from
	// registerRoutes once we have the long-lived ctx.
	streamCronScheduler, err := streamcron.New(st, bc, dispatcher, deps.NewID, deps.Now)
	if err != nil {
		return nil, fmt.Errorf("init streamcron scheduler: %w", err)
	}

	reg := tools.NewRegistry()
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		return nil, fmt.Errorf("register helix-org builtins: %w", err)
	}

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

	orgServer := helixorgserver.NewFromStore(st, reg, bc, dispatcher, logger).WithPrompts(promptReg)

	// JSON handlers consumed by the React pages at
	// /orgs/:org_id/helix-org/*. They mount under
	// /api/v1/orgs/{org}/ via the orgServer's extras list. REST hire and
	// chat-driven hire both call the same workers.Hire service (wired
	// into apiDeps.Workers below) — one implementation, no drift.

	// Fire (DELETE /workers/{id}) cascades Helix-side teardown
	// (project + agent app) plus full org-store cleanup. The Helix
	// runtime port is satisfied by the same in-process adapter every
	// other Helix call goes through.
	lifecycleSvc := &lifecycle.Service{
		Store:   st,
		Helix:   inProcClient,
		Logger:  logger,
		EnvsDir: envsDir,
		Owner:   "w-owner",
		// Single topology reconciler shared with the tools registry and
		// the REST handlers — one owner of activation/team Stream
		// lifecycle across hire, reparent, and fire.
		Topology: deps.Topology,
		Mirror:   mirror, // Fire stops the fired worker's subscription
	}

	// GitHub-App integration (install-status gate + repo picker) — owned
	// by the gitHubIntegration adapter rather than inline closures here.
	gitHubInt := newGitHubIntegration(helixStore, cfg.APIServer.getEncryptionKey, cfg.APIServer.Cfg.GitHub.AppSlug, cfg.APIServer.Cfg.GitHub.WebURL())

	// Application services shared by the REST adapter. Built once here
	// (the composition root) from the store + collaborators; the api
	// package holds these services, never the store (Phase-D seam).
	svc := buildOrgServices(st, deps, bc, dispatcher, envsDir)
	apiDeps := helixorgapi.Deps{
		Streams:       svc.Streams,
		Roles:         svc.Roles,
		Workers:       svc.Workers,
		Subscriptions: svc.Subscriptions,
		Publishing:    svc.Publishing,
		Queries:       svc.Queries,
		Activations:   svc.Activations,
		WorkerRuntime: orgWorkerRuntime{st: st},
		// GitHubInbound builds the inbound github transport per org — it
		// reads matching streams + appends events, so it holds the store
		// here in the composition root rather than in the api adapter.
		GitHubInbound: func(orgID string) http.Handler {
			t := githubtransport.New(orgID, configReg, st, bc, dispatcher, logger)
			if gitHubTokenResolver != nil {
				t = t.WithTokenResolver(githubtransport.TokenResolver(gitHubTokenResolver))
			}
			return t.HandleInbound()
		},
		Configs:        configReg,
		Hub:            bc,
		Dispatcher:     dispatcher,
		Owner:          "w-owner",
		DBPath:         orgRoot,
		EnvsDir:        envsDir,
		Lifecycle:      lifecycleSvc,
		Tools:          reg,
		ProjectEnsurer: projectApplier,
		// Production: the github stream transport's Token() falls
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
		// GitHubInstallation backs the New Stream "Install Helix" gate;
		// GitHubAppRepos backs the repo picker. Both are owned by the
		// gitHubIntegration adapter (helix_org_github.go) — this
		// composition root just constructs it and passes method values.
		GitHubInstallation: gitHubInt.InstallationStatus,
		GitHubAppRepos:     gitHubInt.AppRepos,
		// GitHubManifestStart builds the "create the Helix app" manifest flow.
		GitHubManifestStart: newGitHubManifestStart(cfg.APIServer.getEncryptionKey, cfg.APIServer.Cfg.GitHub.WebURL()),
		// PublicServerURL is the externally-reachable base URL the
		// auto-installed GitHub webhook should POST back to. Helix's
		// SERVER_URL env var is the canonical place it lives.
		PublicServerURL: cfg.APIServer.Cfg.WebServer.URL,
	}
	apiRoutes := helixorgapi.Routes(apiDeps)
	extras := make([]helixorgserver.Route, 0, len(apiRoutes))
	for _, rt := range apiRoutes {
		extras = append(extras, helixorgserver.Route{Pattern: rt.Pattern, Handler: rt.Handler})
	}

	log.Info().
		Str("root", orgRoot).
		Str("envs", envsDir).
		Int("json_api_routes", len(extras)).
		Msg("helix-org mounted at /api/v1/orgs/{org}/helix-org/")
	scope := newHelixOrgScope(configReg, st, envsDir, helixStore, mirror)

	// Public github webhook handler — mounted on the insecure router
	// because GitHub deliveries authenticate via HMAC, not the helix
	// session/api-key layer. Per-request: resolve {org} from mux
	// vars → orgID → build the github.Transport → dispatch.
	ghLogger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	// Reuse the bot-preferring projection so the public webhook's outbound
	// actions act as the installed App when there is one.
	tokenResolver := gitHubTokenResolver
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
		t := githubtransport.New(org.ID, configReg, st, bc, dispatcher, ghLogger)
		if tokenResolver != nil {
			t = t.WithTokenResolver(githubtransport.TokenResolver(tokenResolver))
		}
		t.HandleInbound().ServeHTTP(w, r)
	})

	// Per-stream public github webhook handler. Same auth model as
	// the org-level handler (HMAC over body); routes deliveries to
	// the single stream named in the path so operators can hand
	// GitHub a stream-specific URL.
	publicGitHubWebhookForStream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		orgSlugOrID := vars["org"]
		streamID := vars["stream_id"]
		if orgSlugOrID == "" {
			http.Error(w, "missing org", http.StatusBadRequest)
			return
		}
		if streamID == "" {
			http.Error(w, "missing stream_id", http.StatusBadRequest)
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
		t := githubtransport.New(org.ID, configReg, st, bc, dispatcher, ghLogger)
		if tokenResolver != nil {
			t = t.WithTokenResolver(githubtransport.TokenResolver(tokenResolver))
		}
		t.HandleInboundForStream(streaming.StreamID(streamID)).ServeHTTP(w, r)
	})

	// GitHub App Manifest flow callbacks. Insecure mounts (top-level
	// navigations from github.com): the conversion callback is authenticated
	// by the encrypted ?state=; the setup callback only records a non-secret
	// installation id onto the org's app.
	publicGitHubManifestCallback := newGitHubManifestCallbackHandler(
		cfg.APIServer.getEncryptionKey, helixStore, deps.NewID,
		cfg.APIServer.Cfg.GitHub.WebURL(), cfg.APIServer.Cfg.GitHub.APIBaseURL(),
	)

	return &helixOrgHandlers{
		api:                          orgServer.Handler(extras...),
		scope:                        scope,
		streamCron:                   streamCronScheduler,
		publicGitHubWebhook:          publicGitHubWebhook,
		publicGitHubWebhookForStream: publicGitHubWebhookForStream,
		publicGitHubManifestCallback: publicGitHubManifestCallback,
	}, nil
}

// helixOrgConfig is enough of the surrounding config to bring up the
// embedded org. LocalFSPath roots the per-Worker working-directory
// tree (falls back to os.TempDir() when empty). APIServer=nil
// disables helix-org entirely.
type helixOrgConfig struct {
	LocalFSPath          string
	GitRepositoryService runtimehelix.WorkspaceGit
	APIServer            *HelixAPIServer
}

// dynamicProjectApplier is a chat.ProjectEnsurer that re-reads
// `worker.*` and `helix.*` from the config registry on every Ensure
// call. Building the underlying runtimehelix.WorkerProject at API
// startup and reusing it freezes `worker.runtime`/`credentials`/
// `provider`/`model` at boot time — operators changing those via
// the settings page then had to restart the API container for the new
// values to take effect. Resolving per-call removes that surprise.
//
// Store is exposed directly because helix_org_chat.go needs it to
// load/save the per-Worker session pointer on the same row the
// spawner uses (helix-org's WorkerRuntimeState).
type dynamicProjectApplier struct {
	cfg        *configregistry.Registry
	projectSvc runtimehelix.ProjectService
	Store      *helixorgstore.Store
	// workspace is the single on-branch Workspace shared with the
	// update_role/update_identity tools. Injected here (rather than read
	// from a package global) so initialisation order is explicit. nil is
	// allowed — the applier just builds WorkerProjects without a mirror
	// (the in-memory / no-git wirings).
	workspace *runtimehelix.Workspace
	logger    *slog.Logger
}

// Ensure satisfies chat.ProjectEnsurer. Builds a fresh
// runtimehelix.WorkerProject from the current registry state and
// delegates. WorkerProject.Ensure is itself idempotent — first call
// applies, subsequent calls fast-path on the existing project.
//
// After Ensure succeeds, re-attaches the helix-org MCP entry on the
// per-Worker agent app. ApplyProject (called inside WorkerProject.Ensure)
// wholesale-replaces Config.Helix on update, so any MCPs we attached
// previously are wiped — we re-attach here to keep the MCP present.
// The Spawner does the same on its own activations; owner-chat goes
// through this path only.
func (d *dynamicProjectApplier) Ensure(ctx context.Context, orgID string, workerID orgchart.WorkerID) (projectID, agentAppID, repoID string, err error) {
	applier, mcpBearer, err := buildHelixOrgProjectApplier(ctx, orgID, d.cfg, d.projectSvc, d.Store, d.workspace, d.logger)
	if err != nil {
		return "", "", "", err
	}
	projectID, agentAppID, repoID, err = applier.Ensure(ctx, orgID, workerID)
	if err != nil {
		return "", "", "", err
	}
	if agentAppID != "" && applier.HelixOrgURL != "" {
		if attachErr := runtimehelix.AttachHelixOrgMCP(ctx, d.projectSvc, agentAppID, applier.HelixOrgURL, workerID, mcpBearer); attachErr != nil && d.logger != nil {
			d.logger.Warn("dynamic project applier: attach helix-org MCP", "worker", workerID, "app", agentAppID, "err", attachErr)
		}
	}
	return projectID, agentAppID, repoID, nil
}

// buildHelixOrgProjectApplier constructs the WorkerProject that
// both the chat bridge (owner-chat) and the spawner (AI Worker
// activations) drive. Single source of truth for the embedded
// SaaS's "Worker defaults" — `worker.runtime` from the config
// registry (default `claude_code`), subscription credentials, and
// our MCP-gateway URL so each Worker's agent app phones home for
// helix-org tools via Helix's auth-gated proxy rather than a
// separate tunnel.
//
// Called per Ensure by dynamicProjectApplier so registry edits
// (worker.runtime/credentials/provider/model, helix.url/api_key)
// take effect immediately. The struct it returns is cheap to build
// and short-lived — one apply call, then discarded.
//
// Also returns the service api_key as a separate value (mcpBearer)
// for the caller to feed into runtimehelix.AttachHelixOrgMCP as the
// fallback bearer when no per-request bearer is on ctx. The bearer
// is no longer carried on WorkerProject because Ensure doesn't touch
// MCPs — MCP attachment is a separate, explicit step.
func buildHelixOrgProjectApplier(
	ctx context.Context,
	orgID string,
	cfg *configregistry.Registry,
	projectSvc runtimehelix.ProjectService,
	orgStore *helixorgstore.Store,
	workspace *runtimehelix.Workspace,
	logger *slog.Logger,
) (*runtimehelix.WorkerProject, string, error) {
	apiKey, _ := cfg.GetString(ctx, orgID, "helix.api_key")
	if apiKey == "" {
		return nil, "", fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := cfg.GetString(ctx, orgID, "helix.url")
	if err != nil {
		return nil, "", fmt.Errorf("read helix.url: %w", err)
	}
	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, cfg)
	// HelixOrgMCPBackend.ServeHTTP parses `<org>/workers/<id>/mcp`
	// from the suffix path, so the org segment is required in the
	// URL Zed will dial. The previous form
	// `/api/v1/mcp/helix-org/workers/<id>/mcp` made the backend read
	// "workers" as the org slug and 404 every request — the helix-org
	// MCP was effectively unreachable from inside the sandbox.
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org/" + orgID
	return &runtimehelix.WorkerProject{
		Service:     projectSvc,
		Workspace:   workspace,
		Store:       orgStore,
		HelixOrgURL: helixOrgURL,
		OrgID:       orgID,
		Runtime:     runtime,
		Credentials: credentials,
		Provider:    provider,
		Model:       model,
		Logger:      logger,
	}, apiKey, nil
}

// resolveWorkerAgentConfig reads the four `worker.*` knobs and normalises
// them into the (runtime, credentials, provider, model) tuple that
// matches Helix's per-agent UI:
//
//   - claude_code + subscription → no provider/model (CLI authenticates via OAuth)
//   - claude_code + api_key       → provider+model required, inference via Helix's anthropic provider
//   - zed_agent (or other)        → provider+model required, always Helix-routed (credentials forced to "api_key")
//
// We coerce silly combinations (e.g. zed_agent + subscription) to the
// only mode that actually works for that runtime, mirroring Helix's
// per-agent validator.
func resolveWorkerAgentConfig(ctx context.Context, orgID string, cfg *configregistry.Registry) (runtime, credentials, provider, model string) {
	runtime, _ = cfg.GetString(ctx, orgID, "worker.runtime")
	if runtime == "" {
		runtime = "claude_code"
	}
	credentials, _ = cfg.GetString(ctx, orgID, "worker.credentials")
	if credentials == "" {
		credentials = "subscription"
	}
	if runtime != "claude_code" {
		credentials = "api_key" // subscription is only meaningful for claude_code
	}
	if credentials == "api_key" {
		provider, _ = cfg.GetString(ctx, orgID, "worker.provider")
		model, _ = cfg.GetString(ctx, orgID, "worker.model")
	}
	return runtime, credentials, provider, model
}

// buildInProcHelixClient resolves the service-account *types.User and
// builds the in-process adapter that satisfies
// runtimehelix.ProjectService, runtimehelix.SpawnerClient, and
// chat.ChatBridgeClient. Returns nil when no admin user is available
// to attribute the service identity to — callers treat that as
// "feature disabled" (no production wiring without one).
//
// The service user mirrors ensureHelixOrgServiceAPIKey's owner-pick
// (first admin user) so the auto-provisioned api_key and the
// in-process adapter are attributed to the same identity.
func buildInProcHelixClient(ctx context.Context, apiServer *HelixAPIServer, helixStore helixstore.Store) *inProcHelixClient {
	admins, _, err := helixStore.ListUsers(ctx, &helixstore.ListUsersQuery{Admin: true})
	if err != nil {
		log.Warn().Err(err).Msg("helix-org in-proc adapter disabled — list admins failed")
		return nil
	}
	if len(admins) == 0 {
		log.Warn().Msg("helix-org in-proc adapter disabled — no admin user found (matches ensureHelixOrgServiceAPIKey's failure mode)")
		return nil
	}
	owner := admins[0]
	log.Info().
		Str("owner_id", owner.ID).
		Str("owner_email", owner.Email).
		Msg("helix-org in-proc adapter wired (ProjectService + SpawnerClient + ChatBridgeClient)")
	return NewInProcHelixClient(apiServer, owner)
}

// buildHelixOrgSpawnerConfig assembles the SpawnerConfig for
// helix-org's production zed_external Spawner. The embedded SaaS
// runs Workers on the `claude_code` runtime with subscription
// credentials — the in-sandbox Claude Code CLI authenticates
// Anthropic via the operator's own OAuth, so we don't pass
// Provider/Model and the Helix-side anthropic proxy doesn't need an
// API key configured. HelixOrgURL points at our embedded MCP gateway
// so the Zed sandbox can reach helix-org without external tunneling;
// the service api_key is forwarded as the MCP Authorization header
// so the gateway's alpha-feature check passes.
//
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
	Hub        *streamhub.Hub
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
	apiKey, _ := d.Cfg.GetString(ctx, orgID, "helix.api_key")
	if apiKey == "" {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := d.Cfg.GetString(ctx, orgID, "helix.url")
	if err != nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("read helix.url: %w", err)
	}

	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, d.Cfg)
	// HelixOrgMCPBackend.ServeHTTP parses `<org>/workers/<id>/mcp`
	// from the suffix path, so the org segment is required in the
	// URL Zed will dial.
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org/" + orgID
	specsMandate, _ := d.Cfg.GetString(ctx, orgID, "worker.specs_mandate")
	return runtimehelix.SpawnerConfig{
		Client:         d.SpawnerClient,
		ProjectService: d.ProjectSvc,
		HelixOrgURL:    helixOrgURL,
		OrgID:          orgID,
		Runtime:        runtime,
		Credentials:    credentials,
		Provider:       provider,
		Model:          model,
		MCPAuthBearer:  apiKey,
		SpecsMandate:   specsMandate,
		Store:          d.OrgStore,
		Hub:            d.Hub,
		PubSub:         d.PubSub,
		Snapshotter:    runtimehelix.NoopSessionPreamble{},
		Logger:         d.Logger,
		NewID:          d.NewID,
		Now:            d.Now,
		BearerForUser: func(ctx context.Context, userID string) (string, error) {
			return newHelixAPIKeys(d.HelixStore, d.Cfg).User(ctx, userID)
		},
	}, nil
}

// lazyHelixOrgSpawner returns a runtime.Spawner that builds a fresh
// SpawnerConfig, scoped to the activating org, on every activation.
//
// It MUST NOT cache a single inner Spawner across orgs. SpawnerConfig
// carries tenant-specific identity — OrgID and HelixOrgURL
// (`/api/v1/mcp/helix-org/<orgID>`) — which the inner spawner stamps
// onto every Worker's project (applyReq.OrganizationID, the
// HELIX_ORG_URL project secret) and, critically, onto the helix-org
// MCP entry it re-attaches to the Worker's agent app on every
// activation. A cached spawner freezes the *first* activating org's
// identity and replays it for every other org, so org B's owner ends
// up with an MCP pointing at org A's gateway — and create_role /
// hire_worker land in org A. (Root cause of the cross-tenant leak; see
// design/2026-06-09-org-multitenancy-spawner-leak.md.)
//
// Building per activation is cheap (a handful of config-registry
// reads) and also keeps worker.runtime/credentials/provider/model
// current without any "drift" handling. The one thing the old cache
// legitimately provided — a single process-wide inflight cap — is
// preserved by minting one shared semaphore here and injecting it into
// each per-activation config via SpawnerConfig.Sem.
//
// The dynamic applier still runs first: it provisions (or fast-paths)
// the per-Worker project and attaches the MCP for owner-chat's benefit.
// The inner spawner re-attaches the MCP after its own ensureProject
// (ApplyProject wipes Config.Helix), so both must use the correct
// per-org URL — which they now do.
func lazyHelixOrgSpawner(d spawnerDeps) runtime.Spawner {
	// One inflight cap shared across every per-org spawner config.
	sem := make(chan struct{}, runtimehelix.DefaultMaxInflight)
	return func(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, triggers []activation.Trigger) error {
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
			Str("helix_org_url", cfgVal.HelixOrgURL).
			Str("runtime", cfgVal.Runtime).
			Str("credentials", cfgVal.Credentials).
			Msg("helix-org spawner: per-org activation")
		return runtimehelix.Spawner(cfgVal)(ctx, orgID, workerID, envPath, triggers)
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
	// (workers, roles, streams, runtime state, …) survive an API
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
