package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix-org/agent"
	agentclaude "github.com/helixml/helix-org/agent/claude"
	agenthelix "github.com/helixml/helix-org/agent/helix"
	"github.com/helixml/helix-org/bootstrap"
	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/dispatch"
	"github.com/helixml/helix-org/helix/helixclient"
	"github.com/helixml/helix-org/prompts"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/server/chat"
	"github.com/helixml/helix-org/server/ui"
	"github.com/helixml/helix-org/store"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
	githubtransport "github.com/helixml/helix-org/transports/github"
	"github.com/helixml/helix-org/transports/postmark"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "TCP address to listen on")
	dbPath := fs.String("db", "helix-org.db", "SQLite database path (use ':memory:' for ephemeral)")
	publicURL := fs.String("public-url", "", "Base URL spawned Workers use to reach the MCP endpoint. Defaults to http://localhost<addr-port>.")
	envsDir := fs.String("envs-dir", "./envs", "Directory under which each Worker's Environment lives (one subdirectory per workerID).")
	claudeBin := fs.String("claude-bin", "claude", "Path to the claude CLI used to embody AI Workers")
	model := fs.String("model", "sonnet", "Claude model alias or full name (e.g. 'sonnet', 'opus', 'claude-sonnet-4-6'). Default sonnet to keep activation costs predictable.")
	effort := fs.String("effort", "low", "Claude effort/thinking level (low|medium|high|xhigh|max). Defaults to low to minimise per-activation cost.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *publicURL == "" {
		*publicURL = "http://localhost" + portFromAddr(*addr)
	}
	absEnvsDir, err := filepath.Abs(*envsDir)
	if err != nil {
		return fmt.Errorf("resolve envs-dir %q: %w", *envsDir, err)
	}
	if err := os.MkdirAll(absEnvsDir, 0o750); err != nil {
		return fmt.Errorf("create envs-dir %q: %w", absEnvsDir, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	store, err := sqlite.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	// First start against an empty DB creates the owner Worker. On
	// subsequent starts ErrAlreadyInitialised is the normal case; any
	// other error is fatal.
	ownerEnvPath := filepath.Join(absEnvsDir, "w-owner")
	if err := os.MkdirAll(ownerEnvPath, 0o750); err != nil {
		return fmt.Errorf("create owner env %q: %w", ownerEnvPath, err)
	}
	switch result, err := bootstrap.Run(context.Background(), store, bootstrap.Params{
		EnvironmentPath: ownerEnvPath,
	}); {
	case err == nil:
		logger.Info("bootstrap created owner",
			"workerId", result.WorkerID,
			"roleId", result.RoleID,
			"positionId", result.PositionID,
			"environmentPath", result.EnvironmentPath,
		)
	case errors.Is(err, bootstrap.ErrAlreadyInitialised):
		logger.Info("bootstrap skipped: already initialised", "db", *dbPath)
	default:
		return fmt.Errorf("bootstrap: %w", err)
	}

	bc := broadcast.New()
	deps := tools.DefaultDeps(store)
	deps.Broadcaster = bc
	deps.EnvsDir = absEnvsDir

	// Operational config registry — Postmark + future provider creds
	// live here, mutated only via the helix-org config CLI. See
	// design/config.md.
	configReg := config.New(store.Configs)
	registerAllConfigSpecs(configReg)

	spawner, workspace, err := buildSpawner(context.Background(), configReg, store, bc, deps, logger, *claudeBin, *publicURL, *model, *effort)
	if err != nil {
		return fmt.Errorf("build spawner: %w", err)
	}
	dispatcher := dispatch.New(store, spawner, logger)
	deps.Dispatcher = dispatcher
	deps.Workspace = workspace
	logger.Info("dispatcher enabled", "public-url", *publicURL, "envs-dir", absEnvsDir)

	// Email transport: shares the dispatcher (for inbound activations)
	// and registers itself as the dispatcher's outbound email emitter.
	emailTransport := postmark.New(configReg, store, bc, dispatcher, logger)
	dispatcher.SetEmailEmitter(emailTransport)
	logger.Info("email transport enabled", "provider", "postmark")

	// GitHub transport: inbound only. Webhook deliveries POST to
	// /github/webhook; the transport HMAC-verifies, fans out to every
	// Stream whose repo+events match, and activates subscribed Workers
	// via the dispatcher. Outbound is the Worker's job via `gh`; the
	// publish tool rejects writes to a github stream loudly.
	githubInbound := githubtransport.New(configReg, store, bc, dispatcher, logger)
	logger.Info("github transport enabled")

	reg := tools.NewRegistry()
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		return fmt.Errorf("register builtins: %w", err)
	}

	// Prompts: server-defined slash commands. Surfaced per-worker
	// alongside tools, gated by tool grants so a Worker only sees
	// prompts that end in a tool call they can actually make.
	promptReg := prompts.NewRegistry()
	if err := prompts.RegisterBuiltins(promptReg); err != nil {
		return fmt.Errorf("register prompt builtins: %w", err)
	}

	// UI chat surface. Backend is selected by chat.backend config:
	//   - "claude": long-lived `claude` subprocess in the server cwd,
	//              bridged to the browser via SSE. Dev only.
	//   - "helix": Helix chat session; every owner message becomes a
	//              StartChat / PostFollowup against Helix.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	chatBridge, err := buildChatBackend(context.Background(), configReg, store, logger, *claudeBin, cwd, *publicURL, promptReg)
	if err != nil {
		return fmt.Errorf("build chat backend: %w", err)
	}

	// Snapshot config registry → ui.SettingsView so the UI doesn't
	// need to import config. This is captured once at startup;
	// /ui/settings re-resolves the per-spec configured flag against
	// the store on each render.
	specs := configReg.Specs()
	uiSpecs := make([]ui.SettingsSpec, 0, len(specs))
	for _, sp := range specs {
		uiSpecs = append(uiSpecs, ui.SettingsSpec{
			Key:         sp.Key,
			Type:        string(sp.Type),
			Required:    sp.Required,
			Description: sp.Description,
		})
	}
	uiHandler := ui.Handler(ui.Deps{
		Store:       store,
		Configs:     configReg,
		Bridge:      chatBridge,
		ChatCWD:     cwd,
		Broadcaster: bc,
		Dispatcher:  dispatcher,
		NewID:       deps.NewID,
		Now:         deps.Now,
		Settings: ui.SettingsView{
			Owner:     "w-owner",
			PublicURL: *publicURL,
			DBPath:    *dbPath,
			EnvsDir:   absEnvsDir,
			Specs:     uiSpecs,
		},
	})
	rootRedirect := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusFound)
	})

	srv := &http.Server{
		Addr: *addr,
		Handler: server.New(store, reg, bc, deps.Dispatcher, logger).WithPrompts(promptReg).Handler(
			server.Route{Pattern: "POST /email/postmark", Handler: emailTransport.HandleInbound()},
			server.Route{Pattern: "POST /github/webhook", Handler: githubInbound.HandleInbound()},
			server.Route{Pattern: "GET /ui/chat/stream", Handler: chatBridge.StreamHandler()},
			server.Route{Pattern: "POST /ui/chat/send", Handler: chatBridge.SendHandler()},
			server.Route{Pattern: "POST /ui/chat/commands", Handler: chatBridge.CommandsHandler()},
			server.Route{Pattern: "POST /ui/chat/new", Handler: chatBridge.NewHandler()},
			server.Route{Pattern: "POST /ui/chat/switch", Handler: chatBridge.SwitchHandler()},
			server.Route{Pattern: "/ui/", Handler: uiHandler},
			server.Route{Pattern: "GET /{$}", Handler: rootRedirect},
		),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", *addr, "db", *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	case err, ok := <-errCh:
		if ok && err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	}
	return nil
}

