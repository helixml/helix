package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/function61/holepunch-server/pkg/wsconnadapter"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"gocloud.dev/blob"

	api_skill "github.com/helixml/helix/api/pkg/agent/skill/api_skills"
	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/revdial"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server/spa"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/version"

	_ "net/http/pprof" // enable profiling
)

const APIPrefix = "/api/v1"

type Options struct {
	Config      *config.ServerConfig
	URL         string
	Host        string
	Port        int
	FrontendURL string // Can either be a URL to frontend or a path to static files
	RunnerToken string
	// List of admin user IDs, or contains "all" for dev mode where everyone is admin.
	// If empty, uses database admin field.
	AdminUserIDs []string
	// if this is specified then we provide the option to clone entire
	// sessions into this user without having to logout and login
	EvalUserID string
	// this is for when we are running localfs filesystem
	// and we need to add a route to view files based on their path
	// we are assuming all file storage is open right now
	// so we just deep link to the object path and don't apply auth
	// (this is so helix nodes can see files)
	// later, we might add a token to the URLs
	LocalFilestorePath string
}

type HelixAPIServer struct {
	Cfg                         *config.ServerConfig
	Store                       store.Store
	Stripe                      *stripe.Stripe
	Controller                  *controller.Controller
	Janitor                     *janitor.Janitor
	authMiddleware              *authMiddleware
	pubsub                      pubsub.PubSub
	mcpClientGetter             mcp.ClientGetter
	connman                     *connman.ConnectionManager
	providerManager             manager.ProviderManager
	modelInfoProvider           model.ModelInfoProvider
	externalAgentExecutor       external_agent.Executor
	externalAgentWSManager      *ExternalAgentWSManager
	externalAgentRunnerManager  *ExternalAgentRunnerManager
	contextMappings             map[string]string // Zed context_id -> Helix session_id mapping
	contextMappingsMutex        sync.RWMutex      // Mutex for contextMappings (and related mappings below)
	sessionToWaitingInteraction map[string]string // Helix session_id -> current waiting interaction_id
	requestToSessionMapping     map[string]string // request_id -> Helix session_id mapping (for chat_message routing)
	externalAgentSessionMapping map[string]string // External agent session_id -> Helix session_id mapping
	externalAgentUserMapping    map[string]string // External agent session_id -> user_id mapping
	// Comment processing timeouts - uses database for queue state (QueuedAt/RequestID fields)
	sessionCommentTimeout     map[string]*time.Timer // planning_session_id -> timeout timer for current comment
	sessionCommentMutex       sync.RWMutex           // Mutex for timeout operations
	requestToCommenterMapping map[string]string      // request_id -> commenter user_id (for design review streaming)
	inferenceServer           *openai.InternalHelixServer
	knowledgeManager          knowledge.Manager
	skillManager              *api_skill.Manager
	router                    *mux.Router
	scheduler                 *scheduler.Scheduler
	pingService               *version.PingService
	authenticator             auth.Authenticator
	oidcClient                auth.OIDC
	oauthManager              *oauth.Manager
	fileServerHandler         http.Handler
	cache                     *ristretto.Cache[string, string]
	avatarsBucket             *blob.Bucket
	trigger                   *trigger.Manager
	specDrivenTaskService     *services.SpecDrivenTaskService
	sampleProjectCodeService  *services.SampleProjectCodeService
	gitRepositoryService      *services.GitRepositoryService
	koditService              *services.KoditService
	mcpGateway    *MCPGateway
	gitHTTPServer *services.GitHTTPServer
	// Rate limiting for streaming connections
	streamingRateLimiter       map[string]time.Time // session_id -> last connection time
	streamingRateLimiterMutex  sync.RWMutex
	specTaskOrchestrator       *services.SpecTaskOrchestrator
	projectInternalRepoService *services.ProjectInternalRepoService
	anthropicProxy             *anthropic.Proxy
	auditLogService            *services.AuditLogService
	adminAlerter               *notification.AdminAlerter
	wg                         sync.WaitGroup // Control for goroutines to enable tests
	summaryService             *SummaryService
}

func NewServer(
	cfg *config.ServerConfig,
	store store.Store,
	ps pubsub.PubSub,

	providerManager manager.ProviderManager,
	modelInfoProvider model.ModelInfoProvider,
	inferenceServer *openai.InternalHelixServer,
	authenticator auth.Authenticator,
	stripe *stripe.Stripe,
	controller *controller.Controller,
	janitor *janitor.Janitor,
	knowledgeManager knowledge.Manager,
	scheduler *scheduler.Scheduler,
	pingService *version.PingService,
	oauthManager *oauth.Manager,
	avatarsBucket *blob.Bucket,
	trigger *trigger.Manager,
	anthropicProxy *anthropic.Proxy,
) (*HelixAPIServer, error) {
	if cfg.WebServer.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}

	if cfg.WebServer.Host == "" {
		return nil, fmt.Errorf("server host is required")
	}

	if cfg.WebServer.Port == 0 {
		return nil, fmt.Errorf("server port is required")
	}

	if cfg.WebServer.RunnerToken == "" {
		return nil, fmt.Errorf("runner token is required")
	}

	// Create OIDC client if using OIDC auth provider
	helixRedirectURL := fmt.Sprintf("%s/api/v1/auth/callback", cfg.WebServer.URL)
	var oidcClient auth.OIDC
	if cfg.Auth.Provider == types.AuthProviderOIDC {
		if cfg.Auth.OIDC.Audience == "" {
			return nil, fmt.Errorf("oidc audience is required")
		}
		client, err := auth.NewOIDCClient(controller.Ctx, auth.OIDCConfig{
			ProviderURL:    cfg.Auth.OIDC.URL,
			ClientID:       cfg.Auth.OIDC.ClientID,
			ClientSecret:   cfg.Auth.OIDC.ClientSecret,
			RedirectURL:    helixRedirectURL,
			AdminUserIDs:   cfg.WebServer.AdminUserIDs,
			Audience:       cfg.Auth.OIDC.Audience,
			Scopes:         strings.Split(cfg.Auth.OIDC.Scopes, ","),
			Store:          store,
			ExpectedIssuer: cfg.Auth.OIDC.ExpectedIssuer,
			TokenURL:       cfg.Auth.OIDC.TokenURL,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create oidc client: %w", err)
		}
		oidcClient = client
	}

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 30, // maximum cost of cache (1GB).
		BufferItems: 64,      // number of keys per Get buffer.
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %w", err)
	}

	// Initialize skill manager
	skillManager := api_skill.NewManager()


	// Initialize external agent WebSocket manager BEFORE executor
	externalAgentWSManager := NewExternalAgentWSManager()

	// Initialize connection manager for reverse dial BEFORE executor (needed for RevDial screenshot/clipboard)
	connectionManager := connman.New()

	// Use SandboxAPIURL if set (direct HTTP bypassing reverse proxy), otherwise fall back to main URL
	sandboxAPIURL := cfg.WebServer.SandboxAPIURL
	if sandboxAPIURL == "" {
		sandboxAPIURL = cfg.WebServer.URL
	}

	// Create Hydra executor for container lifecycle management
	log.Info().Msg("Initializing Hydra executor for container management")
	externalAgentExecutor := external_agent.NewHydraExecutor(external_agent.HydraExecutorConfig{
		Store:                         store,
		HelixAPIURL:                   sandboxAPIURL,
		HelixAPIToken:                 cfg.WebServer.RunnerToken,
		WorkspaceBasePathForContainer: "/workspace",       // Path inside dev container
		WorkspaceBasePathForCloning:   "/data/workspaces", // Path on sandbox filesystem (not API - Hydra creates dirs)
		Connman:                       connectionManager,
		GPUVendor:                     os.Getenv("GPU_VENDOR"), // "nvidia", "amd", "intel", or ""
	})

	// Initialize external agent runner connection manager
	externalAgentRunnerManager := NewExternalAgentRunnerManager()

	log.Info().Msg("External agent architecture initialized: WebSocket-based runner pool ready")

	gitRepositoryService := services.NewGitRepositoryService(
		store,
		cfg.FileStore.LocalFSPath, // Use filestore mount for git repositories
		cfg.WebServer.URL,         // Server base URL
		"Helix System",            // Git user name
		"system@helix.ml",         // Git user email
	)

	apiServer := &HelixAPIServer{
		Cfg:                         cfg,
		Store:                       store,
		Stripe:                      stripe,
		Controller:                  controller,
		Janitor:                     janitor,
		gitRepositoryService:        gitRepositoryService,
		externalAgentExecutor:       externalAgentExecutor,
		externalAgentWSManager:      externalAgentWSManager,
		externalAgentRunnerManager:  externalAgentRunnerManager,
		contextMappings:             make(map[string]string),
		sessionToWaitingInteraction: make(map[string]string),
		requestToSessionMapping:     make(map[string]string),
		externalAgentSessionMapping: make(map[string]string),
		externalAgentUserMapping:    make(map[string]string),
		sessionCommentTimeout:       make(map[string]*time.Timer),
		requestToCommenterMapping:   make(map[string]string),
		streamingRateLimiter:        make(map[string]time.Time),
		inferenceServer:             inferenceServer,
		authMiddleware: newAuthMiddleware(
			authenticator,
			oidcClient,
			store,
			authMiddlewareConfig{
				adminUserIDs: cfg.WebServer.AdminUserIDs,
				runnerToken:  cfg.WebServer.RunnerToken,
			},
		),
		providerManager:   providerManager,
		modelInfoProvider: modelInfoProvider,
		pubsub:            ps,
		mcpClientGetter: &mcp.DefaultClientGetter{
			TLSSkipVerify: cfg.Tools.TLSSkipVerify,
		},
		knowledgeManager:  knowledgeManager,
		skillManager:      skillManager,
		scheduler:         scheduler,
		pingService:       pingService,
		authenticator:     authenticator,
		oidcClient:        oidcClient,
		oauthManager:      oauthManager,
		fileServerHandler: http.FileServer(neuteredFileSystem{http.Dir(cfg.FileStore.LocalFSPath)}),
		cache:             cache,
		avatarsBucket:     avatarsBucket,
		trigger:           trigger,
		anthropicProxy:    anthropicProxy,
		specDrivenTaskService: services.NewSpecDrivenTaskService(
			store,
			controller,
			"helix-spec-agent",         // Default Helix agent for spec generation
			[]string{"zed-1", "zed-2"}, // Pool of Zed agents for implementation
			ps,                         // PubSub for Zed integration
			externalAgentExecutor,      // Hydra executor for launching external agents
			gitRepositoryService,
			nil, // Will set callback after apiServer is constructed
		),
		sampleProjectCodeService: services.NewSampleProjectCodeService(),
		connman:                  connectionManager,
		auditLogService:          services.NewAuditLogService(store),
	}

	// Initialize SummaryService for async interaction summaries and session titles
	apiServer.summaryService = NewSummaryService(store, providerManager, ps)

	// Initialize git repository base directory
	if err := apiServer.gitRepositoryService.Initialize(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize git repository service: %w", err)
	}

	// Initialize Kodit Service for code intelligence
	if cfg.Kodit.Enabled {
		apiServer.koditService = services.NewKoditService(cfg.Kodit.BaseURL, cfg.Kodit.APIKey)
		apiServer.gitRepositoryService.SetKoditService(apiServer.koditService)
		apiServer.gitRepositoryService.SetKoditGitURL(cfg.Kodit.GitURL)
		log.Info().
			Str("kodit_base_url", cfg.Kodit.BaseURL).
			Str("kodit_git_url", cfg.Kodit.GitURL).
			Msg("Initialized Kodit code intelligence service")
	} else {
		apiServer.koditService = services.NewKoditService("", "") // Disabled instance
		log.Info().Msg("Kodit code intelligence service disabled")
	}

	// Initialize MCP Gateway for authenticated MCP proxying
	apiServer.mcpGateway = NewMCPGateway()

	// Register Kodit MCP backend (code intelligence)
	apiServer.mcpGateway.RegisterBackend("kodit", NewKoditMCPBackend(&cfg.Kodit))

	// Register Helix native MCP backend (APIs, Knowledge, Zapier)
	apiServer.mcpGateway.RegisterBackend("helix", NewHelixMCPBackend(store, controller))

	// Register Session MCP backend (session navigation and context tools)
	apiServer.mcpGateway.RegisterBackend("session", NewSessionMCPBackend(store))

	log.Info().Msg("Initialized MCP Gateway with Kodit, Helix, and Session backends")

	// Initialize Git HTTP Server for clone/push operations
	apiServer.gitHTTPServer = services.NewGitHTTPServer(
		store,
		apiServer.gitRepositoryService,
		&services.GitHTTPServerConfig{
			ServerBaseURL:     cfg.WebServer.URL,
			GitExecutablePath: "git",
			AuthTokenHeader:   "Authorization",
			EnablePush:        true,
			EnablePull:        true,
			MaxRepoSize:       1024 * 1024 * 1024, // 1GB
			RequestTimeout:    5 * time.Minute,
		},
		apiServer.authorizeUserToResource, // Use server's existing RBAC system
		apiServer.trigger,
	)
	log.Info().Msg("Initialized Git HTTP server for clone/push operations")

	// Set the message sender callback for GitHTTPServer (for sending messages to agents via WebSocket)
	apiServer.gitHTTPServer.SetMessageSender(apiServer.sendMessageToSpecTaskAgent)

	// Initialize Project Repository Service (startup scripts stored in code repos at .helix/startup.sh)
	projectsBasePath := filepath.Join(cfg.FileStore.LocalFSPath, "projects")
	apiServer.projectInternalRepoService = services.NewProjectInternalRepoService(projectsBasePath)
	log.Info().
		Str("projects_base_path", projectsBasePath).
		Msg("Initialized project repository service")

	// Set the request mapping callback for SpecDrivenTaskService
	apiServer.specDrivenTaskService.RegisterRequestMapping = apiServer.RegisterRequestToSessionMapping
	// Set the message sender callback for SpecDrivenTaskService (for sending messages to agents via WebSocket)
	apiServer.specDrivenTaskService.SendMessageToAgent = apiServer.sendMessageToSpecTaskAgent

	// Initialize SpecTask Orchestrator components
	apiServer.specTaskOrchestrator = services.NewSpecTaskOrchestrator(
		store,
		controller,
		apiServer.gitRepositoryService,
		apiServer.specDrivenTaskService,
		apiServer.externalAgentExecutor, // Hydra executor for external agent management
	)

	// Start orchestrator
	go func() {
		if err := apiServer.specTaskOrchestrator.Start(context.Background()); err != nil {
			log.Error().Err(err).Msg("Failed to start SpecTask orchestrator")
		}
	}()

	// Initialize AdminAlerter for sending alerts to admin users
	adminAlerter, err := notification.NewAdminAlerter(&cfg.Notifications, store)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to initialize admin alerter - admin email alerts will be disabled")
	} else {
		apiServer.adminAlerter = adminAlerter
		log.Info().Msg("Initialized admin alerter for email notifications")
	}

	return apiServer, nil
}

