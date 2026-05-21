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
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/agent"
	agenthelix "github.com/helixml/helix-org/agent/helix"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/prompts"
	"github.com/helixml/helix-org/server/chat"
	"github.com/helixml/helix-org/bootstrap"
	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/dispatch"
	"github.com/helixml/helix-org/helix/helixclient"
	helixorgserver "github.com/helixml/helix-org/server"
	helixorgstore "github.com/helixml/helix-org/store"
	helixorgui "github.com/helixml/helix-org/server/ui"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"

	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// helixOrgHandlers bundles the two HTTP surfaces helix-org exposes:
// the JSON-RPC MCP / webhook endpoints (mounted under /api/v1/org/)
// and the htmx-driven UI (mounted at the top-level /ui/ because its
// templates use absolute /ui/... hrefs).
type helixOrgHandlers struct {
	api http.Handler
	ui  http.Handler
}

// alphaFeatureHelixOrg is the alpha-feature flag that gates the
// embedded helix-org surface. Granted per-user via:
//
//	UPDATE users SET alpha_features = array_append(alpha_features, 'helix-org')
//	WHERE email = '...';
const alphaFeatureHelixOrg = "helix-org"

// initHelixOrgHandler builds the in-process helix-org HTTP handler;
// mounted at /api/v1/org/, gated per-user by the `helix-org` alpha
// feature flag. The SQLite store lives at
// $FILESTORE_LOCALFS_PATH/helix-org/helix-org.db so the file survives
// container redeploys on the persistent volume.
//
// If the deployment is not configured with `FILESTORE_TYPE=fs` there
// is no local path to put the SQLite file in — the handler is not
// mounted, so self-hosted installs running gcs (etc.) never see the
// feature. SaaS currently runs fs against a persistent volume.
//
// Every gated user currently shares one owner Worker — see the design
// doc (design/2026-05-17-helix-org-saas-alpha.md) for the multi-tenant
// follow-up.
// Returns nil (and logs) if the embedded org cannot be initialised for
// this deployment — callers must treat that as "don't mount".
func initHelixOrgHandler(cfg helixOrgConfig, helixStore helixstore.Store) (*helixOrgHandlers, error) {
	if cfg.FileStoreType != types.FileStoreTypeLocalFS {
		// helix-org needs a real local path for its SQLite file. SaaS
		// runs fs against a persistent volume; deployments that don't
		// (e.g. gcs) just won't see the feature. Returning nil is the
		// signal to skip the mount.
		return nil, nil
	}
	if cfg.LocalFSPath == "" {
		return nil, fmt.Errorf("FILESTORE_LOCALFS_PATH is empty")
	}

	orgRoot := filepath.Join(cfg.LocalFSPath, "helix-org")
	if err := os.MkdirAll(orgRoot, 0o750); err != nil {
		return nil, fmt.Errorf("create helix-org dir %q: %w", orgRoot, err)
	}
	dbPath := filepath.Join(orgRoot, "helix-org.db")
	envsDir := filepath.Join(orgRoot, "envs")
	ownerEnvPath := filepath.Join(envsDir, "w-owner")
	if err := os.MkdirAll(ownerEnvPath, 0o750); err != nil {
		return nil, fmt.Errorf("create owner env %q: %w", ownerEnvPath, err)
	}

	st, err := sqlite.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open helix-org sqlite: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	switch result, err := bootstrap.Run(context.Background(), st, bootstrap.Params{EnvironmentPath: ownerEnvPath}); {
	case err == nil:
		log.Info().
			Str("worker_id", string(result.WorkerID)).
			Str("role_id", string(result.RoleID)).
			Str("position_id", string(result.PositionID)).
			Str("env_path", result.EnvironmentPath).
			Msg("helix-org bootstrap created owner")
	case errors.Is(err, bootstrap.ErrAlreadyInitialised):
		log.Info().Str("db", dbPath).Msg("helix-org bootstrap skipped: already initialised")
	default:
		return nil, fmt.Errorf("helix-org bootstrap: %w", err)
	}

	bc := broadcast.New()
	deps := tools.DefaultDeps(st)
	deps.Broadcaster = bc
	deps.EnvsDir = envsDir

	// Operational config registry — chat backend creds, model
	// selection, etc. Backed by the same SQLite store so settings
	// survive restarts. Surfaced via helix-org's /ui/settings page.
	// Constructed before the spawner so the spawner can read
	// chat.app_id / helix.url at activation time.
	configReg := config.New(st.Configs)
	registerHelixOrgConfigSpecs(configReg)

	// Auto-provision a Helix API key for the embedded helix-org's
	// loopback HTTP client BEFORE building the spawner — the spawner
	// re-uses this client to provision per-Worker clone apps and to
	// open activation chat sessions.
	if _, err := ensureHelixOrgServiceAPIKey(context.Background(), helixStore, configReg); err != nil {
		log.Warn().Err(err).Msg("helix-org service api key not provisioned — chat will stay disabled")
	}

	// Build the single ProjectApplier shared by the Spawner (for AI
	// Worker activations) and the chat bridge (for owner-chat). The
	// owner is just another Worker — they get a per-Worker Helix
	// project + agent app + git repo + Zed sandbox just like every
	// hired Worker, applied with the same `worker.*` defaults. The
	// chat surface at /ui/ is a window onto the owner's sandbox.
	serviceClient, serviceClientErr := buildHelixOrgServiceClient(context.Background(), configReg)
	if serviceClientErr != nil {
		log.Warn().Err(serviceClientErr).Msg("helix-org service client init failed — chat and worker activations will not run")
	}

	// Wire the helix-backed WorkspaceSync so update_role /
	// update_identity re-push role.md / identity.md to each affected
	// Worker's per-Worker repo on the helix-specs branch.
	if serviceClient != nil {
		deps.Workspace = agenthelix.NewWorkspace(serviceClient, st, "helix-specs", "helix-org", "helix-org@helix.local")
	}

	// Project applier — shared infra for owner-chat and Worker
	// activations. Applies every Worker's project with the same
	// `worker.runtime` (default `claude_code`) and the same MCP
	// wiring (auth-gated gateway URL with the service api_key in
	// headers).
	//
	// The wrapper re-resolves `worker.*` and `helix.*` from the config
	// registry on every Ensure call, so live changes via /ui/settings
	// take effect on the next activation — no API restart needed.
	var projectApplier *dynamicProjectApplier
	if serviceClient != nil {
		projectApplier = &dynamicProjectApplier{
			cfg:    configReg,
			client: serviceClient,
			Store:  st,
			logger: logger,
		}
	}

	// Wire helix-org's production Spawner. The owner is a Worker, so
	// helix-org/server/chat.HelixBridge reuses the same applier; both
	// drive per-Worker projects through the same default settings.
	var spawnerFn agent.Spawner
	if projectApplier != nil {
		spawnerFn = lazyHelixOrgSpawner(configReg, helixStore, serviceClient, st, bc, logger, projectApplier, deps.NewID, deps.Now)
	}
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

	// Chat backend: owner-chat opens against the owner Worker's
	// per-Worker project via the shared ProjectApplier. Same defaults,
	// same MCP wiring, same desktop runtime as any AI Worker.
	var chatBridge chat.Backend
	if projectApplier != nil && serviceClient != nil {
		bridge, err := buildEmbeddedChatBackend(context.Background(), configReg, projectApplier, serviceClient, logger, st, bc, deps.NewID, deps.Now)
		if err != nil {
			log.Warn().Err(err).Msg("helix-org chat backend failed to start — continuing without chat")
		} else {
			chatBridge = bridge
		}
	}
	if hb, ok := chatBridge.(*chat.HelixBridge); ok && hb != nil {
		chatBridge = hb.WithPrompts(promptReg)
	}

	// Snapshot the registered specs for the settings page (the UI
	// package doesn't import config).
	specs := configReg.Specs()
	uiSpecs := make([]helixorgui.SettingsSpec, 0, len(specs))
	for _, sp := range specs {
		uiSpecs = append(uiSpecs, helixorgui.SettingsSpec{
			Key:         sp.Key,
			Type:        string(sp.Type),
			Required:    sp.Required,
			Description: sp.Description,
		})
	}

	baseUIHandler := helixorgui.Handler(helixorgui.Deps{
		Store:       st,
		Configs:     configReg,
		Bridge:      chatBridge,
		ChatCWD:     orgRoot,
		Broadcaster: bc,
		Dispatcher:  dispatcher,
		NewID:       deps.NewID,
		Now:         deps.Now,
		Settings: helixorgui.SettingsView{
			Owner:   "w-owner",
			DBPath:  dbPath,
			EnvsDir: envsDir,
			Specs:   uiSpecs,
		},
	})

	// /ui/chat/* routes provided by the chat bridge live alongside
	// the page handlers. Compose them into a single mux so the
	// top-level /ui/ mount serves everything that begins with /ui/.
	// When chat isn't configured, the /ui/chat/* POSTs simply 404 and
	// the page renders without a working composer.
	innerUIMux := http.NewServeMux()
	if chatBridge != nil {
		innerUIMux.Handle("GET /ui/chat/stream", chatBridge.StreamHandler())
		innerUIMux.Handle("POST /ui/chat/send", chatBridge.SendHandler())
		innerUIMux.Handle("POST /ui/chat/commands", chatBridge.CommandsHandler())
		innerUIMux.Handle("POST /ui/chat/new", chatBridge.NewHandler())
		innerUIMux.Handle("POST /ui/chat/switch", chatBridge.SwitchHandler())
	}
	innerUIMux.Handle("/", baseUIHandler)

	// Wrap the whole UI surface with middleware that forwards the
	// logged-in Helix user's identity to helixclient. Calls from the
	// chat bridge / agent picker then hit Helix as the actual user
	// chatting, not as a shared service account — sessions and
	// permissions are attributed correctly. Falls back to the
	// auto-provisioned helix.api_key when no user is on the request
	// (shouldn't happen for /ui/ routes since they're gated by
	// requireUser, but the fallback keeps tests honest).
	uiMux := withHelixUserBearer(innerUIMux, helixStore)

	orgServer := helixorgserver.New(st, reg, bc, dispatcher, logger).WithPrompts(promptReg)

	log.Info().
		Str("db", dbPath).
		Str("envs", envsDir).
		Bool("chat_enabled", chatBridge != nil).
		Msg("helix-org mounted at /api/v1/org/ + /ui/")
	return &helixOrgHandlers{api: orgServer.Handler(), ui: uiMux}, nil
}

