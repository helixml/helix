package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/agent"
	"github.com/helixml/helix-org/bootstrap"
	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/dispatch"
	"github.com/helixml/helix-org/helix/helixclient"
	helixorgserver "github.com/helixml/helix-org/server"
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

	spawnerClient, spawnerClientErr := buildHelixOrgServiceClient(context.Background(), configReg)
	if spawnerClientErr != nil {
		log.Warn().Err(spawnerClientErr).Msg("helix-org spawner client init failed — worker activations will not run")
	}
	helixURL, _ := configReg.GetString(context.Background(), "helix.url")
	var spawnerFn agent.Spawner
	if spawnerClient != nil {
		spawnerDeps := &embeddedSpawnerDeps{
			orgStore:    st,
			configReg:   configReg,
			broadcaster: bc,
			client:      spawnerClient,
			helixURL:    helixURL,
			logger:      logger,
			newID:       deps.NewID,
			now:         deps.Now,
		}
		spawnerFn = newEmbeddedSpawner(spawnerDeps)
	}
	dispatcher := dispatch.New(st, spawnerFn, logger)
	deps.Dispatcher = dispatcher

	reg := tools.NewRegistry()
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		return nil, fmt.Errorf("register helix-org builtins: %w", err)
	}

	// Chat backend: tries to build a HelixBridge against the
	// surrounding Helix server. Returns (nil, nil) when required
	// config keys are missing — chat then renders disabled until the
	// admin finishes setup under /ui/settings.
	chatBridge, err := buildEmbeddedChatBackend(context.Background(), configReg, st, logger)
	if err != nil {
		log.Warn().Err(err).Msg("helix-org chat backend failed to start — continuing without chat")
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
	innerUIMux.Handle("/ui/alpha-agents", newHelixOrgAgentPickerHandler(configReg, helixStore))
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

	orgServer := helixorgserver.New(st, reg, bc, dispatcher, logger)

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
