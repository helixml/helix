//go:build cgo

package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// CursorBridge manages communication with the Cursor CLI agent.
// Unlike RooCodeBridge which is a Socket.IO server, CursorBridge spawns
// cursor-agent subprocesses for each task.
//
// Architecture:
//
//	[Helix API] <--(WebSocket)--> [AgentClient] <--(subprocess)--> [cursor-agent CLI]
//
// The cursor-agent CLI is run in print mode (-p) with JSON output for
// programmatic interaction. Note: There's a known issue where the CLI
// may not release the terminal even with -p flag - we handle this with timeouts.
type CursorBridge struct {
	sessionID string

	// Current task state
	currentCmd   *exec.Cmd
	currentCtx   context.Context
	currentCancel context.CancelFunc
	cmdMu        sync.Mutex

	// Callbacks for translating events to Helix
	onAgentReady     func()
	onMessageAdded   func(content string, isComplete bool)
	onMessageUpdated func(content string)
	onError          func(err error)
}

// CursorBridgeConfig contains configuration for the bridge
type CursorBridgeConfig struct {
	// SessionID is the Helix session ID for logging
	SessionID string

	// Callbacks
	OnAgentReady     func()
	OnMessageAdded   func(content string, isComplete bool)
	OnMessageUpdated func(content string)
	OnError          func(err error)
}

// CursorAgentEvent represents a JSON event from cursor-agent CLI
// Based on --output-format stream-json which emits NDJSON events
type CursorAgentEvent struct {
	Type    string                 `json:"type"`    // "system", "delta", "tool_call", "result"
	Content string                 `json:"content"` // For delta/result events
	Data    map[string]interface{} `json:"data"`    // Additional event data
}

// NewCursorBridge creates a new Cursor CLI bridge
func NewCursorBridge(config CursorBridgeConfig) (*CursorBridge, error) {
	bridge := &CursorBridge{
		sessionID:        config.SessionID,
		onAgentReady:     config.OnAgentReady,
		onMessageAdded:   config.OnMessageAdded,
		onMessageUpdated: config.OnMessageUpdated,
		onError:          config.OnError,
	}

	return bridge, nil
}

// Start initializes the bridge (no-op for subprocess-based approach)
func (b *CursorBridge) Start() error {
	log.Info().
		Str("session_id", b.sessionID).
		Msg("[CursorBridge] Ready to accept tasks")

	// Signal that we're ready to receive commands
	if b.onAgentReady != nil {
		b.onAgentReady()
	}

	return nil
}

// StartTask sends a prompt to the Cursor CLI agent
func (b *CursorBridge) StartTask(prompt string) error {
	b.cmdMu.Lock()
	defer b.cmdMu.Unlock()

	// Cancel any existing task
	if b.currentCancel != nil {
		b.currentCancel()
	}

	// Create context with timeout (5 minutes max per task)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	b.currentCtx = ctx
	b.currentCancel = cancel

	log.Info().
		Str("session_id", b.sessionID).
		Int("prompt_len", len(prompt)).
		Msg("[CursorBridge] Starting Cursor agent task")

	// Run cursor-agent in a goroutine
	go b.runCursorAgent(ctx, prompt)

	return nil
}

