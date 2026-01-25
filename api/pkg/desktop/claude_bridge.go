//go:build cgo

package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
)

// ClaudeBridge manages a Claude Code CLI session and bridges it to Helix.
// It provides:
// - Terminal streaming via PTY (for web terminal and SSH)
// - JSONL file watching for structured interaction capture
// - Session persistence across Claude's context compaction cycles
type ClaudeBridge struct {
	sessionID     string
	workDir       string
	claudeSession string // Claude's internal session ID (from JSONL)

	// PTY for Claude Code CLI
	cmd *exec.Cmd
	pty *os.File

	// Terminal I/O
	terminalWriters   map[string]io.Writer
	terminalWritersMu sync.RWMutex

	// JSONL watcher
	watcher          *fsnotify.Watcher
	claudeProjectDir string

	// Callbacks for Helix integration
	onInteraction func(interaction *ClaudeInteraction)
	onTerminalOut func(data []byte)
	onAgentReady  func()
	onError       func(err error)

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// ClaudeBridgeConfig contains configuration for the Claude bridge
type ClaudeBridgeConfig struct {
	// SessionID is the Helix session ID
	SessionID string

	// WorkDir is the working directory for Claude Code
	WorkDir string

	// ClaudeProjectDir is the path to Claude's project directory (~/.claude/projects/...)
	// If empty, will be computed from WorkDir
	ClaudeProjectDir string

	// InitialPrompt is an optional prompt to start with
	InitialPrompt string

	// ResumeSessionID is a Claude session ID to resume (optional)
	ResumeSessionID string

	// Env contains additional environment variables
	Env []string

	// Callbacks
	OnInteraction func(interaction *ClaudeInteraction)
	OnTerminalOut func(data []byte)
	OnAgentReady  func()
	OnError       func(err error)
}

// ClaudeInteraction represents a parsed interaction from Claude's JSONL
type ClaudeInteraction struct {
	UUID        string                 `json:"uuid"`
	ParentUUID  string                 `json:"parentUuid,omitempty"`
	Type        string                 `json:"type"` // "user", "assistant", "tool_result", "system"
	SessionID   string                 `json:"sessionId"`
	Timestamp   time.Time              `json:"timestamp"`
	Message     *ClaudeMessage         `json:"message,omitempty"`
	ToolResult  *ClaudeToolResult      `json:"toolResult,omitempty"`
	IsSidechain bool                   `json:"isSidechain"`
	CWD         string                 `json:"cwd,omitempty"`
	GitBranch   string                 `json:"gitBranch,omitempty"`
	Raw         map[string]interface{} `json:"raw,omitempty"` // Full parsed JSON for debugging
}

// ClaudeMessage represents a message in Claude's format
type ClaudeMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"` // Can be string or structured content blocks
}

// ClaudeToolResult represents a tool result
type ClaudeToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// NewClaudeBridge creates a new Claude bridge
func NewClaudeBridge(cfg ClaudeBridgeConfig) (*ClaudeBridge, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Compute Claude project directory if not provided
	claudeProjectDir := cfg.ClaudeProjectDir
	if claudeProjectDir == "" {
		// Claude uses encoded paths: /home/user/project -> -home-user-project
		homeDir, _ := os.UserHomeDir()
		encodedPath := encodeClaudePath(cfg.WorkDir)
		claudeProjectDir = filepath.Join(homeDir, ".claude", "projects", encodedPath)
	}

	bridge := &ClaudeBridge{
		sessionID:        cfg.SessionID,
		workDir:          cfg.WorkDir,
		claudeProjectDir: claudeProjectDir,
		terminalWriters:  make(map[string]io.Writer),
		onInteraction:    cfg.OnInteraction,
		onTerminalOut:    cfg.OnTerminalOut,
		onAgentReady:     cfg.OnAgentReady,
		onError:          cfg.OnError,
		ctx:              ctx,
		cancel:           cancel,
	}

	return bridge, nil
}

