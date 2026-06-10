package server

// Fork-and-pause endpoint. Designed in helix-specs:002081_kickoff-mid-session
// and validated end-to-end against a live stack in
// helix-specs:002082_what-i-want-to-do-is (2026-06-09, 17/17 backend
// assertions + 7/9 UI walkthrough scenarios — see that task's design.md for
// the outcome report and screenshots).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// ForkSessionRequest is the body of POST /api/v1/sessions/{id}/fork.
// Exactly one of HelixAppID / CodeAgentRuntime must drive the target choice.
// In practice the frontend chat-panel dropdown sends HelixAppID (the app the
// user picked); CodeAgentRuntime is a power-user / scripted-caller shortcut.
type ForkSessionRequest struct {
	HelixAppID       string                 `json:"helix_app_id,omitempty"`
	CodeAgentRuntime types.CodeAgentRuntime `json:"code_agent_runtime,omitempty"`
}

// ForkSessionResponse identifies the child session created by the fork.
// The frontend uses NewSessionID to navigate the chat panel.
type ForkSessionResponse struct {
	NewSessionID string `json:"new_session_id"`
}

// forkSession godoc
// @Summary Fork a session to a different agent (fork-and-pause)
// @Description Creates a new session with the target agent, seeded with the parent's transcript, and pauses the parent. The parent remains as a frozen checkpoint.
// @Tags    sessions
// @Accept  json
// @Produce json
// @Param   id path string true "Source session ID to fork from"
// @Param   request body ForkSessionRequest true "Target runtime selection"
// @Success 200 {object} ForkSessionResponse
// @Router  /api/v1/sessions/{id}/fork [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) forkSession(_ http.ResponseWriter, req *http.Request) (*ForkSessionResponse, *system.HTTPError) {
	sourceID := mux.Vars(req)["id"]
	if sourceID == "" {
		return nil, system.NewHTTPError400("cannot fork session without id")
	}

	ctx := req.Context()
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("unauthenticated")
	}

	var body ForkSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		// Empty body is fine — the user may want to fork to the same app with
		// no runtime override (rejected below by the same-runtime check, which
		// is the right error message in that case).
		if !errors.Is(err, io.EOF) {
			return nil, system.NewHTTPError400(fmt.Sprintf("invalid request body: %v", err))
		}
	}

	parent, err := apiServer.Store.GetSession(ctx, sourceID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("source session %s not found", sourceID))
	}

	if err := apiServer.authorizeUserToSession(ctx, user, parent, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if parent.Metadata.AgentType != "zed_external" {
		return nil, system.NewHTTPError400(
			fmt.Sprintf("source session is not an external agent session (agent_type=%q)", parent.Metadata.AgentType))
	}
	if parent.Metadata.Paused {
		return nil, system.NewHTTPError409(
			fmt.Sprintf("source session is paused (reason: %s); fork from its active descendant instead", parent.Metadata.PausedReason))
	}

	targetRuntime, targetAppID, err := apiServer.resolveForkTarget(ctx, parent, body)
	if err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}
	// Reject the fork only when NOTHING about how the child runs would
	// change: same app (or no app override, falling back to the parent's)
	// AND same runtime. Two apps that share a runtime can still differ in
	// model / credentials / system prompt — e.g. claude_code on Opus vs
	// claude_code on Sonnet — so an app change alone is enough to justify
	// the fork.
	sameApp := targetAppID == "" || targetAppID == parent.ParentApp
	sameRuntime := targetRuntime == parent.Metadata.CodeAgentRuntime
	if sameApp && sameRuntime {
		return nil, system.NewHTTPError400(
			fmt.Sprintf("source session is already using %s in this app; pick a different agent or runtime", targetRuntime))
	}

	child, forkErr := apiServer.forkSessionFromParent(ctx, user, parent, targetRuntime, targetAppID)
	if forkErr != nil {
		return nil, forkErr
	}

	return &ForkSessionResponse{NewSessionID: child.ID}, nil
}

