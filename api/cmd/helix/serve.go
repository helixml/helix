package helix

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/lukemarsden/helix/api/pkg/controller"
	"github.com/lukemarsden/helix/api/pkg/dataprep/text"
	"github.com/lukemarsden/helix/api/pkg/filestore"
	"github.com/lukemarsden/helix/api/pkg/janitor"
	"github.com/lukemarsden/helix/api/pkg/server"
	"github.com/lukemarsden/helix/api/pkg/store"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type ServeOptions struct {
	DataPrepTextOptions text.DataPrepTextOptions
	ControllerOptions   controller.ControllerOptions
	FilestoreOptions    filestore.FileStoreOptions
	JanitorOptions      janitor.JanitorOptions
	StoreOptions        store.StoreOptions
	ServerOptions       server.ServerOptions
}

func NewServeOptions() *ServeOptions {
	return &ServeOptions{
		DataPrepTextOptions: text.DataPrepTextOptions{
			Module:       text.DataPrepModule(getDefaultServeOptionString("DATA_PREP_TEXT_MODULE", string(text.DataPrepModule_GPT4))),
			APIKey:       getDefaultServeOptionString("OPENAI_API_KEY", ""),
			ChunkSize:    getDefaultServeOptionInt("DATA_PREP_TEXT_CHUNK_SIZE", 4096),
			OverflowSize: getDefaultServeOptionInt("DATA_PREP_TEXT_OVERFLOW_SIZE", 256),
			// we are exceeding openAI window size at > 30 questions
			QuestionsPerChunk: getDefaultServeOptionInt("DATA_PREP_TEXT_QUESTIONS_PER_CHUNK", 30),
			Temperature:       getDefaultServeOptionFloat("DATA_PREP_TEXT_TEMPERATURE", 0.5),
		},
		ControllerOptions: controller.ControllerOptions{
			FilePrefixGlobal:             getDefaultServeOptionString("FILE_PREFIX_GLOBAL", "dev"),
			FilePrefixUser:               getDefaultServeOptionString("FILE_PREFIX_USER", "users/{{.Owner}}"),
			FilePrefixResults:            getDefaultServeOptionString("FILE_PREFIX_RESULTS", "results"),
			TextExtractionURL:            getDefaultServeOptionString("TEXT_EXTRACTION_URL", ""),
			DataPrepConcurrency:          getDefaultServeOptionInt("DATA_PREP_CONCURRENCY", 5),
			SchedulingDecisionBufferSize: getDefaultServeOptionInt("SCHEDULING_DECISION_BUFFER_SIZE", 10),
		},
		FilestoreOptions: filestore.FileStoreOptions{
			Type:         filestore.FileStoreType(getDefaultServeOptionString("FILESTORE_TYPE", "fs")),
			LocalFSPath:  getDefaultServeOptionString("FILESTORE_LOCALFS_PATH", "/tmp/helix/filestore"),
			GCSKeyBase64: getDefaultServeOptionString("FILESTORE_GCS_KEY_BASE64", ""),
			GCSKeyFile:   getDefaultServeOptionString("FILESTORE_GCS_KEY_FILE", ""),
			GCSBucket:    getDefaultServeOptionString("FILESTORE_GCS_BUCKET", ""),
		},
		StoreOptions: store.StoreOptions{
			Host:        getDefaultServeOptionString("POSTGRES_HOST", ""),
			Port:        getDefaultServeOptionInt("POSTGRES_PORT", 5432),
			Database:    getDefaultServeOptionString("POSTGRES_DATABASE", "helix"),
			Username:    getDefaultServeOptionString("POSTGRES_USER", ""),
			Password:    getDefaultServeOptionString("POSTGRES_PASSWORD", ""),
			AutoMigrate: true,
		},
		ServerOptions: server.ServerOptions{
			URL:           getDefaultServeOptionString("SERVER_URL", ""),
			Host:          getDefaultServeOptionString("SERVER_HOST", "0.0.0.0"),
			Port:          getDefaultServeOptionInt("SERVER_PORT", 80), //nolint:gomnd
			KeyCloakURL:   getDefaultServeOptionString("KEYCLOAK_URL", ""),
			KeyCloakToken: getDefaultServeOptionString("KEYCLOAK_TOKEN", ""),
			// if this is defined it means runner auth is enabled
			RunnerToken: getDefaultServeOptionString("RUNNER_TOKEN", ""),
			AdminIDs:    getDefaultServeOptionStringArray("ADMIN_USER_IDS", []string{}),
		},
		JanitorOptions: janitor.JanitorOptions{
			SlackWebhookURL: getDefaultServeOptionString("JANITOR_SLACK_WEBHOOK_URL", ""),
		},
	}
}

