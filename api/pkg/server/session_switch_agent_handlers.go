package server

// In-place agent switching. Unlike fork-and-pause (session_fork_handlers.go),
// switching keeps the SAME session and the SAME desktop container — only the
// agentic framework changes. The settings-sync-daemon rewrites Zed's config to
// the new agent (its agent_servers + its MCP context_servers); Zed hot-reloads
// that config live (its SettingsStore observers reconcile agent_servers and MCP
// context_servers without a process restart). The daemon then calls back
// /agent-config-applied and Helix delivers a fresh thread over the live Zed
// WebSocket, repopulated with the prior thread's transcript. A clean Zed
// restart is used only as a FALLBACK if the live hot-reload doesn't produce a
// new thread in time (see agentSwitchRestartFallback).
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
// @Description Switches the agentic framework on the SAME session without forking or restarting the container. Rewrites Zed's config to the new agent, which Zed hot-reloads live (MCP context servers reconcile without a process restart), then repopulates a fresh thread with the prior transcript. Falls back to a clean Zed restart only if the live reload doesn't take.
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
	sameRuntime := sessionUsesAgentRuntime(session, targetRuntime)
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

func sessionUsesAgentRuntime(session *types.Session, runtime types.CodeAgentRuntime) bool {
	if session.Metadata.CodeAgentRuntime != runtime {
		return false
	}
	return session.Metadata.ZedAgentName == runtime.ZedAgentName() ||
		(runtime == types.CodeAgentRuntimeZedAgent && session.Metadata.ZedAgentName == "")
}

// switchAgentInPlace performs the in-place switch: snapshot the current
// transcript, repoint the session's agent fields (and the spec task's
// HelixAppID — see repointSpecTaskForSwitch), clear the Zed thread binding,
// seed a fork_seed + Waiting handoff interaction, and publish a config_changed
// event. On the fast path the daemon hot-reloads the new config and calls
// /agent-config-applied, which delivers the handoff over the live Zed
// WebSocket (new ZedThreadID is empty → new thread; maybePrependTranscript
// injects the transcript). agentSwitchRestartFallback forces a clean Zed
// restart only if no new thread appears in time. No new session, no new
// container.
func (apiServer *HelixAPIServer) switchAgentInPlace(
	ctx context.Context,
	session *types.Session,
	targetRuntime types.CodeAgentRuntime,
	targetAppID string,
) *system.HTTPError {
	return apiServer.switchAgentInPlaceForNextTurn(ctx, session, targetRuntime, targetAppID, true)
}

// reconcileSessionAgentWithApp repairs sessions whose persisted ACP binding
// no longer matches their app. This can happen when an app runtime is edited
// while a durable org-bot or spec-task session is offline. Reconciliation runs
// before the next user turn, so that turn itself becomes the first message on
// the replacement thread; no synthetic handoff turn is needed.
func (apiServer *HelixAPIServer) reconcileSessionAgentWithApp(ctx context.Context, session *types.Session) *system.HTTPError {
	if session == nil || session.Metadata.AgentType != string(types.AgentTypeZedExternal) || session.ParentApp == "" {
		return nil
	}

	app, err := apiServer.Store.GetApp(ctx, session.ParentApp)
	if err != nil {
		return system.NewHTTPError500(fmt.Sprintf("failed to load session app for agent reconciliation: %v", err))
	}
	assistant := data.GetAssistant(app, session.Metadata.AssistantID)
	if assistant == nil || assistant.AgentType != types.AgentTypeZedExternal {
		return nil
	}
	targetRuntime := assistant.CodeAgentRuntime
	if targetRuntime == "" {
		targetRuntime = types.CodeAgentRuntimeZedAgent
	}
	if sessionUsesAgentRuntime(session, targetRuntime) {
		return nil
	}

	log.Warn().
		Str("session_id", session.ID).
		Str("app_id", session.ParentApp).
		Str("stored_runtime", string(session.Metadata.CodeAgentRuntime)).
		Str("stored_agent_name", session.Metadata.ZedAgentName).
		Str("target_runtime", string(targetRuntime)).
		Str("target_agent_name", targetRuntime.ZedAgentName()).
		Msg("reconciling stale session agent binding before next turn")

	return apiServer.switchAgentInPlaceForNextTurn(ctx, session, targetRuntime, session.ParentApp, false)
}