// resolveForkTarget chooses the (runtime, app) pair the child session will
// adopt. Precedence:
//   - explicit body.CodeAgentRuntime wins (power-user shortcut)
//   - body.HelixAppID is resolved to the app's zed_external assistant runtime
//   - if neither is supplied, fall back to the parent's app (will hit the
//     "already using <runtime>" 400 above — the user must change something)
//
// When HelixAppID is set we return it as targetAppID so the child can carry
// its own ParentApp linkage; otherwise we reuse the parent's ParentApp.
func (apiServer *HelixAPIServer) resolveForkTarget(
	ctx context.Context,
	parent *types.Session,
	body ForkSessionRequest,
) (types.CodeAgentRuntime, string, error) {
	if body.CodeAgentRuntime != "" {
		appID := body.HelixAppID
		if appID == "" {
			appID = parent.ParentApp
		}
		return body.CodeAgentRuntime, appID, nil
	}

	appID := body.HelixAppID
	if appID == "" {
		appID = parent.ParentApp
	}
	if appID == "" {
		return "", "", fmt.Errorf("target app cannot be resolved: no helix_app_id provided and parent has no ParentApp")
	}

	app, err := apiServer.Store.GetApp(ctx, appID)
	if err != nil {
		return "", "", fmt.Errorf("failed to look up target app %s: %w", appID, err)
	}
	for _, assistant := range app.Config.Helix.Assistants {
		if assistant.AgentType != types.AgentTypeZedExternal {
			continue
		}
		runtime := assistant.CodeAgentRuntime
		if runtime == "" {
			runtime = types.CodeAgentRuntimeZedAgent
		}
		return runtime, appID, nil
	}
	return "", "", fmt.Errorf("target app %s has no zed_external assistant", appID)
}

