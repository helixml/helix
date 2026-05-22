package helix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/pubsub"
)

// SessionClient is the small slice of the chat-session API
// EnsureAndSend depends on. Lifted out of helixclient.Client during
// H1.3a so sessions.go lives in canonical without importing the
// legacy helixclient package.
//
// Two impls satisfy this port:
//
//   - helixclient.Client (transitional, via the helixclient adapter
//     at helix-org/helix/helixclient/runtime_adapter.go): same
//     loopback-HTTP semantics as before, used by the H1 sequence's
//     intermediate states.
//   - A direct controller adapter (future): the wiring layer at
//     api/pkg/server/helix_org.go builds a SessionClient that calls
//     controller.WriteSession / WriteInteractions / ChatCompletion
//     directly. Deferred to its own slice — the structural decoupling
//     (this interface) is the H1.3a/c contribution; the behavioural
//     rewrite happens against this stable surface.
//
// hadStreamErr semantics: the loopback retry at line 130 below is a
// workaround for SSE-error chunks the loopback HTTP path emits. A
// direct controller adapter never sees this race; the retry becomes
// a no-op (the adapter returns hadStreamErr=false). Safe to keep —
// it's a one-shot, idempotent retry.
type SessionClient interface {
	StartChatWithStatus(ctx context.Context, req StartChatRequest) (Session, bool, error)
	ServerStatus(ctx context.Context) (ServerStatus, error)
}

// SpawnerClient is the chat-session surface the helix Spawner uses
// during an activation. Superset of SessionClient:
//
//   - GetOutput: polled by the Spawner's pollUntilDone loop until the
//     session reports a terminal status.
//   - StopExternalAgent: used by the chat bridge's NewHandler.
//
// helixclient.Client transitionally satisfies SpawnerClient; the
// future direct-controller adapter will too.
type SpawnerClient interface {
	SessionClient
	GetOutput(ctx context.Context, sessionID string) (Output, error)
	StopExternalAgent(ctx context.Context, sessionID string) error
}

// sendToSession is the in-package equivalent of
// helixclient.SendToSession: try to push a message to an existing
// session via /sessions/chat with SessionID set. Returns an error if
// the session is no longer running (Helix reports streamHadErr=true).
func sendToSession(ctx context.Context, client SessionClient, req StartChatRequest) (Session, error) {
	if req.SessionID == "" {
		return Session{}, errors.New("sendToSession: SessionID required")
	}
	session, streamHadErr, err := client.StartChatWithStatus(ctx, req)
	if err != nil {
		return Session{}, err
	}
	if streamHadErr {
		return Session{}, errors.New("session no longer running on the server")
	}
	return session, nil
}

// checkDesktopQuota is the in-package equivalent of
// helixclient.CheckDesktopQuota.
func checkDesktopQuota(ctx context.Context, client SessionClient) error {
	status, err := client.ServerStatus(ctx)
	if err != nil {
		// Server-status read failed — log via the caller and proceed.
		// Failing here would block every activation on a transient
		// upstream blip; the desktop quota is a pre-flight gate, not
		// a hard authorisation check.
		return nil
	}
	if !status.HasDesktopRoom() {
		return fmt.Errorf("helix: desktop quota reached (%d/%d) — try again later",
			status.ActiveConcurrentDesktops, status.MaxConcurrentDesktops)
	}
	return nil
}

// SendPromptParams configures one EnsureAndSend call. Every Helix
// chat-style operation in helix-org — owner-chat sends, worker
// activations — goes through this struct so there is exactly one
// shape of StartChat request the system can produce.
//
// SessionID is the persisted "current" session for this worker. Empty
// means "no live session yet, open one". Non-empty means "resume this".
// On resume failure (HTTP error or SSE-reported dispatch error), the
// helper transparently falls through to opening a fresh session — the
// caller doesn't see the distinction.
//
// OnSessionID fires the moment Helix echoes the active session ID
// back. Callers that need to attach side-channels (a WebSocket reader,
// a persistence write, a UI update) wire them in here; the helper
// itself stays free of side-effects beyond the StartChat call.
type SendPromptParams struct {
	SessionID      string
	ProjectID      string
	OrganizationID string
	AppID          string
	AgentType      string
	Provider       string
	Model          string
	Prompt         string
	OnSessionID    func(sessionID string)
}

