package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/application/dispatch"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/application/streamhub"
	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	helixorgserver "github.com/helixml/helix/api/pkg/org/interfaces/server"
	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
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
	// on-branch path layout.
	if cfg.GitRepositoryService != nil {
		gitWriter := cfg.GitRepositoryService.(runtimehelix.WorkspaceGit)
		helixOrgWorkspaceRef = runtimehelix.NewWorkspace(gitWriter, st, "helix-specs", "helix-org", "helix-org@helix.local")
		deps.Workspace = helixOrgWorkspaceRef
	}

	// Wire the helix-runtime HireHook so hire_worker persists the
	// hiring user's identifier onto the new Worker's runtime state.
	// Replaces the direct runtimehelix.SaveHiringUser call hire_worker
	// used to make.
	deps.HireHook = &runtimehelix.Hire{Store: st}

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
	spawnerFn := lazyHelixOrgSpawner(configReg, helixStore, inProcClient, inProcClient, st, bc, cfg.APIServer.pubsub, logger, projectApplier, deps.NewID, deps.Now)
	dispatcher := dispatch.New(st, spawnerFn, logger)
	deps.Dispatcher = dispatcher

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

	orgServer := helixorgserver.New(st, reg, bc, dispatcher, logger).WithPrompts(promptReg)

	// JSON handlers consumed by the React pages at
	// /orgs/:org_id/helix-org/*. They mount under
	// /api/v1/orgs/{org}/ via the orgServer's extras list. REST hire
	// shares the exact tool the MCP registry exposes — same Deps,
	// same Invocation shape, so chat-driven and chart-driven hires
	// can't drift.
	hireTool := tools.NewHireWorker(deps)

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
	}

	apiDeps := helixorgapi.Deps{
		Store:          st,
		Configs:        configReg,
		Hub:            bc,
		Dispatcher:     dispatcher,
		Owner:          "w-owner",
		DBPath:         orgRoot,
		EnvsDir:        envsDir,
		HireWorker:     hireTool,
		Lifecycle:      lifecycleSvc,
		Tools:          reg,
		ProjectEnsurer: projectApplier,
		NewID:          deps.NewID,
		Now:            deps.Now,
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
	scope := newHelixOrgScope(configReg, st, envsDir, helixStore)
	return &helixOrgHandlers{api: orgServer.Handler(extras...), scope: scope}, nil
}

// helixOrgConfig is just enough of the surrounding config to bring
// up the embedded org. LocalFSPath is optional after H4.4 — the
// org store goes through helix's Postgres connection; LocalFSPath
// is only used to root the working-directory tree, falling back to
// os.TempDir() when empty.
type helixOrgConfig struct {
	LocalFSPath string
	// GitRepositoryService is the production git-write surface
	// helix-org's Workspace uses to mirror role.md / identity.md /
	// canonical files onto each Worker's per-Worker repo. The H1.1
	// lift replaced the loopback-HTTP file push with this direct
	// call into the same servicer the HTTP handlers use.
	GitRepositoryService runtimehelix.WorkspaceGit
	// APIServer is the live HelixAPIServer instance. The embedded
	// helix-org module needs it to build the in-process adapter that
	// satisfies runtimehelix.ProjectService + SpawnerClient +
	// chat.ChatBridgeClient. Nil disables helix-org entirely (init
	// returns nil, nil).
	APIServer *HelixAPIServer
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
	logger     *slog.Logger
}

