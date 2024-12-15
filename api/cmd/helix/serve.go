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
	system.SetupLogging()

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	fs, err := getFilestore(ctx, cfg)
	if err != nil {
		return err
	}

	store, err := store.NewPostgresStore(cfg.Store)
	if err != nil {
		return err
	}

	ps, err := pubsub.New(cfg.PubSub.StoreDir)
	if err != nil {
		return err
	}

	if cfg.WebServer.RunnerToken == "" {
		return fmt.Errorf("runner token is required")
	}

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(&cfg.Keycloak)
	if err != nil {
		return fmt.Errorf("failed to create keycloak authenticator: %v", err)
	}

	notifier, err := notification.New(&cfg.Notifications, keycloakAuthenticator)
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

	// Must use the same allocator for both new LLM requests and old sessions
	scheduler := scheduler.NewScheduler(ctx, cfg, func(work *scheduler.Workload, err error) {
		// This function describes what happens when errors occur in jobs.
		// Each request type (session vs. LLM requests) has a differeht code path handling results,
		// hence for now we need to separate cases to handle errors.
		switch work.WorkloadType {
		case scheduler.WorkloadTypeLLMInferenceRequest:
			log.Warn().Err(err).Str("id", work.ID()).Msg("error scheduling work, removing from queue")
			req := work.LLMInferenceRequest()
			resp := &types.RunnerLLMInferenceResponse{
				RequestID:     req.RequestID,
				OwnerID:       req.OwnerID,
				SessionID:     req.SessionID,
				InteractionID: req.InteractionID,
				Error:         err.Error(),
				Done:          true,
			}
			bts, err := json.Marshal(resp)
			if err != nil {
				log.Error().Err(err).Str("id", work.ID()).Msg("error marshalling runner response")
			}

			err = ps.Publish(context.Background(), pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID), bts)
			if err != nil {
				log.Error().Err(err).Str("id", work.ID()).Msg("error publishing runner response")
			}
		case scheduler.WorkloadTypeSession:
			// If we can't retry, write an error to the request and continue so it takes it off
			// the queue
			errSession := work.Session()
			errSession.Interactions = append(errSession.Interactions, &types.Interaction{
				Creator: types.CreatorTypeSystem,
				Error:   err.Error(),
				Message: "Error scheduling session",
			})
			_, err = store.UpdateSession(ctx, *errSession)
			if err != nil {
				log.Error().Err(err).Msg("error updating session")
			}
		default:
			log.Error().Str("workload_type", string(work.WorkloadType)).Msg("unknown workload type")
		}
	})

	helixInference := openai.NewInternalHelixServer(cfg, ps, scheduler)

	// controllerOpenAIClient, err := createOpenAIClient(cfg, helixInference)
	// if err != nil {
	// 	return err
	// }

	logStores := []logger.LogStore{
		store,
		// TODO: bigquery
	}

	providerManager := manager.NewProviderManager(cfg, helixInference, logStores...)

	// controllerOpenAIClient = logger.Wrap(cfg, controllerOpenAIClient, logStores...)

	dataprepOpenAIClient, err := createDataPrepOpenAIClient(cfg, helixInference)
	if err != nil {
		return err
	}
	dataprepOpenAIClient = logger.Wrap(cfg, cfg.FineTuning.Provider, dataprepOpenAIClient, logStores...)

	var ragClient rag.RAG

	switch cfg.RAG.DefaultRagProvider {
	case "typesense":
		ragSettings := &types.RAGSettings{}
		ragSettings.Typesense.URL = cfg.RAG.Typesense.URL
		ragSettings.Typesense.APIKey = cfg.RAG.Typesense.APIKey
		ragClient, err = rag.NewTypesense(ragSettings)
		if err != nil {
			return fmt.Errorf("failed to create typesense RAG client: %v", err)
		}
		log.Info().Msgf("Using Typesense for RAG")
	case "llamaindex":
		ragClient = rag.NewLlamaindex(&types.RAGSettings{
			IndexURL:  cfg.RAG.Llamaindex.RAGIndexingURL,
			QueryURL:  cfg.RAG.Llamaindex.RAGQueryURL,
			DeleteURL: cfg.RAG.Llamaindex.RAGDeleteURL,
		})
		log.Info().Msgf("Using Llamaindex for RAG")
	default:
		return fmt.Errorf("unknown RAG provider: %s", cfg.RAG.DefaultRagProvider)
	}

	var appController *controller.Controller

	controllerOptions := controller.ControllerOptions{
		Config:               cfg,
		Store:                store,
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
	}

	appController, err = controller.NewController(ctx, controllerOptions)
	if err != nil {
		return err
	}

	err = appController.Initialize()
	if err != nil {
		return err
	}

	go appController.Start(ctx)

	// Initialize browser pool
	browserPool, err := browser.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create browser pool: %w", err)
	}

	knowledgeReconciler, err := knowledge.New(cfg, store, fs, extractor, ragClient, browserPool)
	if err != nil {
		return err
	}

	go func() {
		if err := knowledgeReconciler.Start(ctx); err != nil {
			log.Error().Err(err).Msg("failed to start knowledge reconciler")
		}
	}()

	trigger := trigger.NewTriggerManager(cfg, store, appController)
	// Start integrations
	go trigger.Start(ctx)

	stripe := stripe.NewStripe(
		cfg.Stripe,
		func(eventType types.SubscriptionEventType, user types.StripeUser) error {
			return appController.HandleSubscriptionEvent(eventType, user)
		},
	)

	server, err := server.NewServer(cfg, store, ps, gse, providerManager, helixInference, keycloakAuthenticator, stripe, appController, janitor, knowledgeReconciler, scheduler)
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