func (apiServer *HelixAPIServer) ListenAndServe(ctx context.Context, _ *system.CleanupManager) error {
	apiRouter, err := apiServer.registerRoutes(ctx)
	if err != nil {
		return err
	}

	// Seed models from environment variables
	if err := apiServer.Store.SeedModelsFromEnvironment(ctx); err != nil {
		log.Error().Err(err).Msg("failed to seed models from environment - continuing startup")
		// Don't fail startup if seeding fails, just log the error
	}

	// Start background model cache refresh to ensure provider model lists are always cached.
	// This is critical for:
	// 1. API-only clients that don't use the UI (which triggers cache population)
	// 2. Handling HuggingFace model IDs like "Qwen/Qwen3-Coder" correctly
	// 3. Detecting when providers come back online after being down
	apiServer.StartModelCacheRefresh(ctx)

	// Resume comment queue processing for any comments that were pending before restart
	go apiServer.ResumeCommentQueueProcessing(ctx)

	apiServer.startUserWebSocketServer(
		ctx,
		apiRouter,
		"/ws/user",
	)

	apiServer.startRunnerWebSocketServer(
		ctx,
		apiRouter,
		"/ws/runner",
	)

	// Zed Agent Runner WebSocket Server
	// External Agent Runner WebSocket Server (with query parameter auth)
	// Note: External agent runners connect via /ws/external-agent-runner endpoint

	// Start UNIX socket server for embeddings if configured
	if apiServer.Cfg.WebServer.EmbeddingsSocket != "" {
		go func() {
			if err := apiServer.startEmbeddingsSocketServer(ctx); err != nil {
				log.Error().Err(err).Msg("failed to start embeddings socket server")
			}
		}()
	}

	srv := &http.Server{
		Addr: fmt.Sprintf("%s:%d", apiServer.Cfg.WebServer.Host, apiServer.Cfg.WebServer.Port),
		// WriteTimeout and ReadTimeout set to 0 (no timeout) to support:
		// - Large git clone/push operations that can take a long time
		// - Long-running streaming responses (SSE, WebSocket upgrades)
		// - LLM inference streaming that can take minutes
		// Note: ReadHeaderTimeout is kept to prevent slowloris attacks
		WriteTimeout:      0,
		ReadTimeout:       0,
		ReadHeaderTimeout: time.Second * 60,
		IdleTimeout:       time.Minute * 60,
		Handler:           apiServer.router,
	}
	return srv.ListenAndServe()
}

func matchAllRoutes(*http.Request, *mux.RouteMatch) bool {
	return true
}

