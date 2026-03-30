package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

var projectID string

var rootCmd = &cobra.Command{
	Use:     "tui",
	Short:   "Terminal UI for Helix (hmux)",
	Aliases: []string{"ui"},
	Long: `Launch an interactive terminal UI for Helix.

View the kanban board, chat with spec task agents, and split panes
to work on multiple tasks — all from your terminal.

Works great over SSH, mosh, and slow connections.

Examples:
  helix tui                    # start with project picker
  helix tui --project proj_x   # skip picker, go to kanban
  hmux                         # same as helix tui
  hmux at                      # reattach to running session`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		if IsInTmux() {
			tmuxCfg := LoadTmuxConfig()
			fmt.Fprintf(os.Stderr, "Note: running inside tmux. Pane prefix: %s\n", tmuxCfg.Prefix)
		}

		api := NewAPIClient(apiClient)
		m := NewApp(api, projectID)
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

func init() {
	rootCmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (skip project picker)")
}

func New() *cobra.Command {
	return rootCmd
}
