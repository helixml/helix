package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

const (
	sessionTerminalWorkingDirectory = "/home/retro/work"
	sessionTerminalPrompt           = `\[\033[38;5;111m\]helix\[\033[0m\] \[\033[38;5;250m\]\w\[\033[0m\] \[\033[38;5;111m\]❯\[\033[0m\] `
)

// sessionTerminal opens a persistent terminal in an external-agent session's
// development container.
//
// @Summary Session terminal websocket
// @Tags Sessions
// @Param id path string true "Session ID"
// @Router /api/v1/sessions/{id}/terminal [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) sessionTerminal(rw http.ResponseWriter, r *http.Request) {
	session, client := s.loadAuthorizedTerminalSession(rw, r, types.ActionUpdate)
	if session == nil {
		return
	}
	s.openPersistentTerminal(rw, r, client, session.ID, sessionTerminalWorkingDirectory, sessionTerminalPrompt)
}

// sessionTerminalSessions lists Helix-managed tmux sessions in an
// external-agent session's development container.
//
// @Summary List session tmux sessions
// @Tags Sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} server.SandboxTerminalSessionsResponse
// @Router /api/v1/sessions/{id}/terminal/sessions [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) sessionTerminalSessions(rw http.ResponseWriter, r *http.Request) {
	session, client := s.loadAuthorizedTerminalSession(rw, r, types.ActionGet)
	if session == nil {
		return
	}
	s.listPersistentTerminalSessions(rw, r, client, session.ID)
}

// deleteSessionTerminalSession kills one Helix-managed tmux session in an
// external-agent session's development container.
//
// @Summary Delete session tmux session
// @Tags Sessions
// @Param id path string true "Session ID"
// @Param terminal_session path string true "Terminal session ID"
// @Success 204
// @Router /api/v1/sessions/{id}/terminal/sessions/{terminal_session} [delete]
// @Security ApiKeyAuth
func (s *HelixAPIServer) deleteSessionTerminalSession(rw http.ResponseWriter, r *http.Request) {
	session, client := s.loadAuthorizedTerminalSession(rw, r, types.ActionUpdate)
	if session == nil {
		return
	}
	s.deletePersistentTerminalSession(rw, r, client, session.ID, mux.Vars(r)["terminal_session"])
}

func (s *HelixAPIServer) loadAuthorizedTerminalSession(rw http.ResponseWriter, r *http.Request, action types.Action) (*types.Session, *hydra.RevDialClient) {
	sessionID := mux.Vars(r)["id"]
	if sessionID == "" {
		http.Error(rw, "session id is required", http.StatusBadRequest)
		return nil, nil
	}

	session, err := s.Store.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "session not found", http.StatusNotFound)
		} else {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
		return nil, nil
	}
	if err := s.authorizeUserToSession(r.Context(), getRequestUser(r), session, action); err != nil {
		http.Error(rw, "forbidden", http.StatusForbidden)
		return nil, nil
	}

	sandboxID := strings.TrimSpace(session.SandboxID)
	if sandboxID == "" {
		http.Error(rw, "session has no sandbox host assigned", http.StatusServiceUnavailable)
		return nil, nil
	}

	return session, hydra.NewRevDialClient(s.connman, fmt.Sprintf("hydra-%s", sandboxID))
}
