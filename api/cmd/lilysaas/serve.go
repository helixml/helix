package waterlily

import (
	"os"
	"os/signal"

	"github.com/bacalhau-project/bacalhau/pkg/system"
	"github.com/bacalhau-project/lilysaas/api/pkg/bacalhau"
	"github.com/bacalhau-project/lilysaas/api/pkg/contract"
	"github.com/bacalhau-project/lilysaas/api/pkg/controller"
	"github.com/bacalhau-project/lilysaas/api/pkg/server"
	"github.com/bacalhau-project/lilysaas/api/pkg/store"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type AllOptions struct {
	ControllerOptions controller.ControllerOptions
	StoreOptions      store.StoreOptions
	ServerOptions     server.ServerOptions
	BacalhauOptions   bacalhau.BacalhauOptions
	ContractOptions   contract.ContractOptions
}

func NewAllOptions() *AllOptions {
	return &AllOptions{
		ControllerOptions: controller.ControllerOptions{
			AppURL: getDefaultServeOptionString("APP_URL", ""),
		},
		StoreOptions: store.StoreOptions{
			DataFile: getDefaultServeOptionString("SQLITE_DATA_FILE", ""),
		},
		ServerOptions: server.ServerOptions{
			Host:               getDefaultServeOptionString("BIND_HOST", "0.0.0.0"),
			Port:               getDefaultServeOptionInt("BIND_PORT", 80), //nolint:gomnd
			FilestoreToken:     getDefaultServeOptionString("FILESTORE_TOKEN", ""),
			FilestoreDirectory: getDefaultServeOptionString("FILESTORE_DIRECTORY", ""),
		},
		BacalhauOptions: bacalhau.BacalhauOptions{
			Host: getDefaultServeOptionString("BACALHAU_API_HOST", "ai-art-requester.cluster.world"),
			Port: getDefaultServeOptionInt("BACALHAU_API_PORT", 1234),
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
		Short:   "Start the waterlily api server.",
		Long:    "Start the waterlily api server.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serve(cmd, allOptions)
		},
	}

	serveCmd.PersistentFlags().StringVar(
		&allOptions.ControllerOptions.AppURL, "app-url", allOptions.ControllerOptions.AppURL,
		`The URL the api server is listening on (used for image URLs).`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.Host, "host", allOptions.ServerOptions.Host,
		`The host to bind the api server to.`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.ServerOptions.Port, "port", allOptions.ServerOptions.Port,
		`The port to bind the api server to.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.FilestoreToken, "filestore-token", allOptions.ServerOptions.FilestoreToken,
		`The secret for the filestore.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.ServerOptions.FilestoreDirectory, "filestore-directory", allOptions.ServerOptions.FilestoreDirectory,
		`The directory for the filestore.`,
	)
	serveCmd.PersistentFlags().StringVar(
		&allOptions.BacalhauOptions.Host, "bacalhau-host", allOptions.BacalhauOptions.Host,
		`The host to connect the bacalhau api client`,
	)
	serveCmd.PersistentFlags().IntVar(
		&allOptions.BacalhauOptions.Port, "bacalhau-port", allOptions.BacalhauOptions.Port,
		`The port to connect the bacalhau api client.`,
	)
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

	ctx, rootSpan := system.NewRootSpan(ctx, system.GetTracer(), "waterlily/api/cmd/serve")
	defer rootSpan.End()

	bacalhau, err := bacalhau.NewBacalhauClient(options.BacalhauOptions)
	if err != nil {
		return err
	}

	contract, err := contract.NewContract(options.ContractOptions)
	if err != nil {
		return err
	}

	store, err := store.NewSQLiteStore(options.StoreOptions, true)
	if err != nil {
		return err
	}

	controller, err := controller.NewController(controller.ControllerOptions{
		AppURL:         options.ControllerOptions.AppURL,
		FilestoreToken: options.ServerOptions.FilestoreToken,
		Bacalhau:       bacalhau,
		Contract:       contract,
		Store:          store,
	})

	err = controller.Start(ctx)
	if err != nil {
		return err
	}

	server, err := server.NewServer(options.ServerOptions, controller)
	if err != nil {
		return err
	}

	log.Info().Msgf("Waterlily server listening on %s:%d", options.ServerOptions.Host, options.ServerOptions.Port)

	go func() {
		err := server.ListenAndServe(ctx, cm)
		if err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()
	return nil
}
