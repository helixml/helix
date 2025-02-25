package helix

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/controller/knowledge"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/extract"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/license"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/logger"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/scheduler"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/trigger"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/version"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// nolint:unused
func printStackTrace() {
	// Allocate a buffer large enough to store the stack trace
	buf := make([]byte, 1024)
	for {
		n := runtime.Stack(buf, false)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		// Double the buffer size if the trace is larger than the current buffer
		buf = make([]byte, len(buf)*2)
	}
	fmt.Printf("Stack trace:\n%s\n", buf)
}

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
	serverConfig.WebServer.LocalFilestorePath = serverConfig.FileStore.LocalFSPath

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

func newServeCmd() *cobra.Command {
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

	ps, err := pubsub.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create pubsub provider: %w", err)
	}

	if cfg.WebServer.RunnerToken == "" {
		return fmt.Errorf("runner token is required")
	}

	gocloakClient, err := auth.NewGoCloakClient(&cfg.Keycloak)
	if err != nil {
		return fmt.Errorf("failed to create keycloak client: %v", err)
	}

	userRetriever, err := auth.NewKeycloakUserRetriever(gocloakClient.Client, &cfg.Keycloak)

	if err != nil {
		return fmt.Errorf("failed to create user retriever: %v", err)
	}

	notifier, err := notification.New(&cfg.Notifications, userRetriever)
	if err != nil {
		return fmt.Errorf("failed to create notifier: %v", err)
	}

	janitor := janitor.NewJanitor(cfg.Janitor)
	err = janitor.Initialize()
	if err != nil {
		return err
	}

	var gse gptscript.Executor

	if cfg.GPTScript.TestFaster.URL != "" {
		log.Info().Msg("using firecracker based GPTScript executor")
		gse = gptscript.NewTestFasterExecutor(cfg)
	} else {
		log.Info().Msg("using runner based GPTScript executor")
		gse = gptscript.NewExecutor(cfg, ps)
	}

	var extractor extract.Extractor

	switch cfg.TextExtractor.Provider {
	case types.ExtractorTika:
		extractor = extract.NewTikaExtractor(cfg.TextExtractor.Tika.URL)
	case types.ExtractorUnstructured:
		extractor = extract.NewDefaultExtractor(cfg.TextExtractor.Unstructured.URL)
	default:
		return fmt.Errorf("unknown extractor: %s", cfg.TextExtractor.Provider)
	}

	runnerController, err := scheduler.NewRunnerController(ctx, &scheduler.RunnerControllerConfig{
		PubSub: ps,
		FS:     fs,
	})
	if err != nil {
		return err
	}

	var appController *controller.Controller

	scheduler, err := scheduler.NewScheduler(ctx, cfg, &scheduler.Params{
		RunnerController: runnerController,
		QueueSize:        100,
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
					appController.ErrorSession(ctx, work.Session(), err)
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

	helixInference := openai.NewInternalHelixServer(cfg, ps, scheduler)

	var logStores []logger.LogStore

	if !cfg.DisableLLMCallLogging {
		logStores = []logger.LogStore{
			postgresStore,
			// TODO: bigquery
		}
	}

	providerManager := manager.NewProviderManager(cfg, postgresStore, helixInference, logStores...)

	// Will run async and watch for changes in the API keys, non-blocking
	providerManager.StartRefresh(ctx)

	dataprepOpenAIClient, err := createDataPrepOpenAIClient(cfg, helixInference)
	if err != nil {
		return err
	}
	dataprepOpenAIClient = logger.Wrap(cfg, cfg.FineTuning.Provider, dataprepOpenAIClient, logStores...)

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
	case config.RAGProviderPGVector:
		pgVectorStore, err := store.NewPGVectorStore(cfg)
		if err != nil {
			return fmt.Errorf("failed to create PGVector store: %v", err)
		}

		ragClient = rag.NewPGVector(cfg, providerManager, pgVectorStore)
		log.Info().Msgf("Using PGVector for RAG")
	default:
		return fmt.Errorf("unknown RAG provider: %s", cfg.RAG.DefaultRagProvider)
	}

	controllerOptions := controller.Options{
		Config:               cfg,
		Store:                postgresStore,
		PubSub:               ps,
		RAG:                  ragClient,
		Extractor:            extractor,
		GPTScriptExecutor:    gse,
		Filestore:            fs,
		Janitor:              janitor,
		Notifier:             notifier,
		ProviderManager:      providerManager,
		DataprepOpenAIClient: dataprepOpenAIClient,
		Scheduler:            scheduler,
		RunnerController:     runnerController,
	}

	appController, err = controller.NewController(ctx, controllerOptions)
	if err != nil {
		return err
	}

	err = appController.Initialize()
	if err != nil {
		return err
	}

	// Initialize browser pool
	browserPool, err := browser.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create browser pool: %w", err)
	}

	knowledgeReconciler, err := knowledge.New(cfg, postgresStore, fs, extractor, ragClient, browserPool)
	if err != nil {
		return err
	}

	go func() {
		if err := knowledgeReconciler.Start(ctx); err != nil {
			log.Error().Err(err).Msg("failed to start knowledge reconciler")
		}
	}()

	trigger := trigger.NewTriggerManager(cfg, postgresStore, appController)
	// Start integrations
	go trigger.Start(ctx)

	stripe := stripe.NewStripe(
		cfg.Stripe,
		func(eventType types.SubscriptionEventType, user types.StripeUser) error {
			return appController.HandleSubscriptionEvent(eventType, user)
		},
	)

	// Initialize ping service if not disabled
	var pingService *version.PingService
	if !cfg.DisableVersionPing {
		pingService = version.NewPingService(postgresStore, cfg.LicenseKey, cfg.LaunchpadURL, &cfg.Keycloak)
		pingService.Start(ctx)
		defer pingService.Stop()
	}

	adminConfig := auth.AdminConfig{
		AdminUserIDs: cfg.WebServer.AdminIDs,
		AdminUserSrc: cfg.WebServer.AdminSrc,
	}

	var oidcAuthenticator auth.OIDCAuthenticator

	if cfg.OIDC.Enabled {
		oidcAuthenticator, err = auth.NewOIDCJwtAuthenticator(&cfg.OIDC, &adminConfig)
		if err != nil {
			return fmt.Errorf("unable to create oidc authenticator: %v", err)
		}
	} else {
		oidcAuthenticator, err = auth.NewKeycloakAuthenticator(
			gocloakClient.Client,
			&cfg.Keycloak,
			gocloakClient.Token,
			userRetriever,
			&adminConfig,
		)
		if err != nil {
			return fmt.Errorf("failed to create keycloak authenticator: %v", err)
		}
	}

	authConfig := &server.AuthConfig{
		OIDCAuth:   oidcAuthenticator,
		RunnerAuth: auth.RunnerTokenAuthenticator{RunnerToken: cfg.WebServer.RunnerToken},
		ApiKeyAuth: auth.ApiKeyAuthenticator{
			Store:         postgresStore,
			UserRetriever: userRetriever,
			AdminConfig:   &adminConfig,
		},
	}

	server, err := server.NewServer(
		cfg,
		postgresStore,
		ps,
		gse,
		providerManager,
		helixInference,
		authConfig,
		stripe,
		appController,
		janitor,
		knowledgeReconciler,
		scheduler,
		pingService,
	)
	if err != nil {
		return err
	}

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
