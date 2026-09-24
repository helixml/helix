package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// threadRouteKey identifies an ACP thread by the agent connection that reported
// it. Thread IDs are chosen by each harness and are only unique within one agent
// process — Goose numbers its sessions per day (20260923_1) — so a server-wide
// key routes one sandbox's events into another sandbox's session.
type threadRouteKey struct {
	connection string // agent connection ID (the session_id the sandbox connected with)
	thread     string // ACP thread ID
}

func routeKey(connection, thread string) threadRouteKey {
	return threadRouteKey{connection: connection, thread: thread}
}

// connectionForSession returns the agent connection that carries a session's
// threads: child sessions created for new Zed threads share their parent's
// WebSocket, and a session with no live connection is its own scope.
func (apiServer *HelixAPIServer) connectionForSession(sessionID string) string {
	if wsConn, ok := apiServer.externalAgentWSManager.getConnection(sessionID); ok && wsConn != nil {
		return wsConn.SessionID
	}
	return sessionID
}

// sessionConnection is the agent connection recorded on a session: its own ID
// for the session a sandbox connected with, the parent's for child sessions.
func sessionConnection(session *types.Session) string {
	if session.Metadata.ExternalAgentID != "" {
		return session.Metadata.ExternalAgentID
	}
	return session.ID
}

// findSessionByZedThreadID is the database fallback when the in-memory route is
// missing (e.g. after an API restart). Only sessions belonging to the reporting
// connection are candidates, so a thread ID reused by another sandbox never
// resolves to another user's session.
func (apiServer *HelixAPIServer) findSessionByZedThreadID(ctx context.Context, connection, zedThreadID string) (*types.Session, error) {
	connSession, err := apiServer.Controller.Options.Store.GetSession(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("load connection session %s: %w", connection, err)
	}
	if connSession.Metadata.ZedThreadID == zedThreadID {
		return connSession, nil
	}
	sessions, _, err := apiServer.Controller.Options.Store.ListSessions(ctx, store.ListSessionsQuery{
		Owner:   connSession.Owner,
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	for _, session := range sessions {
		if session.Metadata.ZedThreadID != zedThreadID {
			continue
		}
		// Child sessions created before connections were recorded carry no
		// ExternalAgentID; accept them only within the connection's project.
		legacyChild := session.Metadata.ExternalAgentID == "" && session.ProjectID == connSession.ProjectID
		if sessionConnection(session) == connection || legacyChild {
			return session, nil
		}
	}
	return nil, fmt.Errorf("no session on connection %s with ZedThreadID %s", connection, zedThreadID)
}

// sameOwnerAsConnection rejects a session named by an agent event unless it
// belongs to the user whose sandbox is connected.
func (apiServer *HelixAPIServer) sameOwnerAsConnection(ctx context.Context, connection string, session *types.Session) error {
	if session.ID == connection {
		return nil
	}
	connSession, err := apiServer.Controller.Options.Store.GetSession(ctx, connection)
	if err != nil {
		return fmt.Errorf("load connection session %s: %w", connection, err)
	}
	if connSession.Owner != session.Owner {
		return fmt.Errorf("session %s does not belong to the owner of connection %s", session.ID, connection)
	}
	return nil
}

// authorizeExternalAgentSync lets a caller open the sync WebSocket only for a
// session it may act as: the runner token, an admin, or the session's owner
// (sandboxes connect with a session-scoped key owned by the session owner).
func (apiServer *HelixAPIServer) authorizeExternalAgentSync(next http.HandlerFunc) http.HandlerFunc {
	return func(res http.ResponseWriter, req *http.Request) {
		user := getRequestUser(req)
		if user == nil {
			http.Error(res, "unauthorized", http.StatusUnauthorized)
			return
		}
		if user.TokenType == types.TokenTypeRunner || user.Admin {
			next(res, req)
			return
		}
		sessionID := req.URL.Query().Get("session_id")
		if !strings.HasPrefix(sessionID, "ses_") {
			http.Error(res, "session_id is required", http.StatusForbidden)
			return
		}
		session, err := apiServer.Store.GetSession(req.Context(), sessionID)
		if err != nil || session.Owner != user.ID {
			http.Error(res, "forbidden", http.StatusForbidden)
			return
		}
		next(res, req)
	}
}