// helixOrgConfig is just enough of the surrounding config to decide
// whether and where to bring up the embedded org.
type helixOrgConfig struct {
	FileStoreType types.FileStoreType
	LocalFSPath   string
}

// dynamicProjectApplier is a chat.ProjectEnsurer that re-reads
// `worker.*` and `helix.*` from the config registry on every Ensure
// call. Building the underlying agenthelix.ProjectApplier at API
// startup and reusing it freezes `worker.runtime`/`credentials`/
// `provider`/`model` at boot time — operators changing those via
// /ui/settings then had to restart the API container for the new
// values to take effect. Resolving per-call removes that surprise.
//
// Store is exposed directly because helix_org_chat.go needs it to
// load/save the per-Worker session pointer on the same SQLite row
// the spawner uses (helix-org's WorkerRuntimeState).
type dynamicProjectApplier struct {
	cfg    *config.Registry
	client helixclient.Client
	Store  *helixorgstore.Store
	logger *slog.Logger
}

// Ensure satisfies chat.ProjectEnsurer. Builds a fresh
// agenthelix.ProjectApplier from the current registry state and
// delegates. ProjectApplier.Ensure is itself idempotent — first call
// applies, subsequent calls fast-path on the existing project.
func (d *dynamicProjectApplier) Ensure(ctx context.Context, workerID domain.WorkerID) (projectID, agentAppID, repoID string, err error) {
	applier, err := buildHelixOrgProjectApplier(ctx, d.cfg, d.client, d.Store, d.logger)
	if err != nil {
		return "", "", "", err
	}
	return applier.Ensure(ctx, workerID)
}

