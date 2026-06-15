package server

// In-place agent switching. Unlike fork-and-pause (session_fork_handlers.go),
// switching keeps the SAME session and the SAME desktop container — only the
// agentic framework changes. The settings-sync-daemon rewrites Zed's config to
// the new agent (its agent_servers + its MCP context_servers) and restarts Zed
// so the new MCP surface comes up cleanly; a fresh Zed thread is then created
// for the new agent and repopulated with the prior thread's transcript.
//
// Designed in helix-specs:002111_so-we-recently-added-a (Strategy B —
// current-agent-only config, Helix dropdown is the sole switch path).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SwitchAgentRequest is the body of POST /api/v1/sessions/{id}/switch-agent.
// The frontend dropdown sends HelixAppID (the app the user picked);
// CodeAgentRuntime is a power-user / scripted-caller shortcut.
type SwitchAgentRequest struct {
	HelixAppID       string                 `json:"helix_app_id,omitempty"`
	CodeAgentRuntime types.CodeAgentRuntime `json:"code_agent_runtime,omitempty"`
}

// SwitchAgentResponse echoes the (unchanged) session id and the new agent so
// the frontend can confirm the switch without a refetch.
type SwitchAgentResponse struct {
	SessionID    string                 `json:"session_id"`
	HelixAppID   string                 `json:"helix_app_id"`
	AgentRuntime types.CodeAgentRuntime `json:"agent_runtime"`
}

// switchAgent godoc
// @Summary Switch the agent framework on a running session in place
// @Description Switches the agentic framework on the SAME session without forking or restarting the container. Rewrites Zed's config to the new agent, restarts Zed to load the new MCP surface, and repopulates a fresh thread with the prior transcript.
// @Tags    sessions
// @Accept  json
// @Produce json
// @Param   id path string true "Session ID to switch the agent on"
// @Param   request body SwitchAgentRequest true "Target agent selection"
// @Success 200 {object} SwitchAgentResponse
// @Router  /api/v1/sessions/{id}/switch-agent [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) switchAgent(_ http.ResponseWriter, req *http.Request) (*SwitchAgentResponse, *system.HTTPError) {
	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		return nil, system.NewHTTPError400("cannot switch agent without session id")
	}

	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("unauthenticated")
	}

	var body SwitchAgentRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		if !errors.Is(err, io.EOF) {
			return nil, system.NewHTTPError400(fmt.Sprintf("invalid request body: %v", err))
		}
	}

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", sessionID))
	}

	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if session.Metadata.AgentType != "zed_external" {
		return nil, system.NewHTTPError400(
			fmt.Sprintf("session is not an external agent session (agent_type=%q)", session.Metadata.AgentType))
	}
	// Switching only makes sense on a live session. A paused session is a
	// frozen checkpoint — switch on its active descendant instead.
	if session.Metadata.Paused {
		return nil, system.NewHTTPError409(
			fmt.Sprintf("session is paused (reason: %s); switch on its active descendant instead", session.Metadata.PausedReason))
	}

	// Reuse the fork target resolver — same (runtime, app) resolution logic.
	targetRuntime, targetAppID, err := apiServer.resolveForkTarget(ctx, session, ForkSessionRequest{
		HelixAppID:       body.HelixAppID,
		CodeAgentRuntime: body.CodeAgentRuntime,
	})
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// No-op guard: reject only when NOTHING about how the agent runs would
	// change (same app AND same runtime). Two apps sharing a runtime can still
	// differ in model / credentials / system prompt, so an app change alone
	// justifies the switch.
	sameApp := targetAppID == "" || targetAppID == session.ParentApp
	sameRuntime := targetRuntime == session.Metadata.CodeAgentRuntime
	if sameApp && sameRuntime {
		return nil, system.NewHTTPError400(
			fmt.Sprintf("session is already using %s in this app; pick a different agent or runtime", targetRuntime))
	}

	if switchErr := apiServer.switchAgentInPlace(ctx, session, targetRuntime, targetAppID); switchErr != nil {
		return nil, switchErr
	}

	return &SwitchAgentResponse{
		SessionID:    session.ID,
		HelixAppID:   targetAppID,
		AgentRuntime: targetRuntime,
	}, nil
}

