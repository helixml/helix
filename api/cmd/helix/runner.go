package helix

import (
	"os"
	"os/signal"

	"github.com/inhies/go-bytesize"
	"github.com/lukemarsden/helix/api/pkg/runner"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

type RunnerOptions struct {
	ApiURL string
	Memory string
}

func NewRunnerOptions() *RunnerOptions {
	return &RunnerOptions{
		ApiURL: getDefaultServeOptionString("API_URL", ""),
		Memory: getDefaultServeOptionString("MEMORY", ""),
	}
}

func newRunnerCmd() *cobra.Command {
	allOptions := NewRunnerOptions()

	runnerCmd := &cobra.Command{
		Use:     "serve",
		Short:   "Start a helix runner.",
		Long:    "Start a helix runner.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if allOptions.Memory == "" {
				return cmd.Help()
			}
			bytes, err := bytesize.Parse(allOptions.Memory)
			if err != nil {
				return err
			}
			convertedOptions := &runner.RunnerOptions{
				ApiURL: allOptions.ApiURL,
				Memory: uint64(bytes),
			}
			return runnerCLI(cmd, convertedOptions)
		},
	}

	runnerCmd.PersistentFlags().StringVar(
		&allOptions.ApiURL, "api-url", allOptions.ApiURL,
		`The base URL of the api - should end with /api/v1`,
	)

	return runnerCmd
}

func runnerCLI(cmd *cobra.Command, options *runner.RunnerOptions) error {

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	// Cleanup manager ensures that resources are freed before exiting:
	cm := system.NewCleanupManager()
	defer cm.Cleanup(cmd.Context())
	ctx := cmd.Context()

	// Context ensures main goroutine waits until killed with ctrl+c:
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	runnerController, err := runner.NewRunner(ctx, *options)
	if err != nil {
		return err
	}

	if err := runnerController.Start(); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