// buildHelixOrgProjectApplier constructs the ProjectApplier that
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
	cfg *config.Registry,
	client helixclient.Client,
	orgStore *helixorgstore.Store,
	logger *slog.Logger,
) (*agenthelix.ProjectApplier, error) {
	apiKey, _ := cfg.GetString(ctx, "helix.api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := cfg.GetString(ctx, "helix.url")
	if err != nil {
		return nil, fmt.Errorf("read helix.url: %w", err)
	}
	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, cfg)
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org"
	return &agenthelix.ProjectApplier{
		Client:        client,
		Store:         orgStore,
		HelixOrgURL:   helixOrgURL,
		Runtime:       runtime,
		Credentials:   credentials,
		Provider:      provider,
		Model:         model,
		MCPAuthBearer: apiKey,
		Logger:        logger,
	}, nil
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
func resolveWorkerAgentConfig(ctx context.Context, cfg *config.Registry) (runtime, credentials, provider, model string) {
	runtime, _ = cfg.GetString(ctx, "worker.runtime")
	if runtime == "" {
		runtime = "claude_code"
	}
	credentials, _ = cfg.GetString(ctx, "worker.credentials")
	if credentials == "" {
		credentials = "subscription"
	}
	if runtime != "claude_code" {
		credentials = "api_key" // subscription is only meaningful for claude_code
	}
	if credentials == "api_key" {
		provider, _ = cfg.GetString(ctx, "worker.provider")
		model, _ = cfg.GetString(ctx, "worker.model")
	}
	return runtime, credentials, provider, model
}

// buildHelixOrgServiceClient constructs a helixclient backed by the
// auto-provisioned service api_key. Used by the Spawner — it runs
// outside any HTTP request context (driven by the dispatcher) so
// withHelixUserBearer's per-request override isn't available.
func buildHelixOrgServiceClient(ctx context.Context, cfg *config.Registry) (helixclient.Client, error) {
	apiKey, _ := cfg.GetString(ctx, "helix.api_key")
	if apiKey == "" {
		return nil, fmt.Errorf("helix.api_key not set — service client cannot be built")
	}
	baseURL, err := cfg.GetString(ctx, "helix.url")
	if err != nil {
		return nil, fmt.Errorf("read helix.url: %w", err)
	}
	return helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: apiKey})
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
	cfg *config.Registry,
	helixStore helixstore.Store,
	client helixclient.Client,
	orgStore *helixorgstore.Store,
	bc *broadcast.Broadcaster,
	logger *slog.Logger,
	newID func() string,
	now func() time.Time,
) (agenthelix.SpawnerConfig, error) {
	apiKey, _ := cfg.GetString(ctx, "helix.api_key")
	if apiKey == "" {
		return agenthelix.SpawnerConfig{}, fmt.Errorf("helix.api_key not set")
	}
	baseURL, err := cfg.GetString(ctx, "helix.url")
	if err != nil {
		return agenthelix.SpawnerConfig{}, fmt.Errorf("read helix.url: %w", err)
	}

	runtime, credentials, provider, model := resolveWorkerAgentConfig(ctx, cfg)
	helixOrgURL := strings.TrimRight(baseURL, "/") + "/api/v1/mcp/helix-org"
	return agenthelix.SpawnerConfig{
		Client:      client,
		HelixOrgURL: helixOrgURL,
		Runtime:     runtime,
		Credentials: credentials,
		Provider:    provider,
		Model:       model,
		MCPAuthBearer: apiKey,
		Store:         orgStore,
		Broadcaster:   bc,
		Logger:        logger,
		NewID:         newID,
		Now:           now,
		BearerForUser: func(ctx context.Context, userID string) (string, error) {
			return resolveUserHelixAPIKey(ctx, helixStore, userID)
		},
	}, nil
}