func (apiServer *HelixAPIServer) switchAgentInPlaceForNextTurn(
	ctx context.Context,
	session *types.Session,
	targetRuntime types.CodeAgentRuntime,
	targetAppID string,
	createHandoff bool,
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

	// Repoint the spec task's HelixAppID to the target app. CRITICAL for
	// spec-task sessions: getZedConfig resolves code_agent_config (which drives
	// the claude_code model via managed-settings.json) from specTask.HelixAppID
	// FIRST, then session.ParentApp. Updating only ParentApp above leaves the
	// claude_code agent on the OLD model — the actual inference never switches
	// even though Zed's native default_model does. (The fork path handles this
	// via repointSpecTasksToChild; the in-place switch must too.) Done BEFORE
	// publishAgentConfigChange so the daemon's re-fetch sees the new app.
	apiServer.repointSpecTaskForSwitch(ctx, session, childAppID)

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

	// Auto-fire a Waiting handoff turn so the new agent warms up with the prior
	// context. Delivered either live (daemon → /agent-config-applied →
	// pickupWaitingInteraction over the running Zed WS) or, on the restart
	// fallback, by pickupWaitingInteraction on reconnect.
	//
	// Keep the handoff SHORT: the prior transcript is prepended for context
	// (the model still ingests it), but we explicitly tell the agent NOT to
	// summarise or re-read it — just emit a one-line ready ack. A long
	// "review the whole transcript and acknowledge" instruction made the model
	// generate a big summary before the user could continue, which is the bulk
	// of the perceived switch latency.
	if createHandoff {
		prevLabel := apiServer.agentDescriptor(ctx, prevAppID, prevRuntime, session.ModelName, "the previous agent")
		newLabel := apiServer.agentDescriptor(ctx, childAppID, targetRuntime, session.ModelName, "the new agent")
		handoffPrompt := fmt.Sprintf(
			"[System: you are now %s, taking over this session from %s. The environment, "+
				"files, and workspace are unchanged, and the prior conversation is included above "+
				"for context. Do not summarise or restate it — just reply with a single short line "+
				"confirming you're ready, then wait for the user's next message.]",
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
	}

	// Tell the daemon to rewrite Zed's config for the new agent. field="agent"
	// is the FAST path: the daemon hot-reloads settings (no Zed restart) and
	// calls back /agent-config-applied so we deliver the new thread over the
	// live WebSocket. The in-flight turn, if any, is abandoned with the old
	// thread (a new thread is created for the new agent).
	apiServer.publishAgentConfigChange(ctx, session, "agent")

	// Restart fallback: if the live path doesn't produce a new Zed thread
	// within the timeout (e.g. the daemon callback was lost, or a brand-new
	// custom agent_server didn't register from the hot-reload), force a clean
	// Zed restart so the reconnect path delivers the handoff. Keyed on
	// ZedThreadID: a successful switch always ends with a fresh thread id.
	switchedAt := now
	go apiServer.agentSwitchRestartFallback(context.Background(), session.ID, switchedAt)

	log.Info().
		Str("session_id", session.ID).
		Str("prev_runtime", string(prevRuntime)).
		Str("target_runtime", string(targetRuntime)).
		Str("prev_app", prevAppID).
		Str("target_app", childAppID).
		Int("seed_completed_count", completedCount).
		Int("seed_transcript_len", len(transcript)).
		Msg("switch-agent: repointed session to new agent in place (live hot-reload path)")

	return nil
}

// switchAgentLiveDeliveryTimeout bounds how long we wait for the fast
// hot-reload + live-delivery path to produce a new Zed thread before falling
// back to a full Zed restart. Thread creation (thread_created → ZedThreadID)
// happens well before the model finishes reading the transcript, so this only
// needs to cover settings hot-reload + thread spin-up, not the LLM response.
const switchAgentLiveDeliveryTimeout = 9 * time.Second

// agentSwitchRestartFallback waits for the live hot-reload path to create a new
// Zed thread; if it hasn't within the timeout, it requests a clean Zed restart
// (field="agent_restart") so the reconnect path delivers the pending handoff.
// No-ops if the session already got a new thread, was switched again, or paused.
func (apiServer *HelixAPIServer) agentSwitchRestartFallback(ctx context.Context, sessionID string, switchedAt time.Time) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(switchAgentLiveDeliveryTimeout):
	}

	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Warn().Err(err).Str("session_id", sessionID).Msg("switch-agent fallback: failed to reload session")
		return
	}
	// Live path succeeded — a new thread was created.
	if session.Metadata.ZedThreadID != "" {
		return
	}
	// A newer switch superseded this one — let its own fallback handle it.
	if !session.Metadata.AgentSwitchedAt.Equal(switchedAt) {
		return
	}
	// Session paused/forked away in the meantime — don't touch it.
	if session.Metadata.Paused {
		return
	}
	log.Info().
		Str("session_id", sessionID).
		Dur("after", switchAgentLiveDeliveryTimeout).
		Msg("switch-agent fallback: no new thread from live hot-reload, requesting Zed restart")
	apiServer.publishAgentConfigChange(ctx, session, "agent_restart")
}