// forkSessionFromParent does the actual fork: snapshot parent's transcript,
// create the child session row + fork_seed interaction, mark parent paused,
// provision the desktop container for the child. Mutations happen in this
// order so a desktop-provisioning failure leaves the DB consistent (child
// exists with its seed, parent is paused) — re-fork is unnecessary; the
// child can just be started again via the normal resume path.
func (apiServer *HelixAPIServer) forkSessionFromParent(
	ctx context.Context,
	user *types.User,
	parent *types.Session,
	targetRuntime types.CodeAgentRuntime,
	targetAppID string,
) (*types.Session, *system.HTTPError) {
	parentInteractions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    parent.ID,
		GenerationID: parent.GenerationID,
		PerPage:      10_000,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to load parent interactions: %v", err))
	}

	transcript := serializeTranscript(parentInteractions, maxTranscriptBytes)
	completedCount := 0
	var lastCompletedID string
	for _, in := range parentInteractions {
		if in == nil || in.Trigger == types.InteractionTriggerForkSeed {
			continue
		}
		if in.State == types.InteractionStateComplete {
			completedCount++
			lastCompletedID = in.ID
		}
	}

	now := time.Now()
	parentAppID := parent.ParentApp
	childAppID := targetAppID
	if childAppID == "" {
		childAppID = parentAppID
	}

	child := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           parent.Name,
		Created:        now,
		Updated:        now,
		Mode:           parent.Mode,
		Type:           parent.Type,
		Provider:       parent.Provider,
		ModelName:      parent.ModelName,
		Owner:          parent.Owner,
		OwnerType:      parent.OwnerType,
		OrganizationID: parent.OrganizationID,
		ProjectID:      parent.ProjectID,
		ParentApp:      childAppID,
		Metadata: types.SessionMetadata{
			// Copy load-bearing parent metadata.
			Stream:              parent.Metadata.Stream,
			SystemPrompt:        parent.Metadata.SystemPrompt,
			AssistantID:         parent.Metadata.AssistantID,
			HelixVersion:        data.GetHelixVersion(),
			AgentType:           "zed_external",
			ExternalAgentConfig: parent.Metadata.ExternalAgentConfig,
			SpecTaskID:          parent.Metadata.SpecTaskID,
			ProjectID:           parent.Metadata.ProjectID,
			WorkSessionID:       parent.Metadata.WorkSessionID,
			SessionRole:         parent.Metadata.SessionRole,
			CallbackURL:         parent.Metadata.CallbackURL,
			// Target runtime/agent — overrides parent's.
			CodeAgentRuntime: targetRuntime,
			ZedAgentName:     targetRuntime.ZedAgentName(),
			// Fork lineage.
			ParentSessionID:       parent.ID,
			ForkedAt:              now,
			ForkedAtInteractionID: lastCompletedID,
		},
	}

	createdChild, err := apiServer.Store.CreateSession(ctx, *child)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create child session: %v", err))
	}

	// Copy the parent's interaction history into the child as actual
	// interaction rows, marked with Trigger=fork_inherited. This makes
	// the fork a self-contained snapshot: a fork-of-fork inherits the
	// full ancestry by copying the immediate parent's history (which
	// already contains everything the grandparent had). The chat panel
	// can therefore render a forked session by reading just its own
	// interactions — no chain-walking, no cross-session fetches.
	//
	// Skip the parent's own fork_seed and fork_handoff entries — both
	// are synthetic system markers tied to the *previous* fork point.
	// The child gets a fresh fork_seed/fork_handoff pair below pointing
	// at this fork. The parent's *inherited* rows (Trigger=fork_inherited)
	// and *handoff replies* (the agent's response to the previous
	// handoff) ARE copied so chain depth ≥ 2 keeps the full history.
	for _, in := range parentInteractions {
		if in == nil {
			continue
		}
		if in.Trigger == types.InteractionTriggerForkSeed ||
			in.Trigger == types.InteractionTriggerForkHandoff {
			continue
		}
		copyInteraction := *in
		copyInteraction.ID = ""             // let the store mint a fresh ID
		copyInteraction.SessionID = createdChild.ID
		copyInteraction.GenerationID = createdChild.GenerationID
		copyInteraction.Trigger = types.InteractionTriggerForkInherited
		if _, err := apiServer.Store.CreateInteraction(ctx, &copyInteraction); err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to copy parent interaction %s: %v", in.ID, err))
		}
	}

	// fork_seed interaction. Its ResponseMessage still carries the
	// parent's serialized transcript so maybePrependTranscript can
	// inject it into the child's first outgoing message — the agent
	// (Zed thread is fresh) needs this prepend to receive the context;
	// the inherited rows above are for UI display, not for Zed state.
	// The fork_seed also serves as the visual divider between inherited
	// history and the child's own future turns.
	seedInteraction := &types.Interaction{
		Created:         now,
		Updated:         now,
		SessionID:       createdChild.ID,
		UserID:          createdChild.Owner,
		GenerationID:    createdChild.GenerationID,
		Mode:            types.SessionModeInference,
		Trigger:         types.InteractionTriggerForkSeed,
		State:           types.InteractionStateComplete,
		PromptMessage:   fmt.Sprintf("Session forked from %s at turn %d", parent.ID, completedCount),
		ResponseMessage: transcript,
	}
	if _, err := apiServer.Store.CreateInteraction(ctx, seedInteraction); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to create fork_seed interaction: %v", err))
	}

	// Auto-fire a synthetic handoff turn so the new agent loads the
	// prior context *before* the user sends their first real message —
	// otherwise Zed's thread is empty on the child until the user
	// prompts (and the "all the prior text streams in" sensation only
	// happens because the LLM is reading the prepended transcript on
	// that first call). Creating it in Waiting state means the existing
	// pickupWaitingInteraction path delivers it as soon as the agent
	// websocket connects; maybePrependTranscript fires on it (same as
	// any other first-message-on-a-forked-session) and prepends the
	// transcript. The prompt is short and meaningful so the agent's
	// acknowledgment is a useful confirmation, not noise.
	prevAgent := strings.TrimSpace(string(parent.Metadata.CodeAgentRuntime))
	if prevAgent == "" {
		prevAgent = "the previous agent"
	}
	newAgent := strings.TrimSpace(string(targetRuntime))
	if newAgent == "" {
		newAgent = "the new agent"
	}
	handoffPrompt := fmt.Sprintf(
		"[System handoff: a new agent has just been switched in to continue this conversation. "+
			"You are %s; the previous agent was %s. The full prior conversation transcript is "+
			"included above. Please briefly acknowledge that you've reviewed it and are ready to "+
			"continue — keep the acknowledgment to one or two sentences so the user can pick up "+
			"with their next message.]",
		newAgent, prevAgent,
	)
	handoffInteraction := &types.Interaction{
		Created:       now,
		Updated:       now,
		SessionID:     createdChild.ID,
		UserID:        createdChild.Owner,
		GenerationID:  createdChild.GenerationID,
		Mode:          types.SessionModeInference,
		Trigger:       types.InteractionTriggerForkHandoff,
		State:         types.InteractionStateWaiting,
		PromptMessage: handoffPrompt,
	}
	if _, err := apiServer.Store.CreateInteraction(ctx, handoffInteraction); err != nil {
		// Best-effort: a failed handoff just degrades to the previous
		// "cold until first user message" behaviour. Don't fail the fork.
		log.Warn().Err(err).
			Str("child_session_id", createdChild.ID).
			Msg("fork: failed to create handoff interaction; child will warm up on user's first message instead")
	}

	// Mark parent paused. Done AFTER the child exists so a transient pause-update
	// failure doesn't leave us with a child whose parent is still live.
	parent.Metadata.Paused = true
	parent.Metadata.PausedReason = fmt.Sprintf("forked_to:%s", createdChild.ID)
	parent.Metadata.PausedAt = now
	if _, err := apiServer.Store.UpdateSession(ctx, *parent); err != nil {
		// Surface as 500 but don't roll back the child — the child is valid;
		// the user can retry the pause later or it can be done lazily.
		log.Error().Err(err).
			Str("parent_session_id", parent.ID).
			Str("child_session_id", createdChild.ID).
			Msg("fork: failed to mark parent paused after child creation")
		return nil, system.NewHTTPError500(fmt.Sprintf("child created (%s) but failed to pause parent: %v", createdChild.ID, err))
	}

	// Re-point any SpecTask that was tracking the parent session at the
	// child. Without this the spec-task page keeps loading the now-paused
	// parent and the user lands on the "this session is paused" banner
	// every time they revisit the task — the fork effectively orphans
	// itself. After the re-point, the spec task's chat panel mounts on
	// the active child; the parent is still reachable through the
	// ForkBadge on the child for users who want to inspect history.
	apiServer.repointSpecTasksToChild(ctx, parent.ID, createdChild)

	// Provision the desktop for the child. Same path as initial session start.
	if err := apiServer.provisionForkedSessionDesktop(ctx, user, createdChild); err != nil {
		// As above, leave the child row in place — its desktop can be started on
		// next access via the normal autoStartDevContainerForSession path. Log
		// loudly so the user sees the surface error.
		log.Error().Err(err).
			Str("child_session_id", createdChild.ID).
			Msg("fork: failed to provision child desktop; child will be started lazily")
	}

	log.Info().
		Str("parent_session_id", parent.ID).
		Str("child_session_id", createdChild.ID).
		Str("target_runtime", string(targetRuntime)).
		Int("seed_completed_count", completedCount).
		Int("seed_transcript_len", len(transcript)).
		Msg("fork: created child session, paused parent")

	return createdChild, nil
}

