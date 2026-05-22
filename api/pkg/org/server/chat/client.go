// Package chat — client.go defines the ChatBridgeClient port: the
// slice of helix's session+project+agent API the owner-chat bridge
// (helix_bridge.go) depends on. Extracted in H1-chat so the bridge
// no longer imports api/pkg/org/helixclient directly.
//
// Production satisfier is api/pkg/server.inProcHelixClient (built by
// buildInProcHelixClient in helix_org.go); tests can supply a fake
// without HTTP / WebSocket plumbing.
//
// This file MUST NOT import api/pkg/org/helixclient — that's the
// whole point of the H1-chat split. The chat bridge talks to Helix
// through this narrow port; the in-proc adapter or the legacy HTTP
// client both satisfy it.
package chat

import (
	"context"
	"errors"

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
)

// ChatBridgeClient is the minimum surface helix_bridge.go consumes
// from a Helix backend. Superset of runtimehelix.SpawnerClient
// (StartChatWithStatus + ServerStatus + GetOutput + StopExternalAgent)
// plus three bridge-specific calls:
//
//   - GetSession: used by History() to reconstruct the chat surface
//     after a page refresh.
//   - GetProject: used by resolveProjectOrg() to look up the org_id
//     of a per-Worker project (required on /sessions/chat so desktop
//     quota is checked against the project's org, not the user's
//     personal one).
//   - SubscribeUpdates: used by runWebsocket() to stream live frame
//     updates for the active chat session.
//
// GetProject returns types.Project (the canonical Helix type, not
// helixclient.Project). The bridge only reads OrganizationID off the
// returned value; the rest of the fields ride along free.
type ChatBridgeClient interface {
	runtimehelix.SpawnerClient
	GetSession(ctx context.Context, id string) (types.Session, error)
	GetProject(ctx context.Context, id string) (types.Project, error)
	SubscribeUpdates(ctx context.Context, sessionID string) (<-chan types.WebsocketEvent, error)
}

// SendToSession posts `prompt` to an existing Helix session via the
// /sessions/chat continuation path. Returns nil iff Helix accepted
// the message AND no error chunk appeared on the SSE stream. Either
// failure mode means the persisted session is dead on the server
// side (in-memory external-agent state evicted after restart, session
// row deleted, etc.) and the caller should treat the persisted
// SessionID as stale and open a fresh one.
//
// Local copy of helixclient.SendToSession, lifted into the chat
// package so helix_bridge.go can drop its helixclient import. The
// runtime/helix package has an unexported sibling (sendToSession)
// used by EnsureAndSend; that one is on the resume path of
// EnsureAndSend's own state machine and isn't exported.
func SendToSession(ctx context.Context, client ChatBridgeClient, req runtimehelix.StartChatRequest) (types.Session, error) {
	if req.SessionID == "" {
		return types.Session{}, errors.New("SendToSession: SessionID required")
	}
	session, streamHadErr, err := client.StartChatWithStatus(ctx, req)
	if err != nil {
		return types.Session{}, err
	}
	if streamHadErr {
		return types.Session{}, errors.New("session no longer running on the server")
	}
	return session, nil
}