// runCursorAgent executes the cursor-agent CLI with the given prompt
func (b *CursorBridge) runCursorAgent(ctx context.Context, prompt string) {
	// Build command
	// cursor-agent -p "prompt" --output-format stream-json --force
	cmd := exec.CommandContext(ctx, "cursor-agent",
		"-p", prompt,
		"--output-format", "stream-json",
		"--force", // Allow file changes without confirmation
	)

	b.cmdMu.Lock()
	b.currentCmd = cmd
	b.cmdMu.Unlock()

	// Capture stdout for JSON events
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Error().Err(err).Msg("[CursorBridge] Failed to create stdout pipe")
		if b.onError != nil {
			b.onError(err)
		}
		return
	}

	// Capture stderr for error messages
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Error().Err(err).Msg("[CursorBridge] Failed to create stderr pipe")
		if b.onError != nil {
			b.onError(err)
		}
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Msg("[CursorBridge] Failed to start cursor-agent")
		if b.onError != nil {
			b.onError(err)
		}
		return
	}

	log.Debug().
		Str("session_id", b.sessionID).
		Int("pid", cmd.Process.Pid).
		Msg("[CursorBridge] cursor-agent started")

	// Process output in goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		b.processStdout(stdout)
	}()

	go func() {
		defer wg.Done()
		b.processStderr(stderr)
	}()

	// Wait for output processing to complete
	wg.Wait()

	// Wait for command to exit (with timeout handling)
	// Note: cursor-agent has a known bug where it may not exit properly
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Warn().Err(err).Msg("[CursorBridge] cursor-agent exited with error")
		} else {
			log.Info().Str("session_id", b.sessionID).Msg("[CursorBridge] cursor-agent completed")
		}
	case <-ctx.Done():
		// Context was cancelled or timed out
		log.Warn().Str("session_id", b.sessionID).Msg("[CursorBridge] Task timed out or cancelled, killing process")
		_ = cmd.Process.Kill()
	}

	// Signal completion
	if b.onMessageAdded != nil {
		b.onMessageAdded("", true) // Empty content with complete=true signals end
	}

	b.cmdMu.Lock()
	b.currentCmd = nil
	b.cmdMu.Unlock()
}

// processStdout handles NDJSON events from cursor-agent stdout
func (b *CursorBridge) processStdout(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for large responses
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event CursorAgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			// Not JSON, treat as plain text output
			log.Debug().Str("line", line).Msg("[CursorBridge] Non-JSON output")
			if b.onMessageAdded != nil {
				b.onMessageAdded(line, false)
			}
			continue
		}

		b.handleEvent(event)
	}

	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Msg("[CursorBridge] Scanner error")
	}
}

// processStderr logs error output from cursor-agent
func (b *CursorBridge) processStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			log.Warn().Str("stderr", line).Msg("[CursorBridge] cursor-agent stderr")
		}
	}
}

// handleEvent processes a parsed JSON event from cursor-agent
func (b *CursorBridge) handleEvent(event CursorAgentEvent) {
	log.Debug().
		Str("type", event.Type).
		Str("session_id", b.sessionID).
		Msg("[CursorBridge] Received event")

	switch event.Type {
	case "system":
		// System initialization event
		log.Debug().Str("session_id", b.sessionID).Msg("[CursorBridge] System initialized")

	case "delta":
		// Streaming content delta
		if event.Content != "" {
			if b.onMessageUpdated != nil {
				b.onMessageUpdated(event.Content)
			}
		}

	case "tool_call":
		// Tool call event (file read, write, command execution)
		if b.onMessageAdded != nil {
			// Format tool call as message
			toolInfo, _ := json.Marshal(event.Data)
			b.onMessageAdded(fmt.Sprintf("[Tool: %s]", string(toolInfo)), false)
		}

	case "result":
		// Final result
		if event.Content != "" && b.onMessageAdded != nil {
			b.onMessageAdded(event.Content, false)
		}

	default:
		log.Debug().
			Str("type", event.Type).
			Interface("data", event.Data).
			Msg("[CursorBridge] Unknown event type")
	}
}

// SendMessage sends a follow-up message (starts new task with context)
func (b *CursorBridge) SendMessage(message string) error {
	// For Cursor CLI, each message is essentially a new task
	// Context is maintained through the working directory state
	return b.StartTask(message)
}

// StopTask cancels the current running task
func (b *CursorBridge) StopTask() error {
	b.cmdMu.Lock()
	defer b.cmdMu.Unlock()

	if b.currentCancel != nil {
		b.currentCancel()
		log.Info().Str("session_id", b.sessionID).Msg("[CursorBridge] Task cancelled")
	}

	return nil
}

// Close shuts down the bridge
func (b *CursorBridge) Close() error {
	// Cancel any running task
	b.cmdMu.Lock()
	if b.currentCancel != nil {
		b.currentCancel()
	}
	b.cmdMu.Unlock()

	log.Info().Str("session_id", b.sessionID).Msg("[CursorBridge] Closed")
	return nil
}

// IsConnected returns true (subprocess is always "connected" when available)
func (b *CursorBridge) IsConnected() bool {
	return true
}