// lazyHelixOrgSpawner returns an agent.Spawner that defers building
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
// via /ui/settings takes effect on the next activation, without
// disturbing the shared MaxInflight semaphore inside the cached
// spawner.
func lazyHelixOrgSpawner(
	cfg *config.Registry,
	helixStore helixstore.Store,
	client helixclient.Client,
	orgStore *helixorgstore.Store,
	bc *broadcast.Broadcaster,
	logger *slog.Logger,
	applier *dynamicProjectApplier,
	newID func() string,
	now func() time.Time,
) agent.Spawner {
	var (
		mu      sync.Mutex
		spawner agent.Spawner
	)
	return func(ctx context.Context, workerID domain.WorkerID, envPath string, triggers []agent.Trigger) error {
		// Apply (or fast-path) the per-Worker project with the current
		// worker.* settings before delegating. Without this, the cached
		// spawner's first activation bakes whatever worker.* values
		// were live at boot into the project; later edits via
		// /ui/settings never propagate.
		if applier != nil {
			if _, _, _, err := applier.Ensure(ctx, workerID); err != nil {
				return fmt.Errorf("helix-org spawner: pre-apply project for %s: %w", workerID, err)
			}
		}
		mu.Lock()
		current := spawner
		mu.Unlock()
		if current == nil {
			cfgVal, err := buildHelixOrgSpawnerConfig(ctx, cfg, helixStore, client, orgStore, bc, logger, newID, now)
			if err != nil {
				return fmt.Errorf("helix-org spawner not configured: %w", err)
			}
			built := agenthelix.Spawner(cfgVal)
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
		return current(ctx, workerID, envPath, triggers)
	}
}