func (apiServer *HelixAPIServer) registerRoutes(_ context.Context) (*mux.Router, error) {
	router := mux.NewRouter()
	err := apiServer.Janitor.InjectMiddleware(router)
	if err != nil {
		return nil, err
	}

	// we do token extraction for all routes
	// if there is a token we will assign the user if not then oh well no user it's all gravy
	router.Use(ErrorLoggingMiddleware)

	// insecure router is under /api/v1 but not protected by auth
	insecureRouter := router.PathPrefix(APIPrefix).Subrouter()

	// any route that lives under /api/v1
	// Extract auth for /api/v1 routes only (not frontend static assets)
	subRouter := router.PathPrefix(APIPrefix).Subrouter()
	subRouter.Use(apiServer.authMiddleware.extractMiddleware)

	// auth router requires a valid token from keycloak or api key
	authRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	authRouter.Use(requireUser)

	// runner router requires a valid runner token
	runnerRouter := subRouter.MatcherFunc(matchAllRoutes).Subrouter()
	runnerRouter.Use(requireRunner)

	// admin auth requires a user with admin flag
	adminRouter := authRouter.MatcherFunc(matchAllRoutes).Subrouter()
	adminRouter.Use(requireAdmin)

	// Setup OAuth routes with the auth router (except for callback)
	apiServer.setupOAuthRoutes(authRouter)

	// Setup OAuth callback route (no auth required)
	insecureRouter.HandleFunc("/oauth/flow/callback", apiServer.handleOAuthCallback).Methods("GET")

	insecureRouter.HandleFunc("/webhooks/{id}", apiServer.webhookTriggerHandler).Methods(http.MethodPost, http.MethodPut)

	// Teams Bot Framework webhook - auth handled by Bot Framework JWT validation
	insecureRouter.HandleFunc("/teams/webhook/{appID}", apiServer.teamsWebhookHandler).Methods(http.MethodPost)

	insecureRouter.HandleFunc("/config", system.DefaultWrapperWithConfig(apiServer.config, system.WrapperConfig{
		SilenceErrors: true,
	})).Methods(http.MethodGet)

	insecureRouter.HandleFunc("/config/js", apiServer.configJS).Methods(http.MethodGet)
	insecureRouter.Handle("/swagger", apiServer.swaggerHandler()).Methods(http.MethodGet)

	// this is not authenticated because we use the webhook signing secret
	// the stripe library handles http management
	subRouter.HandleFunc("/stripe/webhook", apiServer.subscriptionWebhook).Methods(http.MethodPost)

	authRouter.HandleFunc("/status", system.DefaultWrapper(apiServer.status)).Methods(http.MethodGet)

	// the auth here is handled because we prefix the user path based on the auth context
	// e.g. /sessions/123 becomes /users/456/sessions/123
	// so - the point is, the auth is done by injecting the user id based on the token
	authRouter.HandleFunc("/filestore/config", system.DefaultWrapper(apiServer.filestoreConfig)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/list", system.DefaultWrapper(apiServer.filestoreList)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/get", system.DefaultWrapper(apiServer.filestoreGet)).Methods(http.MethodGet)
	authRouter.HandleFunc("/filestore/folder", system.DefaultWrapper(apiServer.filestoreCreateFolder)).Methods(http.MethodPost)
	authRouter.HandleFunc("/filestore/upload", system.DefaultWrapper(apiServer.filestoreUpload)).Methods(http.MethodPost)
	authRouter.HandleFunc("/filestore/rename", system.DefaultWrapper(apiServer.filestoreRename)).Methods(http.MethodPut)
	authRouter.HandleFunc("/filestore/delete", system.DefaultWrapper(apiServer.filestoreDelete)).Methods(http.MethodDelete)
	// Authentication is done within the handler itself based on API path
	subRouter.PathPrefix("/filestore/viewer/").Handler(http.StripPrefix(APIPrefix+"/filestore/viewer/", http.HandlerFunc(apiServer.filestoreViewerHandler)))

	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.createAPIKey)).Methods(http.MethodPost)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.getAPIKeys)).Methods(http.MethodGet)
	authRouter.HandleFunc("/api_keys", system.DefaultWrapper(apiServer.deleteAPIKey)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/api_keys/check", system.DefaultWrapper(apiServer.checkAPIKey)).Methods(http.MethodGet)

	// User search endpoint
	authRouter.HandleFunc("/users/search", apiServer.searchUsers).Methods(http.MethodGet)
	authRouter.HandleFunc("/users/token-usage", apiServer.getUserTokenUsage).Methods(http.MethodGet)

	// User guidelines (personal workspace)
	authRouter.HandleFunc("/users/me/guidelines", apiServer.getUserGuidelines).Methods(http.MethodGet)
	authRouter.HandleFunc("/users/me/guidelines", apiServer.updateUserGuidelines).Methods(http.MethodPut)
	authRouter.HandleFunc("/users/me/guidelines-history", apiServer.getUserGuidelinesHistory).Methods(http.MethodGet)

	authRouter.HandleFunc("/users/{id}", apiServer.getUserDetails).Methods(http.MethodGet)

	// Billing
	authRouter.HandleFunc("/wallet", system.Wrapper(apiServer.getWalletHandler)).Methods(http.MethodGet)

	authRouter.HandleFunc("/subscription/new", system.DefaultWrapper(apiServer.subscriptionCreate)).Methods(http.MethodPost)
	authRouter.HandleFunc("/subscription/manage", system.DefaultWrapper(apiServer.subscriptionManage)).Methods(http.MethodPost)

	authRouter.HandleFunc("/top-ups/new", system.DefaultWrapper(apiServer.createTopUp)).Methods(http.MethodPost)

	// Usage
	authRouter.HandleFunc("/usage", system.Wrapper(apiServer.getUsage)).Methods(http.MethodGet)

	// Security telemetry monitoring routes
	// TODO: Fix compilation errors in security_telemetry_handlers.go before enabling
	// apiServer.registerSecurityRoutes(subRouter)

	// OpenAI API compatible routes
	router.HandleFunc("/v1/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/embeddings", apiServer.authMiddleware.auth(apiServer.createEmbeddings)).Methods(http.MethodPost, http.MethodOptions)
	router.HandleFunc("/v1/models", apiServer.authMiddleware.auth(apiServer.listModels)).Methods(http.MethodGet)
	// Anthropic API compatible routes
	router.HandleFunc("/v1/messages", apiServer.authMiddleware.auth(apiServer.anthropicAPIProxyHandler)).Methods(http.MethodPost, http.MethodOptions)
	// Azure OpenAI API compatible routes
	router.HandleFunc("/openai/deployments/{model}/chat/completions", apiServer.authMiddleware.auth(apiServer.createChatCompletion)).Methods(http.MethodPost, http.MethodOptions)

	authRouter.HandleFunc("/providers", apiServer.listProviders).Methods(http.MethodGet)

	// Insecure router as unauthenticated users will see all public provider endpoints
	subRouter.HandleFunc("/provider-endpoints", apiServer.listProviderEndpoints).Methods(http.MethodGet)

	authRouter.HandleFunc("/provider-endpoints", apiServer.createProviderEndpoint).Methods(http.MethodPost)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.updateProviderEndpoint).Methods(http.MethodPut)
	authRouter.HandleFunc("/provider-endpoints/{id}", apiServer.deleteProviderEndpoint).Methods(http.MethodDelete)
	authRouter.HandleFunc("/provider-endpoints/{id}/daily-usage", apiServer.getProviderDailyUsage).Methods(http.MethodGet)
	authRouter.HandleFunc("/provider-endpoints/{id}/users-daily-usage", apiServer.getProviderUsersDailyUsage).Methods(http.MethodGet)
	// Helix inference route
	authRouter.HandleFunc("/sessions/chat", apiServer.startChatSessionHandler).Methods(http.MethodPost)

	authRouter.HandleFunc("/sessions", system.DefaultWrapper(apiServer.listSessions)).Methods(http.MethodGet)
	subRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.getSession)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.deleteSession)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/sessions/{id}", system.Wrapper(apiServer.updateSession)).Methods(http.MethodPut)
	authRouter.HandleFunc("/sessions/{id}/interactions", system.Wrapper(apiServer.listInteractions)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/interactions/{interaction_id}", system.Wrapper(apiServer.getInteraction)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/interactions/{interaction_id}/feedback", system.Wrapper(apiServer.feedbackInteraction)).Methods(http.MethodPost)

	authRouter.HandleFunc("/sessions/{id}/step-info", system.Wrapper(apiServer.getSessionStepInfo)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/rdp-connection", apiServer.getSessionRDPConnection).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/sandbox-state", apiServer.getSessionSandboxState).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/resume", apiServer.resumeSession).Methods(http.MethodPost)
	authRouter.HandleFunc("/sessions/{id}/idle-status", system.Wrapper(apiServer.getSessionIdleStatus)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/stop-external-agent", system.Wrapper(apiServer.stopExternalAgentSession)).Methods(http.MethodDelete)

	// Session TOC and turn-based navigation for agent context retrieval
	authRouter.HandleFunc("/sessions/{id}/toc", system.Wrapper(apiServer.getSessionTOC)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/turns/{turn}", system.Wrapper(apiServer.getInteractionByTurn)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/search", system.Wrapper(apiServer.searchSessionInteractions)).Methods(http.MethodGet)

	authRouter.HandleFunc("/question-sets", system.Wrapper(apiServer.listQuestionSets)).Methods(http.MethodGet)
	authRouter.HandleFunc("/question-sets", system.Wrapper(apiServer.createQuestionSet)).Methods(http.MethodPost)
	authRouter.HandleFunc("/question-sets/{id}", system.Wrapper(apiServer.getQuestionSet)).Methods(http.MethodGet)
	authRouter.HandleFunc("/question-sets/{id}", system.Wrapper(apiServer.updateQuestionSet)).Methods(http.MethodPut)
	authRouter.HandleFunc("/question-sets/{id}", system.Wrapper(apiServer.deleteQuestionSet)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/question-sets/{id}/executions", system.Wrapper(apiServer.executeQuestionSet)).Methods(http.MethodPost)
	authRouter.HandleFunc("/question-sets/{id}/executions", system.Wrapper(apiServer.listQuestionSetExecutions)).Methods(http.MethodGet)
	authRouter.HandleFunc("/question-sets/{question_set_id}/executions/{id}", apiServer.getQuestionSetExecutionResults).Methods(http.MethodGet)

	authRouter.HandleFunc("/secrets", system.Wrapper(apiServer.listSecrets)).Methods(http.MethodGet)
	authRouter.HandleFunc("/secrets", system.Wrapper(apiServer.createSecret)).Methods(http.MethodPost)
	authRouter.HandleFunc("/secrets/{id}", system.Wrapper(apiServer.updateSecret)).Methods(http.MethodPut)
	authRouter.HandleFunc("/secrets/{id}", system.Wrapper(apiServer.deleteSecret)).Methods(http.MethodDelete)

	// Prompt history endpoints (cross-device sync)
	authRouter.HandleFunc("/prompt-history", system.Wrapper(apiServer.listPromptHistory)).Methods(http.MethodGet)
	authRouter.HandleFunc("/prompt-history/sync", system.Wrapper(apiServer.syncPromptHistory)).Methods(http.MethodPost)
	authRouter.HandleFunc("/prompt-history/pinned", system.Wrapper(apiServer.listPinnedPrompts)).Methods(http.MethodGet)
	authRouter.HandleFunc("/prompt-history/search", system.Wrapper(apiServer.searchPrompts)).Methods(http.MethodGet)
	authRouter.HandleFunc("/prompt-history/{id}/pin", system.Wrapper(apiServer.updatePromptPin)).Methods(http.MethodPut)
	authRouter.HandleFunc("/prompt-history/{id}/tags", system.Wrapper(apiServer.updatePromptTags)).Methods(http.MethodPut)
	authRouter.HandleFunc("/prompt-history/{id}/use", system.Wrapper(apiServer.incrementPromptUsage)).Methods(http.MethodPost)

	// Unified search endpoint
	authRouter.HandleFunc("/search", system.Wrapper(apiServer.unifiedSearch)).Methods(http.MethodGet)

	// Zed config endpoints
	authRouter.HandleFunc("/sessions/{id}/zed-config", system.Wrapper(apiServer.getZedConfig)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sessions/{id}/zed-config/user", system.Wrapper(apiServer.updateZedUserSettings)).Methods(http.MethodPost)
	authRouter.HandleFunc("/sessions/{id}/zed-settings", system.Wrapper(apiServer.getMergedZedSettings)).Methods(http.MethodGet)

	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.listApps)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps", system.Wrapper(apiServer.createApp)).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.getApp)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.updateApp)).Methods(http.MethodPut)
	authRouter.HandleFunc("/apps/{id}", system.Wrapper(apiServer.deleteApp)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/apps/{id}/daily-usage", system.Wrapper(apiServer.getAppDailyUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/users-daily-usage", system.Wrapper(apiServer.getAppUsersDailyUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/llm-calls", system.Wrapper(apiServer.listAppLLMCalls)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/interactions", system.Wrapper(apiServer.listAppInteractions)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/step-info", system.Wrapper(apiServer.listAppStepInfo)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/api-actions", system.Wrapper(apiServer.appRunAPIAction)).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/user-access", system.Wrapper(apiServer.getAppUserAccess)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/access-grants", apiServer.listAppAccessGrants).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/access-grants", apiServer.createAppAccessGrant).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/access-grants/{grant_id}", apiServer.deleteAppAccessGrant).Methods(http.MethodDelete)
	authRouter.HandleFunc("/apps/{id}/duplicate", system.Wrapper(apiServer.duplicateApp)).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/memories", system.Wrapper(apiServer.listAppMemories)).Methods(http.MethodGet)
	authRouter.HandleFunc("/apps/{id}/memories/{memory_id}", system.Wrapper(apiServer.deleteAppMemory)).Methods(http.MethodDelete)

	authRouter.HandleFunc("/apps/{id}/triggers", system.Wrapper(apiServer.listAppTriggers)).Methods(http.MethodGet)

	// Triggers provide an ability for users to create recurring tasks for agents or
	// to connect an agent built by another user to their own slack/dicord/etc.
	authRouter.HandleFunc("/triggers", system.Wrapper(apiServer.listTriggers)).Methods(http.MethodGet)
	authRouter.HandleFunc("/triggers", system.Wrapper(apiServer.createAppTrigger)).Methods(http.MethodPost)
	authRouter.HandleFunc("/triggers/{trigger_id}", system.Wrapper(apiServer.updateAppTrigger)).Methods(http.MethodPut)
	authRouter.HandleFunc("/triggers/{trigger_id}", system.Wrapper(apiServer.deleteAppTrigger)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/triggers/{trigger_id}/execute", system.Wrapper(apiServer.executeAppTrigger)).Methods(http.MethodPost)

	authRouter.HandleFunc("/triggers/{trigger_id}/executions", system.Wrapper(apiServer.listTriggerExecutions)).Methods(http.MethodGet)

	// Avatar routes
	authRouter.HandleFunc("/apps/{id}/avatar", apiServer.uploadAppAvatar).Methods(http.MethodPost)
	authRouter.HandleFunc("/apps/{id}/avatar", apiServer.deleteAppAvatar).Methods(http.MethodDelete)
	// Anyone can get the avatar
	insecureRouter.HandleFunc("/apps/{id}/avatar", apiServer.getAppAvatar).Methods(http.MethodGet)

	// Trigger status routes
	authRouter.HandleFunc("/apps/{id}/trigger-status", apiServer.getAppTriggerStatus).Methods(http.MethodGet)

	authRouter.HandleFunc("/search", system.Wrapper(apiServer.knowledgeSearch)).Methods(http.MethodGet)

	authRouter.HandleFunc("/knowledge", system.Wrapper(apiServer.listKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.getKnowledge)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}", system.Wrapper(apiServer.deleteKnowledge)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/knowledge/{id}/refresh", system.Wrapper(apiServer.refreshKnowledge)).Methods(http.MethodPost)
	authRouter.HandleFunc("/knowledge/{id}/complete", system.Wrapper(apiServer.completeKnowledgePreparation)).Methods(http.MethodPost)
	authRouter.HandleFunc("/knowledge/{id}/versions", system.Wrapper(apiServer.listKnowledgeVersions)).Methods(http.MethodGet)
	authRouter.HandleFunc("/knowledge/{id}/download", apiServer.downloadKnowledgeFiles).Methods(http.MethodGet)

	// Skill routes
	authRouter.HandleFunc("/skills", system.DefaultWrapper(apiServer.handleListSkills)).Methods("GET")
	authRouter.HandleFunc("/skills/{id}", system.DefaultWrapper(apiServer.handleGetSkill)).Methods("GET")
	authRouter.HandleFunc("/skills/reload", system.DefaultWrapper(apiServer.handleReloadSkills)).Methods("POST")
	authRouter.HandleFunc("/skills/validate", system.DefaultWrapper(apiServer.handleValidateMcpSkill)).Methods("POST")

	// External agent routes
	authRouter.HandleFunc("/external-agents", apiServer.createExternalAgent).Methods("POST")
	authRouter.HandleFunc("/external-agents", apiServer.listExternalAgents).Methods("GET")
	// Specific routes must come before parametric routes
	authRouter.HandleFunc("/external-agents/connections", apiServer.getExternalAgentConnections).Methods("GET")
	authRouter.HandleFunc("/external-agents/sync", apiServer.handleExternalAgentSync).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}", apiServer.getExternalAgent).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}", apiServer.updateExternalAgent).Methods("PUT")
	authRouter.HandleFunc("/external-agents/{sessionID}", apiServer.deleteExternalAgent).Methods("DELETE")
	authRouter.HandleFunc("/external-agents/{sessionID}/rdp", apiServer.getExternalAgentRDP).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}/stats", apiServer.getExternalAgentStats).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}/screenshot", apiServer.getExternalAgentScreenshot).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}/bandwidth-probe", apiServer.getBandwidthProbe).Methods("GET")
	authRouter.HandleFunc("/bandwidth-probe", apiServer.getInitialBandwidthProbe).Methods("GET") // Initial probe (no session required)
	authRouter.HandleFunc("/external-agents/{sessionID}/clipboard", apiServer.getExternalAgentClipboard).Methods("GET")
	authRouter.HandleFunc("/external-agents/{sessionID}/clipboard", apiServer.setExternalAgentClipboard).Methods("POST")
	authRouter.HandleFunc("/external-agents/{sessionID}/upload", apiServer.uploadFileToSandbox).Methods("POST")
	authRouter.HandleFunc("/external-agents/{sessionID}/input", apiServer.sendInputToSandbox).Methods("POST")
	authRouter.HandleFunc("/external-agents/{sessionID}/exec", apiServer.execInSandbox).Methods("POST") // Execute safe commands in sandbox (vkcube, glxgears)
	authRouter.HandleFunc("/external-agents/{sessionID}/ws/input", apiServer.proxyInputWebSocket).Methods("GET")   // Direct WebSocket input
	authRouter.HandleFunc("/external-agents/{sessionID}/ws/stream", apiServer.proxyStreamWebSocket).Methods("GET") // Direct WebSocket video streaming
	authRouter.HandleFunc("/external-agents/{sessionID}/configure-pending-session", apiServer.configurePendingSession).Methods("POST")

	// Sandbox instance registry routes (multi-sandbox support)
	authRouter.HandleFunc("/sandboxes/register", apiServer.registerSandbox).Methods("POST")
	authRouter.HandleFunc("/sandboxes/{id}/heartbeat", apiServer.sandboxHeartbeat).Methods("POST")
	authRouter.HandleFunc("/sandboxes/{id}/disk-history", apiServer.getDiskUsageHistory).Methods("GET")
	authRouter.HandleFunc("/sandboxes", apiServer.listSandboxes).Methods("GET")
	authRouter.HandleFunc("/sandboxes/{id}", apiServer.deregisterSandbox).Methods("DELETE")
	// Reverse dial endpoint for user sandboxes (spec tasks, PDEs)
	// Accepts user API tokens with session ownership validation
	authRouter.Handle("/revdial", apiServer.handleRevDial()).Methods("GET")

	// RDP proxy management endpoints
	// Note: RDP proxy health endpoint removed - not implemented

	// External agent WebSocket runner endpoint
	apiServer.startExternalAgentRunnerWebSocketServer(subRouter, "/ws/external-agent-runner")

	authRouter.HandleFunc("/external-agents/{sessionID}/command", apiServer.sendCommandToExternalAgentHandler).Methods("POST")

	// Agent Sandboxes debugging routes (admin only)
	authRouter.HandleFunc("/admin/agent-sandboxes/debug", apiServer.getAgentSandboxesDebug).Methods("GET")
	authRouter.HandleFunc("/admin/agent-sandboxes/events", apiServer.getAgentSandboxesEvents).Methods("GET")

	// UI @ functionality
	authRouter.HandleFunc("/context-menu", system.Wrapper(apiServer.contextMenuHandler)).Methods(http.MethodGet)

	// User auth, BFF
	insecureRouter.HandleFunc("/auth/register", apiServer.register).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/login", apiServer.login).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/callback", apiServer.callback).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/user", apiServer.user).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/logout", apiServer.logout).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/authenticated", apiServer.authenticated).Methods(http.MethodGet)
	insecureRouter.HandleFunc("/auth/refresh", apiServer.refresh).Methods(http.MethodPost)

	insecureRouter.HandleFunc("/auth/password-reset", apiServer.passwordReset).Methods(http.MethodPost)
	insecureRouter.HandleFunc("/auth/password-reset-complete", apiServer.passwordResetComplete).Methods(http.MethodPost)

	authRouter.HandleFunc("/auth/password-update", apiServer.passwordUpdate).Methods(http.MethodPost) // Update for authenticated users
	authRouter.HandleFunc("/auth/update", apiServer.accountUpdate).Methods(http.MethodPost)           // Update for authenticated users

	// Orgs, authz
	authRouter.HandleFunc("/organizations", apiServer.listOrganizations).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations", apiServer.createOrganization).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}", apiServer.getOrganization).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}", apiServer.updateOrganization).Methods(http.MethodPut)
	authRouter.HandleFunc("/organizations/{id}", apiServer.deleteOrganization).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/members", apiServer.listOrganizationMembers).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/members", apiServer.addOrganizationMember).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/members/{user_id}", apiServer.removeOrganizationMember).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/members/{user_id}", apiServer.updateOrganizationMember).Methods(http.MethodPut)

	authRouter.HandleFunc("/organizations/{id}/roles", apiServer.listOrganizationRoles).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/guidelines-history", apiServer.getOrganizationGuidelinesHistory).Methods(http.MethodGet)

	// Teams
	authRouter.HandleFunc("/organizations/{id}/teams", apiServer.listTeams).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/teams", apiServer.createTeam).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}", apiServer.updateTeam).Methods(http.MethodPut)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}", apiServer.deleteTeam).Methods(http.MethodDelete)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members", apiServer.listTeamMembers).Methods(http.MethodGet)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members", apiServer.addTeamMember).Methods(http.MethodPost)
	authRouter.HandleFunc("/organizations/{id}/teams/{team_id}/members/{user_id}", apiServer.removeTeamMember).Methods(http.MethodDelete)

	adminRouter.HandleFunc("/dashboard", system.DefaultWrapper(apiServer.dashboard)).Methods(http.MethodGet)
	adminRouter.HandleFunc("/users", system.DefaultWrapper(apiServer.usersList)).Methods(http.MethodGet)
	adminRouter.HandleFunc("/users", system.DefaultWrapper(apiServer.createUser)).Methods(http.MethodPost)
	adminRouter.HandleFunc("/admin/users/{id}/password", system.DefaultWrapper(apiServer.adminResetPassword)).Methods(http.MethodPut)
	adminRouter.HandleFunc("/admin/users/{id}", system.DefaultWrapper(apiServer.adminDeleteUser)).Methods(http.MethodDelete)
	adminRouter.HandleFunc("/scheduler/heartbeats", system.DefaultWrapper(apiServer.getSchedulerHeartbeats)).Methods(http.MethodGet)
	adminRouter.HandleFunc("/llm_calls", system.Wrapper(apiServer.listLLMCalls)).Methods(http.MethodGet)
	authRouter.HandleFunc("/slots/{slot_id}", system.DefaultWrapper(apiServer.deleteSlot)).Methods(http.MethodDelete)

	// Logs endpoints - proxy to runner
	adminRouter.HandleFunc("/logs", apiServer.getLogsSummary).Methods(http.MethodGet)
	adminRouter.HandleFunc("/logs/{slot_id}", apiServer.getSlotLogs).Methods(http.MethodGet)

	// Helix models
	authRouter.HandleFunc("/helix-models", apiServer.listHelixModels).Methods(http.MethodGet)
	// Memory estimation endpoints
	authRouter.HandleFunc("/helix-models/memory-estimate", apiServer.estimateModelMemory).Methods(http.MethodGet)
	authRouter.HandleFunc("/helix-models/memory-estimates", apiServer.listModelMemoryEstimates).Methods(http.MethodGet)
	// only admins can create, update, or delete helix models
	adminRouter.HandleFunc("/helix-models", apiServer.createHelixModel).Methods(http.MethodPost)
	adminRouter.HandleFunc("/helix-models/{id:.*}", apiServer.updateHelixModel).Methods(http.MethodPut)
	adminRouter.HandleFunc("/helix-models/{id:.*}", apiServer.deleteHelixModel).Methods(http.MethodDelete)

	// Dynamic model info - all operations require admin privileges
	adminRouter.HandleFunc("/model-info", apiServer.listDynamicModelInfos).Methods(http.MethodGet)
	adminRouter.HandleFunc("/model-info", apiServer.createDynamicModelInfo).Methods(http.MethodPost)
	adminRouter.HandleFunc("/model-info/{id:.*}", apiServer.getDynamicModelInfo).Methods(http.MethodGet)
	adminRouter.HandleFunc("/model-info/{id:.*}", apiServer.updateDynamicModelInfo).Methods(http.MethodPut)
	adminRouter.HandleFunc("/model-info/{id:.*}", apiServer.deleteDynamicModelInfo).Methods(http.MethodDelete)

	// System settings - only admins can access
	adminRouter.HandleFunc("/system/settings", apiServer.getSystemSettings).Methods(http.MethodGet)
	adminRouter.HandleFunc("/system/settings", apiServer.updateSystemSettings).Methods(http.MethodPut)

	// all these routes are secured via runner tokens
	insecureRouter.HandleFunc("/runner/{runner_id}/ws", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		runnerID := vars["runner_id"]
		log.Info().
			Str("runner_id", runnerID).
			Str("request_path", r.URL.Path).
			Msg("proxying runner websocket request to NATS")

		defer log.Info().Str("runner_id", runnerID).Msg("websocket proxy to NATS disconnected")

		// Upgrade the incoming HTTP connection to a WebSocket connection.
		upgrader := websocket.Upgrader{
			// TODO(Phil): check origin
			CheckOrigin: func(r *http.Request) bool {
				log.Debug().Interface("headers", r.Header).Interface("vars", r.RemoteAddr).Msg("nats check origin")
				return true
			},
		}

		clientConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().Err(err).Msg("Failed to upgrade client connection")
			return
		}
		// Ensure the client connection is closed on function exit.
		defer clientConn.Close()

		// Connect to the backend WebSocket server.
		backendConn, resp, err := websocket.DefaultDialer.Dial("ws://localhost:8433", nil) // TODO(Phil): make this configurable
		if err != nil {
			log.Error().Err(err).Msg("Failed to connect to backend WebSocket server")
			return
		}
		// Ensure the backend connection is closed on function exit.
		defer backendConn.Close()
		defer resp.Body.Close()

		// Mutex for thread-safe writes to client WebSocket (ping and backend→client can race)
		var clientMu sync.Mutex

		// Start three goroutines: client→backend, backend→client, and ping
		errCh := make(chan error, 3)

		// Start server-initiated ping goroutine to keep client connection alive through proxies/firewalls
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				<-ticker.C
				clientMu.Lock()
				err := clientConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
				clientMu.Unlock()
				if err != nil {
					log.Debug().Err(err).Str("runner_id", runnerID).Msg("NATS WebSocket proxy ping failed, connection closing")
					errCh <- nil
					return
				}
			}
		}()

		// Copy messages from the client to the backend.
		go func() {
			for {
				messageType, message, err := clientConn.ReadMessage()
				if err != nil {
					errCh <- err
					return
				}
				if err := backendConn.WriteMessage(messageType, message); err != nil {
					errCh <- err
					return
				}
			}
		}()

		// Copy messages from the backend to the client.
		go func() {
			for {
				messageType, message, err := backendConn.ReadMessage()
				if err != nil {
					errCh <- err
					return
				}
				// Mutex protects against concurrent ping writes
				clientMu.Lock()
				writeErr := clientConn.WriteMessage(messageType, message)
				clientMu.Unlock()
				if writeErr != nil {
					errCh <- writeErr
					return
				}
			}
		}()

		// Wait until one side returns an error (or closes the connection).
		if err := <-errCh; err != nil {
			log.Printf("WebSocket proxy error: %v", err)
		}
	})
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/file", apiServer.runnerSessionDownloadFile).Methods(http.MethodGet)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/download/folder", apiServer.runnerSessionDownloadFolder).Methods(http.MethodGet)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/files", system.DefaultWrapper(apiServer.runnerSessionUploadFiles)).Methods(http.MethodPost)
	runnerRouter.HandleFunc("/runner/{runnerid}/session/{sessionid}/upload/folder", system.DefaultWrapper(apiServer.runnerSessionUploadFolder)).Methods(http.MethodPost)

	// register pprof routes
	router.PathPrefix("/debug/pprof/").Handler(http.DefaultServeMux)

	// Register Git HTTP protocol routes for clone/push operations BEFORE default handler
	// These routes don't use authRouter - they have their own auth middleware
	// IMPORTANT: Must be before registerDefaultHandler to avoid being proxied to frontend
	apiServer.gitHTTPServer.RegisterRoutes(router)

	// Set a custom NotFoundHandler for /api/v1/ routes to log unknown paths
	subRouter.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Error().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Msg("unknown API path")
		http.Error(w, "Not Found", http.StatusNotFound)
	})

	// proxy other routes to frontend (MUST BE LAST - catch-all handler)
	apiServer.registerDefaultHandler(router)

	// only admins can manage licenses
	adminRouter.HandleFunc("/license", apiServer.handleGetLicenseKey).Methods("GET")
	adminRouter.HandleFunc("/license", apiServer.handleSetLicenseKey).Methods("POST")

	// OAuth routes
	// These routes are already set up by apiServer.setupOAuthRoutes(authRouter) above

	// Project routes
	authRouter.HandleFunc("/projects", system.Wrapper(apiServer.listProjects)).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects", system.Wrapper(apiServer.createProject)).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}", system.Wrapper(apiServer.getProject)).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}", system.Wrapper(apiServer.updateProject)).Methods(http.MethodPut)
	authRouter.HandleFunc("/projects/{id}", system.Wrapper(apiServer.deleteProject)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/projects/{id}/repositories", system.Wrapper(apiServer.getProjectRepositories)).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/repositories/{repo_id}/primary", system.Wrapper(apiServer.setProjectPrimaryRepository)).Methods(http.MethodPut)
	authRouter.HandleFunc("/projects/{id}/repositories/{repo_id}/attach", system.Wrapper(apiServer.attachRepositoryToProject)).Methods(http.MethodPut)
	authRouter.HandleFunc("/projects/{id}/repositories/{repo_id}/detach", system.Wrapper(apiServer.detachRepositoryFromProject)).Methods(http.MethodPut)
	authRouter.HandleFunc("/projects/{id}/exploratory-session", system.Wrapper(apiServer.getProjectExploratorySession)).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/exploratory-session", system.Wrapper(apiServer.startExploratorySession)).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/exploratory-session", system.Wrapper(apiServer.stopExploratorySession)).Methods(http.MethodDelete)
	authRouter.HandleFunc("/projects/{id}/startup-script/history", system.Wrapper(apiServer.getProjectStartupScriptHistory)).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/guidelines-history", system.Wrapper(apiServer.getProjectGuidelinesHistory)).Methods(http.MethodGet)

	// Project access grant routes
	authRouter.HandleFunc("/projects/{id}/access-grants", apiServer.listProjectAccessGrants).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/access-grants", apiServer.createProjectAccessGrant).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/access-grants/{grant_id}", apiServer.deleteProjectAccessGrant).Methods(http.MethodDelete)

	// Project audit log routes
	authRouter.HandleFunc("/projects/{id}/audit-logs", system.Wrapper(apiServer.listProjectAuditLogs)).Methods(http.MethodGet)

	// Sample project routes (simple in-memory)
	authRouter.HandleFunc("/sample-projects/simple", system.Wrapper(apiServer.listSimpleSampleProjects)).Methods(http.MethodGet)
	authRouter.HandleFunc("/sample-projects/simple/fork", system.Wrapper(apiServer.forkSimpleProject)).Methods(http.MethodPost)

	// Spec-driven task routes
	authRouter.HandleFunc("/spec-tasks/from-prompt", apiServer.createTaskFromPrompt).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks", apiServer.listTasks).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{taskId}", apiServer.getTask).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{taskId}", apiServer.updateSpecTask).Methods(http.MethodPut)
	authRouter.HandleFunc("/spec-tasks/{taskId}/archive", apiServer.archiveSpecTask).Methods(http.MethodPatch)
	authRouter.HandleFunc("/spec-tasks/{taskId}/specs", apiServer.getTaskSpecs).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{taskId}/progress", apiServer.getTaskProgress).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{taskId}/start-planning", apiServer.startPlanning).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{taskId}/approve-specs", apiServer.approveSpecs).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{taskId}/clone", apiServer.cloneSpecTask).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{taskId}/clone-groups", apiServer.listCloneGroups).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{id}/usage", system.Wrapper(apiServer.getSpecTaskUsage)).Methods(http.MethodGet)
	authRouter.HandleFunc("/clone-groups/{groupId}/progress", apiServer.getCloneGroupProgress).Methods(http.MethodGet)
	authRouter.HandleFunc("/repositories/without-projects", apiServer.listReposWithoutProjects).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/quick-create", apiServer.quickCreateProject).Methods(http.MethodPost)

	// Workflow automation routes
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/approve-implementation", apiServer.approveImplementation).Methods(http.MethodPost) // MOVE
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/stop-agent", apiServer.stopAgentSession).Methods(http.MethodPost)

	// Design review routes
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews", apiServer.listDesignReviews).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}", apiServer.getDesignReview).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}/submit", apiServer.submitDesignReview).Methods(http.MethodPost) // TODO: move
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments", apiServer.createDesignReviewComment).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments", apiServer.listDesignReviewComments).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments/{comment_id}/resolve", apiServer.resolveDesignReviewComment).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comment-queue-status", apiServer.getDesignReviewCommentQueueStatus).Methods(http.MethodGet)

	// Zed integration routes
	authRouter.HandleFunc("/zed/events", apiServer.handleZedInstanceEvent).Methods(http.MethodPost)
	authRouter.HandleFunc("/zed/instances/{instanceId}/threads/{threadId}/events", apiServer.handleZedThreadEvent).Methods(http.MethodPost)
	authRouter.HandleFunc("/zed/instances/{instanceId}/heartbeat", apiServer.handleZedConnectionHeartbeat).Methods(http.MethodPost)
	authRouter.HandleFunc("/zed/threads/{threadId}/activity", apiServer.updateZedThreadActivity).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{taskId}/zed-instance", apiServer.getZedInstanceStatus).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{taskId}/zed-instance", apiServer.shutdownZedInstance).Methods(http.MethodDelete)
	authRouter.HandleFunc("/spec-tasks/{taskId}/zed-threads", apiServer.listZedThreads).Methods(http.MethodGet)
	authRouter.HandleFunc("/work-sessions/{sessionId}/zed-thread", apiServer.createZedThreadForWorkSession).Methods(http.MethodPost)

	// Git repository routes (actual git repository management)
	authRouter.HandleFunc("/git/repositories", apiServer.createGitRepository).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories", apiServer.listGitRepositories).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}", apiServer.getGitRepository).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}", apiServer.updateGitRepository).Methods(http.MethodPut)
	authRouter.HandleFunc("/git/repositories/{id}", apiServer.deleteGitRepository).Methods(http.MethodDelete)
	authRouter.HandleFunc("/git/repositories/{id}/clone-command", apiServer.getGitRepositoryCloneCommand).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/branches", apiServer.listGitRepositoryBranches).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/branches", apiServer.createGitRepositoryBranch).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories/{id}/tree", apiServer.browseGitRepositoryTree).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/contents", apiServer.getGitRepositoryFileContents).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/contents", apiServer.createOrUpdateGitRepositoryFileContents).Methods(http.MethodPut)
	authRouter.HandleFunc("/git/repositories/{id}/enrichments", apiServer.getRepositoryEnrichments).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/enrichments/{enrichmentId}", apiServer.getEnrichment).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/kodit-commits", apiServer.getRepositoryKoditCommits).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/search-snippets", apiServer.searchRepositorySnippets).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/kodit-status", apiServer.getRepositoryIndexingStatus).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/push-pull", apiServer.pushPullGitRepository).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories/{id}/pull", apiServer.pullFromRemote).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories/{id}/push", apiServer.pushToRemote).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories/{id}/commits", apiServer.listGitRepositoryCommits).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/pull-requests", apiServer.listGitRepositoryPullRequests).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/pull-requests", apiServer.createGitRepositoryPullRequest).Methods(http.MethodPost)

	// Browse remote repositories using PAT credentials (without needing OAuth)
	authRouter.HandleFunc("/git/browse-remote", apiServer.browseRemoteRepositories).Methods(http.MethodPost)

	// Git provider connections - persistent PAT-based connections for browsing repositories
	authRouter.HandleFunc("/git-provider-connections", apiServer.listGitProviderConnections).Methods(http.MethodGet)
	authRouter.HandleFunc("/git-provider-connections", apiServer.createGitProviderConnection).Methods(http.MethodPost)
	authRouter.HandleFunc("/git-provider-connections/{id}", apiServer.deleteGitProviderConnection).Methods(http.MethodDelete)
	authRouter.HandleFunc("/git-provider-connections/{id}/repositories", apiServer.browseGitProviderConnectionRepositories).Methods(http.MethodGet)

	// Service connections - admin-configured GitHub Apps, ADO Service Principals, etc.
	authRouter.HandleFunc("/service-connections", apiServer.listServiceConnections).Methods(http.MethodGet)
	authRouter.HandleFunc("/service-connections", apiServer.createServiceConnection).Methods(http.MethodPost)
	authRouter.HandleFunc("/service-connections/{id}", apiServer.getServiceConnection).Methods(http.MethodGet)
	authRouter.HandleFunc("/service-connections/{id}", apiServer.updateServiceConnection).Methods(http.MethodPut)
	authRouter.HandleFunc("/service-connections/{id}", apiServer.deleteServiceConnection).Methods(http.MethodDelete)
	authRouter.HandleFunc("/service-connections/{id}/test", apiServer.testServiceConnectionEndpoint).Methods(http.MethodPost)

	// Git repository access grant routes
	authRouter.HandleFunc("/git/repositories/{id}/access-grants", apiServer.listRepositoryAccessGrants).Methods(http.MethodGet)
	authRouter.HandleFunc("/git/repositories/{id}/access-grants", apiServer.createRepositoryAccessGrant).Methods(http.MethodPost)
	authRouter.HandleFunc("/git/repositories/{id}/access-grants/{grant_id}", apiServer.deleteRepositoryAccessGrant).Methods(http.MethodDelete)

	// MCP Gateway routes - unified endpoint for all MCP backends (Kodit, Helix native, etc.)
	// Supports both streamable HTTP and SSE transports for MCP protocol
	// Route: /api/v1/mcp/{server}/{path...} where server is "kodit", "helix", etc.
	authRouter.HandleFunc("/mcp/{server}", apiServer.mcpGatewayHandler).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/mcp/{server}/{path:.*}", apiServer.mcpGatewayHandler).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	// Legacy Kodit MCP routes (redirect to gateway) - keep for backwards compatibility
	authRouter.HandleFunc("/kodit/mcp", apiServer.koditMCPProxy).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/kodit/mcp/{path:.*}", apiServer.koditMCPProxy).Methods(http.MethodGet, http.MethodPost, http.MethodOptions)

	// Spec-driven task routes
	authRouter.HandleFunc("/specs/sample-types", apiServer.getSampleTypes).Methods(http.MethodGet)

	// SpecTask orchestrator routes
	authRouter.HandleFunc("/spec-tasks/from-demo", system.Wrapper(apiServer.createSpecTaskFromDemo)).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{id}/design-docs", system.Wrapper(apiServer.getSpecTaskDesignDocs)).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{id}/external-agent/status", apiServer.getSpecTaskExternalAgentStatus).Methods(http.MethodGet)
	authRouter.HandleFunc("/spec-tasks/{id}/external-agent/start", apiServer.startSpecTaskExternalAgent).Methods(http.MethodPost)
	authRouter.HandleFunc("/spec-tasks/{id}/external-agent/stop", apiServer.stopSpecTaskExternalAgent).Methods(http.MethodPost)

	// SpecTask shareable design docs (authenticated)
	authRouter.HandleFunc("/spec-tasks/{id}/design-docs/share", system.Wrapper(apiServer.generateDesignDocsShareLink)).Methods(http.MethodPost)

	// Public design docs viewer (token-based, no auth)
	subRouter.HandleFunc("/spec-tasks/{id}/view", apiServer.viewDesignDocsPublic).Methods(http.MethodGet)

	// Sample repository routes
	authRouter.HandleFunc("/samples/repositories", apiServer.createSampleRepository).Methods(http.MethodPost)
	authRouter.HandleFunc("/samples/initialize", apiServer.initializeSampleRepositories).Methods(http.MethodPost)

	apiServer.router = router

	// Initialize skills
	log.Info().Msg("Loading YAML skills")
	ctx := context.Background()
	if err := apiServer.skillManager.LoadSkills(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to load skills, continuing without them")
	}

	return subRouter, nil
}

