package desktop

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var ptyUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const tmuxSessionName = "helix-shell"

// ensureTmuxSession starts the shared tmux session if it doesn't exist.
// The session uses C-] as prefix (avoids conflicts with user's tmux/hmux)
// and has the status bar hidden.
func (s *Server) ensureTmuxSession() error {
	// Check if session exists
	cmd := exec.Command("tmux", "has-session", "-t", tmuxSessionName)
	if cmd.Run() == nil {
		return nil // already running
	}

	// Create new session with hidden prefix and no status bar
	cmd = exec.Command("tmux", "new-session",
		"-d",                    // detached
		"-s", tmuxSessionName,   // session name
		"-x", "80", "-y", "24", // default size
	)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if err := cmd.Run(); err != nil {
		return err
	}

	// Configure the session: hide status bar, set obscure prefix
	for _, setting := range []string{
		"set -g status off",
		"set -g prefix C-]",
		"unbind C-b",
		"bind C-] send-prefix",
		"set -g mouse on",
		"set -g history-limit 10000",
	} {
		exec.Command("tmux", "send-keys", "-t", tmuxSessionName, "", "").Run()
		args := append([]string{"set-option", "-t", tmuxSessionName}, splitTmuxArgs(setting)...)
		exec.Command("tmux", args...).Run()
	}

	s.logger.Info("pty: created tmux session", "name", tmuxSessionName)
	return nil
}

func splitTmuxArgs(s string) []string {
	// Simple split on spaces for tmux set commands
	var args []string
	for _, a := range splitFields(s) {
		args = append(args, a)
	}
	return args
}

func splitFields(s string) []string {
	var fields []string
	field := ""
	for _, c := range s {
		if c == ' ' {
			if field != "" {
				fields = append(fields, field)
				field = ""
			}
		} else {
			field += string(c)
		}
	}
	if field != "" {
		fields = append(fields, field)
	}
	return fields
}

// handlePTY upgrades to WebSocket and attaches to the shared tmux session.
// If no tmux session exists, creates one. All I/O is streamed bidirectionally.
//
// Query params:
//   - cols: terminal columns (default 80)
//   - rows: terminal rows (default 24)
//
// WebSocket messages:
//   - Binary: raw terminal I/O
//   - Text JSON: control messages {"type":"resize","cols":N,"rows":N}
func (s *Server) handlePTY(w http.ResponseWriter, r *http.Request) {
	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("pty: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	cols := 80
	rows := 24
	if c, err := strconv.Atoi(r.URL.Query().Get("cols")); err == nil && c > 0 {
		cols = c
	}
	if rv, err := strconv.Atoi(r.URL.Query().Get("rows")); err == nil && rv > 0 {
		rows = rv
	}

	// Ensure shared tmux session exists
	if err := s.ensureTmuxSession(); err != nil {
		s.logger.Error("pty: failed to create tmux session", "err", err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"failed to start shell"}`))
		return
	}

	// Attach to the tmux session with a new PTY
	cmd := exec.Command("tmux", "attach-session", "-t", tmuxSessionName)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		s.logger.Error("pty: failed to attach to tmux", "err", err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"failed to attach to shell"}`))
		return
	}
	defer ptmx.Close()

	s.logger.Info("pty: client attached to tmux session",
		"pid", cmd.Process.Pid, "cols", cols, "rows", rows)

	var wg sync.WaitGroup

	// PTY stdout → WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
	}()

	// WebSocket → PTY stdin (+ handle resize)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			// Don't kill the tmux session when client disconnects —
			// just detach. The session persists for reconnect.
			cmd.Process.Signal(syscall.SIGHUP)
		}()

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			if msgType == websocket.TextMessage {
				s.handlePTYControl(ptmx, data)
				continue
			}

			if _, err := ptmx.Write(data); err != nil {
				return
			}
		}
	}()

	cmd.Wait()
	s.logger.Info("pty: client detached from tmux session", "pid", cmd.Process.Pid)
	wg.Wait()
}

func (s *Server) handlePTYControl(ptmx *os.File, data []byte) {
	type controlMsg struct {
		Type string `json:"type"`
		Cols int    `json:"cols"`
		Rows int    `json:"rows"`
	}

	var msg controlMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	if msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
		setWinsize(ptmx, msg.Cols, msg.Rows)
		s.logger.Debug("pty: resized", "cols", msg.Cols, "rows", msg.Rows)
	}
}

func setWinsize(f *os.File, cols, rows int) {
	ws := struct {
		Rows uint16
		Cols uint16
		X    uint16
		Y    uint16
	}{
		Rows: uint16(rows),
		Cols: uint16(cols),
	}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}