// Ensure satisfies chat.ProjectEnsurer. Builds a fresh
// runtimehelix.WorkerProject from the current registry state and
// delegates. WorkerProject.Ensure is itself idempotent — first call
// applies, subsequent calls fast-path on the existing project.
func (d *dynamicProjectApplier) Ensure(ctx context.Context, orgID string, workerID orgchart.WorkerID) (projectID, agentAppID, repoID string, err error) {
	applier, err := buildHelixOrgProjectApplier(ctx, orgID, d.cfg, d.projectSvc, d.Store, d.logger)
	if err != nil {
		return "", "", "", err
	}
	return applier.Ensure(ctx, orgID, workerID)
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
func buildHelixOrgProjectApplier(
	ctx context.Context,
	orgID string,
	cfg *configregistry.Registry,
	projectSvc runtimehelix.ProjectService,
	orgStore *helixorgstore.Store,
	logger *slog.Logger,
) (*runtimehelix.WorkerProject, error) {
	apiKey, _ := cfg.GetString(ctx, orgID, "helix.api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := cfg.GetString(ctx, orgID, "helix.url")
	if err != nil {
		return nil, fmt.Errorf("read helix.url: %w", err)
	}
	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, cfg)
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org"
	return &runtimehelix.WorkerProject{
		Service:       projectSvc,
		Workspace:     helixOrgWorkspaceRef,
		Store:         orgStore,
		HelixOrgURL:   helixOrgURL,
		OrgID:         orgID,
		Runtime:       runtime,
		Credentials:   credentials,
		Provider:      provider,
		Model:         model,
		MCPAuthBearer: apiKey,
		Logger:        logger,
	}, nil
}

// helixOrgWorkspaceRef is the production Workspace, set at
// initHelixOrgHandler time. buildHelixOrgProjectApplier picks it up
// because it has no access to the helixOrgConfig directly. The same
// Workspace also drives update_role / update_identity tools (the
// only public WorkspaceSync surface).
var helixOrgWorkspaceRef *runtimehelix.Workspace

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
func buildHelixOrgSpawnerConfig(
	ctx context.Context,
	orgID string,
	cfg *configregistry.Registry,
	helixStore helixstore.Store,
	spawnerClient runtimehelix.SpawnerClient,
	// projectSvc lets the spawner's *internal* ensureProject pass
	// (spawner.go:200 fast-path) verify the Helix project exists
	// without a nil-deref. Forgetting it caused the chart UI to crash
	// the API on any AI-worker click; we now require a non-nil value
	// up front rather than letting the closure discover the missing
	// dependency mid-activation.
	projectSvc runtimehelix.ProjectService,
	orgStore *helixorgstore.Store,
	bc *streamhub.Hub,
	// ps is the host API's NATS pubsub. The spawner's per-activation
	// bridge calls SubscribeSessionUpdates on it to stream the helix
	// session's events into the org-graph transcript. Without it (the
	// bug behind the segfault that took the whole API process down on
	// every AI activation), the bridge panicked at sessions.go:257.
	ps pubsub.PubSub,
	logger *slog.Logger,
	newID func() string,
	now func() time.Time,
) (runtimehelix.SpawnerConfig, error) {
	if ps == nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix-org spawner: PubSub is required")
	}
	if projectSvc == nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix-org spawner: ProjectService is required")
	}
	apiKey, _ := cfg.GetString(ctx, orgID, "helix.api_key")
	if apiKey == "" {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := cfg.GetString(ctx, orgID, "helix.url")
	if err != nil {
		return runtimehelix.SpawnerConfig{}, fmt.Errorf("read helix.url: %w", err)
	}

	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, orgID, cfg)
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org"
	specsMandate, _ := cfg.GetString(ctx, orgID, "worker.specs_mandate")
	return runtimehelix.SpawnerConfig{
		Client:         spawnerClient,
		ProjectService: projectSvc,
		HelixOrgURL:    helixOrgURL,
		OrgID:          orgID,
		Runtime:        runtime,
		Credentials:    credentials,
		Provider:       provider,
		Model:          model,
		MCPAuthBearer:  apiKey,
		SpecsMandate:   specsMandate,
		Store:          orgStore,
		Hub:            bc,
		// PubSub is the helix-host NATS pubsub; Snapshotter is the
		// noop preamble — helix-org spawned sessions originate inside
		// the spawner so there is no separately-tracked browser-WS
		// snapshot to replay. Subscriber sees live frames only, which
		// is correct for the bridge.
		PubSub:      ps,
		Snapshotter: runtimehelix.NoopSessionPreamble{},
		Logger:      logger,
		NewID:       newID,
		Now:         now,
		BearerForUser: func(ctx context.Context, userID string) (string, error) {
			return resolveUserHelixAPIKey(ctx, helixStore, userID)
		},
	}, nil
}