// EnsureAndSend is the single primitive for "make this Helix session
// run this prompt." Both the owner-chat bridge and the worker
// activation Spawner call it — same request shape, same resume-or-fresh
// recovery, same session_role ("exploratory" — so every session is
// visible via Helix's per-project desktop view). Without one shared
// primitive these two paths drift and behave differently against
// stale state, broken WS connections, etc.
//
// Behaviour:
//
//  1. If params.SessionID is set: try to resume via SendToSession.
//     Success → invoke OnSessionID(SessionID) and return.
//     Any failure (HTTP error or SSE error chunk after the ID echo) →
//     log nothing here (caller decides) and fall through to step 2.
//  2. Pre-flight the desktop quota: a fresh session always boots a
//     Zed sandbox, and Helix fails late if the quota is exhausted.
//  3. Open a new session via StartChatWithStatus. OnSessionID is
//     wired into the StartChatRequest so it fires the moment Helix
//     emits the session ID — before the agent has produced anything,
//     so callers can attach a WS subscriber early.
//
// Returns the active session ID and a bool indicating whether step 1
// (resume) succeeded. fresh=true means a new session was opened and
// the caller should persist the returned ID.
//
// session_role is fixed at "exploratory" so the new session is
// discoverable from Helix's per-project UI (the project handlers
// query store.GetProjectExploratorySession, which filters on this
// role). Worker activations and owner chats both go through this
// path, so neither is special-cased in Helix.
const exploratoryRole = "exploratory"

func EnsureAndSend(ctx context.Context, client SessionClient, params SendPromptParams) (sessionID string, fresh bool, err error) {
	if params.Prompt == "" {
		return "", false, fmt.Errorf("EnsureAndSend: Prompt is required")
	}
	if params.ProjectID == "" && params.AppID == "" {
		return "", false, fmt.Errorf("EnsureAndSend: ProjectID or AppID is required")
	}
	if params.AgentType == "" {
		return "", false, fmt.Errorf("EnsureAndSend: AgentType is required")
	}

	// Step 1 — try to resume.
	if params.SessionID != "" {
		resumeReq := StartChatRequest{
			SessionID:           params.SessionID,
			ProjectID:           params.ProjectID,
			OrganizationID:      params.OrganizationID,
			AppID:               params.AppID,
			SessionRole:         exploratoryRole,
			AgentType:           params.AgentType,
			Type:                "text",
			ExternalAgentConfig: &ExternalAgentConfig{},
			Messages:            []SessionChatMessage{NewTextMessage("user", params.Prompt)},
		}
		if _, sendErr := sendToSession(ctx, client, resumeReq); sendErr == nil {
			if params.OnSessionID != nil {
				params.OnSessionID(params.SessionID)
			}
			return params.SessionID, false, nil
		}
		// Resume failed — caller will see fresh=true and can take
		// the opportunity to log + persist the new ID.
	}

	// Step 2 — pre-flight quota.
	if err := checkDesktopQuota(ctx, client); err != nil {
		return "", false, err
	}

	// Step 3 — open fresh.
	startReq := StartChatRequest{
		ProjectID:           params.ProjectID,
		OrganizationID:      params.OrganizationID,
		AppID:               params.AppID,
		SessionRole:         exploratoryRole,
		AgentType:           params.AgentType,
		Type:                "text",
		Provider:            params.Provider,
		Model:               params.Model,
		ExternalAgentConfig: &ExternalAgentConfig{},
		Messages:            []SessionChatMessage{NewTextMessage("user", params.Prompt)},
		OnSessionID:         params.OnSessionID,
	}
	session, hadStreamErr, err := client.StartChatWithStatus(ctx, startReq)
	if err != nil {
		return "", false, fmt.Errorf("open fresh helix session: %w", err)
	}

	// Step 4 — cold-start fallback. With Helix's per-session readiness
	// check (waitForExternalAgentReady now polls the agent's own WS
	// rather than the global connection list), the first dispatch
	// should land cleanly. If hadStreamErr is still set we re-issue
	// once on the same session via SessionID continuation. Belt-and-
	// braces — the original race that made this critical has been
	// fixed at the source, so this should be rare.
	if hadStreamErr {
		retryReq := StartChatRequest{
			SessionID:           session.ID,
			ProjectID:           params.ProjectID,
			AppID:               params.AppID,
			SessionRole:         exploratoryRole,
			AgentType:           params.AgentType,
			Type:                "text",
			ExternalAgentConfig: &ExternalAgentConfig{},
			Messages:            []SessionChatMessage{NewTextMessage("user", params.Prompt)},
		}
		_, _, _ = client.StartChatWithStatus(ctx, retryReq)
	}
	return session.ID, true, nil
}

