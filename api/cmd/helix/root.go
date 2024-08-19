package helix

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli/app"
	"github.com/helixml/helix/api/pkg/cli/knowledge"
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
	RootCmd.AddCommand(newGptScriptCmd())
	RootCmd.AddCommand(newRunnerCmd())
	RootCmd.AddCommand(newGptScriptRunnerCmd())
	RootCmd.AddCommand(newRunCmd())
	RootCmd.AddCommand(newQapairCommand())
	RootCmd.AddCommand(newEvalsCommand())
	RootCmd.AddCommand(newVersionCommand())

	// CLI
	RootCmd.AddCommand(app.New())
	RootCmd.AddCommand(app.NewApplyCmd()) // Shortcut for apply
	RootCmd.AddCommand(knowledge.New())

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