// encodeClaudePath encodes a path the way Claude Code does for project directories
func encodeClaudePath(path string) string {
	// Claude replaces / with - and removes leading slash
	encoded := strings.TrimPrefix(path, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	return encoded
}

// Start launches Claude Code CLI and begins monitoring
func (b *ClaudeBridge) Start(initialPrompt string, resumeSessionID string, env []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Ensure Claude project directory exists
	if err := os.MkdirAll(b.claudeProjectDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", b.claudeProjectDir).Msg("[ClaudeBridge] Failed to create Claude project dir")
	}

	// Build Claude command
	args := []string{}

	// Resume session if provided
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}

	// Add initial prompt if provided (for non-interactive start)
	if initialPrompt != "" && resumeSessionID == "" {
		args = append(args, initialPrompt)
	}

	b.cmd = exec.CommandContext(b.ctx, "claude", args...)
	b.cmd.Dir = b.workDir

	// Set up environment
	b.cmd.Env = append(os.Environ(), env...)

	// Start with PTY
	ptmx, err := pty.Start(b.cmd)
	if err != nil {
		return fmt.Errorf("failed to start Claude Code with PTY: %w", err)
	}
	b.pty = ptmx

	log.Info().
		Str("session_id", b.sessionID).
		Str("work_dir", b.workDir).
		Str("claude_project_dir", b.claudeProjectDir).
		Msg("[ClaudeBridge] Started Claude Code CLI")

	// Start PTY output reader
	go b.readPTY()

	// Start JSONL watcher
	go b.watchJSONL()

	// Signal agent ready after a brief delay (Claude needs time to start)
	go func() {
		time.Sleep(2 * time.Second)
		if b.onAgentReady != nil {
			b.onAgentReady()
		}
	}()

	return nil
}

// readPTY reads from the PTY and distributes to all connected writers
func (b *ClaudeBridge) readPTY() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-b.ctx.Done():
			return
		default:
		}

		n, err := b.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Warn().Err(err).Msg("[ClaudeBridge] PTY read error")
			}
			return
		}

		data := buf[:n]

		// Callback for raw terminal output
		if b.onTerminalOut != nil {
			b.onTerminalOut(data)
		}

		// Distribute to all connected writers (web terminals, SSH sessions)
		b.terminalWritersMu.RLock()
		for id, w := range b.terminalWriters {
			if _, err := w.Write(data); err != nil {
				log.Debug().Err(err).Str("writer_id", id).Msg("[ClaudeBridge] Writer error")
			}
		}
		b.terminalWritersMu.RUnlock()
	}
}

// watchJSONL watches Claude's JSONL files for new interactions
func (b *ClaudeBridge) watchJSONL() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error().Err(err).Msg("[ClaudeBridge] Failed to create JSONL watcher")
		return
	}
	defer watcher.Close()
	b.watcher = watcher

	// Watch the Claude project directory
	if err := watcher.Add(b.claudeProjectDir); err != nil {
		log.Warn().Err(err).Str("path", b.claudeProjectDir).Msg("[ClaudeBridge] Failed to watch Claude project dir")
		// Continue anyway - directory might not exist yet
	}

	// Track file positions for tailing
	filePositions := make(map[string]int64)

	for {
		select {
		case <-b.ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only care about JSONL files being written
			if !strings.HasSuffix(event.Name, ".jsonl") {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				b.processJSONLFile(event.Name, filePositions)
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Warn().Err(err).Msg("[ClaudeBridge] JSONL watcher error")
		}
	}
}

// processJSONLFile reads new lines from a JSONL file
func (b *ClaudeBridge) processJSONLFile(path string, positions map[string]int64) {
	file, err := os.Open(path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("[ClaudeBridge] Failed to open JSONL file")
		return
	}
	defer file.Close()

	// Seek to last known position
	pos := positions[path]
	if pos > 0 {
		if _, err := file.Seek(pos, 0); err != nil {
			log.Warn().Err(err).Msg("[ClaudeBridge] Failed to seek in JSONL file")
			pos = 0
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		interaction, err := b.parseJSONLLine(line)
		if err != nil {
			log.Debug().Err(err).Str("line", line).Msg("[ClaudeBridge] Failed to parse JSONL line")
			continue
		}

		// Track the Claude session ID
		if interaction.SessionID != "" && b.claudeSession == "" {
			b.claudeSession = interaction.SessionID
			log.Info().Str("claude_session", b.claudeSession).Msg("[ClaudeBridge] Detected Claude session ID")
		}

		// Callback with parsed interaction
		if b.onInteraction != nil {
			b.onInteraction(interaction)
		}
	}

	// Update position
	newPos, _ := file.Seek(0, 1) // Current position
	positions[path] = newPos
}

// parseJSONLLine parses a single line from Claude's JSONL format
func (b *ClaudeBridge) parseJSONLLine(line string) (*ClaudeInteraction, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		return nil, err
	}

	interaction := &ClaudeInteraction{
		Raw: raw,
	}

	// Extract common fields
	if v, ok := raw["uuid"].(string); ok {
		interaction.UUID = v
	}
	if v, ok := raw["parentUuid"].(string); ok {
		interaction.ParentUUID = v
	}
	if v, ok := raw["type"].(string); ok {
		interaction.Type = v
	}
	if v, ok := raw["sessionId"].(string); ok {
		interaction.SessionID = v
	}
	if v, ok := raw["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			interaction.Timestamp = t
		}
	}
	if v, ok := raw["isSidechain"].(bool); ok {
		interaction.IsSidechain = v
	}
	if v, ok := raw["cwd"].(string); ok {
		interaction.CWD = v
	}
	if v, ok := raw["gitBranch"].(string); ok {
		interaction.GitBranch = v
	}

	// Parse message
	if msgRaw, ok := raw["message"].(map[string]interface{}); ok {
		interaction.Message = &ClaudeMessage{}
		if role, ok := msgRaw["role"].(string); ok {
			interaction.Message.Role = role
		}
		if content, ok := msgRaw["content"]; ok {
			// Content can be a string or an array
			switch c := content.(type) {
			case string:
				interaction.Message.Content = []interface{}{c}
			case []interface{}:
				interaction.Message.Content = c
			}
		}
	}

	// Parse tool result (if present)
	if v, ok := raw["tool_use_id"].(string); ok {
		interaction.ToolResult = &ClaudeToolResult{
			ToolUseID: v,
		}
		if content, ok := raw["content"].(string); ok {
			interaction.ToolResult.Content = content
		}
		if isError, ok := raw["is_error"].(bool); ok {
			interaction.ToolResult.IsError = isError
		}
	}

	return interaction, nil
}

