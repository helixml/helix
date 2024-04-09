package helix

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/dataprep/text"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/janitor"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type ServeOptions struct {
	ServerOptions server.ServerOptions
	StripeOptions stripe.StripeOptions

	Cfg *config.ServerConfig

	// NotifierCfg is used to configure the notifier which sends emails
	// to users on finetuning progress
	NotifierCfg *config.Notifications

	// KeycloakCfg is used to configure the keycloak authenticator, which
	// is used to get user information from the keycloak server
	KeycloakCfg *config.Keycloak
}

func NewServeOptions() (*ServeOptions, error) {
	serverConfig, err := config.LoadServerConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load server config: %v", err)
	}

	if serverConfig.Controller.FilestorePresignSecret == "" {
		serverConfig.Controller.FilestorePresignSecret = system.GenerateUUID()
	}

	return &ServeOptions{
		ServerOptions: server.ServerOptions{
			// TODO: unify the config by using the config pkg
			// and then we can get rid of all those flags too
			Config:      &serverConfig,
			URL:         getDefaultServeOptionString("SERVER_URL", ""),
			Host:        getDefaultServeOptionString("SERVER_HOST", "0.0.0.0"),
			Port:        getDefaultServeOptionInt("SERVER_PORT", 80), //nolint:gomnd
			FrontendURL: getDefaultServeOptionString("FRONTEND_URL", "http://frontend:8081"),
			// if this is defined it means runner auth is enabled
			RunnerToken:    getDefaultServeOptionString("RUNNER_TOKEN", ""),
			AdminIDs:       getDefaultServeOptionStringArray("ADMIN_USER_IDS", []string{}),
			EvalUserID:     getDefaultServeOptionString("EVAL_USER_ID", ""),
			ToolsGlobalIDS: getDefaultServeOptionStringArray("TOOLS_GLOBAL_IDS", []string{}),
		},
		StripeOptions: stripe.StripeOptions{
			SecretKey:            serverConfig.Stripe.SecretKey,
			WebhookSigningSecret: serverConfig.Stripe.WebhookSigningSecret,
			PriceLookupKey:       serverConfig.Stripe.PriceLookupKey,
		},
		Cfg:         &serverConfig,
		KeycloakCfg: &serverConfig.Keycloak,
		NotifierCfg: &serverConfig.Notifications,
	}, nil
}

func newServeCmd() *cobra.Command {
	allOptions, err := NewServeOptions()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create serve options")
	}

	serveCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Start the helix api server.",
		Long:    "Start the helix api server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := serve(cmd, allOptions, allOptions.Cfg)
			if err != nil {
				log.Fatal().Err(err).Msg("failed to run server")
			}
			return nil
		},
	}

	// ServerOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.URL, "server-url", allOptions.ServerOptions.URL,
		`The URL the api server is listening on.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.Host, "server-host", allOptions.ServerOptions.Host,
		`The host to bind the api server to.`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.ServerOptions.Port, "server-port", allOptions.ServerOptions.Port,
		`The port to bind the api server to.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.RunnerToken, "runner-token", allOptions.ServerOptions.RunnerToken,
		`The token for runner auth.`,
	)
	serveCmd.PersistentFlags().StringArrayVar(
		&allOptions.ServerOptions.AdminIDs, "admin-ids", allOptions.ServerOptions.AdminIDs,
		`Keycloak admin IDs`,
	)

	// StripeOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StripeOptions.SecretKey, "stripe-secret-key", allOptions.StripeOptions.SecretKey,
		`The secret key for stripe.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.StripeOptions.WebhookSigningSecret, "stripe-webhook-signing-secret", allOptions.StripeOptions.WebhookSigningSecret,
		`The webhook signing secret for stripe.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.StripeOptions.PriceLookupKey, "stripe-price-lookup-key", allOptions.StripeOptions.PriceLookupKey,
		`The lookup key for the stripe price.`,
	)

	return serveCmd
}

