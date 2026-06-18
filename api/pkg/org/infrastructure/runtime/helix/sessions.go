package helix

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/helixml/helix/api/pkg/pubsub"
)

// SessionClient is the slice of the session API EnsureAndSend depends
// on, backed by the same primitives the cron trigger / spec tasks use:
//   - StartSession → StartExternalAgentSession (create session + start
//     desktop + queue first message).
//   - SendMessage → POST /sessions/{id}/messages (fire-and-forget
//     continuation; Helix auto-resumes a downed desktop on the same
//     session).
//
// Neither blocks on the turn, so neither hits the external-agent response
// timeout; the Spawner observes completion via pollUntilDone + the mirror.
type SessionClient interface {
	StartSession(ctx context.Context, params StartSessionParams) (sessionID string, err error)
	SendMessage(ctx context.Context, sessionID, prompt string) error
	ServerStatus(ctx context.Context) (ServerStatus, error)
}

// StartSessionParams configures one-time creation of a worker's session.
// The adapter always sets session_role "exploratory" so it's resolvable
// by the mirror's GetProjectExploratorySession lookup.
type StartSessionParams struct {
	ProjectID      string
	OrganizationID string
	AppID          string
	AgentType      string
	Provider       string
	Model          string
	Prompt         string
}

// SpawnerClient is the chat-session surface the helix Spawner uses
// during an activation. Superset of SessionClient:
//
//   - GetOutput: polled by the Spawner's pollUntilDone loop until the
//     session reports a terminal status.
//   - StopExternalAgent: used by the chat bridge's NewHandler.
//   - SessionOwner: resolves the owning user ID so the transcript
//     bridge can subscribe to the correct per-session pubsub topic.
//     Helix publishes session updates to GetSessionQueue(owner, id), so
//     the bridge must subscribe with the owner — not an empty string.
//
// Production impl is the in-process inProcHelixClient adapter.
type SpawnerClient interface {
	SessionClient
	GetOutput(ctx context.Context, sessionID string) (types.SessionOutputResponse, error)
	StopExternalAgent(ctx context.Context, sessionID string) error
	SessionOwner(ctx context.Context, sessionID string) (string, error)
	// ClearSession wipes the session's prior conversation (the DB
	// interactions, and for a Zed/ACP session the Zed thread too) while
	// keeping the session row. The Spawner calls this before every
	// re-activation so each worker turn starts on a fresh context window
	// instead of growing one long-lived session until it hits the model
	// limit and compacts. See SpawnerConfig.ensureSession.
	ClearSession(ctx context.Context, sessionID string) error
}

// checkDesktopQuota pre-flights the desktop quota gate before
// opening a new session.
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

// EnsureAndSend makes a worker's session run a prompt. A worker has one
// durable session, so there's no staleness to detect:
//   - existing session → SendMessage (fire-and-forget; Helix recovers a
//     downed desktop on the same session). fresh=false.
//   - no session → quota pre-flight, then StartSession. fresh=true so the
//     caller persists the new id.
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

	if params.SessionID != "" {
		if err := client.SendMessage(ctx, params.SessionID, params.Prompt); err != nil {
			return "", false, fmt.Errorf("send message to session %s: %w", params.SessionID, err)
		}
		if params.OnSessionID != nil {
			params.OnSessionID(params.SessionID)
		}
		return params.SessionID, false, nil
	}

	if err := checkDesktopQuota(ctx, client); err != nil {
		return "", false, err
	}
	sid, err := client.StartSession(ctx, StartSessionParams{
		ProjectID:      params.ProjectID,
		OrganizationID: params.OrganizationID,
		AppID:          params.AppID,
		AgentType:      params.AgentType,
		Provider:       params.Provider,
		Model:          params.Model,
		Prompt:         params.Prompt,
	})
	if err != nil {
		return "", false, fmt.Errorf("start helix session: %w", err)
	}
	if params.OnSessionID != nil {
		params.OnSessionID(sid)
	}
	return sid, true, nil
}

// SessionPreamble exposes the late-joiner catch-up snapshot the
// HTTP-WS handler emits before any patches arrive. In-process
// subscribers call Snapshot before subscribing so they see a baseline
// frame equivalent to the browser WS. Empty snapshot means no
// streaming in progress; no preamble is emitted.
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
// decoded types.WebsocketEvent frames. Mirrors the order the browser WS
// handler uses (websocket_server_user.go:124-156):
//
//  1. Subscribe FIRST so no frames are missed.
//  2. Request the late-joiner snapshot from the host (if any).
//  3. Synthesise an initial types.WebsocketEvent from the snapshot bytes and
//     emit it on the channel before the live stream starts arriving.
//
// The buffer size matches the typical burst (per-token-emit). Raise
// it if logs show drops.
//
// Topic format: pubsub.GetSessionQueue(ownerID, sessionID) — the
// same topic websocket_server_user.go subscribes the browser WS to.
func SubscribeSessionUpdates(ctx context.Context, ps pubsub.PubSub, snapshotter SessionPreamble, ownerID, sessionID string) (<-chan types.WebsocketEvent, error) {
	// Defensive guard against a mis-wired SpawnerConfig handing us a
	// nil PubSub. The bridge runs in its own goroutine, so a segfault
	// here used to take the whole API process down on every AI-worker
	// activation; a regular error lets the caller's reconnect loop log
	// and back off instead.
	if ps == nil {
		return nil, fmt.Errorf("helix subscribe: pubsub is nil — SpawnerConfig.PubSub not wired")
	}
	out := make(chan types.WebsocketEvent, 64)
	topic := pubsub.GetSessionQueue(ownerID, sessionID)
	sub, err := ps.Subscribe(ctx, topic, func(payload []byte) error {
		var u types.WebsocketEvent
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
			var u types.WebsocketEvent
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
