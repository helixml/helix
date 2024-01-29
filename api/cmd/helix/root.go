package helix

import (
	"context"
	"os"

	"github.com/spf13/cobra"
)

var Fatal = FatalErrorHandler

func init() { //nolint:gochecknoinits
	NewRootCmd()
}

func NewRootCmd() *cobra.Command {
	RootCmd := &cobra.Command{
		Use:   getCommandLineExecutable(),
		Short: "Helix",
		Long:  `Private GenAI Platform`,
	}
	RootCmd.AddCommand(newServeCmd())
	RootCmd.AddCommand(newRunnerCmd())
	RootCmd.AddCommand(newRunCmd())
	RootCmd.AddCommand(newQapairCommand())
	return RootCmd
}

func Execute() {
	RootCmd := NewRootCmd()
	RootCmd.SetContext(context.Background())
	RootCmd.SetOutput(os.Stdout)
	if err := RootCmd.Execute(); err != nil {
		Fatal(RootCmd, err.Error(), 1)
	}
}