// buildSpawner reads spawner.kind from the config registry and
// returns the corresponding agent.Spawner plus the matching
// WorkspaceSync. The two are paired: each runtime backend supplies
// both the activation runner and the role/identity-sync surface that
// keeps the agent's view fresh between activations.
func buildSpawner(
	ctx context.Context,
	cfg *config.Registry,
	st *store.Store,
	bc *broadcast.Broadcaster,
	deps tools.Deps,
	logger *slog.Logger,
	claudeBin, publicURL, model, effort string,
) (agent.Spawner, agent.WorkspaceSync, error) {
	kind, err := cfg.GetString(ctx, "spawner.kind")
	if err != nil {
		return nil, nil, fmt.Errorf("read spawner.kind: %w", err)
	}
	switch kind {
	case "claude":
		logger.Info("spawner: claude", "claude-bin", claudeBin, "model", model, "effort", effort)
		spawner := agentclaude.Spawner(agentclaude.SpawnerConfig{
			ClaudeBin:   claudeBin,
			PublicURL:   publicURL,
			Model:       model,
			Effort:      effort,
			Logger:      logger,
			Store:       st,
			Broadcaster: bc,
			Now:         deps.Now,
			NewID:       deps.NewID,
		})
		return spawner, agentclaude.NewWorkspace(deps.EnvsDir), nil
	case "helix":
		baseURL, err := cfg.GetString(ctx, "helix.url")
		if err != nil {
			return nil, nil, fmt.Errorf("read helix.url: %w", err)
		}
		apiKey, err := cfg.GetString(ctx, "helix.api_key")
		if err != nil {
			return nil, nil, fmt.Errorf("read helix.api_key: %w", err)
		}
		orgURL, err := cfg.GetString(ctx, "helix.org_url")
		if err != nil {
			return nil, nil, fmt.Errorf("read helix.org_url: %w", err)
		}
		timeoutStr, err := cfg.GetString(ctx, "helix.activation_timeout")
		if err != nil {
			return nil, nil, fmt.Errorf("read helix.activation_timeout: %w", err)
		}
		timeout, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return nil, nil, fmt.Errorf("parse helix.activation_timeout %q: %w", timeoutStr, err)
		}
		maxInflight, err := cfg.GetInt(ctx, "helix.max_inflight")
		if err != nil {
			return nil, nil, fmt.Errorf("read helix.max_inflight: %w", err)
		}
		// Provider/Model drive every per-Worker project's Agent App
		// config (set at apply time inside the spawner).
		provider, _ := cfg.GetString(ctx, "chat.provider")
		model, _ := cfg.GetString(ctx, "chat.model")
		client, err := helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: apiKey})
		if err != nil {
			return nil, nil, fmt.Errorf("helix client: %w", err)
		}
		// Fail-fast validation: a typo in chat.provider / chat.model
		// would otherwise surface much later as a 422 from
		// /sessions/{id}/zed-config when the desktop boots, with no
		// obvious link back to the bad config key. Catch it here.
		if err := helixclient.ValidateProviderModel(ctx, client, provider, model); err != nil {
			return nil, nil, fmt.Errorf("invalid chat.provider / chat.model (run `helix-org config set chat.provider <name>` and `helix-org config set chat.model <name>`): %w", err)
		}
		workspace := agenthelix.NewWorkspace(client, st, "helix-specs", "helix-org", "helix-org@local")
		logger.Info("spawner: helix",
			"helix-url", baseURL,
			"org-url", orgURL,
			"provider", provider,
			"model", model,
			"timeout", timeout,
			"max-inflight", maxInflight,
		)
		spawner := agenthelix.Spawner(agenthelix.SpawnerConfig{
			Client:            client,
			HelixOrgURL:       orgURL,
			Provider:          provider,
			Model:             model,
			AgentMD:           agent.Policy,
			ActivationTimeout: timeout,
			MaxInflight:       int(maxInflight),
			Logger:            logger,
			Store:             st,
			Broadcaster:       bc,
			Now:               deps.Now,
			NewID:             deps.NewID,
		})
		return spawner, workspace, nil
	default:
		return nil, nil, fmt.Errorf("unknown spawner.kind %q (valid: claude, helix)", kind)
	}
}