// switchAgentInPlace performs the in-place switch: snapshot the current
// transcript, repoint the session's agent fields, clear the Zed thread binding,
// seed a fork_seed + Waiting handoff interaction, and publish a config_changed
// event so the daemon rewrites config and restarts Zed. When Zed reconnects
// after the restart, pickupWaitingInteraction delivers the handoff to the NEW
// agent (new ZedThreadID is empty → new thread; maybePrependTranscript injects
// the transcript). No new session, no new container.
func (apiServer *HelixAPIServer) switchAgentInPlace(
	ctx context.Context,
	session *types.Session,
	targetRuntime types.CodeAgentRuntime,
	targetAppID string,
) *system.HTTPError {
	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      10_000,
	})
	if err != nil {
		return system.NewHTTPError500(fmt.Sprintf("failed to load interactions: %v", err))
	}

	transcript := serializeTranscript(interactions, maxTranscriptBytes)
	completedCount := 0
	for _, in := range interactions {
		if in == nil || in.Trigger == types.InteractionTriggerForkSeed {
			continue
		}
		if in.State == types.InteractionStateComplete {
			completedCount++
		}
	}

	now := time.Now()
	prevRuntime := session.Metadata.CodeAgentRuntime
	prevAppID := session.ParentApp
	childAppID := targetAppID
	if childAppID == "" {
		childAppID = prevAppID
	}

	// Repoint the session's agent in place. Clearing ZedThreadID makes the
	// next outgoing message create a NEW Zed thread bound to the new agent.
	// AgentSwitchedAt lets maybePrependTranscript seed that new thread.
	session.ParentApp = childAppID
	session.Metadata.CodeAgentRuntime = targetRuntime
	session.Metadata.ZedAgentName = targetRuntime.ZedAgentName()
	session.Metadata.ZedThreadID = ""
	session.Metadata.AgentSwitchedAt = now
	session.Updated = now
	session.Metadata.HelixVersion = data.GetHelixVersion()
	if _, err := apiServer.Store.UpdateSession(ctx, *session); err != nil {
		return system.NewHTTPError500(fmt.Sprintf("failed to update session for agent switch: %v", err))
	}

	// fork_seed interaction carries the prior transcript for
	// maybePrependTranscript to inject into the new thread's first message.
	seedInteraction := &types.Interaction{
		Created:         now,
		Updated:         now,
		SessionID:       session.ID,
		UserID:          session.Owner,
		GenerationID:    session.GenerationID,
		Mode:            types.SessionModeInference,
		Trigger:         types.InteractionTriggerForkSeed,
		State:           types.InteractionStateComplete,
		PromptMessage:   fmt.Sprintf("Agent switched to %s at turn %d", targetRuntime, completedCount),
		ResponseMessage: transcript,
	}
	if _, err := apiServer.Store.CreateInteraction(ctx, seedInteraction); err != nil {
		return system.NewHTTPError500(fmt.Sprintf("failed to create fork_seed interaction: %v", err))
	}

	// Auto-fire a Waiting handoff turn so the new agent loads the prior
	// context as soon as Zed reconnects after the restart — delivered by
	// pickupWaitingInteraction on agent reconnect (not to the old, still
	// connected agent, which is about to be killed by the restart).
	prevLabel := apiServer.agentDescriptor(ctx, prevAppID, prevRuntime, session.ModelName, "the previous agent")
	newLabel := apiServer.agentDescriptor(ctx, childAppID, targetRuntime, session.ModelName, "the new agent")
	handoffPrompt := fmt.Sprintf(
		"[System handoff: the agent for this session has just been switched. The "+
			"environment, files, and workspace are unchanged.\n\n"+
			"You are now: %s\nPrevious agent: %s\n\n"+
			"The full prior conversation transcript is included above. Please briefly acknowledge "+
			"that you've reviewed it and are ready to continue — keep the acknowledgment to one or "+
			"two sentences so the user can pick up with their next message.]",
		newLabel, prevLabel,
	)
	handoffInteraction := &types.Interaction{
		Created:       now,
		Updated:       now,
		SessionID:     session.ID,
		UserID:        session.Owner,
		GenerationID:  session.GenerationID,
		Mode:          types.SessionModeInference,
		Trigger:       types.InteractionTriggerForkHandoff,
		State:         types.InteractionStateWaiting,
		PromptMessage: handoffPrompt,
	}
	if _, err := apiServer.Store.CreateInteraction(ctx, handoffInteraction); err != nil {
		// Best-effort: a failed handoff just degrades to "cold until the
		// user's first message after the switch". Don't fail the switch.
		log.Warn().Err(err).
			Str("session_id", session.ID).
			Msg("switch-agent: failed to create handoff interaction; agent will warm up on user's first message instead")
	}

	// Tell the daemon to rewrite Zed's config for the new agent and restart
	// Zed (so the new MCP surface loads). The in-flight turn, if any, is torn
	// down by the restart — no explicit cancel needed.
	apiServer.publishAgentConfigChange(ctx, session)

	log.Info().
		Str("session_id", session.ID).
		Str("prev_runtime", string(prevRuntime)).
		Str("target_runtime", string(targetRuntime)).
		Str("prev_app", prevAppID).
		Str("target_app", childAppID).
		Int("seed_completed_count", completedCount).
		Int("seed_transcript_len", len(transcript)).
		Msg("switch-agent: repointed session to new agent in place, restart requested")

	return nil
}

// publishAgentConfigChange notifies the session's in-desktop settings-sync
// daemon that the agent changed, so it re-syncs Zed config and restarts Zed.
// field="agent" distinguishes this from the theme color_scheme change so the
// daemon knows a full Zed restart (not just a settings rewrite) is required.
func (apiServer *HelixAPIServer) publishAgentConfigChange(ctx context.Context, session *types.Session) {
	payload, err := json.Marshal(map[string]string{
		"type":  "config_changed",
		"field": "agent",
	})
	if err != nil {
		log.Warn().Err(err).Msg("switch-agent: failed to marshal agent config event")
		return
	}
	topic := pubsub.GetSessionQueue(session.Owner, session.ID)
	if err := apiServer.pubsub.Publish(ctx, topic, payload); err != nil {
		log.Warn().Err(err).Str("topic", topic).Msg("switch-agent: failed to publish agent config event")
		return
	}
	log.Info().Str("session_id", session.ID).Str("topic", topic).Msg("switch-agent: published agent config_changed event")
}
