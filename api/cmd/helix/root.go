package helix

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli/app"
	"github.com/helixml/helix/api/pkg/cli/chat"
	"github.com/helixml/helix/api/pkg/cli/fs"
	"github.com/helixml/helix/api/pkg/cli/knowledge"
	"github.com/helixml/helix/api/pkg/cli/mcp"
	"github.com/helixml/helix/api/pkg/cli/member"
	"github.com/helixml/helix/api/pkg/cli/model"
	"github.com/helixml/helix/api/pkg/cli/moonlight"
	"github.com/helixml/helix/api/pkg/cli/organization"
	"github.com/helixml/helix/api/pkg/cli/provider"
	"github.com/helixml/helix/api/pkg/cli/roles"
	"github.com/helixml/helix/api/pkg/cli/secret"
	"github.com/helixml/helix/api/pkg/cli/system"
	"github.com/helixml/helix/api/pkg/cli/team"
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

	// CLI commands (available on all platforms)
	RootCmd.AddCommand(app.New())
	RootCmd.AddCommand(app.NewApplyCmd()) // Shortcut for apply
	RootCmd.AddCommand(chat.New())
	RootCmd.AddCommand(knowledge.New())
	RootCmd.AddCommand(fs.New())
	RootCmd.AddCommand(fs.NewUploadCmd()) // Shortcut for upload
	RootCmd.AddCommand(secret.New())
	RootCmd.AddCommand(mcp.New())
	RootCmd.AddCommand(model.New())
	RootCmd.AddCommand(moonlight.New())
	RootCmd.AddCommand(provider.New())
	RootCmd.AddCommand(organization.New())
	RootCmd.AddCommand(roles.New())
	RootCmd.AddCommand(system.New())
	RootCmd.AddCommand(team.New())
	RootCmd.AddCommand(member.New())

	// Commands available on all platforms
	RootCmd.AddCommand(NewServeCmd())
	RootCmd.AddCommand(NewVersionCommand())

	RootCmd.AddCommand(NewQapairCommand())
	RootCmd.AddCommand(NewEvalsCommand())
	RootCmd.AddCommand(NewTestCmd()) // Use the NewTestCmd function from the current package

	return RootCmd
}

func Execute() {
	RootCmd := NewRootCmd()
	RootCmd.SetContext(context.Background())
	RootCmd.SetOutput(os.Stdout)

	// Check for HELIX_COMMAND environment variable to support air hot reloading
	if helixCmd := os.Getenv("HELIX_COMMAND"); helixCmd != "" {
		// Split the command and inject it into os.Args
		cmdParts := strings.Fields(helixCmd)
		if len(cmdParts) > 0 {
			// Replace os.Args to include the subcommand
			newArgs := []string{os.Args[0]}
			newArgs = append(newArgs, cmdParts...)
			os.Args = newArgs
		}
	}

	if err := RootCmd.Execute(); err != nil {
		Fatal(RootCmd, err.Error(), 1)
	}
}
