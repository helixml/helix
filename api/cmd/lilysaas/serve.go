package lilysaas

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/bacalhau-project/lilysaas/api/pkg/filestore"
	"github.com/bacalhau-project/lilysaas/api/pkg/job"
	"github.com/bacalhau-project/lilysaas/api/pkg/server"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type AllOptions struct {
	ControllerOptions controller.ControllerOptions
	FilestoreOptions  filestore.FileStoreOptions
	StoreOptions      store.StoreOptions
	ServerOptions     server.ServerOptions
}

func NewAllOptions() *AllOptions {
	return &AllOptions{
		ControllerOptions: controller.ControllerOptions{
			FilePrefixGlobal:  getDefaultServeOptionString("FILE_PREFIX_GLOBAL", "dev"),
			FilePrefixUser:    getDefaultServeOptionString("FILE_PREFIX_USER", "users/{{.Owner}}"),
			FilePrefixResults: getDefaultServeOptionString("FILE_PREFIX_RESULTS", "results"),
		},
		FilestoreOptions: filestore.FileStoreOptions{
			Type:         filestore.FileStoreType(getDefaultServeOptionString("FILESTORE_TYPE", "fs")),
			LocalFSPath:  getDefaultServeOptionString("FILESTORE_LOCALFS_PATH", "/tmp/lilysaas/filestore"),
			GCSKeyBase64: getDefaultServeOptionString("FILESTORE_GCS_KEY_BASE64", ""),
			GCSKeyFile:   getDefaultServeOptionString("FILESTORE_GCS_KEY_FILE", ""),
			GCSBucket:    getDefaultServeOptionString("FILESTORE_GCS_BUCKET", ""),
		},
		StoreOptions: store.StoreOptions{
			Host:        getDefaultServeOptionString("POSTGRES_HOST", ""),
			Port:        getDefaultServeOptionInt("POSTGRES_PORT", 5432),
			Database:    getDefaultServeOptionString("POSTGRES_DATABASE", "lilysaas"),
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
		},
	}
}

func newServeCmd() *cobra.Command {
	allOptions := NewAllOptions()

	serveCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Start the lilysaas api server.",
		Long:    "Start the lilysaas api server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd, allOptions)
		},
	}

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

	return serveCmd
}

func getFilestore(ctx context.Context, options *AllOptions) (filestore.FileStore, error) {
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

func serve(cmd *cobra.Command, options *AllOptions) error {

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

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

	jobRunner, err := job.NewJobRunner(ctx)
	if err != nil {
		return err
	}

	store, err := store.NewPostgresStore(options.StoreOptions)
	if err != nil {
		return err
	}

	options.ControllerOptions.Store = store
	options.ControllerOptions.Filestore = fs
	options.ControllerOptions.JobRunner = jobRunner

	if options.FilestoreOptions.Type == filestore.FileStoreTypeLocalFS {
		options.ServerOptions.LocalFilestorePath = options.FilestoreOptions.LocalFSPath
	}

	controller, err := controller.NewController(ctx, options.ControllerOptions)
	if err != nil {
		return err
	}

	err = controller.Start()
	if err != nil {
		return err
	}

	server, err := server.NewServer(options.ServerOptions, store, controller)
	if err != nil {
		return err
	}

	log.Info().Msgf("LilySaaS server listening on %s:%d", options.ServerOptions.Host, options.ServerOptions.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}
