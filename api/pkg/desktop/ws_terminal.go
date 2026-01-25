//go:build cgo

// Package desktop provides WebSocket terminal streaming for Claude Code.
// Connects to a tmux session and relays I/O to web clients via xterm.js.
package desktop

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"

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