// WriteTerminal writes data to the Claude Code terminal (user input)
func (b *ClaudeBridge) WriteTerminal(data []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pty == nil {
		return fmt.Errorf("PTY not initialized")
	}

	_, err := b.pty.Write(data)
	return err
}

// AddTerminalWriter adds a writer that receives terminal output
func (b *ClaudeBridge) AddTerminalWriter(id string, w io.Writer) {
	b.terminalWritersMu.Lock()
	defer b.terminalWritersMu.Unlock()
	b.terminalWriters[id] = w
}

// RemoveTerminalWriter removes a terminal output writer
func (b *ClaudeBridge) RemoveTerminalWriter(id string) {
	b.terminalWritersMu.Lock()
	defer b.terminalWritersMu.Unlock()
	delete(b.terminalWriters, id)
}

// Resize resizes the terminal
func (b *ClaudeBridge) Resize(rows, cols uint16) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pty == nil {
		return fmt.Errorf("PTY not initialized")
	}

	return pty.Setsize(b.pty, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

// GetClaudeSessionID returns Claude's internal session ID
func (b *ClaudeBridge) GetClaudeSessionID() string {
	return b.claudeSession
}

// SendMessage sends a message to Claude Code
// This writes directly to the terminal as user input
func (b *ClaudeBridge) SendMessage(message string) error {
	// Write message followed by Enter
	return b.WriteTerminal([]byte(message + "\n"))
}

// Stop stops Claude Code and cleans up
func (b *ClaudeBridge) Stop() error {
	b.cancel()

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.pty != nil {
		b.pty.Close()
		b.pty = nil
	}

	if b.cmd != nil && b.cmd.Process != nil {
		b.cmd.Process.Kill()
		b.cmd = nil
	}

	if b.watcher != nil {
		b.watcher.Close()
		b.watcher = nil
	}

	log.Info().Str("session_id", b.sessionID).Msg("[ClaudeBridge] Stopped")
	return nil
}

// ToHelixInteraction converts a ClaudeInteraction to Helix's Interaction type.
// Note: Claude interactions map differently to Helix interactions:
// - Claude "user" messages -> PromptMessage
// - Claude "assistant" messages -> ResponseMessage
// - Claude "tool_result" messages -> appended to ResponseMessage context
func (ci *ClaudeInteraction) ToHelixInteraction(helixSessionID string) *types.Interaction {
	interaction := &types.Interaction{
		ID:        ci.UUID,
		SessionID: helixSessionID,
		Created:   ci.Timestamp,
		Updated:   ci.Timestamp,
		Completed: ci.Timestamp,
	}

	switch ci.Type {
	case "user":
		if ci.Message != nil && len(ci.Message.Content) > 0 {
			if text, ok := ci.Message.Content[0].(string); ok {
				interaction.PromptMessage = text
			}
		}

	case "assistant":
		if ci.Message != nil {
			// Extract text content from assistant response
			var responseText string
			for _, block := range ci.Message.Content {
				switch b := block.(type) {
				case string:
					responseText += b
				case map[string]interface{}:
					if text, ok := b["text"].(string); ok {
						responseText += text
					}
					// Could also extract tool_use blocks here for display
				}
			}
			interaction.ResponseMessage = responseText
		}

	case "tool_result":
		// Tool results are typically context for the assistant, not user-facing
		// We could store these in a separate field or append to response
		if ci.ToolResult != nil {
			interaction.ResponseMessage = fmt.Sprintf("[Tool Result: %s]", ci.ToolResult.Content)
		}
	}

	return interaction
}