// SessionPreamble exposes the late-joiner catch-up snapshot the
// HTTP-WS handler in api/pkg/server emits before any patches arrive.
// In-process subscribers (Spawner bridge, chat bridge after H1.3d)
// call Snapshot before subscribing so they see a baseline frame
// equivalent to what the browser WS would receive.
//
// Empty snapshot ([]byte{}) is a valid "no streaming in progress"
// response — no preamble frame is emitted.
type SessionPreamble interface {
	Snapshot(ctx context.Context, sessionID string) ([]byte, error)
}

// NoopSessionPreamble is a SessionPreamble that returns no
// snapshot for any session. Useful in tests and when no streaming
// context tracking is available.
type NoopSessionPreamble struct{}

// Snapshot returns no preamble frame.
func (NoopSessionPreamble) Snapshot(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// SubscribeSessionUpdates subscribes to the pubsub topic that mirrors
// the per-session WebSocket Helix publishes to. Returns a channel of
// decoded SessionUpdate frames. Mirrors the order the browser WS
// handler uses (websocket_server_user.go:124-156):
//
//  1. Subscribe FIRST so no frames are missed.
//  2. Request the late-joiner snapshot from the host (if any).
//  3. Synthesise an initial SessionUpdate from the snapshot bytes and
//     emit it on the channel before the live stream starts arriving.
//
// The buffer size matches the typical burst (per-token-emit). Raise
// it if logs show drops.
//
// Topic format: pubsub.GetSessionQueue(ownerID, sessionID) — the
// same topic websocket_server_user.go subscribes the browser WS to.
func SubscribeSessionUpdates(ctx context.Context, ps pubsub.PubSub, snapshotter SessionPreamble, ownerID, sessionID string) (<-chan SessionUpdate, error) {
	out := make(chan SessionUpdate, 64)
	topic := pubsub.GetSessionQueue(ownerID, sessionID)
	sub, err := ps.Subscribe(ctx, topic, func(payload []byte) error {
		var u SessionUpdate
		if err := json.Unmarshal(payload, &u); err != nil {
			// Best-effort: drop malformed frames; the next frame is
			// likely well-formed.
			return nil
		}
		select {
		case out <- u:
		case <-ctx.Done():
		}
		return nil
	})
	if err != nil {
		close(out)
		return nil, err
	}
	// Snapshot AFTER subscribe so we never miss a frame published
	// between snapshot and subscribe. Send the snapshot frame
	// (if any) first so the consumer sees a baseline before deltas.
	if snapshotter != nil {
		if snap, err := snapshotter.Snapshot(ctx, sessionID); err == nil && len(snap) > 0 {
			var u SessionUpdate
			if jsonErr := json.Unmarshal(snap, &u); jsonErr == nil {
				select {
				case out <- u:
				case <-ctx.Done():
				}
			}
		}
	}
	go func() {
		<-ctx.Done()
		_ = sub.Unsubscribe()
		close(out)
	}()
	return out, nil
}
