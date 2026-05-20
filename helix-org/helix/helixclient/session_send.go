package helixclient

import (
	"context"
	"fmt"
)

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

func EnsureAndSend(ctx context.Context, client Client, params SendPromptParams) (sessionID string, fresh bool, err error) {
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
		if _, sendErr := SendToSession(ctx, client, resumeReq); sendErr == nil {
			if params.OnSessionID != nil {
				params.OnSessionID(params.SessionID)
			}
			return params.SessionID, false, nil
		}
		// Resume failed — caller will see fresh=true and can take
		// the opportunity to log + persist the new ID.
	}

	// Step 2 — pre-flight quota.
	if err := CheckDesktopQuota(ctx, client); err != nil {
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

	// Step 4 — cold-start race recovery. Helix's /sessions/chat reports
	// "agent ready" on a heuristic (~500ms after the sandbox starts),
	// then immediately tries to dispatch the prompt to the agent's
	// WebSocket. The agent often isn't actually plumbed yet at that
	// point, so the dispatch fails with "no external agent WebSocket
	// connection" — Helix surfaces that on the SSE stream
	// (hadStreamErr=true) AND marks the interaction state=error.
	// Because the interaction is `error` (not `waiting`),
	// pickupWaitingInteraction won't redeliver it when the agent
	// finally connects: the prompt is *lost*.
	//
	// Re-issue the same prompt with SessionID set. The continuation
	// path persists a fresh `waiting` interaction on the same session;
	// pickupWaitingInteraction delivers it as soon as the agent's WS
	// dials in. This is the same mechanism the previous bespoke
	// owner-chat / spawner code paths each had — folded into one
	// helper so both callers get the recovery without duplication.
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
		// Best-effort: if the second send also fails we let the caller
		// proceed with the session ID anyway — the user can resend, or
		// the next event-driven activation will reuse this session and
		// hit a warm path. Hard failure here would surface as a worse
		// UX (nothing visible at all) than letting the session exist.
		_, _, _ = client.StartChatWithStatus(ctx, retryReq)
	}
	return session.ID, true, nil
}
