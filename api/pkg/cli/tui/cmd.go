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
	Use:     "hmux",
	Short:   "Helix agent multiplexer",
	Aliases: []string{"tui"},
	Long: `hmux — the Helix agent multiplexer.

Manage multiple AI agents from your terminal. Kanban board, split-pane
chat, spec review, and more. Works great over SSH, mosh, and slow
connections.

Keybindings are parsed from your ~/.tmux.conf automatically.

Examples:
  helix hmux                    # start with project picker
  helix hmux --project proj_x   # skip picker, go to kanban
  helix hmux attach             # reattach to previous session
  helix hmux demo               # explore with mock data`,
	RunE: runTUI,
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start TUI SSH server",
	Long: `Start an SSH server that serves the Helix TUI to connecting clients.

Users can connect with:
  ssh -p 2222 apikey:hlx_xxx@your-helix-host
  mosh your-helix-host -- helix tui`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := DefaultSSHServerConfig()
		if sshPort > 0 {
			cfg.Port = sshPort
		}
		if sshHost != "" {
			cfg.Host = sshHost
		}
		return StartSSHServer(cfg)
	},
}

var sshPort int
var sshHost string

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Run TUI with mock data (no Helix instance needed)",
	Long: `Launch the TUI against a built-in mock server with realistic
fake data. Great for exploring the interface, testing keybindings,
and demoing without a real Helix deployment.

The mock server simulates:
  - 2 projects with 7 spec tasks across all kanban columns
  - Chat history with realistic agent conversations
  - Fake LLM responses when you send messages`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Starting demo mode with mock Helix server...")

		api, cleanup := startDemoServer()
		defer cleanup()

		m := NewApp(api, "")
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
		_, err := p.Run()
		return err
	},
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
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}

func init() {
	rootCmd.Flags().StringVarP(&projectID, "project", "p", "", "Project ID (skip project picker)")

	serveCmd.Flags().IntVar(&sshPort, "port", 2222, "SSH server port")
	serveCmd.Flags().StringVar(&sshHost, "host", "0.0.0.0", "SSH server listen address")

	rootCmd.AddCommand(attachCmd)
	rootCmd.AddCommand(demoCmd)
	rootCmd.AddCommand(serveCmd)
}

func New() *cobra.Command {
	return rootCmd
}
