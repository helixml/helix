package hydra

// HTTP handlers exposing the Sandboxes API operations on hydra.
// These run inside the sandbox-nvidia container and are reached from the API
// server via RevDial.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

// terminalUpgrader is the websocket upgrader used by the terminal handler.
// CheckOrigin returns true because the connection is on a private hydra socket
// reachable only from the API server through RevDial — there is no browser
// origin to police.
var terminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// handleSandboxExec runs (or starts in the background) a command inside the
// container associated with sessionID.
func (s *Server) handleSandboxExec(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request: %s", err), http.StatusBadRequest)
		return
	}
	if req.SandboxID == "" {
		req.SandboxID = sessionID
	}
	if req.CmdID == "" {
		req.CmdID = fmt.Sprintf("cmd-%d", time.Now().UnixNano())
	}

	ctx := r.Context()
	if !req.Detached && req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 60
	}
	rec, err := s.sandboxOps.RunCommand(ctx, sessionID, &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := buildExecResponse(rec)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleSandboxExecList returns every command tracked for this sandbox.
func (s *Server) handleSandboxExecList(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	records := s.sandboxOps.ListCommands(sessionID)
	out := make([]map[string]interface{}, 0, len(records))
	for _, rec := range records {
		out = append(out, buildExecResponse(rec))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"commands": out})
}

// handleSandboxExecGet returns a single command record.
func (s *Server) handleSandboxExecGet(w http.ResponseWriter, r *http.Request) {
	cmdID := mux.Vars(r)["cmd_id"]
	rec, err := s.sandboxOps.GetCommand(cmdID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildExecResponse(rec))
}

