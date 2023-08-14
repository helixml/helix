package lilysaas

import (
	"os"
	"os/signal"

	"github.com/bacalhau-project/lilysaas/api/pkg/contract"
	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/bacalhau-project/lilysaas/api/pkg/server"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/bacalhau-project/lilysaas/api/pkg/system"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type AllOptions struct {
	ControllerOptions controller.ControllerOptions
	StoreOptions      store.StoreOptions
	ServerOptions     server.ServerOptions
	ContractOptions   contract.ContractOptions
}

func NewAllOptions() *AllOptions {
	return &AllOptions{
		ControllerOptions: controller.ControllerOptions{
			AppURL: getDefaultServeOptionString("APP_URL", ""),
		},
		StoreOptions: store.StoreOptions{
			Host:        getDefaultServeOptionString("POSTGRES_HOST", ""),
			Port:        getDefaultServeOptionInt("POSTGRES_PORT", 5432),
			Database:    getDefaultServeOptionString("POSTGRES_DATABASE", "lilysaas"),
			Username:    getDefaultServeOptionString("POSTGRES_USERNAME", ""),
			Password:    getDefaultServeOptionString("POSTGRES_PASSWORD", ""),
			AutoMigrate: true,
		},
		ServerOptions: server.ServerOptions{
			Host: getDefaultServeOptionString("BIND_HOST", "0.0.0.0"),
			Port: getDefaultServeOptionInt("BIND_PORT", 80), //nolint:gomnd
		},
		ContractOptions: contract.ContractOptions{
			Address:     getDefaultServeOptionString("CONTRACT_ADDRESS", ""),
			PrivateKey:  getDefaultServeOptionString("WALLET_PRIVATE_KEY", ""),
			RPCEndpoint: getDefaultServeOptionString("RPC_ENDPOINT", ""),
			ChainID:     getDefaultServeOptionString("CHAIN_ID", ""),
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
		&allOptions.ControllerOptions.AppURL, "app-url", allOptions.ControllerOptions.AppURL,
		`The URL the api server is listening on (used for image URLs).`,
	)

	// StoreOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Host, "host", allOptions.StoreOptions.Host,
		`The host to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.StoreOptions.Port, "port", allOptions.StoreOptions.Port,
		`The port to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Database, "database", allOptions.StoreOptions.Database,
		`The database to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Username, "username", allOptions.StoreOptions.Username,
		`The username to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.StoreOptions.Password, "password", allOptions.StoreOptions.Password,
		`The password to connect to the postgres server.`,
	)
	serveCmd.PersistentFlags().BoolVar(
		&allOptions.StoreOptions.AutoMigrate, "auto-migrate", allOptions.StoreOptions.AutoMigrate,
		`Should we automatically run the migrations?`,
	)

	// ServerOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.Host, "host", allOptions.ServerOptions.Host,
		`The host to bind the api server to.`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.ServerOptions.Port, "port", allOptions.ServerOptions.Port,
		`The port to bind the api server to.`,
	)

	// ContractOptions
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ContractOptions.Address, "contract-address", allOptions.ContractOptions.Address,
		`The host to connect the bacalhau api client`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ContractOptions.PrivateKey, "private-key", allOptions.ContractOptions.PrivateKey,
		`The host to connect the bacalhau api client`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ContractOptions.RPCEndpoint, "rpc-endpoint", allOptions.ContractOptions.RPCEndpoint,
		`The host to connect the bacalhau api client`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ContractOptions.ChainID, "chainid", allOptions.ContractOptions.ChainID,
		`The host to connect the bacalhau api client`,
	)

	return serveCmd
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

	contract, err := contract.NewContract(options.ContractOptions)
	if err != nil {
		return err
	}

	store, err := store.NewPostgresStore(options.StoreOptions)
	if err != nil {
		return err
	}

	controller, err := controller.NewController(controller.ControllerOptions{
		AppURL:   options.ControllerOptions.AppURL,
		Contract: contract,
		Store:    store,
	})

	err = controller.Start(ctx)
	if err != nil {
		return err
	}

	server, err := server.NewServer(options.ServerOptions, controller)
	if err != nil {
		return err
	}

	log.Info().Msgf("Lilysaas server listening on %s:%d", options.ServerOptions.Host, options.ServerOptions.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}