func getFilestore(ctx context.Context, options *ServeOptions, cfg *config.ServerConfig) (filestore.FileStore, error) {
	var store filestore.FileStore
	if options.ServerOptions.URL == "" {
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
		store = filestore.NewFileSystemStorage(cfg.FileStore.LocalFSPath, fmt.Sprintf("%s/api/v1/filestore/viewer", options.ServerOptions.URL), cfg.Controller.FilestorePresignSecret)
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

func serve(cmd *cobra.Command, options *ServeOptions, cfg *config.ServerConfig) error {
	system.SetupLogging()

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	fs, err := getFilestore(ctx, options, cfg)
	if err != nil {
		return err
	}

	store, err := store.NewPostgresStore(cfg.Store)
	if err != nil {
		return err
	}

	if options.ServerOptions.RunnerToken == "" {
		return fmt.Errorf("runner token is required")
	}

	keycloakAuthenticator, err := auth.NewKeycloakAuthenticator(options.KeycloakCfg)
	if err != nil {
		return fmt.Errorf("failed to create keycloak authenticator: %v", err)
	}

	notifier, err := notification.New(options.NotifierCfg, keycloakAuthenticator)
	if err != nil {
		return fmt.Errorf("failed to create notifier: %v", err)
	}

	janitor := janitor.NewJanitor(cfg.Janitor)
	err = janitor.Initialize()
	if err != nil {
		return err
	}

	planner, err := tools.NewChainStrategy(options.Cfg)
	if err != nil {
		return fmt.Errorf("failed to create tools planner: %v", err)
	}

	var appController *controller.Controller

	controllerOptions := controller.ControllerOptions{
		Store:     store,
		Filestore: fs,
		Janitor:   janitor,
		Notifier:  notifier,
		Planner:   planner,
	}

	// a text.DataPrepText factory that runs jobs on ourselves
	// dogfood nom nom nom
	controllerOptions.DataPrepTextFactory = func(session *types.Session) (text.DataPrepTextQuestionGenerator, *text.DataPrepTextSplitter, error) {
		if appController == nil {
			return nil, nil, fmt.Errorf("app controller is not initialized")
		}

		var questionGenerator text.DataPrepTextQuestionGenerator
		var err error

		// if we are using openai then let's do that
		// otherwise - we use our own mistral plugin
		if cfg.DataPrepText.Module == types.DataPrepModule_HelixMistral {
			// we give the mistal data prep module a way to run and read sessions
			questionGenerator, err = text.NewDataPrepTextHelixMistral(
				cfg.DataPrepText,
				session,
				func(req types.CreateSessionRequest) (*types.Session, error) {
					return appController.CreateSession(types.RequestContext{}, req)
				},
				func(id string) (*types.Session, error) {
					return appController.Options.Store.GetSession(context.Background(), id)
				},
			)
			if err != nil {
				return nil, nil, err
			}
		} else if cfg.DataPrepText.Module == types.DataPrepModule_Dynamic {
			// empty values = use defaults
			questionGenerator = text.NewDynamicDataPrep("", []string{})
		} else {
			return nil, nil, fmt.Errorf("unknown data prep module: %s", cfg.DataPrepText.Module)
		}

		splitter, err := text.NewDataPrepSplitter(text.DataPrepTextSplitterOptions{
			ChunkSize: questionGenerator.GetChunkSize(),
			Overflow:  cfg.DataPrepText.OverflowSize,
		})

		if err != nil {
			return nil, nil, err
		}

		return questionGenerator, splitter, nil
	}

	if cfg.FileStore.Type == types.FileStoreTypeLocalFS {
		options.ServerOptions.LocalFilestorePath = cfg.FileStore.LocalFSPath
	}

	// options.DataPrepTextOptions.Concurrency = options.ControllerOptions.DataPrepConcurrency

	appController, err = controller.NewController(ctx, controllerOptions)
	if err != nil {
		return err
	}

	err = appController.Initialize()
	if err != nil {
		return err
	}

	go appController.StartLooping()

	options.StripeOptions.AppURL = options.ServerOptions.URL
	stripe := stripe.NewStripe(
		options.StripeOptions,
		func(eventType types.SubscriptionEventType, user types.StripeUser) error {
			return appController.HandleSubscriptionEvent(eventType, user)
		},
	)

	server, err := server.NewServer(options.ServerOptions, store, keycloakAuthenticator, stripe, appController, janitor)
	if err != nil {
		return err
	}

	log.Info().Msgf("Helix server listening on %s:%d", options.ServerOptions.Host, options.ServerOptions.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}