func getID(r *http.Request) string {
	vars := mux.Vars(r)
	return vars["id"]
}

// Static files router
func (apiServer *HelixAPIServer) registerDefaultHandler(router *mux.Router) {

	// if we are in prod - then the frontend has been burned into the filesystem of the container
	// and the FrontendURL will actually have the value "/www"
	// so this switch is "are we in dev or not"
	if strings.HasPrefix(apiServer.Cfg.WebServer.FrontendURL, "http://") || strings.HasPrefix(apiServer.Cfg.WebServer.FrontendURL, "https://") {

		router.PathPrefix("/").Handler(spa.NewSPAReverseProxyServer(
			apiServer.Cfg.WebServer.FrontendURL,
		))
	} else {
		log.Info().Msgf("serving static UI files from %s", apiServer.Cfg.WebServer.FrontendURL)

		fileSystem := http.Dir(apiServer.Cfg.WebServer.FrontendURL)

		router.PathPrefix("/").Handler(spa.NewSPAFileServer(fileSystem))
	}
}

func writeResponse(rw http.ResponseWriter, data interface{}, statusCode int) {
	rw.Header().Set("Content-Type", "application/json")

	rw.WriteHeader(statusCode)

	if data == nil {
		return
	}

	err := json.NewEncoder(rw).Encode(data)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
	}
}