// buildChatBackend selects the owner-chat backend based on
// chat.backend. The claude path keeps full backwards compat; the
// helix path constructs a fresh helixclient and delegates the chat
// surface to a Helix session — closing the "all LLM calls go through
// Helix" gap. Slash-command prompts are wired into both backends.
func buildChatBackend(
	ctx context.Context,
	cfg *config.Registry,
	st *store.Store,
	logger *slog.Logger,
	claudeBin, cwd, publicURL string,
	promptReg *prompts.Registry,
) (chat.Backend, error) {
	kind, err := cfg.GetString(ctx, "chat.backend")
	if err != nil {
		return nil, fmt.Errorf("read chat.backend: %w", err)
	}
	switch kind {
	case "claude":
		logger.Info("chat backend: claude", "claude-bin", claudeBin)
		// claude.model is the alias passed to claude as --model. Use
		// it as the footer label too so the UI truthfully reports
		// which model the chat is running on.
		claudeModel, _ := cfg.GetString(ctx, "claude.model")
		label := "claude"
		if claudeModel != "" {
			label = "claude · " + claudeModel
		}
		b := chat.New(claudeBin, cwd, strings.TrimRight(publicURL, "/")+"/workers/w-owner/mcp", logger).
			WithPrompts(promptReg).
			WithLabel(label)
		return b, nil
	case "helix":
		baseURL, err := cfg.GetString(ctx, "helix.url")
		if err != nil {
			return nil, fmt.Errorf("read helix.url: %w", err)
		}
		apiKey, err := cfg.GetString(ctx, "helix.api_key")
		if err != nil {
			return nil, fmt.Errorf("read helix.api_key: %w", err)
		}
		orgURL, err := cfg.GetString(ctx, "helix.org_url")
		if err != nil {
			return nil, fmt.Errorf("read helix.org_url: %w", err)
		}
		sessionRole, err := cfg.GetString(ctx, "chat.session_role")
		if err != nil {
			return nil, fmt.Errorf("read chat.session_role: %w", err)
		}
		provider, err := cfg.GetString(ctx, "chat.provider")
		if err != nil {
			return nil, fmt.Errorf("read chat.provider: %w", err)
		}
		model, err := cfg.GetString(ctx, "chat.model")
		if err != nil {
			return nil, fmt.Errorf("read chat.model: %w", err)
		}
		client, err := helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: apiKey})
		if err != nil {
			return nil, fmt.Errorf("helix client: %w", err)
		}
		// Fail-fast validation — see buildSpawner for rationale.
		if err := helixclient.ValidateProviderModel(ctx, client, provider, model); err != nil {
			return nil, fmt.Errorf("invalid chat.provider / chat.model (run `helix-org config set chat.provider <name>` and `helix-org config set chat.model <name>`): %w", err)
		}
		// Chat backend uses the same per-Worker project flow and the
		// same fixed runtime / agent_type as the AI Worker spawner —
		// see helix.Runtime / helix.AgentType. The auto-provisioned
		// Agent App carries our MCP wiring (attached via UpdateApp
		// after project apply); helix.org_url must be a tunnel URL
		// reachable from Helix's runner so the in-sandbox agent can
		// reach /workers/{id}/mcp.
		applier := &agenthelix.ProjectApplier{
			Client:      client,
			Store:       st,
			HelixOrgURL: orgURL,
			Provider:    provider,
			Model:       model,
			AgentMD:     agent.Policy,
			Logger:      logger,
		}
		hb, err := chat.NewHelix(chat.HelixConfig{
			Client:      client,
			Ensure:      applier,
			OwnerID:     "w-owner",
			SessionRole: sessionRole,
			Provider:    provider,
			Model:       model,
			CWD:         cwd,
			Logger:      logger,
		})
		if err != nil {
			return nil, err
		}
		logger.Info("chat backend: helix",
			"helix-url", baseURL,
			"session-role", sessionRole,
			"runtime", agenthelix.Runtime,
			"agent-type", agenthelix.AgentType,
			"provider", provider,
			"model", model,
		)
		return hb.WithPrompts(promptReg), nil
	default:
		return nil, fmt.Errorf("unknown chat.backend %q (valid: claude, helix)", kind)
	}
}

// portFromAddr extracts the ":PORT" suffix from a TCP address such as
// ":8080", "127.0.0.1:8080", or "0.0.0.0:8080". Returns ":8080" for an
// addr that has no explicit port (which mirrors net.http's own default).
func portFromAddr(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return ":8080"
}