// repointSpecTasksToChild finds any SpecTask whose PlanningSessionID points
// at the just-paused parent and updates it to point at the freshly-forked
// child. Best-effort: failures are logged but do not abort the fork — the
// child + parent rows are already consistent and the user can manually
// navigate via the ForkBadge if the re-point silently fails.
func (apiServer *HelixAPIServer) repointSpecTasksToChild(
	ctx context.Context,
	parentSessionID string,
	child *types.Session,
) {
	tasks, err := apiServer.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		PlanningSessionID: parentSessionID,
	})
	if err != nil {
		log.Warn().Err(err).
			Str("parent_session_id", parentSessionID).
			Str("child_session_id", child.ID).
			Msg("fork: failed to look up spec tasks pointing at parent; chat panel may still show paused parent")
		return
	}
	if len(tasks) == 0 {
		return // standalone session — no spec task to re-point
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		oldSessionID := task.PlanningSessionID
		oldAppID := task.HelixAppID
		task.PlanningSessionID = child.ID
		if child.ParentApp != "" {
			task.HelixAppID = child.ParentApp
		}
		if err := apiServer.Store.UpdateSpecTask(ctx, task); err != nil {
			log.Warn().Err(err).
				Str("spec_task_id", task.ID).
				Str("parent_session_id", parentSessionID).
				Str("child_session_id", child.ID).
				Msg("fork: failed to re-point spec task to child; chat panel may still show paused parent")
			continue
		}
		log.Info().
			Str("spec_task_id", task.ID).
			Str("old_session_id", oldSessionID).
			Str("new_session_id", child.ID).
			Str("old_helix_app_id", oldAppID).
			Str("new_helix_app_id", task.HelixAppID).
			Msg("fork: re-pointed spec task to child session")
	}
}