func writeErrResponse(rw http.ResponseWriter, err error, statusCode int) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(statusCode)

	_ = json.NewEncoder(rw).Encode(&system.HTTPError{
		StatusCode: statusCode,
		Message:    err.Error(),
	})
}

// startEmbeddingsSocketServer starts a UNIX socket server that serves just the /v1/embeddings endpoint with no auth
func (apiServer *HelixAPIServer) startEmbeddingsSocketServer(ctx context.Context) error {
	socketPath := apiServer.Cfg.WebServer.EmbeddingsSocket

	// Remove socket file if it already exists
	if _, err := os.Stat(socketPath); err == nil {
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("failed to remove existing socket file: %w", err)
		}
	}

	// Create socket listener
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket: %w", err)
	}

	// Set socket permissions
	if err := os.Chmod(socketPath, 0666); err != nil {
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Create a new router for the socket server
	router := mux.NewRouter()

	router.Use(ErrorLoggingMiddleware)

	// If configured, load user from database and set in request context
	if apiServer.Cfg.WebServer.EmbeddingsSocketUserID != "" {
		user, err := apiServer.Store.GetUser(ctx, &store.GetUserQuery{
			ID: apiServer.Cfg.WebServer.EmbeddingsSocketUserID,
		})
		if err != nil {
			return fmt.Errorf("failed to get user for socket: %w", err)
		}

		log.Info().
			Str("user_id", apiServer.Cfg.WebServer.EmbeddingsSocketUserID).
			Str("user_email", user.Email).
			Msg("setting user for embeddings socket")

		router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Set user to the request context
				r = r.WithContext(setRequestUser(r.Context(), *user))
				next.ServeHTTP(w, r)
			})
		})
	}

	// Register only the necessary endpoints with no auth
	router.HandleFunc("/v1/embeddings", apiServer.createEmbeddings).Methods(http.MethodPost, http.MethodOptions)

	// Add models endpoint to allow checking available models
	router.HandleFunc("/v1/models", apiServer.listModels).Methods(http.MethodGet, http.MethodOptions)

	// Add chat completions endpoint for Haystack LLM access
	router.HandleFunc("/v1/chat/completions", apiServer.createChatCompletion).Methods(http.MethodPost, http.MethodOptions)

	// Create HTTP server
	srv := &http.Server{
		Handler:      router,
		ReadTimeout:  0, // No timeout for long-running operations
		WriteTimeout: 0, // No timeout for streaming responses
	}

	log.Info().Str("socket", socketPath).Msg("starting embeddings socket server")

	// Ensure the server is shut down when the context is canceled
	go func() {
		<-ctx.Done()
		log.Info().Msg("shutting down embeddings socket server")
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("error shutting down embeddings socket server")
		}
		if err := listener.Close(); err != nil {
			log.Error().Err(err).Msg("error closing embeddings socket listener")
		}
	}()

	// Start the server
	if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("embeddings socket server error: %w", err)
	}

	return nil
}

