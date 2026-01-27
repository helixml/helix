//go:build cgo

package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// ClaudeJSONLWatcher watches Claude Code's JSONL session files and reports
// interactions to a callback. This enables Helix to capture the full conversation
// history even when Claude compacts its context.
//
// Architecture:
// - Claude Code writes interactions to ~/.claude/projects/[encoded-path]/*.jsonl
// - This watcher uses fsnotify to detect new lines being appended
// - Parsed interactions are sent to the OnInteraction callback
// - The callback can relay these to the Helix session API
type ClaudeJSONLWatcher struct {
	projectDir string // ~/.claude/projects or custom path
	workDir    string // Working directory to encode for project path

	// Callbacks
	onInteraction func(interaction *ClaudeInteraction)
	onError       func(err error)

	// File tracking
	watcher       *fsnotify.Watcher
	filePositions map[string]int64
	mu            sync.Mutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// ClaudeInteraction represents a parsed interaction from Claude's JSONL format.
// This matches the structure Claude Code writes to its session files.
type ClaudeInteraction struct {
	UUID       string                 `json:"uuid"`
	ParentUUID string                 `json:"parentUuid,omitempty"`
	Type       string                 `json:"type"` // "user", "assistant", "tool_result", "system"
	SessionID  string                 `json:"sessionId"`
	Timestamp  time.Time              `json:"timestamp"`
	Message    *ClaudeMessage         `json:"message,omitempty"`
	ToolResult *ClaudeToolResult      `json:"toolResult,omitempty"`
	CWD        string                 `json:"cwd,omitempty"`
	GitBranch  string                 `json:"gitBranch,omitempty"`
	Raw        map[string]interface{} `json:"raw,omitempty"` // Full parsed JSON for debugging
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

// ClaudeJSONLWatcherConfig contains configuration for the JSONL watcher
type ClaudeJSONLWatcherConfig struct {
	// WorkDir is the working directory Claude is running in
	// Used to compute the Claude project path
	WorkDir string

	// ProjectDir is optional - if set, overrides the computed project directory
	ProjectDir string

	// Callbacks
	OnInteraction func(interaction *ClaudeInteraction)
	OnError       func(err error)
}

// NewClaudeJSONLWatcher creates a new JSONL watcher
func NewClaudeJSONLWatcher(cfg ClaudeJSONLWatcherConfig) (*ClaudeJSONLWatcher, error) {
	ctx, cancel := context.WithCancel(context.Background())

	// Compute Claude project directory if not provided
	projectDir := cfg.ProjectDir
	if projectDir == "" {
		homeDir, _ := os.UserHomeDir()
		encodedPath := encodeClaudePath(cfg.WorkDir)
		projectDir = filepath.Join(homeDir, ".claude", "projects", encodedPath)
	}

	watcher := &ClaudeJSONLWatcher{
		projectDir:    projectDir,
		workDir:       cfg.WorkDir,
		onInteraction: cfg.OnInteraction,
		onError:       cfg.OnError,
		filePositions: make(map[string]int64),
		ctx:           ctx,
		cancel:        cancel,
	}

	return watcher, nil
}

// encodeClaudePath encodes a path the way Claude Code does for project directories
func encodeClaudePath(path string) string {
	// Claude replaces / with - and removes leading slash
	encoded := strings.TrimPrefix(path, "/")
	encoded = strings.ReplaceAll(encoded, "/", "-")
	return encoded
}

// Start begins watching for JSONL file changes
func (w *ClaudeJSONLWatcher) Start() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = watcher

	// Ensure project directory exists
	if err := os.MkdirAll(w.projectDir, 0755); err != nil {
		log.Warn().Err(err).Str("path", w.projectDir).Msg("[ClaudeJSONL] Failed to create project dir")
	}

	// Watch the project directory
	if err := watcher.Add(w.projectDir); err != nil {
		log.Warn().Err(err).Str("path", w.projectDir).Msg("[ClaudeJSONL] Failed to watch project dir")
		// Continue anyway - directory might not exist yet, we'll retry
	}

	log.Info().
		Str("project_dir", w.projectDir).
		Str("work_dir", w.workDir).
		Msg("[ClaudeJSONL] Started watching for Claude interactions")

	go w.watchLoop()

	return nil
}

// watchLoop is the main event loop for file watching
func (w *ClaudeJSONLWatcher) watchLoop() {
	// Retry adding watch if initial add failed
	retryTicker := time.NewTicker(5 * time.Second)
	defer retryTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return

		case <-retryTicker.C:
			// Try to add watch if directory now exists
			if w.watcher != nil {
				_ = w.watcher.Add(w.projectDir)
			}

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only care about JSONL files being written
			if !strings.HasSuffix(event.Name, ".jsonl") {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				w.processJSONLFile(event.Name)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Warn().Err(err).Msg("[ClaudeJSONL] Watcher error")
			if w.onError != nil {
				w.onError(err)
			}
		}
	}
}

// processJSONLFile reads new lines from a JSONL file
func (w *ClaudeJSONLWatcher) processJSONLFile(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("[ClaudeJSONL] Failed to open file")
		return
	}
	defer file.Close()

	// Seek to last known position
	pos := w.filePositions[path]
	if pos > 0 {
		if _, err := file.Seek(pos, 0); err != nil {
			log.Warn().Err(err).Msg("[ClaudeJSONL] Failed to seek")
			pos = 0
		}
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		interaction, err := parseJSONLLine(line)
		if err != nil {
			log.Debug().Err(err).Str("line", line[:min(100, len(line))]).Msg("[ClaudeJSONL] Failed to parse line")
			continue
		}

		// Callback with parsed interaction
		if w.onInteraction != nil {
			w.onInteraction(interaction)
		}
	}

	// Update position
	newPos, _ := file.Seek(0, 1)
	w.filePositions[path] = newPos
}

// parseJSONLLine parses a single line from Claude's JSONL format
func parseJSONLLine(line string) (*ClaudeInteraction, error) {
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

// Stop stops watching for JSONL changes
func (w *ClaudeJSONLWatcher) Stop() error {
	w.cancel()

	if w.watcher != nil {
		return w.watcher.Close()
	}
	return nil
}

// GetTextContent extracts plain text content from a ClaudeInteraction
func (ci *ClaudeInteraction) GetTextContent() string {
	if ci.Message == nil || len(ci.Message.Content) == 0 {
		return ""
	}

	var text strings.Builder
	for _, block := range ci.Message.Content {
		switch b := block.(type) {
		case string:
			text.WriteString(b)
		case map[string]interface{}:
			if t, ok := b["text"].(string); ok {
				text.WriteString(t)
			}
		}
	}
	return text.String()
}
