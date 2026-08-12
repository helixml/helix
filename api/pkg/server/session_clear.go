package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SessionBackend is the per-runtime half of a "clear session" operation. The
// shared DB interactions (the source of truth for both runtimes) are cleared by
// the ClearSession coordinator; each backend only resets the runtime-specific
// conversational state it owns. Adding a future runtime means implementing this
// one method, not editing the handler.
type SessionBackend interface {
	// Clear resets the runtime-specific conversational state for a session.
	Clear(ctx context.Context, sessionID string) error
}

// externalAgentTransport is the slice of the external-agent WebSocket machinery
// the Zed backend needs. *HelixAPIServer satisfies it in production; tests pass a
// fake so the backend can be exercised without a live WebSocket connection.
type externalAgentTransport interface {
	cancelCurrentTurnIfActive(ctx context.Context, sessionID string)
	sendCommandToExternalAgent(sessionID string, command types.ExternalAgentCommand) error
}

// internalAgentBackend resets the in-process Go agent. Internal agent sessions
// are request-scoped (created per turn in controller.runAgent and re-seeded from
// the DB), so there is normally no long-lived in-memory history to clear — once
// the DB rows are gone, the next turn naturally starts empty. liveSession exists
// only so a live, in-memory history (if one is ever held) can be flushed too; in
// production it is nil and Clear is a no-op.
type internalAgentBackend struct {
	liveSession func(sessionID string) *agent.Session
}

func (b *internalAgentBackend) Clear(_ context.Context, sessionID string) error {
	if b.liveSession == nil {
		return nil
	}
	if s := b.liveSession(sessionID); s != nil {
		s.GetMessageHistory().Clear()
	}
	return nil
}

// zedACPBackend resets a headless-Zed external agent. Zed keeps its own thread
// context independent of the Helix DB, so clearing the DB is not enough: the Zed
// thread must be reset too.
//
// The server cannot mint a Zed-valid thread ID (Zed creates them and persists
// them in threads.db; open_thread only re-opens an existing thread). The
// canonical "start fresh" signal is acp_thread_id=nil on the next chat_message,
// which Zed turns into a brand-new thread — the same path forked sessions use.
// So Clear resets ZedThreadID to "" and the next message opens a clean thread,
// discarding prior context. No new protocol command is required.
type zedACPBackend struct {
	store     store.Store
	transport externalAgentTransport
}

func (b *zedACPBackend) Clear(ctx context.Context, sessionID string) error {
	// 1. Cancel any in-flight turn first so an in-flight handleMessageCompleted
	//    can't re-insert an interaction after the DB clear. Best-effort: a
	//    disconnected agent or absent turn is fine.
	b.transport.cancelCurrentTurnIfActive(ctx, sessionID)

	// 2. Reset the Zed thread association so the next chat_message opens a fresh
	//    thread (acp_thread_id=nil). If there is no live WebSocket connection the
	//    persisted empty ZedThreadID is applied when the agent next connects, so
	//    a transient disconnect is not surfaced as an error.
	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to load session for clear: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session %s not found", sessionID)
	}
	if session.Metadata.ZedThreadID == "" {
		return nil // already at a fresh thread
	}
	session.Metadata.ZedThreadID = ""
	if err := b.store.UpdateSessionMetadata(ctx, sessionID, session.Metadata); err != nil {
		return fmt.Errorf("failed to reset zed thread id: %w", err)
	}
	return nil
}

// backendFor selects the runtime backend for a session. A session is Zed/ACP
// backed when it is an external Zed agent (AgentType, ExternalAgentID or
// ExternalAgentConfig set); otherwise it is driven by the internal Go agent.
func (apiServer *HelixAPIServer) backendFor(session *types.Session) SessionBackend {
	m := session.Metadata
	if m.AgentType == "zed_external" || m.ExternalAgentID != "" || m.ExternalAgentConfig != nil {
		return &zedACPBackend{store: apiServer.Store, transport: apiServer}
	}
	return &internalAgentBackend{}
}

// ClearSession is the single entry point that "hangs off" a Helix session: it
// performs the shared DB clear (source of truth for both runtimes), then
// delegates the runtime-specific reset to the appropriate backend, and returns
// the updated (now empty) session.
func (apiServer *HelixAPIServer) ClearSession(ctx context.Context, sessionID string) (*types.Session, error) {
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load session: %w", err)
	}
	if session == nil {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	// Shared, source-of-truth clear: removes all interactions from the DB.
	if err := apiServer.Store.ClearSessionInteractions(ctx, sessionID); err != nil {
		return nil, fmt.Errorf("failed to clear session interactions: %w", err)
	}

	// Runtime-specific reset.
	if err := apiServer.backendFor(session).Clear(ctx, sessionID); err != nil {
		return nil, err
	}

	if err := apiServer.Store.TouchSession(ctx, sessionID); err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("failed to touch session after clear")
	}

	return apiServer.Store.GetSession(ctx, sessionID)
}

// clearSessionHandler godoc
// @Summary Clear a session's conversation
// @Description Removes all interactions for a session while preserving the session
// @Description record (ID, name, project, owner, model, metadata). For Zed-backed
// @Description sessions the Zed thread is also reset so the agent starts fresh.
// @Tags    sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} types.Session
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sessions/{id}/clear [post]
func (apiServer *HelixAPIServer) clearSessionHandler(_ http.ResponseWriter, req *http.Request) (*types.Session, *system.HTTPError) {
	id := mux.Vars(req)["id"]
	if id == "" {
		return nil, system.NewHTTPError400("cannot clear session without id")
	}
	ctx := req.Context()
	user := getRequestUser(req)

	session, err := apiServer.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError404("session not found")
	}
	if session == nil {
		return nil, system.NewHTTPError404("session not found")
	}

	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	cleared, err := apiServer.ClearSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}
	return cleared, nil
}