// startExternalAgentRunnerWebSocketServer starts a WebSocket server for external agent runners
// Follows the exact same pattern as GPTScript runner for consistency
func (apiServer *HelixAPIServer) startExternalAgentRunnerWebSocketServer(r *mux.Router, path string) {
	r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		// Extract authentication and runner info from query parameters (like GPTScript)
		runnerID := r.URL.Query().Get("runnerid")
		accessToken := r.URL.Query().Get("access_token")
		concurrencyStr := r.URL.Query().Get("concurrency")

		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_connection_attempt").
			Str("path", path).
			Str("remote_addr", r.RemoteAddr).
			Str("runner_id", runnerID).
			Msg("🔗 EXTERNAL_AGENT_DEBUG: External agent runner attempting WebSocket connection")

		if runnerID == "" {
			log.Error().
				Str("EXTERNAL_AGENT_DEBUG", "missing_runner_id").
				Msg("❌ EXTERNAL_AGENT_DEBUG: runnerid is required")
			http.Error(w, "runnerid is required", http.StatusBadRequest)
			return
		}
		if accessToken == "" {
			log.Error().
				Str("EXTERNAL_AGENT_DEBUG", "missing_access_token").
				Msg("❌ EXTERNAL_AGENT_DEBUG: access_token is required")
			http.Error(w, "access_token is required", http.StatusBadRequest)
			return
		}

		// Validate the runner token (like GPTScript does)
		if accessToken != apiServer.Cfg.WebServer.RunnerToken {
			log.Warn().
				Str("EXTERNAL_AGENT_DEBUG", "invalid_token").
				Str("provided_token", accessToken).
				Str("expected_token", apiServer.Cfg.WebServer.RunnerToken).
				Str("runner_id", runnerID).
				Msg("❌ EXTERNAL_AGENT_DEBUG: Invalid runner token for external agent runner")
			http.Error(w, "Invalid access token", http.StatusUnauthorized)
			return
		}

		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "auth_success").
			Str("runner_id", runnerID).
			Msg("✅ EXTERNAL_AGENT_DEBUG: External agent runner authenticated successfully")

		wsConn, err := userWebsocketUpgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Error().
				Str("EXTERNAL_AGENT_DEBUG", "websocket_upgrade_error").
				Err(err).
				Msg("❌ EXTERNAL_AGENT_DEBUG: Error upgrading external agent runner websocket")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer wsConn.Close()

		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "websocket_upgraded").
			Msg("🔌 EXTERNAL_AGENT_DEBUG: WebSocket connection upgraded successfully")

		// Set initial read deadline to prevent hanging connections
		const readTimeout = 60 * time.Second
		wsConn.SetReadDeadline(time.Now().Add(readTimeout))

		// Set up ping handler to track when runner sends pings to us
		wsConn.SetPingHandler(func(appData string) error {
			// log.Info().
			// 	Str("EXTERNAL_AGENT_DEBUG", "ping_received").
			// 	Str("runner_id", runnerID).
			// 	Str("app_data", appData).
			// 	Msg("🏓 EXTERNAL_AGENT_DEBUG: Received ping from external agent runner")

			// Update last ping time in connection manager
			if apiServer.externalAgentRunnerManager != nil {
				apiServer.externalAgentRunnerManager.updatePingByRunner(runnerID)
				// log.Info().
				// 	Str("EXTERNAL_AGENT_DEBUG", "ping_timestamp_updated").
				// 	Str("runner_id", runnerID).
				// 	Msg("🏓 EXTERNAL_AGENT_DEBUG: Updated last ping timestamp in connection manager")
			} else {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "no_connection_manager").
					Str("runner_id", runnerID).
					Msg("❌ EXTERNAL_AGENT_DEBUG: No external agent runner manager available to update ping")
			}

			// Refresh read deadline on ping to keep connection alive
			wsConn.SetReadDeadline(time.Now().Add(readTimeout))

			// Send pong response back to runner (this is what the default ping handler does)
			err := wsConn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
			if err != nil {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "pong_send_error").
					Str("runner_id", runnerID).
					Err(err).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to send pong response")
			} else {
				log.Info().
					Str("EXTERNAL_AGENT_DEBUG", "pong_sent").
					Str("runner_id", runnerID).
					Msg("🏓 EXTERNAL_AGENT_DEBUG: Sent pong response to external agent runner")
			}

			return nil
		})

		concurrency := 1
		if concurrencyStr != "" {
			if c, err := strconv.Atoi(concurrencyStr); err == nil {
				concurrency = c
			} else {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "concurrency_parse_error").
					Err(err).
					Str("concurrency_str", concurrencyStr).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Error parsing concurrency")
			}
		}

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Mutex for thread-safe WebSocket writes (pings + NATS messages can race)
		var wsMu sync.Mutex

		// Start server-initiated ping goroutine to keep connection alive through proxies/firewalls
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					wsMu.Lock()
					err := wsConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
					wsMu.Unlock()
					if err != nil {
						log.Debug().
							Err(err).
							Str("runner_id", runnerID).
							Msg("Failed to send ping to external agent runner, connection may be closing")
						return
					}
				}
			}
		}()

		// Declare connectionID before defer so it's in scope
		var connectionID string

		defer func() {
			// Update runner status to offline in store
			if updateErr := apiServer.Store.UpdateAgentRunnerStatus(ctx, runnerID, "offline"); updateErr != nil {
				log.Warn().Err(updateErr).Str("runner_id", runnerID).Msg("Failed to update runner status to offline")
			}

			// Remove the connection from the runner manager
			if connectionID != "" {
				apiServer.externalAgentRunnerManager.removeConnection(runnerID, connectionID)
			}

			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "runner_disconnect").
				Str("runner_id", runnerID).
				Msg("🟠 EXTERNAL_AGENT_DEBUG: External agent runner disconnected")
		}()

		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "runner_connected").
			Str("action", "🟢 External agent runner connected").
			Str("runner_id", runnerID).
			Int("concurrency", concurrency).
			Msg("🎉 EXTERNAL_AGENT_DEBUG: Connected external agent runner websocket")

		// Track the connection in the runner manager
		connectionID = apiServer.externalAgentRunnerManager.addConnection(runnerID, concurrency)

		// Create or update agent runner in store with new RDP password
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "creating_or_updating_runner_in_store").
			Str("runner_id", runnerID).
			Msg("💾 EXTERNAL_AGENT_DEBUG: Creating or updating agent runner in store")

		agentRunner, err := apiServer.Store.GetOrCreateAgentRunner(ctx, runnerID)
		if err != nil {
			log.Error().
				Err(err).
				Str("runner_id", runnerID).
				Msg("Failed to create or update agent runner in store")
			// Continue anyway - don't fail the connection for this
		} else {
			log.Info().
				Str("runner_id", runnerID).
				Str("status", agentRunner.Status).
				Msg("✅ Agent runner created/updated in store")

			// Update runner status to online
			err = apiServer.Store.UpdateAgentRunnerStatus(ctx, runnerID, "online")
			if err != nil {
				log.Warn().Err(err).Str("runner_id", runnerID).Msg("Failed to update runner status to online")
			}
		}

		// Subscribe to Zed agent tasks (using ZedAgentRunnerStream like GPTScript uses ScriptRunnerStream)
		log.Info().
			Str("ZED_FLOW_DEBUG", "websocket_subscribing_to_nats").
			Str("stream", pubsub.ZedAgentRunnerStream).
			Str("queue", pubsub.ZedAgentQueue).
			Str("runner_id", runnerID).
			Msg("📡 ZED_FLOW_DEBUG: [STEP 2.5] WebSocket server about to subscribe to NATS stream")

		// Track consecutive WebSocket write failures for circuit breaker pattern
		var consecutiveFailures int
		const maxConsecutiveFailures = 3

		zedAgentSub, err := apiServer.pubsub.StreamConsume(ctx, pubsub.ZedAgentRunnerStream, pubsub.ZedAgentQueue, func(msg *pubsub.Message) error {
			log.Info().
				Str("ZED_FLOW_DEBUG", "message_from_nats_stream").
				Str("runner_id", runnerID).
				Str("kind", msg.Header.Get("kind")).
				Str("reply", msg.Reply).
				Int("data_length", len(msg.Data)).
				Msg("🎯 ZED_FLOW_DEBUG: [STEP 3] Received message from NATS stream - about to forward to WebSocket")

			var messageType types.RunnerEventRequestType

			switch msg.Header.Get("kind") {
			case "zed_agent":
				messageType = types.RunnerEventRequestDesktopAgent
				log.Info().
					Str("ZED_FLOW_DEBUG", "message_type_zed_agent").
					Str("runner_id", runnerID).
					Msg("🎯 ZED_FLOW_DEBUG: Message type identified as zed_agent")
			case "stop_zed_agent":
				messageType = types.RunnerEventRequestDesktopAgent // Handle stop requests
				log.Info().
					Str("ZED_FLOW_DEBUG", "message_type_stop_zed_agent").
					Str("runner_id", runnerID).
					Msg("🎯 ZED_FLOW_DEBUG: Message type identified as stop_zed_agent")
			default:
				log.Warn().
					Str("ZED_FLOW_DEBUG", "unknown_message_type").
					Str("kind", msg.Header.Get("kind")).
					Str("runner_id", runnerID).
					Msg("⚠️ ZED_FLOW_DEBUG: Unknown message kind, defaulting to zed_agent")
				messageType = types.RunnerEventRequestDesktopAgent
			}

			envelope := &types.RunnerEventRequestEnvelope{
				RequestID: system.GenerateRequestID(),
				Reply:     msg.Reply, // Runner will need this inbox channel to send messages back to the requestor
				Type:      messageType,
				Payload:   msg.Data, // The actual payload (Zed agent request)
			}

			log.Info().
				Str("ZED_FLOW_DEBUG", "envelope_created").
				Str("runner_id", runnerID).
				Str("request_id", envelope.RequestID).
				Str("reply", envelope.Reply).
				Int("type", int(envelope.Type)).
				Int("payload_length", len(envelope.Payload)).
				Msg("📦 ZED_FLOW_DEBUG: Created envelope - about to send via WebSocket")

			wsMu.Lock()
			err := wsConn.WriteJSON(envelope)
			wsMu.Unlock()
			if err != nil {
				consecutiveFailures++
				log.Error().
					Str("ZED_FLOW_DEBUG", "websocket_write_error").
					Err(err).
					Str("runner_id", runnerID).
					Str("request_id", envelope.RequestID).
					Int("consecutive_failures", consecutiveFailures).
					Msg("❌ ZED_FLOW_DEBUG: [STEP 4 FAILED] Error writing envelope to WebSocket - NAK'ing message but continuing subscription")

				// NAK the message so it can be redelivered to another runner
				if nakErr := msg.Nak(); nakErr != nil {
					log.Error().
						Str("ZED_FLOW_DEBUG", "nats_nak_error").
						Err(nakErr).
						Str("runner_id", runnerID).
						Str("request_id", envelope.RequestID).
						Msg("❌ ZED_FLOW_DEBUG: Failed to NAK message after WebSocket write error")
				}

				// Circuit breaker: if too many consecutive failures, break the subscription
				// This allows the WebSocket to close cleanly and the runner to reconnect
				if consecutiveFailures >= maxConsecutiveFailures {
					log.Error().
						Str("ZED_FLOW_DEBUG", "circuit_breaker_triggered").
						Str("runner_id", runnerID).
						Int("consecutive_failures", consecutiveFailures).
						Int("max_failures", maxConsecutiveFailures).
						Msg("🔥 ZED_FLOW_DEBUG: Circuit breaker triggered - too many consecutive WebSocket failures, closing connection to allow reconnect")
					return fmt.Errorf("circuit breaker: %d consecutive WebSocket write failures", consecutiveFailures)
				}

				// Don't return error for isolated failures - this would break the entire NATS subscription
				// Instead, let the WebSocket connection detection handle the cleanup
				// The message will be redelivered to another healthy runner
				return nil
			}

			// Reset failure counter on successful write
			consecutiveFailures = 0

			log.Info().
				Str("ZED_FLOW_DEBUG", "websocket_write_success").
				Str("runner_id", runnerID).
				Str("request_id", envelope.RequestID).
				Msg("✅ ZED_FLOW_DEBUG: [STEP 4] Successfully sent envelope to external agent runner via WebSocket")

			if err := msg.Ack(); err != nil {
				log.Error().
					Str("ZED_FLOW_DEBUG", "nats_ack_error").
					Err(err).
					Str("runner_id", runnerID).
					Str("request_id", envelope.RequestID).
					Msg("❌ ZED_FLOW_DEBUG: Failed to acknowledge NATS message")
				return fmt.Errorf("failed to ack the message: %v", err)
			}

			log.Info().
				Str("ZED_FLOW_DEBUG", "nats_message_acked").
				Str("runner_id", runnerID).
				Str("request_id", envelope.RequestID).
				Msg("✅ ZED_FLOW_DEBUG: NATS message acknowledged successfully")

			return nil
		})
		if err != nil {
			log.Error().
				Str("ZED_FLOW_DEBUG", "nats_subscription_failed").
				Err(err).
				Str("stream", pubsub.ZedAgentRunnerStream).
				Str("queue", pubsub.ZedAgentQueue).
				Str("runner_id", runnerID).
				Msg("❌ ZED_FLOW_DEBUG: [STEP 2.5 FAILED] WebSocket server failed to subscribe to NATS - no messages will be received")
			return
		}

		log.Info().
			Str("ZED_FLOW_DEBUG", "nats_subscription_success").
			Str("runner_id", runnerID).
			Msg("✅ ZED_FLOW_DEBUG: [STEP 2.5] WebSocket server successfully subscribed to NATS - waiting for messages")
		defer func() {
			if err := zedAgentSub.Unsubscribe(); err != nil {
				log.Err(err).
					Str("ZED_FLOW_DEBUG", "nats_unsubscribe_error").
					Str("runner_id", runnerID).
					Msg("❌ ZED_FLOW_DEBUG: Error unsubscribing from NATS stream")
			}
		}()

		// Block reads in order to detect disconnects and handle responses
		log.Info().
			Str("ZED_FLOW_DEBUG", "websocket_message_loop_start").
			Str("runner_id", runnerID).
			Msg("🔄 ZED_FLOW_DEBUG: Starting WebSocket message read loop - ready to receive responses from external agent")

		for {
			messageType, messageBytes, err := wsConn.ReadMessage()
			if err != nil || messageType == websocket.CloseMessage {
				// Only log as error if it's an unexpected close
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Error().
						Str("EXTERNAL_AGENT_DEBUG", "runner_disconnect").
						Str("action", "🟠 External agent runner ws DISCONNECT").
						Str("runner_id", runnerID).
						Err(err).
						Msg("🟠 EXTERNAL_AGENT_DEBUG: Unexpected close error from external agent runner websocket")
				} else {
					log.Info().
						Str("EXTERNAL_AGENT_DEBUG", "runner_disconnect").
						Str("action", "🟠 External agent runner ws DISCONNECT").
						Str("runner_id", runnerID).
						Err(err).
						Msg("🟠 EXTERNAL_AGENT_DEBUG: Disconnected external agent runner websocket")
				}
				return
			}

			// Refresh read deadline on any message to keep connection alive
			wsConn.SetReadDeadline(time.Now().Add(readTimeout))

			// Log all incoming WebSocket messages with their types
			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "websocket_message_received").
				Str("runner_id", runnerID).
				Int("message_type", int(messageType)).
				Str("message_type_name", getWebSocketMessageTypeName(messageType)).
				Int("message_length", len(messageBytes)).
				Msg("📨 EXTERNAL_AGENT_DEBUG: WebSocket message received from external agent runner")

			// Note: Ping messages are now handled automatically by the WebSocket library
			// and our SetPongHandler above will track the ping timestamps

			// Handle pong messages (if any - though these should be handled by SetPongHandler)
			if messageType == websocket.PongMessage {
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "pong_received_in_readloop").
					Str("runner_id", runnerID).
					Msg("🏓 EXTERNAL_AGENT_DEBUG: Received pong in read message loop (unexpected)")
				continue
			}

			// Only process text messages as JSON
			if messageType != websocket.TextMessage {
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "non_text_message").
					Str("runner_id", runnerID).
					Int("message_type", int(messageType)).
					Msg("🔄 EXTERNAL_AGENT_DEBUG: Received non-text message, skipping")
				continue
			}

			log.Debug().
				Str("EXTERNAL_AGENT_DEBUG", "runner_response").
				Str("runner_id", runnerID).
				Int("message_type", int(messageType)).
				Int("message_length", len(messageBytes)).
				Str("message_preview", func() string {
					if len(messageBytes) > 200 {
						return string(messageBytes[:200]) + "..."
					}
					return string(messageBytes)
				}()).
				Msg("📨 EXTERNAL_AGENT_DEBUG: External agent runner websocket response")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			var resp types.RunnerEventResponseEnvelope
			err = json.Unmarshal(messageBytes, &resp)
			if err != nil {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "response_unmarshal_error").
					Err(err).
					Str("runner_id", runnerID).
					Str("raw_message", string(messageBytes)).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Error unmarshalling websocket event")
				continue
			}

			log.Debug().
				Str("EXTERNAL_AGENT_DEBUG", "response_parsed").
				Str("runner_id", runnerID).
				Str("request_id", resp.RequestID).
				Str("reply", resp.Reply).
				Msg("📋 EXTERNAL_AGENT_DEBUG: Parsed runner response envelope")

			err = apiServer.pubsub.Publish(ctx, resp.Reply, resp.Payload)
			if err != nil {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "publish_response_error").
					Err(err).
					Str("runner_id", runnerID).
					Str("reply", resp.Reply).
					Msg("❌ EXTERNAL_AGENT_DEBUG: Error publishing external agent response")
			} else {
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "response_published").
					Str("runner_id", runnerID).
					Str("reply", resp.Reply).
					Msg("✅ EXTERNAL_AGENT_DEBUG: External agent response published successfully")
			}
		}
	})
}