// provisionForkedSessionDesktop mirrors the desktop-startup branch of
// startChatSessionHandler for forked sessions. Kept separate so the fork
// path is easy to read and so a desktop failure can degrade gracefully
// (the child row is already persisted; lazy startup will pick it up).
func (apiServer *HelixAPIServer) provisionForkedSessionDesktop(
	ctx context.Context,
	user *types.User,
	child *types.Session,
) error {
	if apiServer.externalAgentExecutor == nil {
		return fmt.Errorf("external agent executor unavailable")
	}

	zedAgent := &types.DesktopAgent{
		OrganizationID: child.OrganizationID,
		ProjectID:      child.ProjectID,
		SessionID:      child.ID,
		UserID:         user.ID,
		Input:          "Continue forked session",
		ProjectPath:    "workspace",
	}

	if child.ProjectID != "" {
		projectRepos, repoErr := apiServer.Controller.Options.Store.ListGitRepositories(ctx, &types.ListGitRepositoriesRequest{
			ProjectID: child.ProjectID,
		})
		if repoErr == nil && len(projectRepos) > 0 {
			zedAgent.RepositoryIDs = make([]string, 0, len(projectRepos))
			for _, repo := range projectRepos {
				if repo.ID != "" {
					zedAgent.RepositoryIDs = append(zedAgent.RepositoryIDs, repo.ID)
				}
			}
			if project, projErr := apiServer.Controller.Options.Store.GetProject(ctx, child.ProjectID); projErr == nil && project != nil {
				primaryRepoID := project.DefaultRepoID
				if primaryRepoID == "" {
					primaryRepoID = projectRepos[0].ID
				}
				zedAgent.PrimaryRepositoryID = primaryRepoID
			}
		}
	}

	if child.Metadata.ExternalAgentConfig != nil {
		cfg := child.Metadata.ExternalAgentConfig
		if cfg.DisplayWidth > 0 {
			zedAgent.DisplayWidth = cfg.DisplayWidth
		}
		if cfg.DisplayHeight > 0 {
			zedAgent.DisplayHeight = cfg.DisplayHeight
		}
		if cfg.DisplayRefreshRate > 0 {
			zedAgent.DisplayRefreshRate = cfg.DisplayRefreshRate
		}
		if cfg.Resolution != "" {
			zedAgent.Resolution = cfg.Resolution
		}
		if cfg.ZoomLevel > 0 {
			zedAgent.ZoomLevel = cfg.ZoomLevel
		}
		if cfg.DesktopType != "" {
			zedAgent.DesktopType = cfg.DesktopType
		}
	}

	sessionOwner := child.Owner
	zedAgent.OnBeforeCreate = func(hookCtx context.Context, a *types.DesktopAgent) error {
		return apiServer.addUserAPITokenToAgent(hookCtx, a, sessionOwner)
	}

	agentResp, err := apiServer.externalAgentExecutor.StartDesktop(ctx, zedAgent)
	if err != nil {
		return fmt.Errorf("StartDesktop: %w", err)
	}
	if agentResp == nil {
		return nil
	}
	if agentResp.DevContainerID != "" || agentResp.SandboxID != "" {
		freshChild, fetchErr := apiServer.Store.GetSession(ctx, child.ID)
		if fetchErr != nil {
			return fmt.Errorf("re-fetch after StartDesktop: %w", fetchErr)
		}
		freshChild.Metadata.DevContainerID = agentResp.DevContainerID
		freshChild.SandboxID = agentResp.SandboxID
		if _, err := apiServer.Store.UpdateSession(ctx, *freshChild); err != nil {
			return fmt.Errorf("persist container info: %w", err)
		}
	}
	return nil
}
