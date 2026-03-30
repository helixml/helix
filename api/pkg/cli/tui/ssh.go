package tui

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/wish"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/ssh"

	"github.com/helixml/helix/api/pkg/client"
)

// SSHServerConfig holds configuration for the SSH server mode.
type SSHServerConfig struct {
	Host       string // listen host (default: "0.0.0.0")
	Port       int    // listen port (default: 2222)
	HostKeyDir string // directory for host keys (default: ~/.helix/tui/ssh)
	HelixURL   string // Helix API URL
}

// DefaultSSHServerConfig returns sensible defaults.
func DefaultSSHServerConfig() *SSHServerConfig {
	home, _ := os.UserHomeDir()
	return &SSHServerConfig{
		Host:       "0.0.0.0",
		Port:       2222,
		HostKeyDir: filepath.Join(home, ".helix", "tui", "ssh"),
		HelixURL:   os.Getenv("HELIX_URL"),
	}
}

// StartSSHServer starts the TUI SSH server. Each SSH connection gets its
// own TUI instance. Users authenticate via SSH key or password, mapped
// to a Helix API key.
//
// Usage:
//
//	ssh -t helix.example.com -p 2222          # launches TUI
//	mosh helix.example.com -- helix tui       # via mosh
func StartSSHServer(cfg *SSHServerConfig) error {
	if cfg == nil {
		cfg = DefaultSSHServerConfig()
	}

	// Ensure host key directory exists
	if err := os.MkdirAll(cfg.HostKeyDir, 0700); err != nil {
		return fmt.Errorf("failed to create host key dir: %w", err)
	}

	hostKeyPath := filepath.Join(cfg.HostKeyDir, "host_key")

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	s, err := wish.NewServer(
		wish.WithAddress(addr),
		wish.WithHostKeyPath(hostKeyPath),
		wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			// User must be "helix", password is the API key
			if ctx.User() != "helix" {
				return false
			}
			ctx.SetValue("helix_api_key", password)
			return password != ""
		}),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			// Accept all public keys for now
			// TODO: map SSH keys to Helix users/API keys
			ctx.SetValue("helix_api_key", "")
			return true
		}),
		wish.WithMiddleware(
			bm.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				return newSSHSessionModel(s, cfg), []tea.ProgramOption{tea.WithAltScreen()}
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create SSH server: %w", err)
	}

	// Handle signals for graceful shutdown
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	log.Printf("Helix TUI SSH server listening on %s", addr)
	log.Printf("Connect with: ssh -p %d user@%s", cfg.Port, cfg.Host)

	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Printf("SSH server error: %v", err)
		}
	}()

	<-done
	log.Println("Shutting down SSH server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.Shutdown(ctx)
}

// sshSessionModel wraps the App model for an SSH session.
type sshSessionModel struct {
	app    *App
	apiKey string
	cfg    *SSHServerConfig
	err    error
}

func newSSHSessionModel(s ssh.Session, cfg *SSHServerConfig) *sshSessionModel {
	// Get API key from SSH auth — user is "helix", password is the API key
	apiKey := ""
	if key, ok := s.Context().Value("helix_api_key").(string); ok {
		apiKey = key
	}

	m := &sshSessionModel{
		apiKey: apiKey,
		cfg:    cfg,
	}

	if apiKey == "" {
		m.err = fmt.Errorf("no API key provided. Use: ssh helix@host -p %d (password = your API key)", cfg.Port)
		return m
	}

	// Connect to the local Helix API (SSH server runs inside the Helix server process)
	helixURL := cfg.HelixURL
	if helixURL == "" {
		helixURL = "http://localhost:8080"
	}

	helixClient, err := client.NewClient(helixURL, apiKey, false)
	if err != nil {
		m.err = fmt.Errorf("failed to create API client: %w", err)
		return m
	}

	api := NewAPIClient(helixClient)
	m.app = NewApp(api, "")
	m.app.conn.isSSHSession = true

	return m
}

func (m *sshSessionModel) Init() tea.Cmd {
	if m.err != nil {
		return nil
	}
	return m.app.Init()
}

func (m *sshSessionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.err != nil {
		if msg, ok := msg.(tea.KeyMsg); ok {
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
		return m, nil
	}
	return m.app.Update(msg)
}

func (m *sshSessionModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("\n  Helix TUI\n\n  Error: %v\n\n  Press q to quit.\n", m.err)
	}
	return m.app.View()
}