// Helper function to get WebSocket message type names for logging
func getWebSocketMessageTypeName(messageType int) string {
	switch messageType {
	case websocket.TextMessage:
		return "TextMessage"
	case websocket.BinaryMessage:
		return "BinaryMessage"
	case websocket.CloseMessage:
		return "CloseMessage"
	case websocket.PingMessage:
		return "PingMessage"
	case websocket.PongMessage:
		return "PongMessage"
	default:
		return fmt.Sprintf("Unknown(%d)", messageType)
	}
}

// handleRevDial handles reverse dial connections from external agent runners
// ensureSandboxRegistered auto-registers a sandbox instance if it doesn't exist
// This allows sandbox containers to self-register on first connection
func (apiServer *HelixAPIServer) ensureSandboxRegistered(ctx context.Context, sandboxID string, remoteAddr string) {
	// Check if already registered
	instances, err := apiServer.Store.ListSandboxes(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to check existing sandboxes for auto-registration")
		return
	}

	for _, instance := range instances {
		if instance.ID == sandboxID {
			// Already registered - reset sandbox count and mark online
			// This handles reconnects after crashes/restarts where stale counts remain
			err := apiServer.Store.ResetSandboxOnReconnect(ctx, sandboxID)
			if err != nil {
				log.Error().
					Err(err).
					Str("sandbox_id", sandboxID).
					Msg("Failed to reset sandbox on reconnect")
			} else {
				log.Info().
					Str("sandbox_id", sandboxID).
					Int("previous_container_count", instance.ActiveSandboxes).
					Msg("Reset sandbox on reconnect (cleared stale container count)")
			}
			return
		}
	}

	// Not registered - auto-register it
	instance := &types.SandboxInstance{
		ID:           sandboxID,
		Hostname:     fmt.Sprintf("sandbox-%s", sandboxID),
		IPAddress:    remoteAddr,
		MaxSandboxes: 20, // Default capacity
		Status:       "online",
	}

	err = apiServer.Store.RegisterSandbox(ctx, instance)
	if err != nil {
		log.Error().
			Err(err).
			Str("sandbox_id", sandboxID).
			Msg("Failed to auto-register sandbox")
		return
	}

	log.Info().
		Str("sandbox_id", sandboxID).
		Str("hostname", instance.Hostname).
		Str("ip_address", instance.IPAddress).
		Int("max_sandboxes", instance.MaxSandboxes).
		Msg("Auto-registered sandbox on first RevDial connection")
}

