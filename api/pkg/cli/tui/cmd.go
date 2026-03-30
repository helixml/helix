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

Keybindings are parsed from your ~/.tmux.conf automatically.

Examples:
  helix tui                     # start with project picker
  helix tui --project proj_x    # skip picker, go to kanban
  helix tui attach              # reattach to previous session`,
	RunE: runTUI,
}

var attachCmd = &cobra.Command{
	Use:     "attach",
	Short:   "Reattach to previous TUI session",
	Aliases: []string{"at"},
	RunE: func(cmd *cobra.Command, args []string) error {
		state := LoadState()
		if state == nil || state.ProjectID == "" {
			return fmt.Errorf("no saved TUI session found")
		}
		// Use saved project ID
		projectID = state.ProjectID
		return runTUI(cmd, args)
	},
}

func runTUI(cmd *cobra.Command, args []string) error {
	apiClient, err := client.NewClientFromEnv()
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	if IsInTmux() {
		tmuxCfg := LoadTmuxConfig()
		fmt.Fprintf(os.Stderr, "Note: running inside tmux (prefix: %s)\n", tmuxCfg.Prefix)
	}

	api := NewAPIClient(apiClient)
	m := NewApp(api, projectID)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func init() {
	rootCmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (skip project picker)")
	rootCmd.AddCommand(attachCmd)
}

func New() *cobra.Command {
	return rootCmd
}
