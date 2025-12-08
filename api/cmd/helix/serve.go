package helix

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	// Register file driver
	"gocloud.dev/blob/fileblob"

	"github.com/helixml/helix/api/pkg/anthropic"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/turn"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/version"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func NewServeConfig() (*config.ServerConfig, error) {
	serverConfig, err := config.LoadServerConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load server config: %v", err)
	}

	if serverConfig.Controller.FilestorePresignSecret == "" {
		serverConfig.Controller.FilestorePresignSecret = system.GenerateUUID()
	}

	serverConfig.Janitor.AppURL = serverConfig.WebServer.URL
	serverConfig.Stripe.AppURL = serverConfig.WebServer.URL

	if serverConfig.GitHub.Enabled {
		if serverConfig.GitHub.ClientID == "" {
			return nil, fmt.Errorf("github client id is required")
		}
		if serverConfig.GitHub.ClientSecret == "" {
			return nil, fmt.Errorf("github client secret is required")
		}
	}

	return &serverConfig, nil
}

func NewServeCmd() *cobra.Command {
	serveConfig, err := NewServeConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create serve options")
	}

	envHelpText := generateEnvHelpText(serveConfig, "")

	serveCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Start the helix api server.",
		Long:    "Start the helix api server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := serve(cmd, serveConfig)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to run server")
			}
			return nil
		},
	}

	serveCmd.Long += "\n\nEnvironment Variables:\n\n" + envHelpText

	return serveCmd
}

func getFilestore(ctx context.Context, cfg *config.ServerConfig) (filestore.FileStore, error) {
	var store filestore.FileStore
	if cfg.WebServer.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	if cfg.FileStore.Type == types.FileStoreTypeLocalFS {
		if cfg.FileStore.LocalFSPath == "" {
			return nil, fmt.Errorf("local fs path is required")
		}
		rootPath := filepath.Join(cfg.FileStore.LocalFSPath, cfg.Controller.FilePrefixGlobal)
		if _, err := os.Stat(rootPath); os.IsNotExist(err) {
			err := os.MkdirAll(rootPath, 0755)
			if err != nil {
				return nil, err
			}
		}
		store = filestore.NewFileSystemStorage(cfg.FileStore.LocalFSPath, fmt.Sprintf("%s/api/v1/filestore/viewer", cfg.WebServer.URL), cfg.Controller.FilestorePresignSecret)
	} else if cfg.FileStore.Type == types.FileStoreTypeLocalGCS {
		if cfg.FileStore.GCSKeyBase64 != "" {
			keyfile, err := func() (string, error) {
				decoded, err := base64.StdEncoding.DecodeString(cfg.FileStore.GCSKeyBase64)
				if err != nil {
					return "", fmt.Errorf("failed to decode GCS key: %v", err)
				}
				tmpfile, err := os.CreateTemp("", "gcskey")
				if err != nil {
					return "", fmt.Errorf("failed to create temporary file for GCS key: %v", err)
				}
				defer tmpfile.Close()
				if _, err := tmpfile.Write(decoded); err != nil {
					return "", fmt.Errorf("failed to write GCS key to temporary file: %v", err)
				}
				return tmpfile.Name(), nil
			}()
			if err != nil {
				return nil, err
			}
			cfg.FileStore.GCSKeyFile = keyfile
		}
		if cfg.FileStore.GCSKeyFile == "" {
			return nil, fmt.Errorf("gcs key is required")
		}
		if _, err := os.Stat(cfg.FileStore.GCSKeyFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("gcs key file does not exist")
		}
		gcs, err := filestore.NewGCSStorage(ctx, cfg.FileStore.GCSKeyFile, cfg.FileStore.GCSBucket)
		if err != nil {
			return nil, err
		}
		store = gcs
	} else {
		return nil, fmt.Errorf("unknown filestore type: %s", cfg.FileStore.Type)
	}
	// let's make sure the global prefix folder exists
	// from here on it will be user directories being created
	_, err := store.CreateFolder(ctx, cfg.Controller.FilePrefixGlobal)
	if err != nil {
		return nil, err
	}

	// Create the users and apps top-level directories
	_, err = store.CreateFolder(ctx, filepath.Join(cfg.Controller.FilePrefixGlobal, "users"))
	if err != nil {
		return nil, err
	}

	_, err = store.CreateFolder(ctx, filepath.Join(cfg.Controller.FilePrefixGlobal, "apps"))
	if err != nil {
		return nil, err
	}

	return store, nil
}