// handleSandboxExecLogs streams logs over Server-Sent Events.
// Query: ?stream=stdout|stderr|both (default both), ?follow=1 to keep open.
func (s *Server) handleSandboxExecLogs(w http.ResponseWriter, r *http.Request) {
	cmdID := mux.Vars(r)["cmd_id"]
	rec, err := s.sandboxOps.GetCommand(cmdID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	wantStream := r.URL.Query().Get("stream")
	if wantStream == "" {
		wantStream = "both"
	}
	follow := r.URL.Query().Get("follow") == "1"

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	if !follow {
		// One-shot snapshot.
		if wantStream == "stdout" || wantStream == "both" {
			fmt.Fprintf(w, "event: stdout\ndata: %s\n\n", jsonEscape(rec.Stdout()))
		}
		if wantStream == "stderr" || wantStream == "both" {
			fmt.Fprintf(w, "event: stderr\ndata: %s\n\n", jsonEscape(rec.Stderr()))
		}
		fmt.Fprintf(w, "event: end\ndata: {}\n\n")
		flusher.Flush()
		return
	}

	ch, cancel := rec.Subscribe()
	defer cancel()
	notify := r.Context().Done()
	for {
		select {
		case chunk, ok := <-ch:
			if !ok {
				fmt.Fprintf(w, "event: end\ndata: {}\n\n")
				flusher.Flush()
				return
			}
			if wantStream != "both" && chunk.Stream != wantStream {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", chunk.Stream, jsonEscape(chunk.Data))
			flusher.Flush()
		case <-notify:
			return
		}
	}
}

// handleSandboxExecKill sends a signal to a running command.
// Query: ?signal=TERM (default).
func (s *Server) handleSandboxExecKill(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["session_id"]
	cmdID := vars["cmd_id"]
	signal := r.URL.Query().Get("signal")
	if err := s.sandboxOps.KillCommand(r.Context(), sessionID, cmdID, signal); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleSandboxFile reads, writes, or deletes a file inside the sandbox.
// GET    ?path=...                returns raw bytes
// PUT    ?path=...&mode=0644      writes raw body to the file
// DELETE ?path=...&recursive=1
func (s *Server) handleSandboxFile(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path query parameter required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		data, err := s.sandboxOps.ReadFile(r.Context(), sessionID, path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(data)
	case http.MethodPut:
		modeStr := r.URL.Query().Get("mode")
		mode := int64(0)
		if modeStr != "" {
			parsed, err := strconv.ParseInt(modeStr, 8, 32)
			if err != nil {
				http.Error(w, "invalid mode", http.StatusBadRequest)
				return
			}
			mode = parsed
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.sandboxOps.WriteFile(r.Context(), sessionID, path, body, mode); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		recursive := r.URL.Query().Get("recursive") == "1"
		if err := s.sandboxOps.DeleteFile(r.Context(), sessionID, path, recursive); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSandboxFileList enumerates a directory.
func (s *Server) handleSandboxFileList(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/root"
	}
	entries, err := s.sandboxOps.ListDirectory(r.Context(), sessionID, path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":    path,
		"entries": entries,
	})
}

// handleSandboxForget drops every cached command record for a sandbox; called
// when the API server tears the sandbox down.
func (s *Server) handleSandboxForget(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	s.sandboxOps.ForgetSandbox(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

// handleSandboxTerminal opens a websocket-backed PTY into the sandbox.
//
// Frame format:
//   - Binary frames: stdin (browser → server) and stdout (server → browser).
//   - Text frames: control messages, JSON-encoded:
//     {"type":"resize","cols":80,"rows":24}
//
// The handler issues `docker exec -it /bin/bash` (or sh fallback) and bridges
// stdio between the websocket and the docker hijacked stream.
func (s *Server) handleSandboxTerminal(w http.ResponseWriter, r *http.Request) {
	sessionID := mux.Vars(r)["session_id"]
	dc := s.devContainerManager.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}

	dockerClient, err := s.devContainerManager.getDockerClient(dc.DockerSocket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dockerClient.Close()

	conn, err := terminalUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("terminal upgrade failed")
		return
	}
	defer conn.Close()

	cmd := []string{"/bin/bash"}
	if shell := r.URL.Query().Get("shell"); shell != "" {
		// A shell value containing whitespace is treated as a /bin/sh -c
		// expression so callers can pass multi-arg commands like
		// "tmux new-session -A -s foo". A bare value like "/bin/zsh" is
		// still executed directly.
		if strings.ContainsAny(shell, " \t") {
			cmd = []string{"/bin/sh", "-c", shell}
		} else {
			cmd = []string{shell}
		}
	}

	cfg := dockertypes.ExecConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
		Env:          []string{"TERM=xterm-256color"},
	}
	created, err := dockerClient.ContainerExecCreate(r.Context(), dc.ContainerID, cfg)
	if err != nil {
		// Try /bin/sh as fallback.
		cfg.Cmd = []string{"/bin/sh"}
		created, err = dockerClient.ContainerExecCreate(r.Context(), dc.ContainerID, cfg)
		if err != nil {
			conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"failed to create exec"}`))
			return
		}
	}

	attach, err := dockerClient.ContainerExecAttach(r.Context(), created.ID, dockertypes.ExecStartCheck{Tty: true})
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"failed to attach exec"}`))
		return
	}
	defer attach.Close()

	// stdin: ws → docker
	go func() {
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				attach.CloseWrite()
				return
			}
			switch mt {
			case websocket.BinaryMessage:
				if _, werr := attach.Conn.Write(data); werr != nil {
					return
				}
			case websocket.TextMessage:
				var ctrl struct {
					Type string `json:"type"`
					Cols uint   `json:"cols"`
					Rows uint   `json:"rows"`
				}
				if err := json.Unmarshal(data, &ctrl); err != nil {
					continue
				}
				if ctrl.Type == "resize" {
					_ = dockerClient.ContainerExecResize(r.Context(), created.ID, dockertypes.ResizeOptions{
						Height: ctrl.Rows,
						Width:  ctrl.Cols,
					})
				}
			}
		}
	}()

	// stdout: docker → ws (TTY mode means stdout/stderr are merged).
	wsWriter := wsBinaryWriter{conn: conn}
	if _, err := stdcopyOrTty(&wsWriter, attach.Reader, true); err != nil && err != io.EOF {
		log.Debug().Err(err).Msg("terminal stream ended")
	}
}

// wsBinaryWriter writes bytes as websocket binary frames.
type wsBinaryWriter struct {
	conn *websocket.Conn
}

func (w *wsBinaryWriter) Write(p []byte) (int, error) {
	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// stdcopyOrTty copies output. With TTY=true, the byte stream is raw;
// otherwise it's the docker multiplexed stream.
func stdcopyOrTty(dst io.Writer, src io.Reader, tty bool) (int64, error) {
	if tty {
		return io.Copy(dst, src)
	}
	return stdcopy.StdCopy(dst, dst, src)
}

// buildExecResponse converts a record into a JSON-friendly map.
func buildExecResponse(rec *SandboxCmdRecord) map[string]interface{} {
	out := map[string]interface{}{
		"id":         rec.ID,
		"sandbox_id": rec.SandboxID,
		"cmd":        rec.Cmd,
		"args":       rec.Args,
		"cwd":        rec.Cwd,
		"sudo":       rec.Sudo,
		"detached":   rec.Detached,
		"status":     rec.Status,
		"started_at": rec.StartedAt,
	}
	if rec.ExitCode != nil {
		out["exit_code"] = *rec.ExitCode
	}
	if rec.FinishedAt != nil {
		out["finished_at"] = *rec.FinishedAt
	}
	out["stdout"] = rec.Stdout()
	out["stderr"] = rec.Stderr()
	return out
}

// jsonEscape returns a JSON-safe string with surrounding quotes stripped, so
// it can be inlined as SSE data.
func jsonEscape(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// Compile-time guard so unused-import linters don't flag context.
var _ = context.Background