func newServeCmd() *cobra.Command {
	allOptions := NewServeOptions()

	serveCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Start the helix api server.",
		Long:    "Start the helix api server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd, allOptions)
		},
	}

	var dataprepModule string
	serveCmd.PersistentFlags().StringVar(
		&dataprepModule, "dataprep-module", string(allOptions.DataPrepTextOptions.Module),
		`Which module to use for text data prep`,
	)
	allOptions.DataPrepTextOptions.Module = text.DataPrepModule(dataprepModule)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.DataPrepTextOptions.APIKey, "openai-key", allOptions.DataPrepTextOptions.APIKey,
		`The API Key for OpenAI`,
	)

	serveCmd.PersistentFlags().IntVar(
		&allOptions.DataPrepTextOptions.ChunkSize, "dataprep-chunk-size", allOptions.DataPrepTextOptions.ChunkSize,
		`The chunk size for the text data prep`,
	)

	serveCmd.PersistentFlags().IntVar(
		&allOptions.DataPrepTextOptions.OverflowSize, "dataprep-overflow-size", allOptions.DataPrepTextOptions.OverflowSize,
		`The overflow size for the text data prep`,
	)

	serveCmd.PersistentFlags().IntVar(
		&allOptions.DataPrepTextOptions.QuestionsPerChunk, "dataprep-questions-per-chunk", allOptions.DataPrepTextOptions.QuestionsPerChunk,
		`The questions per chunk for the text data prep`,
	)

	serveCmd.PersistentFlags().Float32Var(
		&allOptions.DataPrepTextOptions.Temperature, "dataprep-temperature", allOptions.DataPrepTextOptions.Temperature,
		`The temperature for the text data prep prompt`,
	)

	// ControllerOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ControllerOptions.FilePrefixGlobal, "file-prefix-global", allOptions.ControllerOptions.FilePrefixGlobal,
		`The global prefix path for the filestore.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ControllerOptions.FilePrefixUser, "file-prefix-user", allOptions.ControllerOptions.FilePrefixUser,
		`The go template that produces the prefix path for a user.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ControllerOptions.FilePrefixResults, "file-prefix-results", allOptions.ControllerOptions.FilePrefixResults,
		`The go template that produces the prefix path for a user.`,
	)

	serveCmd.PersistentFlags().IntVar(
		&allOptions.ControllerOptions.SchedulingDecisionBufferSize, "scheduling-decision-buffer-size", allOptions.ControllerOptions.SchedulingDecisionBufferSize,
		`How many scheduling decisions to buffer before we start dropping them.`,
	)

	serveCmd.PersistentFlags().IntVar(
		&allOptions.ControllerOptions.DataPrepConcurrency, "dataprep-concurrency", allOptions.ControllerOptions.DataPrepConcurrency,
		`How many data prep steps to run concurrently per user`,
	)

	// FileStoreOptions
	var filestoreType string
	serveCmd.PersistentFlags().StringVar(
		&filestoreType, "filestore-type", string(allOptions.FilestoreOptions.Type),
		`What type of filestore should we use (fs | gcs).`,
	)
	allOptions.FilestoreOptions.Type = filestore.FileStoreType(filestoreType)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.FilestoreOptions.LocalFSPath, "filestore-localfs-path", allOptions.FilestoreOptions.LocalFSPath,
		`The local path that is the root for the local fs filestore.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.FilestoreOptions.GCSKeyBase64, "filestore-gcs-key-base64", allOptions.FilestoreOptions.GCSKeyBase64,
		`The base64 encoded service account json file for GCS.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.FilestoreOptions.GCSKeyFile, "filestore-gcs-key-file", allOptions.FilestoreOptions.GCSKeyFile,
		`The local path to the service account json file for GCS.`,
	)

	serveCmd.PersistentFlags().StringVar(
		&allOptions.FilestoreOptions.GCSBucket, "filestore-gcs-bucket", allOptions.FilestoreOptions.GCSBucket,
		`The bucket we are storing things in GCS.`,
	)

	// StoreOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Host, "postgres-host", allOptions.StoreOptions.Host,
		`The host to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.StoreOptions.Port, "postgres-port", allOptions.StoreOptions.Port,
		`The port to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Database, "postgres-database", allOptions.StoreOptions.Database,
		`The database to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Username, "postgres-username", allOptions.StoreOptions.Username,
		`The username to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Password, "postgres-password", allOptions.StoreOptions.Password,
		`The password to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().BoolVar(
		&allOptions.StoreOptions.AutoMigrate, "postgres-auto-migrate", allOptions.StoreOptions.AutoMigrate,
		`Should we automatically run the migrations?`,
	)

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
		&allOptions.ServerOptions.KeyCloakURL, "keycloak-url", allOptions.ServerOptions.KeyCloakURL,
		`The url for the keycloak server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.KeyCloakToken, "keycloak-token", allOptions.ServerOptions.KeyCloakToken,
		`The api token for the keycloak server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.RunnerToken, "runner-token", allOptions.ServerOptions.RunnerToken,
		`The token for runner auth.`,
	)
	serveCmd.PersistentFlags().StringArrayVar(
		&allOptions.ServerOptions.AdminIDs, "admin-ids", allOptions.ServerOptions.AdminIDs,
		`Keycloak admin IDs`,
	)

	// JanitorOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.JanitorOptions.SlackWebhookURL, "janitor-slack-webhook", allOptions.JanitorOptions.SlackWebhookURL,
		`The slack webhook URL to ping messages to.`,
	)

	return serveCmd
}

func getFilestore(ctx context.Context, options *ServeOptions) (filestore.FileStore, error) {
	var store filestore.FileStore
	if options.ServerOptions.URL == "" {
		return nil, fmt.Errorf("server url is required")
	}
	if options.FilestoreOptions.Type == filestore.FileStoreTypeLocalFS {
		if options.FilestoreOptions.LocalFSPath == "" {
			return nil, fmt.Errorf("local fs path is required")
		}
		rootPath := filepath.Join(options.FilestoreOptions.LocalFSPath, options.ControllerOptions.FilePrefixGlobal)
		if _, err := os.Stat(rootPath); os.IsNotExist(err) {
			err := os.MkdirAll(rootPath, 0755)
			if err != nil {
				return nil, err
			}
		}
		store = filestore.NewFileSystemStorage(options.FilestoreOptions.LocalFSPath, fmt.Sprintf("%s/api/v1/filestore/viewer", options.ServerOptions.URL))
	} else if options.FilestoreOptions.Type == filestore.FileStoreTypeLocalGCS {
		if options.FilestoreOptions.GCSKeyBase64 != "" {
			keyfile, err := func() (string, error) {
				decoded, err := base64.StdEncoding.DecodeString(options.FilestoreOptions.GCSKeyBase64)
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
			options.FilestoreOptions.GCSKeyFile = keyfile
		}
		if options.FilestoreOptions.GCSKeyFile == "" {
			return nil, fmt.Errorf("gcs key is required")
		}
		if _, err := os.Stat(options.FilestoreOptions.GCSKeyFile); os.IsNotExist(err) {
			return nil, fmt.Errorf("gcs key file does not exist")
		}
		gcs, err := filestore.NewGCSStorage(ctx, options.FilestoreOptions.GCSKeyFile, options.FilestoreOptions.GCSBucket)
		if err != nil {
			return nil, err
		}
		store = gcs
	} else {
		return nil, fmt.Errorf("unknown filestore type: %s", options.FilestoreOptions.Type)
	}
	// let's make sure the global prefix folder exists
	// from here on it will be user directories being created
	_, err := store.CreateFolder(ctx, options.ControllerOptions.FilePrefixGlobal)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func serve(cmd *cobra.Command, options *ServeOptions) error {
	system.SetupLogging()

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	fs, err := getFilestore(ctx, options)
	if err != nil {
		return err
	}

	store, err := store.NewPostgresStore(options.StoreOptions)
	if err != nil {
		return err
	}

	if options.DataPrepTextOptions.APIKey == "" {
		return fmt.Errorf("openai api key is required")
	}

	options.JanitorOptions.AppURL = options.ServerOptions.URL
	janitor := janitor.NewJanitor(options.JanitorOptions)

	var appController *controller.Controller

	options.ControllerOptions.Store = store
	options.ControllerOptions.Filestore = fs
	options.ControllerOptions.Janitor = janitor

	// a text.DataPrepText factory that runs jobs on ourselves
	// dogfood nom nom nom
	options.ControllerOptions.DataPrepTextFactory = func(session *types.Session) (text.DataPrepTextQuestionGenerator, *text.DataPrepTextSplitter, error) {
		if appController == nil {
			return nil, nil, fmt.Errorf("app controller is not initialized")
		}

		var questionGenerator text.DataPrepTextQuestionGenerator
		var err error

		// if we are using openai then let's do that
		// otherwise - we use our own mistral plugin
		if options.DataPrepTextOptions.Module == text.DataPrepModule_GPT4 {
			questionGenerator, err = text.NewDataPrepTextGPT4(options.DataPrepTextOptions)
			if err != nil {
				return nil, nil, err
			}
		} else if options.DataPrepTextOptions.Module == text.DataPrepModule_GPT3Point5 {
			questionGenerator, err = text.NewDataPrepTextGPT3Point5(options.DataPrepTextOptions)
			if err != nil {
				return nil, nil, err
			}
		} else if options.DataPrepTextOptions.Module == text.DataPrepModule_HelixMistral {
			// we give the mistal data prep module a way to run and read sessions
			questionGenerator, err = text.NewDataPrepTextHelixMistral(
				options.DataPrepTextOptions,
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
		} else {
			return nil, nil, fmt.Errorf("unknown data prep module: %s", options.DataPrepTextOptions.Module)
		}

		splitter, err := text.NewDataPrepSplitter(text.DataPrepTextSplitterOptions{
			ChunkSize: options.DataPrepTextOptions.ChunkSize,
			Overflow:  options.DataPrepTextOptions.OverflowSize,
		})

		if err != nil {
			return nil, nil, err
		}

		return questionGenerator, splitter, nil
	}

	if options.FilestoreOptions.Type == filestore.FileStoreTypeLocalFS {
		options.ServerOptions.LocalFilestorePath = options.FilestoreOptions.LocalFSPath
	}

	options.DataPrepTextOptions.Concurrency = options.ControllerOptions.DataPrepConcurrency

	appController, err = controller.NewController(ctx, options.ControllerOptions)
	if err != nil {
		return err
	}

	err = appController.Initialize()
	if err != nil {
		return err
	}

	go appController.StartLooping()

	server, err := server.NewServer(options.ServerOptions, store, appController)
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