func serve(cmd *cobra.Command, cfg *config.ServerConfig) error {
	// Validate license key if provided
	var userLicense *license.License
	if cfg.LicenseKey != "" {
		validator := license.NewLicenseValidator()
		var err error
		userLicense, err = validator.Validate(cfg.LicenseKey)
		if err != nil {
			return fmt.Errorf("invalid license key: %w", err)
		}
	}

	system.SetupLogging()

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())

	// Create a cancellable context for license checks
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Create license manager
	lm := license.NewLicenseManager(userLicense)

	// Run background license checks
	go func() {
		err := lm.Run(ctx)
		if err != nil {
			log.Error().Err(err).Msg("license is not valid anymore")
			// don't actually shut down the server yet, we'll start enforcing licenses in the next version
			// cancel() // Cancel context when license becomes invalid
		}
	}()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, signalCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer signalCancel()

	fs, err := getFilestore(ctx, cfg)
	if err != nil {
		return err
	}

	postgresStore, err := store.NewPostgresStore(cfg.Store)
	if err != nil {
		return err
	}

	// Initialize dynamic providers from environment variable
	err = postgresStore.InitializeDynamicProviders(ctx, cfg.Providers.DynamicProviders)
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize dynamic providers, continuing with startup")
		// Don't fail the entire startup if dynamic providers fail to initialize
	}

	// Reset any running executions
	err = postgresStore.ResetRunningExecutions(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to reset running executions")
	}

	log.Info().Msg("resetting running interactions")

	err = postgresStore.ResetRunningInteractions(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to reset running interactions")
	}

	ps, err := pubsub.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create pubsub provider: %w", err)
	}

	// Start TURN server for WebRTC NAT traversal if enabled
	var turnServer *turn.Server
	if cfg.TURN.Enabled {
		turnServer, err = turn.New(turn.Config{
			PublicIP: cfg.TURN.PublicIP,
			Port:     cfg.TURN.Port,
			Realm:    cfg.TURN.Realm,
			Username: cfg.TURN.Username,
			Password: cfg.TURN.Password,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to start TURN server, WebRTC may not work properly")
		} else {
			cm.RegisterCallbackWithContext(func(ctx context.Context) error {
				return turnServer.Close()
			})
			log.Info().Msgf("TURN server enabled for WebRTC at %s:%d", cfg.TURN.PublicIP, cfg.TURN.Port)
		}
	}

	if cfg.WebServer.RunnerToken == "" {
		return fmt.Errorf("runner token is required")
	}

	notifier, err := notification.New(&cfg.Notifications, postgresStore)
	if err != nil {
		return fmt.Errorf("failed to create notifier: %v", err)
	}

	var authenticator auth.Authenticator

	switch cfg.Auth.Provider {
	case types.AuthProviderKeycloak:
		authenticator, err = auth.NewKeycloakAuthenticator(cfg, postgresStore)
		if err != nil {
			return fmt.Errorf("failed to create keycloak authenticator: %v", err)
		}
	default:
		// Default authenticator, using regular authentication
		authenticator, err = auth.NewHelixAuthenticator(cfg, postgresStore, cfg.Auth.Regular.JWTSecret, notifier)
		if err != nil {
			return fmt.Errorf("failed to create helix authenticator: %v", err)
		}
	}

	janitor := janitor.NewJanitor(cfg.Janitor)
	err = janitor.Initialize()
	if err != nil {
		return err
	}

	// External agent executor not used - following GPTScript pattern with WebSocket + PubSub
	log.Info().Msg("Using GPTScript-style external agent pattern (WebSocket + PubSub)")
	var gse external_agent.Executor // nil executor - communication via WebSocket + PubSub

	var extractor extract.Extractor

	switch cfg.TextExtractor.Provider {
	case types.ExtractorTika:
		extractor = extract.NewTikaExtractor(cfg.TextExtractor.Tika.URL)
	case types.ExtractorUnstructured:
		extractor = extract.NewDefaultExtractor(cfg.TextExtractor.Unstructured.URL)
	case types.ExtractorHaystack:
		extractor = extract.NewHaystackExtractor(cfg.RAG.Haystack.URL)
	default:
		return fmt.Errorf("unknown extractor: %s", cfg.TextExtractor.Provider)
	}

	runnerController, err := scheduler.NewRunnerController(ctx, &scheduler.RunnerControllerConfig{
		PubSub: ps,
		FS:     fs,
		Store:  postgresStore,
	})
	if err != nil {
		return err
	}

	var appController *controller.Controller

	// Create memory estimation service for scheduler
	memoryEstimationService := controller.NewMemoryEstimationService(
		runnerController, // Implements RunnerSender interface
		controller.NewStoreModelProvider(postgresStore), // Wrapped store implementing ModelProvider interface
	)
	// memoryEstimationService.StartBackgroundCacheRefresh(ctx) // DISABLED FOR DEBUGGING
	// memoryEstimationService.StartCacheCleanup(ctx) // DISABLED FOR DEBUGGING

	scheduler, err := scheduler.NewScheduler(ctx, cfg, &scheduler.Params{
		RunnerController:        runnerController,
		Store:                   postgresStore,
		MemoryEstimationService: memoryEstimationService,
		QueueSize:               100,
		OnSchedulingErr: func(work *scheduler.Workload, err error) {
			if appController != nil {
				switch work.WorkloadType {
				case scheduler.WorkloadTypeLLMInferenceRequest:
					request := work.LLMInferenceRequest()
					response := types.RunnerNatsReplyResponse{
						OwnerID:   request.OwnerID,
						RequestID: request.RequestID,
						Error:     err.Error(),
						Response:  []byte{},
					}
					bts, err := json.Marshal(response)
					if err != nil {
						log.Error().Err(err).Msg("error marshalling runner response")
					}
					err = ps.Publish(ctx, pubsub.GetRunnerResponsesQueue(request.OwnerID, request.RequestID), bts)
					if err != nil {
						log.Error().Err(err).Msg("error publishing runner response")
					}
				case scheduler.WorkloadTypeSession:

					// Get the last interaction
					// TODO: update scheduler func to keep the interaction
					interaction := work.Session().Interactions[len(work.Session().Interactions)-1]

					appController.ErrorSession(ctx, work.Session(), interaction, err)
				}
			}
		},
		OnResponseHandler: func(_ context.Context, _ *types.RunnerLLMInferenceResponse) error {
			return nil
		},
	})
	if err != nil {
		return err
	}

	// Set up prewarming callback now that both components exist
	runnerController.SetOnRunnerConnectedCallback(scheduler.PrewarmNewRunner)
	log.Info().Msg("Prewarming enabled - new runners will be prewarmed with configured models")

	helixInference := openai.NewInternalHelixServer(cfg, postgresStore, ps, scheduler)

	var logStores []logger.LogStore

	if !cfg.DisableLLMCallLogging {
		logStores = []logger.LogStore{
			postgresStore,
			// TODO: bigquery
		}
	}

	if !cfg.DisableUsageLogging {
		logStores = append(logStores, logger.NewUsageLogger(postgresStore))
	}

	baseInfoProvider, err := model.NewBaseModelInfoProvider()
	if err != nil {
		return fmt.Errorf("failed to create model info provider: %w", err)
	}

	// Dynamic info providers allows overriding the base model prices and model information (defining Helix LLM prices)
	dynamicInfoProvider := model.NewDynamicModelInfoProvider(postgresStore, baseInfoProvider)

	providerManager := manager.NewProviderManager(cfg, postgresStore, helixInference, dynamicInfoProvider, logStores...)

	// Connect the runner controller to the provider manager
	providerManager.SetRunnerController(runnerController)
	log.Info().Msg("Connected runner controller to provider manager to enable hiding Helix provider when no runners are available")

	// Will run async and watch for changes in the API keys, non-blocking
	providerManager.StartRefresh(ctx)

	var ragClient rag.RAG

	switch cfg.RAG.DefaultRagProvider {
	case config.RAGProviderTypesense:
		ragSettings := &types.RAGSettings{}
		ragSettings.Typesense.URL = cfg.RAG.Typesense.URL
		ragSettings.Typesense.APIKey = cfg.RAG.Typesense.APIKey
		ragClient, err = rag.NewTypesense(ragSettings)
		if err != nil {
			return fmt.Errorf("failed to create typesense RAG client: %v", err)
		}
		log.Info().Msgf("Using Typesense for RAG")
	case config.RAGProviderLlamaindex:
		ragClient = rag.NewLlamaindex(&types.RAGSettings{
			IndexURL:  cfg.RAG.Llamaindex.RAGIndexingURL,
			QueryURL:  cfg.RAG.Llamaindex.RAGQueryURL,
			DeleteURL: cfg.RAG.Llamaindex.RAGDeleteURL,
		})
		log.Info().Msgf("Using Llamaindex for RAG")
	case config.RAGProviderHaystack:
		ragClient = rag.NewHaystackRAG(cfg.RAG.Haystack.URL)
		log.Info().Msgf("Using Haystack for RAG")
	default:
		return fmt.Errorf("unknown RAG provider: %s", cfg.RAG.DefaultRagProvider)
	}

	// Initialize browser pool
	browserPool, err := browser.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create browser pool: %w", err)
	}

	searchProvider := searxng.NewSearXNG(&searxng.Config{
		BaseURL: cfg.Search.SearXNGBaseURL,
	})

	controllerOptions := controller.Options{
		Config:                cfg,
		Store:                 postgresStore,
		PubSub:                ps,
		RAG:                   ragClient,
		Extractor:             extractor,
		ExternalAgentExecutor: gse, // Using external agent executor
		Filestore:             fs,
		Janitor:               janitor,
		Notifier:              notifier,
		ProviderManager:       providerManager,
		Scheduler:             scheduler,
		RunnerController:      runnerController,
		Browser:               browserPool,
		SearchProvider:        searchProvider,
	}

	// Create the OAuth manager
	oauthManager := oauth.NewManager(postgresStore, cfg.Tools.TLSSkipVerify)
	err = oauthManager.LoadProviders(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to load oauth providers")
	} else {
		// Start the OAuth manager
		oauthManager.Start(ctx)
	}

	// Update controller options with the OAuth manager
	controllerOptions.OAuthManager = oauthManager

	appController, err = controller.NewController(ctx, controllerOptions)
	if err != nil {
		return err
	}

	err = appController.Initialize()
	if err != nil {
		return err
	}

	knowledgeReconciler, err := knowledge.New(cfg, postgresStore, fs, extractor, ragClient, browserPool, oauthManager)
	if err != nil {
		return err
	}

	go func() {
		if err := knowledgeReconciler.Start(ctx); err != nil {
			log.Error().Err(err).Msg("failed to start knowledge reconciler")
		}
	}()

	trigger := trigger.NewTriggerManager(cfg, postgresStore, notifier, appController)
	// Start integrations
	go trigger.Start(ctx)

	// Start agent work queue processor
	workQueueProcessor := controller.NewAgentWorkQueueProcessor(postgresStore, appController)
	go workQueueProcessor.Start(ctx)

	stripe := stripe.NewStripe(
		cfg.Stripe,
		postgresStore,
	)

	// Initialize ping service if not disabled
	var pingService *version.PingService
	if !cfg.DisableVersionPing {
		pingService = version.NewPingService(postgresStore, cfg.LicenseKey, cfg.LaunchpadURL)
		pingService.Start(ctx)
		defer pingService.Stop()
	}

	// Ensure the directory exists
	err = os.MkdirAll(filepath.Join(cfg.FileStore.AvatarsPath, "avatars"), 0755)
	if err != nil {
		log.Error().Err(err).Msg("failed to create avatars directory, app avatars will not be saved")
	}

	avatarsBucket, err := fileblob.OpenBucket(cfg.FileStore.AvatarsPath, &fileblob.Options{
		NoTempDir: true,
	})
	if err != nil {
		return err
	}

	anthropicProxy := anthropic.New(cfg, postgresStore, dynamicInfoProvider, logStores...)

	server, err := server.NewServer(
		cfg,
		postgresStore,
		ps,
		providerManager,
		dynamicInfoProvider,
		helixInference,
		authenticator,
		stripe,
		appController,
		janitor,
		knowledgeReconciler,
		scheduler,
		pingService,
		oauthManager,
		avatarsBucket,
		trigger,
		anthropicProxy,
	)
	if err != nil {
		return err
	}

	// Start Wolf health monitor for multi-Wolf distributed deployment
	wolfScheduler := store.NewWolfScheduler(postgresStore)
	wolfHealthMonitor := store.NewWolfHealthMonitor(postgresStore, wolfScheduler)
	go wolfHealthMonitor.Start(ctx)
	log.Info().Msg("Wolf health monitor started")

	log.Info().Msgf("Helix server listening on %s:%d", cfg.WebServer.Host, cfg.WebServer.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}