func (apiServer *HelixAPIServer) handleRevDial() http.Handler {
	// Create the WebSocket handler for data connections using revdial.ConnHandler
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
		// Disable strict header validation for compatibility
		EnableCompression: false,
	}

	revDialConnHandler := revdial.ConnHandler(upgrader)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a DATA connection (WebSocket with revdial.dialer parameter)
		// Control connections are WebSocket too, but without the dialer parameter
		dialerParam := r.URL.Query().Get("revdial.dialer")
		if websocket.IsWebSocketUpgrade(r) && dialerParam != "" {
			// This is a data connection - use the revdial ConnHandler
			revDialConnHandler.ServeHTTP(w, r)
			return
		}

		// This is a CONTROL connection (WebSocket without dialer parameter, or non-WebSocket)
		log.Info().
			Bool("is_websocket", websocket.IsWebSocketUpgrade(r)).
			Str("upgrade_header", r.Header.Get("Upgrade")).
			Str("connection_header", r.Header.Get("Connection")).
			Str("remote_addr", r.RemoteAddr).
			Msg("Handling revdial CONTROL connection")

		// Get authenticated user from middleware (accepts both runner and user tokens)
		user := getRequestUser(r)
		if user == nil {
			log.Error().Msg("Unauthorized reverse dial request - authentication required")
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}

		// Extract runner ID from query parameter
		runnerID := r.URL.Query().Get("runnerid")
		if runnerID == "" {
			log.Error().Msg("Missing runnerid in reverse dial request")
			http.Error(w, "runnerid is required", http.StatusBadRequest)
			return
		}

		// If using a user token (not runner token), validate session ownership
		if user.TokenType != types.TokenTypeRunner {
			// Extract session ID from runner ID (format: desktop-{session_id})
			// Note: "desktop-" prefix is for per-session containers, "sandbox-" is for the outer sandbox
			sessionID := strings.TrimPrefix(runnerID, "desktop-")
			if sessionID == runnerID {
				log.Error().
					Str("runner_id", runnerID).
					Msg("Invalid runner ID format - must be desktop-{session_id}")
				http.Error(w, "invalid runner ID format", http.StatusBadRequest)
				return
			}

			// Verify the session belongs to this user
			session, err := apiServer.Store.GetSession(r.Context(), sessionID)
			if err != nil || session.Owner != user.ID {
				log.Error().
					Err(err).
					Str("user_id", user.ID).
					Str("session_id", sessionID).
					Msg("Unauthorized: session not found or not owned by user")
				http.Error(w, "unauthorized: session not owned by user", http.StatusForbidden)
				return
			}

			log.Info().
				Str("user_id", user.ID).
				Str("session_id", sessionID).
				Msg("User token validated for RevDial connection")
		}

		log.Info().
			Str("remote_addr", r.RemoteAddr).
			Str("runner_id", runnerID).
			Str("token_type", string(user.TokenType)).
			Msg("Authenticated RevDial connection (runner or user token)")

		// Auto-register sandbox instances if this is a sandbox-* or hydra-* runner ID
		var sandboxID string
		switch {
		case strings.HasPrefix(runnerID, "hydra-"):
			sandboxID = strings.TrimPrefix(runnerID, "hydra-")
		case strings.HasPrefix(runnerID, "sandbox-"):
			sandboxID = strings.TrimPrefix(runnerID, "sandbox-")
		}
		if sandboxID != "" {
			apiServer.ensureSandboxRegistered(r.Context(), sandboxID, r.RemoteAddr)
		}

		log.Info().Str("runner_id", runnerID).Msg("Establishing reverse dial connection")

		// Handle WebSocket control connection vs non-WebSocket control connection
		var conn net.Conn
		if websocket.IsWebSocketUpgrade(r) {
			// Upgrade WebSocket for control connection
			log.Debug().Msg("Upgrading WebSocket for RevDial control connection")
			wsConn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to upgrade WebSocket")
				http.Error(w, "WebSocket upgrade failed", http.StatusInternalServerError)
				return
			}

			// Start server-initiated ping goroutine to keep connection alive through proxies/firewalls
			// This runs until the WebSocket is closed (WriteControl fails)
			go func(ws *websocket.Conn, rID string) {
				ticker := time.NewTicker(15 * time.Second)
				defer ticker.Stop()
				for {
					<-ticker.C
					if err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
						log.Debug().
							Err(err).
							Str("runner_id", rID).
							Msg("RevDial WebSocket ping failed, connection closing")
						return
					}
				}
			}(wsConn, runnerID)

			// Wrap WebSocket as net.Conn using wsconnadapter
			conn = wsconnadapter.New(wsConn)
			log.Debug().Str("runner_id", runnerID).Msg("WebSocket control connection established")
		} else {
			// HTTP hijack for non-WebSocket control connection
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				log.Error().Msg("HTTP hijacking not supported")
				http.Error(w, "HTTP hijacking not supported", http.StatusInternalServerError)
				return
			}

			// Send HTTP 200 response before hijacking
			w.WriteHeader(http.StatusOK)

			// Hijack the connection to get raw TCP
			var err error
			conn, _, err = hijacker.Hijack()
			if err != nil {
				log.Error().Err(err).Str("runner_id", runnerID).Msg("Failed to hijack connection")
				return
			}
			log.Debug().Str("runner_id", runnerID).Msg("HTTP control connection hijacked")
		}

		// Register the reverse dial connection in connman
		apiServer.connman.Set(runnerID, conn)
		log.Info().Str("runner_id", runnerID).Msg("Registered reverse dial connection in connman")

		// The connection is now managed by connman
		// It will be used when external_agent_handlers.go calls connman.Dial(runnerID)
	})
}

// getUserDefaultExternalAgentApp gets the user's default app for external agents (zed_external)
// Returns the first app with zed_external agent type, or the first app if none found
func (apiServer *HelixAPIServer) getUserDefaultExternalAgentApp(ctx context.Context, userID string) (*types.App, error) {
	apps, err := apiServer.Store.ListApps(ctx, &store.ListAppsQuery{
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list user apps: %w", err)
	}

	if len(apps) == 0 {
		return nil, fmt.Errorf("user has no apps configured")
	}

	// Find the first app with zed_external default agent type
	for _, app := range apps {
		if app.Config.Helix.DefaultAgentType == "zed_external" {
			return app, nil
		}
	}

	// Fall back to first app if no zed_external app found
	return apps[0], nil
}