// repointSpecTaskForSwitch updates any spec task tracking THIS session so its
// HelixAppID points at the switched-to app. Without this, getZedConfig keeps
// resolving the claude_code model from the OLD app (managed-settings.json stays
// stale) and the underlying model never actually changes on a switch. Unlike
// the fork's repointSpecTasksToChild, the session is unchanged, so we only
// rewrite HelixAppID — PlanningSessionID stays the same. Best-effort: failures
// are logged, not fatal (the Zed-native model still switches; only the
// claude_code/managed-settings model would stay stale).
func (apiServer *HelixAPIServer) repointSpecTaskForSwitch(ctx context.Context, session *types.Session, targetAppID string) {
	if targetAppID == "" {
		return
	}
	update := func(task *types.SpecTask) {
		if task == nil || task.HelixAppID == targetAppID {
			return
		}
		oldApp := task.HelixAppID
		task.HelixAppID = targetAppID
		if err := apiServer.Store.UpdateSpecTask(ctx, task); err != nil {
			log.Warn().Err(err).
				Str("spec_task_id", task.ID).
				Str("session_id", session.ID).
				Msg("switch-agent: failed to repoint spec task HelixAppID; claude_code model may stay stale")
			return
		}
		log.Info().
			Str("spec_task_id", task.ID).
			Str("old_app", oldApp).
			Str("new_app", targetAppID).
			Msg("switch-agent: repointed spec task to new agent app")
	}

	// getZedConfig reads session.Metadata.SpecTaskID first — update that exact
	// task so the next zed-config fetch returns the new app's model.
	if session.Metadata.SpecTaskID != "" {
		if task, err := apiServer.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID); err == nil {
			update(task)
		}
	}

	// Also cover any spec task that tracks this session via PlanningSessionID
	// (defensive: keeps the spec-task page / other lookups consistent).
	tasks, err := apiServer.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{PlanningSessionID: session.ID})
	if err != nil {
		return
	}
	for _, task := range tasks {
		if task != nil && task.ID != session.Metadata.SpecTaskID {
			update(task)
		}
	}
}

// publishAgentConfigChange notifies the session's in-desktop settings-sync
// daemon about an in-place agent change. field="agent" triggers the fast
// hot-reload + live-delivery path; field="agent_restart" triggers the clean
// Zed restart fallback.
func (apiServer *HelixAPIServer) publishAgentConfigChange(ctx context.Context, session *types.Session, field string) {
	payload, err := json.Marshal(map[string]string{
		"type":  "config_changed",
		"field": field,
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
	log.Info().Str("session_id", session.ID).Str("field", field).Str("topic", topic).Msg("switch-agent: published config_changed event")
}

// AgentConfigAppliedResponse is the trivial ack for the daemon callback.
type AgentConfigAppliedResponse struct {
	Status string `json:"status"`
}

// agentConfigApplied godoc
// @Summary Notify that an in-place agent switch's config has been applied in the container
// @Description Called by the in-desktop settings-sync daemon after it hot-reloads Zed's config for an agent switch. Delivers the pending handoff to the live Zed thread without waiting for a process restart. Internal coordination endpoint.
// @Tags    sessions
// @Produce json
// @Param   id path string true "Session ID"
// @Success 200 {object} AgentConfigAppliedResponse
// @Router  /api/v1/sessions/{id}/agent-config-applied [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) agentConfigApplied(_ http.ResponseWriter, req *http.Request) (*AgentConfigAppliedResponse, *system.HTTPError) {
	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		return nil, system.NewHTTPError400("missing session id")
	}
	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("unauthenticated")
	}
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", sessionID))
	}
	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}
	// Deliver the pending Waiting handoff to the live Zed connection. This is
	// the same call the reconnect path makes; here we invoke it on-demand after
	// the daemon hot-reloaded the new agent's config, so no restart is needed.
	// If there's no live connection, queueOrSend holds it and the restart
	// fallback will eventually fire.
	apiServer.pickupWaitingInteraction(ctx, session.ID, session, "")
	return &AgentConfigAppliedResponse{Status: "ok"}, nil
}