// lazyHelixOrgSpawner returns an runtime.Spawner that defers building
// the underlying SpawnerConfig (and the wrapped helix.Spawner closure)
// until the first activation arrives. Subsequent activations reuse
// the same built Spawner — semaphore + MaxInflight live on the
// inner closure, so they're shared across calls.
//
// Re-reads SpawnerConfig only if the first attempt failed; this lets
// "pick an agent" flow seamlessly after API boot without restart.
//
// Worker.* drift handling: once the inner Spawner is built, its
// captured SpawnerConfig.Runtime/Provider/Model/Credentials are frozen
// for the life of the process. Those fields are only consumed inside
// the spawner's own ensureProject call, so we run the dynamic applier
// first — it re-reads worker.* on every activation and materialises
// (or fast-paths) the per-Worker project with current settings. The
// spawner's internal ensureProject then fast-paths against the project
// our wrapper just touched, and the frozen fields are dead weight.
// Net effect: changing worker.runtime / credentials / provider / model
// via the settings page takes effect on the next activation, without
// disturbing the shared MaxInflight semaphore inside the cached
// spawner.
func lazyHelixOrgSpawner(
	cfg *configregistry.Registry,
	helixStore helixstore.Store,
	spawnerClient runtimehelix.SpawnerClient,
	projectSvc runtimehelix.ProjectService,
	orgStore *helixorgstore.Store,
	bc *streamhub.Hub,
	ps pubsub.PubSub,
	logger *slog.Logger,
	applier *dynamicProjectApplier,
	newID func() string,
	now func() time.Time,
) runtime.Spawner {
	var (
		mu      sync.Mutex
		spawner runtime.Spawner
	)
	return func(ctx context.Context, orgID string, workerID orgchart.WorkerID, envPath string, triggers []activation.Trigger) error {
		// Apply (or fast-path) the per-Worker project with the current
		// worker.* settings before delegating. Without this, the cached
		// spawner's first activation bakes whatever worker.* values
		// were live at boot into the project; later edits via the
		// settings page never propagate.
		if applier != nil {
			if _, _, _, err := applier.Ensure(ctx, orgID, workerID); err != nil {
				return fmt.Errorf("helix-org spawner: pre-apply project for %s: %w", workerID, err)
			}
		}
		mu.Lock()
		current := spawner
		mu.Unlock()
		if current == nil {
			cfgVal, err := buildHelixOrgSpawnerConfig(ctx, orgID, cfg, helixStore, spawnerClient, projectSvc, orgStore, bc, ps, logger, newID, now)
			if err != nil {
				return fmt.Errorf("helix-org spawner not configured: %w", err)
			}
			built := runtimehelix.Spawner(cfgVal)
			mu.Lock()
			if spawner == nil {
				spawner = built
			}
			current = spawner
			mu.Unlock()
			log.Info().
				Str("helix_org_url", cfgVal.HelixOrgURL).
				Str("runtime", cfgVal.Runtime).
				Str("credentials", cfgVal.Credentials).
				Msg("helix-org spawner built (zed_external workers)")
		}
		return current(ctx, orgID, workerID, envPath, triggers)
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
	// ResetSchema is OFF so org_* rows (workers, roles, positions,
	// grants, streams, runtime state, …) survive an API restart.
	// While the helix-org alpha was wiping data each boot was tolerable
	// — operators were now actually relying on hired workers persisting,
	// and dropping the tables on restart was indistinguishable from
	// "we're using some in-memory thing". The composite-PK schema
	// (id, org_id) is the only shape in production; AutoMigrate is
	// idempotent against it. If a hand-written breaking migration ever
	// becomes necessary, prefer an explicit migration script over
	// flipping this back on.
	st, err := orggorm.OpenWithDB(accessor.GormDB(), orggorm.Options{
		ResetSchema:           false,
		InstallOrganizationFK: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open helix-org gorm: %w", err)
	}
	return st, nil
}
