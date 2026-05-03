package server

// Sandboxes API handlers — REST surface for the user-facing Sandboxes
// feature. All routes are scoped to an organization the calling user is a
// member of, except websocket-style streaming endpoints which authorize the
// sandbox by id.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// terminalUpgrader bridges the user's browser websocket to the hydra-side
// terminal stream. CheckOrigin allows same-origin and CORS-relaxed for
// in-page xterm clients; rely on the auth middleware (the route runs through
// authRouter) for actual authorization.
var sandboxTerminalUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// listSandboxRuntimes returns the runtimes the operator has configured on
// this server. Used by the UI dropdown and the CLI to validate before posting.
//
// @Summary List sandbox runtimes
// @Description List the sandbox runtimes available on this server
// @Tags Sandboxes
// @Produce json
// @Success 200 {object} map[string][]string
// @Security ApiKeyAuth
// @Router /api/v1/sandbox-runtimes [get]
func (s *HelixAPIServer) listSandboxRuntimes(rw http.ResponseWriter, _ *http.Request) {
	writeJSON(rw, http.StatusOK, map[string]any{
		"runtimes": s.sandboxController.Runtimes().Names(),
	})
}

// listOrgSandboxes lists sandboxes for an organization the caller is a
// member of.
//
// @Summary List sandboxes
// @Description List sandboxes belonging to an organization
// @Tags Sandboxes
// @Produce json
// @Param org_id path string true "Organization ID"
// @Success 200 {object} types.SandboxListResponse
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/organizations/{org_id}/sandboxes [get]
func (s *HelixAPIServer) listOrgSandboxes(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["org_id"]

	if _, err := s.authorizeOrgMember(r.Context(), user, orgID); err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	projectID := r.URL.Query().Get("project_id")
	sandboxes, err := s.sandboxController.List(r.Context(), orgID, projectID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(rw, http.StatusOK, types.SandboxListResponse{
		Sandboxes: sandboxes,
		Total:     len(sandboxes),
	})
}

// createOrgSandbox spins up a new sandbox in the given organization.
//
// @Summary Create sandbox
// @Description Create a new sandbox in an organization
// @Tags Sandboxes
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param payload body types.CreateSandboxRequest true "Sandbox spec"
// @Success 201 {object} types.Sandbox
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/organizations/{org_id}/sandboxes [post]
func (s *HelixAPIServer) createOrgSandbox(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	orgID := mux.Vars(r)["org_id"]

	if _, err := s.authorizeOrgMember(r.Context(), user, orgID); err != nil {
		http.Error(rw, err.Error(), http.StatusForbidden)
		return
	}

	var req types.CreateSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(rw, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}

	sb, err := s.sandboxController.Create(r.Context(), orgID, user.ID, &req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(rw, http.StatusCreated, sb)
}

// getSandbox returns a single sandbox by id.
//
// @Summary Get sandbox
// @Tags Sandboxes
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Success 200 {object} types.Sandbox
// @Router /api/v1/organizations/{org_id}/sandboxes/{id} [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) getSandbox(rw http.ResponseWriter, r *http.Request) {
	sb, err := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	_ = err
	writeJSON(rw, http.StatusOK, sb)
}

// updateSandbox patches name/tags/timeout.
//
// @Summary Update sandbox
// @Tags Sandboxes
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param payload body types.UpdateSandboxRequest true "Patch"
// @Success 200 {object} types.Sandbox
// @Router /api/v1/organizations/{org_id}/sandboxes/{id} [patch]
// @Security ApiKeyAuth
func (s *HelixAPIServer) updateSandbox(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	var req types.UpdateSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(rw, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := s.sandboxController.Update(r.Context(), sb.ID, &req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(rw, http.StatusOK, updated)
}

// deleteSandbox tears down the underlying container and clears all state.
//
// @Summary Delete sandbox
// @Tags Sandboxes
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Success 204
// @Router /api/v1/organizations/{org_id}/sandboxes/{id} [delete]
// @Security ApiKeyAuth
func (s *HelixAPIServer) deleteSandbox(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	if err := s.sandboxController.Delete(r.Context(), sb.ID); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// runSandboxCommand starts a command in the sandbox.
//
// @Summary Run a command in a sandbox
// @Tags Sandboxes
// @Accept json
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param payload body types.RunSandboxCommandRequest true "Command spec"
// @Success 200 {object} hydra.SandboxCommandResponse
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/commands [post]
// @Security ApiKeyAuth
func (s *HelixAPIServer) runSandboxCommand(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}

	var body types.RunSandboxCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Cmd == "" {
		http.Error(rw, "cmd is required", http.StatusBadRequest)
		return
	}
	envSlice := make([]string, 0, len(body.Env))
	for k, v := range body.Env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}

	req := &hydra.ExecRequest{
		SandboxID:      sb.ID,
		CmdID:          system.GenerateSandboxCommandID(),
		Cmd:            body.Cmd,
		Args:           body.Args,
		Cwd:            body.Cwd,
		Env:            envSlice,
		Sudo:           body.Sudo,
		Detached:       body.Detached,
		TimeoutSeconds: body.TimeoutSeconds,
	}
	resp, err := client.RunSandboxCommand(r.Context(), sb.ID, req)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(rw, http.StatusOK, resp)
}

// listSandboxCommands returns every command tracked for the sandbox.
//
// @Summary List sandbox commands
// @Tags Sandboxes
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Success 200 {object} hydra.ListSandboxCommandsResponse
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/commands [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) listSandboxCommands(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	resp, err := client.ListSandboxCommands(r.Context(), sb.ID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(rw, http.StatusOK, resp)
}

// getSandboxCommand returns a specific command.
//
// @Summary Get a sandbox command
// @Tags Sandboxes
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param cmd_id path string true "Command ID"
// @Success 200 {object} hydra.SandboxCommandResponse
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id} [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) getSandboxCommand(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	cmdID := mux.Vars(r)["cmd_id"]
	resp, err := client.GetSandboxCommand(r.Context(), sb.ID, cmdID)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(rw, http.StatusOK, resp)
}

// streamSandboxCommandLogs proxies the SSE log stream from hydra.
//
// @Summary Stream sandbox command logs
// @Tags Sandboxes
// @Produce text/event-stream
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param cmd_id path string true "Command ID"
// @Param stream query string false "stdout|stderr|both"
// @Param follow query string false "1 to follow"
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/logs [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) streamSandboxCommandLogs(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	cmdID := mux.Vars(r)["cmd_id"]
	stream := r.URL.Query().Get("stream")
	follow := r.URL.Query().Get("follow") == "1"

	body, err := client.StreamSandboxCommandLogs(r.Context(), sb.ID, cmdID, stream, follow)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer body.Close()

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")
	flusher, _ := rw.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := body.Read(buf)
		if n > 0 {
			rw.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// killSandboxCommand sends a signal to a running command.
//
// @Summary Kill a sandbox command
// @Tags Sandboxes
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param cmd_id path string true "Command ID"
// @Param signal query string false "Signal name (default TERM)"
// @Success 204
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/commands/{cmd_id}/kill [post]
// @Security ApiKeyAuth
func (s *HelixAPIServer) killSandboxCommand(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	cmdID := mux.Vars(r)["cmd_id"]
	if err := client.KillSandboxCommand(r.Context(), sb.ID, cmdID, r.URL.Query().Get("signal")); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}

// sandboxFile reads/writes/deletes a single file inside the sandbox.
//
// @Summary Read/write/delete sandbox file
// @Tags Sandboxes
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param path query string true "Absolute path inside the sandbox"
// @Param mode query string false "Octal permission for write"
// @Param recursive query string false "1 to delete recursively"
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/files [get]
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/files [put]
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/files [delete]
// @Security ApiKeyAuth
func (s *HelixAPIServer) sandboxFile(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(rw, "path query parameter required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, err := client.ReadSandboxFile(r.Context(), sb.ID, path)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.Header().Set("Content-Type", "application/octet-stream")
		rw.Write(data)
	case http.MethodPut:
		modeStr := r.URL.Query().Get("mode")
		mode := 0
		if modeStr != "" {
			parsed, err := strconv.ParseInt(modeStr, 8, 32)
			if err != nil {
				http.Error(rw, "invalid mode", http.StatusBadRequest)
				return
			}
			mode = int(parsed)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}
		if err := client.WriteSandboxFile(r.Context(), sb.ID, path, body, mode); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		recursive := r.URL.Query().Get("recursive") == "1"
		if err := client.DeleteSandboxFile(r.Context(), sb.ID, path, recursive); err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusNoContent)
	default:
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// listSandboxFiles enumerates a directory inside the sandbox.
//
// @Summary List directory in sandbox
// @Tags Sandboxes
// @Produce json
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Param path query string false "Directory path (default /root)"
// @Success 200 {object} hydra.ListSandboxFilesResponse
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/files/list [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) listSandboxFiles(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}
	resp, err := client.ListSandboxFiles(r.Context(), sb.ID, r.URL.Query().Get("path"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(rw, http.StatusOK, resp)
}

// sandboxTerminal opens a websocket-backed PTY into the sandbox.
//
// Frame protocol (browser ↔ this handler):
//   - Binary frames: stdin (browser → server) and stdout (server → browser).
//   - Text JSON frames: control messages, e.g. {"type":"resize","cols":80,"rows":24}.
//
// @Summary Sandbox terminal websocket
// @Tags Sandboxes
// @Param org_id path string true "Organization ID"
// @Param id path string true "Sandbox ID"
// @Router /api/v1/organizations/{org_id}/sandboxes/{id}/terminal [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) sandboxTerminal(rw http.ResponseWriter, r *http.Request) {
	sb, _ := s.loadAuthorizedSandbox(rw, r)
	if sb == nil {
		return
	}
	client, err := s.sandboxController.HydraClient(sb)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusServiceUnavailable)
		return
	}

	wsConn, err := sandboxTerminalUpgrader.Upgrade(rw, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("sandbox terminal upgrade failed")
		return
	}
	defer wsConn.Close()

	hydraConn, err := client.OpenSandboxTerminal(r.Context(), sb.ID, r.URL.Query().Get("shell"))
	if err != nil {
		wsConn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","message":"`+jsonEscapeString(err.Error())+`"}`))
		return
	}
	defer hydraConn.Close()

	// Bridge: forward each websocket message between browser and hydra using
	// the same message type, so binary stays binary (stdin/stdout) and text
	// stays text (control JSON like {"type":"resize"} or error reports).
	browserDone := make(chan struct{})
	go func() {
		defer close(browserDone)
		for {
			mt, data, err := wsConn.ReadMessage()
			if err != nil {
				return
			}
			if werr := hydraConn.WriteMessage(mt, data); werr != nil {
				return
			}
		}
	}()

	hydraDone := make(chan struct{})
	go func() {
		defer close(hydraDone)
		for {
			mt, data, err := hydraConn.ReadMessage()
			if err != nil {
				return
			}
			if werr := wsConn.WriteMessage(mt, data); werr != nil {
				return
			}
		}
	}()

	select {
	case <-browserDone:
	case <-hydraDone:
	}
}

// loadAuthorizedSandbox fetches the sandbox by id, verifies the caller is a
// member of its organization, and confirms the URL's org_id matches the
// sandbox's org so cross-org id-guessing is blocked. Writes an HTTP error and
// returns nil if access is denied.
func (s *HelixAPIServer) loadAuthorizedSandbox(rw http.ResponseWriter, r *http.Request) (*types.Sandbox, error) {
	user := getRequestUser(r)
	vars := mux.Vars(r)
	id := vars["id"]
	urlOrgID := vars["org_id"]
	sb, err := s.sandboxController.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "sandbox not found", http.StatusNotFound)
		} else {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		return nil, err
	}
	if urlOrgID != "" && urlOrgID != sb.OrganizationID {
		http.Error(rw, "sandbox not found", http.StatusNotFound)
		return nil, errors.New("org mismatch")
	}
	if _, err := s.authorizeOrgMember(r.Context(), user, sb.OrganizationID); err != nil {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return nil, err
	}
	return sb, nil
}

// writeJSON writes a JSON response with status.
func writeJSON(rw http.ResponseWriter, status int, body interface{}) {
	rw.Header().Set("Content-Type", "application/json")
	rw.WriteHeader(status)
	_ = json.NewEncoder(rw).Encode(body)
}

// jsonEscapeString returns a JSON-safe inner string (no surrounding quotes).
func jsonEscapeString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	if len(b) < 2 {
		return ""
	}
	return string(b[1 : len(b)-1])
}
