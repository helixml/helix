//go:build cgo

// Package desktop provides WebSocket terminal streaming for Claude Code.
// Connects to a tmux session and relays I/O to web clients via xterm.js.
package desktop

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// TerminalMessage represents a message from the web terminal client
type TerminalMessage struct {
	Type string `json:"type"` // "input", "resize"
	Data string `json:"data,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
	Cols uint16 `json:"cols,omitempty"`
}

// handleWSTerminal handles WebSocket connections for Claude Code terminal access.
// It attaches to the claude-helix tmux session and relays I/O to the web client.
//
// Protocol:
// - Client -> Server: JSON messages with type "input" (data) or "resize" (rows/cols)
// - Server -> Client: Raw terminal output bytes (binary)
func (s *Server) handleWSTerminal(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for now
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("failed to upgrade WebSocket for terminal", "error", err)
		return
	}
	defer conn.Close()

	s.logger.Info("terminal WebSocket connected")

	// Get tmux session name from environment or use default
	tmuxSession := os.Getenv("HELIX_TMUX_SESSION")
	if tmuxSession == "" {
		tmuxSession = "claude-helix"
	}

	// Attach to tmux session using tmux attach-session
	// We use -d to detach other clients (only one active PTY at a time)
	cmd := exec.Command("tmux", "attach-session", "-t", tmuxSession)

	// Start with a PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		s.logger.Error("failed to start tmux attach", "error", err, "session", tmuxSession)
		// If tmux session doesn't exist, try creating one with a shell
		cmd = exec.Command("tmux", "new-session", "-s", tmuxSession)
		ptmx, err = pty.Start(cmd)
		if err != nil {
			s.logger.Error("failed to create tmux session", "error", err)
			conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to start terminal"))
			return
		}
	}
	defer ptmx.Close()

	// Set initial terminal size (default 120x40 if not specified)
	initialCols := 120
	initialRows := 40
	if colsStr := r.URL.Query().Get("cols"); colsStr != "" {
		if c, err := strconv.Atoi(colsStr); err == nil && c > 0 {
			initialCols = c
		}
	}
	if rowsStr := r.URL.Query().Get("rows"); rowsStr != "" {
		if r, err := strconv.Atoi(rowsStr); err == nil && r > 0 {
			initialRows = r
		}
	}
	pty.Setsize(ptmx, &pty.Winsize{
		Rows: uint16(initialRows),
		Cols: uint16(initialCols),
	})

	var wg sync.WaitGroup
	done := make(chan struct{})

	// PTY -> WebSocket (terminal output)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
			}

			n, err := ptmx.Read(buf)
			if err != nil {
				if err != io.EOF {
					s.logger.Debug("terminal PTY read error", "error", err)
				}
				return
			}

			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				s.logger.Debug("terminal WebSocket write error", "error", err)
				return
			}
		}
	}()

	// WebSocket -> PTY (user input and resize)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(done)
		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
					s.logger.Debug("terminal WebSocket read error", "error", err)
				}
				return
			}

			// Handle JSON control messages
			if msgType == websocket.TextMessage {
				var msg TerminalMessage
				if err := json.Unmarshal(data, &msg); err != nil {
					s.logger.Debug("terminal invalid JSON message", "error", err)
					continue
				}

				switch msg.Type {
				case "input":
					// Write input to PTY
					if _, err := ptmx.Write([]byte(msg.Data)); err != nil {
						s.logger.Debug("terminal PTY write error", "error", err)
						return
					}
				case "resize":
					// Resize PTY
					if msg.Rows > 0 && msg.Cols > 0 {
						pty.Setsize(ptmx, &pty.Winsize{
							Rows: msg.Rows,
							Cols: msg.Cols,
						})
					}
				}
			} else if msgType == websocket.BinaryMessage {
				// Raw binary input (legacy support)
				if _, err := ptmx.Write(data); err != nil {
					s.logger.Debug("terminal PTY write error (binary)", "error", err)
					return
				}
			}
		}
	}()

	// Wait for either goroutine to finish
	wg.Wait()

	// Kill the tmux attach process
	if cmd.Process != nil {
		cmd.Process.Kill()
	}

	s.logger.Info("terminal WebSocket disconnected")
}

// PromptRequest represents a request to inject a prompt into Claude Code
type PromptRequest struct {
	Prompt string `json:"prompt"`
}

// handleClaudePrompt injects a prompt into the Claude Code tmux session.
// This enables programmatic control of Claude Code without requiring
// a WebSocket terminal connection.
//
// Protocol:
// - POST with JSON body: {"prompt": "your prompt text"}
// - Returns 200 on success, error status on failure
func (s *Server) handleClaudePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req PromptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.Prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}

	// Get tmux session name
	tmuxSession := os.Getenv("HELIX_TMUX_SESSION")
	if tmuxSession == "" {
		tmuxSession = "claude-helix"
	}

	// Use tmux send-keys to inject the prompt
	// We send literal text followed by Enter to submit
	cmd := exec.Command("tmux", "send-keys", "-t", tmuxSession, req.Prompt, "Enter")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Error("failed to send prompt to tmux", "error", err, "output", string(output))
		http.Error(w, "Failed to send prompt: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("prompt injected into Claude Code", "prompt_length", len(req.Prompt))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Prompt sent to Claude Code",
	})
}

// handleClaudeInteractions streams Claude Code interactions via Server-Sent Events.
// This enables real-time monitoring of Claude's conversation without parsing
// the terminal output.
//
// Protocol:
// - GET request with Accept: text/event-stream
// - Streams SSE events with type "interaction" and JSON data
func (s *Server) handleClaudeInteractions(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Create a channel to receive interactions
	interactionCh := make(chan *ClaudeInteraction, 100)

	// Get work directory from environment
	workDir := os.Getenv("WORKSPACE_DIR")
	if workDir == "" {
		workDir = os.Getenv("HOME") + "/work"
	}

	// Create JSONL watcher
	watcher, err := NewClaudeJSONLWatcher(ClaudeJSONLWatcherConfig{
		WorkDir: workDir,
		OnInteraction: func(interaction *ClaudeInteraction) {
			select {
			case interactionCh <- interaction:
			default:
				// Channel full, drop interaction
			}
		},
		OnError: func(err error) {
			s.logger.Warn("JSONL watcher error", "error", err)
		},
	})
	if err != nil {
		s.logger.Error("failed to create JSONL watcher", "error", err)
		http.Error(w, "Failed to start interaction watcher", http.StatusInternalServerError)
		return
	}

	if err := watcher.Start(); err != nil {
		s.logger.Error("failed to start JSONL watcher", "error", err)
		http.Error(w, "Failed to start interaction watcher", http.StatusInternalServerError)
		return
	}
	defer watcher.Stop()

	s.logger.Info("SSE client connected for Claude interactions")

	// Send keep-alive and interactions
	keepAlive := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				keepAlive <- struct{}{}
			case <-r.Context().Done():
				return
			}
		}
	}()

	for {
		select {
		case <-r.Context().Done():
			s.logger.Info("SSE client disconnected")
			return

		case <-keepAlive:
			fmt.Fprintf(w, ": keep-alive\n\n")
			flusher.Flush()

		case interaction := <-interactionCh:
			data, err := json.Marshal(interaction)
			if err != nil {
				s.logger.Warn("failed to marshal interaction", "error", err)
				continue
			}
			fmt.Fprintf(w, "event: interaction\ndata: %s\n\n", data)
			flusher.Flush()
		}
	}
}
