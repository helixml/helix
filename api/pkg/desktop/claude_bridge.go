//go:build cgo

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// ClaudeCodeBridge handles communication between the Helix AgentClient and
// Claude Code running in tmux. It implements the same interface pattern as
// RooCodeBridge so AgentClient can route commands uniformly.
//
// Architecture:
// - Messages from Helix are sent to Claude via tmux paste-buffer
// - Claude's responses are captured via ClaudeJSONLWatcher
// - Interactions are converted to message_added events and sent back to Helix
type ClaudeCodeBridge struct {
	sessionID   string
	tmuxSession string
	workDir     string

	// JSONL watcher for Claude interactions
	jsonlWatcher *ClaudeJSONLWatcher

	// Callbacks
	onAgentReady    func()
	onMessageAdded  func(content string, isComplete bool)
	onError         func(err error)

	// Lifecycle
	started bool
	mu      sync.Mutex
}

// ClaudeCodeBridgeConfig contains configuration for the Claude Code bridge
type ClaudeCodeBridgeConfig struct {
	// SessionID is the Helix session ID
	SessionID string

	// TmuxSession is the tmux session name (default: claude-helix)
	TmuxSession string

	// WorkDir is the working directory for Claude Code
	WorkDir string

	// Callbacks
	OnAgentReady   func()
	OnMessageAdded func(content string, isComplete bool)
	OnError        func(err error)
}

// NewClaudeCodeBridge creates a new Claude Code bridge
func NewClaudeCodeBridge(cfg ClaudeCodeBridgeConfig) (*ClaudeCodeBridge, error) {
	tmuxSession := cfg.TmuxSession
	if tmuxSession == "" {
		tmuxSession = os.Getenv("HELIX_TMUX_SESSION")
		if tmuxSession == "" {
			tmuxSession = "claude-helix"
		}
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		workDir = os.Getenv("WORKSPACE_DIR")
		if workDir == "" {
			workDir = os.Getenv("HOME") + "/work"
		}
	}

	bridge := &ClaudeCodeBridge{
		sessionID:      cfg.SessionID,
		tmuxSession:    tmuxSession,
		workDir:        workDir,
		onAgentReady:   cfg.OnAgentReady,
		onMessageAdded: cfg.OnMessageAdded,
		onError:        cfg.OnError,
	}

	return bridge, nil
}

// Start begins the Claude Code bridge
func (b *ClaudeCodeBridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.started {
		return nil
	}

	log.Info().
		Str("session_id", b.sessionID).
		Str("tmux_session", b.tmuxSession).
		Str("work_dir", b.workDir).
		Msg("[ClaudeCodeBridge] Starting")

	// Create JSONL watcher for Claude interactions
	watcher, err := NewClaudeJSONLWatcher(ClaudeJSONLWatcherConfig{
		WorkDir: b.workDir,
		OnInteraction: func(interaction *ClaudeInteraction) {
			b.handleInteraction(interaction)
		},
		OnError: func(err error) {
			log.Warn().Err(err).Msg("[ClaudeCodeBridge] JSONL watcher error")
			if b.onError != nil {
				b.onError(err)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create JSONL watcher: %w", err)
	}

	if err := watcher.Start(); err != nil {
		return fmt.Errorf("failed to start JSONL watcher: %w", err)
	}
	b.jsonlWatcher = watcher

	b.started = true

	// Wait a bit for Claude to be ready, then signal agent ready
	// In practice, Claude Code is ready once the tmux session exists
	go func() {
		// Check if tmux session exists
		for i := 0; i < 60; i++ {
			cmd := exec.Command("tmux", "has-session", "-t", b.tmuxSession)
			if err := cmd.Run(); err == nil {
				log.Info().Str("session_id", b.sessionID).Msg("[ClaudeCodeBridge] Claude Code tmux session ready")
				if b.onAgentReady != nil {
					b.onAgentReady()
				}
				return
			}
			// Wait 1 second before retry
			select {
			case <-make(chan struct{}):
				// Will never fire - just for select timeout pattern
			default:
			}
			exec.Command("sleep", "1").Run()
		}
		log.Warn().Str("session_id", b.sessionID).Msg("[ClaudeCodeBridge] Timeout waiting for Claude Code tmux session")
	}()

	return nil
}

// handleInteraction processes a Claude interaction and converts it to a message
func (b *ClaudeCodeBridge) handleInteraction(interaction *ClaudeInteraction) {
	// Only relay assistant messages (Claude's responses)
	if interaction.Type != "assistant" && interaction.Message == nil {
		return
	}

	// Extract text content
	content := interaction.GetTextContent()
	if content == "" {
		return
	}

	log.Debug().
		Str("session_id", b.sessionID).
		Str("type", interaction.Type).
		Int("content_len", len(content)).
		Msg("[ClaudeCodeBridge] Relaying interaction")

	if b.onMessageAdded != nil {
		// Claude interactions are complete messages (not streaming)
		b.onMessageAdded(content, true)
	}
}

// SendMessage sends a prompt to Claude Code via tmux paste-buffer
func (b *ClaudeCodeBridge) SendMessage(message string) error {
	log.Info().
		Str("session_id", b.sessionID).
		Int("message_len", len(message)).
		Msg("[ClaudeCodeBridge] Sending message to Claude")

	// Use tmux load-buffer + paste-buffer for reliable delivery
	// This handles multi-line input properly
	loadCmd := exec.Command("tmux", "load-buffer", "-")
	loadCmd.Stdin = strings.NewReader(message)
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load tmux buffer: %w", err)
	}

	// Paste the buffer into the Claude session
	pasteCmd := exec.Command("tmux", "paste-buffer", "-t", b.tmuxSession)
	if err := pasteCmd.Run(); err != nil {
		return fmt.Errorf("failed to paste to tmux: %w", err)
	}

	// Send Enter to submit the prompt
	enterCmd := exec.Command("tmux", "send-keys", "-t", b.tmuxSession, "Enter")
	if err := enterCmd.Run(); err != nil {
		return fmt.Errorf("failed to send Enter: %w", err)
	}

	return nil
}

// StopTask attempts to stop Claude's current task
// Claude Code uses Escape to cancel, Ctrl+C to interrupt
func (b *ClaudeCodeBridge) StopTask() error {
	log.Info().Str("session_id", b.sessionID).Msg("[ClaudeCodeBridge] Stopping Claude task")

	// Send Escape to cancel any pending input
	escCmd := exec.Command("tmux", "send-keys", "-t", b.tmuxSession, "Escape")
	if err := escCmd.Run(); err != nil {
		log.Warn().Err(err).Msg("[ClaudeCodeBridge] Failed to send Escape")
	}

	// Send Ctrl+C to interrupt any running command
	ctrlCCmd := exec.Command("tmux", "send-keys", "-t", b.tmuxSession, "C-c")
	if err := ctrlCCmd.Run(); err != nil {
		return fmt.Errorf("failed to send Ctrl+C: %w", err)
	}

	return nil
}

// Close stops the Claude Code bridge
func (b *ClaudeCodeBridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.started {
		return nil
	}

	log.Info().Str("session_id", b.sessionID).Msg("[ClaudeCodeBridge] Closing")

	if b.jsonlWatcher != nil {
		b.jsonlWatcher.Stop()
	}

	b.started = false
	return nil
}
