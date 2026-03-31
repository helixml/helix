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

// handlePTY upgrades to WebSocket and spawns a bash shell with a real PTY.
// All I/O is streamed bidirectionally over the WebSocket.
//
// Query params:
//   - cols: terminal columns (default 80)
//   - rows: terminal rows (default 24)
//
// WebSocket message types:
//   - Binary: raw terminal I/O (stdin/stdout)
//   - Text JSON: control messages {"type":"resize","cols":N,"rows":N}
func (s *Server) handlePTY(w http.ResponseWriter, r *http.Request) {
	conn, err := ptyUpgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("pty: websocket upgrade failed", "err", err)
		return
	}
	defer conn.Close()

	// Parse initial size
	cols := 80
	rows := 24
	if c, err := strconv.Atoi(r.URL.Query().Get("cols")); err == nil && c > 0 {
		cols = c
	}
	if rv, err := strconv.Atoi(r.URL.Query().Get("rows")); err == nil && rv > 0 {
		rows = rv
	}

	// Start bash with PTY
	cmd := exec.Command("/bin/bash", "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Rows: uint16(rows),
		Cols: uint16(cols),
	})
	if err != nil {
		s.logger.Error("pty: failed to start shell", "err", err)
		conn.WriteMessage(websocket.TextMessage, []byte(`{"error":"failed to start shell"}`))
		return
	}
	defer ptmx.Close()

	s.logger.Info("pty: shell started", "pid", cmd.Process.Pid, "cols", cols, "rows", rows)

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
		defer cmd.Process.Signal(syscall.SIGHUP)

		for {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			if msgType == websocket.TextMessage {
				// Control message — check for resize
				s.handlePTYControl(ptmx, data)
				continue
			}

			// Binary — write to PTY stdin
			if _, err := ptmx.Write(data); err != nil {
				return
			}
		}
	}()

	// Wait for shell to exit
	cmd.Wait()
	s.logger.Info("pty: shell exited", "pid", cmd.Process.Pid)
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
