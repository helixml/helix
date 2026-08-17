package server

// Shared code-agent execution configuration. SpecTasks and general sessions
// own complete execution configs. An org-agent session still keeps its parent
// App for identity and behavior, while this config selects the coding runtime.
// Both surfaces share the same Zed/ACP restart lifecycle.

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// effectiveCodeAgentOverrides picks legacy task overrides before startup
// migration and session overrides for App-backed chat sessions.
func effectiveCodeAgentOverrides(session *types.Session, task *types.SpecTask) *types.CodeAgentOverrides {
	if task != nil {
		return task.CodeAgentOverrides
	}
	if session == nil {
		return nil
	}
	return session.Metadata.CodeAgentOverrides
}

// resolveExecutionConfig describes the coding identity produced by an Agent
// plus a set of overrides. snapshotSessionID, when set, is used to reconstruct
// the identity of surfaces whose Agent has since been deleted, from the
// session's interaction snapshots.
func (s *HelixAPIServer) resolveExecutionConfig(
	ctx context.Context,
	appID string,
	overrides *types.CodeAgentOverrides,
	snapshotSessionID string,
) (*types.AgentExecutionConfig, error) {
	config := &types.AgentExecutionConfig{AgentID: appID, CodeAgentOverrides: overrides}

	if appID != "" {
		app, err := s.Store.GetApp(ctx, appID)
		switch {
		case err == nil:
			config.AgentAvailable = true
			config.AgentName = app.Config.Helix.Name
			effectiveApp := external_agent.ApplyCodeAgentOverrides(app, overrides)
			if assistant := external_agent.FindZedExternalAssistant(effectiveApp); assistant != nil {
				config.Runtime = assistant.CodeAgentRuntime
				if config.Runtime == "" {
					config.Runtime = types.CodeAgentRuntimeZedAgent
				}
				config.CredentialType = assistant.CodeAgentCredentialType
				config.ProviderRef, config.Model = acpUsageProviderAndModel(assistant)
				if assistant.CodeAgentRuntime != types.CodeAgentRuntimeClaudeCode &&
					assistant.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI {
					if assistant.GenerationModelProvider != "" {
						config.ProviderRef = assistant.GenerationModelProvider
					}
					if assistant.GenerationModel != "" {
						config.Model = assistant.GenerationModel
					}
				}
				config.ReasoningEffort = assistant.ReasoningEffort
			}
		case errors.Is(err, store.ErrNotFound):
			// Continue with interaction/session snapshots below.
		default:
			return nil, fmt.Errorf("failed to load agent: %w", err)
		}
	}

	if !config.AgentAvailable && snapshotSessionID != "" {
		interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
			SessionID: snapshotSessionID,
			Page:      0,
			PerPage:   20,
			Order:     "created DESC",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to load interactions: %w", err)
		}
		for _, interaction := range interactions {
			if interaction.CodeAgentConfigSnapshot == nil {
				continue
			}
			snapshot := interaction.CodeAgentConfigSnapshot
			config.Runtime = snapshot.Runtime
			config.CredentialType = snapshot.CredentialType
			config.ProviderRef = snapshot.Provider
			config.Model = snapshot.Model
			break
		}

		session, err := s.Store.GetSession(ctx, snapshotSessionID)
		switch {
		case err == nil:
			if config.Runtime == "" {
				config.Runtime = session.Metadata.CodeAgentRuntime
			}
			if config.AgentName == "" {
				config.AgentName = session.Metadata.ZedAgentName
			}
		case errors.Is(err, store.ErrNotFound):
		default:
			return nil, fmt.Errorf("failed to load session: %w", err)
		}
	}

	if config.Runtime == "" {
		config.Runtime = types.CodeAgentRuntimeZedAgent
	}
	if config.AgentName == "" {
		config.AgentName = string(config.Runtime)
	}
	if overrides != nil {
		if overrides.ProviderRef != "" {
			config.ProviderRef = overrides.ProviderRef
		}
		if overrides.Model != "" {
			config.Model = overrides.Model
		}
		if overrides.ReasoningEffort != "" {
			config.ReasoningEffort = overrides.ReasoningEffort
		}
		config.ServiceTier = overrides.ServiceTier
	}

	return config, nil
}

// validateCodeAgentOverrides rejects overrides that the Agent cannot honour —
// unsupported reasoning efforts, a service tier on a non-Codex runtime, a
// provider on a subscription agent, or a model the actor has no provider for.
func (s *HelixAPIServer) validateCodeAgentOverrides(
	ctx context.Context,
	appID string,
	overrides *types.CodeAgentOverrides,
	actorID string,
) error {
	if overrides == nil {
		return fmt.Errorf("code-agent overrides are required")
	}
	if overrides.ProviderRef != "" && overrides.Model == "" {
		return fmt.Errorf("model is required when provider_ref is set")
	}
	effort := strings.ToLower(strings.TrimSpace(overrides.ReasoningEffort))
	switch effort {
	case "", "none", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return fmt.Errorf("unsupported reasoning effort %q", overrides.ReasoningEffort)
	}
	if overrides.ServiceTier != "" && overrides.ServiceTier != "fast" {
		return fmt.Errorf("unsupported service tier %q", overrides.ServiceTier)
	}
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return fmt.Errorf("failed to load agent: %w", err)
	}
	effectiveApp := external_agent.ApplyCodeAgentOverrides(app, overrides)
	assistant := external_agent.FindZedExternalAssistant(effectiveApp)
	if assistant == nil {
		return fmt.Errorf("agent has no coding assistant")
	}
	if overrides.ServiceTier != "" && assistant.CodeAgentRuntime != types.CodeAgentRuntimeCodexCLI {
		return fmt.Errorf("service tier is only supported by Codex")
	}
	if assistant.CodeAgentCredentialType.IsSubscription() && overrides.ProviderRef != "" {
		return fmt.Errorf("subscription agents cannot override provider_ref")
	}
	snapshot, err := s.getProviderSnapshot(ctx, actorID, app)
	if err != nil {
		return fmt.Errorf("failed to load providers: %w", err)
	}
	if reason := external_agent.ValidateAssistantModelConfig(effectiveApp, snapshot); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func (s *HelixAPIServer) validateCodeAgentExecutionConfig(
	ctx context.Context,
	config *types.CodeAgentExecutionConfig,
	actorID string,
	ownerID string,
	organizationID string,
) error {
	if config == nil {
		return fmt.Errorf("code_agent_config is required")
	}
	switch config.Runtime {
	case types.CodeAgentRuntimeZedAgent,
		types.CodeAgentRuntimeQwenCode,
		types.CodeAgentRuntimeClaudeCode,
		types.CodeAgentRuntimeGeminiCLI,
		types.CodeAgentRuntimeCodexCLI,
		types.CodeAgentRuntimeGooseCode,
		types.CodeAgentRuntimeOpenCode:
	default:
		return fmt.Errorf("unsupported code-agent runtime %q", config.Runtime)
	}
	if err := s.validateOrgCodeAgentHarness(ctx, organizationID, config.Runtime, config.CredentialType, config.ProviderRef); err != nil {
		return err
	}
	if config.CredentialType != types.CodeAgentCredentialTypeAPIKey &&
		config.CredentialType != types.CodeAgentCredentialTypeSubscription {
		return fmt.Errorf("unsupported credential type %q", config.CredentialType)
	}
	if config.CredentialType == types.CodeAgentCredentialTypeAPIKey &&
		(config.ProviderRef == "" || config.Model == "") {
		return fmt.Errorf("provider_ref and model are required for API-key code agents")
	}
	if config.CredentialType.IsSubscription() && config.ProviderRef != "" {
		return fmt.Errorf("subscription code agents cannot set provider_ref")
	}
	effort := strings.ToLower(strings.TrimSpace(config.ReasoningEffort))
	switch effort {
	case "", "none", "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return fmt.Errorf("unsupported reasoning effort %q", config.ReasoningEffort)
	}
	if config.ServiceTier != "" && config.ServiceTier != "fast" {
		return fmt.Errorf("unsupported service tier %q", config.ServiceTier)
	}
	if config.ServiceTier != "" && config.Runtime != types.CodeAgentRuntimeCodexCLI {
		return fmt.Errorf("service tier is only supported by Codex")
	}

	app := external_agent.AppFromCodeAgentConfig(config, ownerID, organizationID)
	snapshot, err := s.getProviderSnapshot(ctx, actorID, app)
	if err != nil {
		return fmt.Errorf("failed to load providers: %w", err)
	}
	if reason := external_agent.ValidateAssistantModelConfig(app, snapshot); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func isDeferredNativeHarnessProjectConfig(config *types.CodeAgentExecutionConfig) bool {
	if config == nil || config.CredentialType != types.CodeAgentCredentialTypeAPIKey ||
		config.ProviderRef != "" || config.Model != "" {
		return false
	}
	return config.Runtime == types.CodeAgentRuntimeClaudeCode ||
		config.Runtime == types.CodeAgentRuntimeCodexCLI
}

// Projects choose a harness, while each task chooses the credential source and
// model. Native harness projects therefore persist a runtime-only API default;
// the complete config is selected and validated when a task is created.
func (s *HelixAPIServer) validateProjectCodeAgentConfig(
	ctx context.Context,
	config *types.CodeAgentExecutionConfig,
	actorID string,
	ownerID string,
	organizationID string,
) error {
	if !isDeferredNativeHarnessProjectConfig(config) {
		return s.validateCodeAgentExecutionConfig(ctx, config, actorID, ownerID, organizationID)
	}
	return s.validateOrgCodeAgentHarness(
		ctx,
		organizationID,
		config.Runtime,
		config.CredentialType,
		"",
	)
}

func taskAgentExecutionConfig(config *types.CodeAgentExecutionConfig) *types.AgentExecutionConfig {
	if config == nil {
		return &types.AgentExecutionConfig{}
	}
	return &types.AgentExecutionConfig{
		AgentAvailable:  true,
		AgentName:       config.Runtime.ZedAgentName(),
		Runtime:         config.Runtime,
		CredentialType:  config.CredentialType,
		ProviderRef:     config.ProviderRef,
		Model:           config.Model,
		ReasoningEffort: config.ReasoningEffort,
		ServiceTier:     config.ServiceTier,
		CodeAgentConfig: config,
	}
}

func (s *HelixAPIServer) applySpecTaskExecutionConfig(
	ctx context.Context,
	user *types.User,
	task *types.SpecTask,
	session *types.Session,
	config *types.CodeAgentExecutionConfig,
	handoffReason string,
) (changed bool, restarted bool, httpErr *system.HTTPError) {
	if err := s.validateCodeAgentExecutionConfig(ctx, config, user.ID, task.UserID, task.OrganizationID); err != nil {
		return false, false, system.NewHTTPError400(err.Error())
	}
	if reflect.DeepEqual(task.CodeAgentConfig, config) &&
		task.HelixAppID == "" && task.CodeAgentOverrides == nil &&
		(session == nil || (session.ParentApp == "" && session.Metadata.CodeAgentOverrides == nil)) {
		return false, false, nil
	}
	live := session != nil && s.hasRunningAgentContainer(ctx, session.ID)
	if session != nil {
		var err error
		if live {
			err = s.cancelTurnsForSwitch(ctx, session.ID)
		} else {
			err = s.cancelOfflineTurnsForSwitch(ctx, session)
		}
		if err != nil {
			return false, false, system.NewHTTPError409(fmt.Sprintf("failed to stop the current agent turn before switching: %v", err))
		}
	}

	oldConfig := task.CodeAgentConfig
	oldAppID := task.HelixAppID
	oldOverrides := task.CodeAgentOverrides
	task.CodeAgentConfig = config
	task.HelixAppID = ""
	task.CodeAgentOverrides = nil
	if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
		task.CodeAgentConfig = oldConfig
		task.HelixAppID = oldAppID
		task.CodeAgentOverrides = oldOverrides
		return false, false, system.NewHTTPError500(fmt.Sprintf("failed to save code-agent config: %v", err))
	}
	rollback := func() {
		task.CodeAgentConfig = oldConfig
		task.HelixAppID = oldAppID
		task.CodeAgentOverrides = oldOverrides
		if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to roll back task code-agent config")
		}
	}
	if session == nil {
		return true, false, nil
	}
	if switchErr := s.switchAgentInPlaceForNextTurn(ctx, session, config.Runtime, "", agentSwitchOptions{
		createHandoff:  live,
		handoffReason:  handoffReason,
		deliverLive:    live,
		clearParentApp: true,
	}); switchErr != nil {
		rollback()
		return false, false, switchErr
	}
	return true, live, nil
}

func (s *HelixAPIServer) applySessionCodeAgentExecutionConfig(
	ctx context.Context,
	user *types.User,
	session *types.Session,
	config *types.CodeAgentExecutionConfig,
	handoffReason string,
) (changed bool, restarted bool, httpErr *system.HTTPError) {
	if err := s.validateCodeAgentExecutionConfig(
		ctx, config, user.ID, session.Owner, session.OrganizationID,
	); err != nil {
		return false, false, system.NewHTTPError400(err.Error())
	}
	if reflect.DeepEqual(session.Metadata.CodeAgentConfig, config) &&
		session.Metadata.CodeAgentOverrides == nil {
		return false, false, nil
	}

	live := s.hasRunningAgentContainer(ctx, session.ID)
	var err error
	if live {
		err = s.cancelTurnsForSwitch(ctx, session.ID)
	} else {
		err = s.cancelOfflineTurnsForSwitch(ctx, session)
	}
	if err != nil {
		return false, false, system.NewHTTPError409(
			fmt.Sprintf("failed to stop the current agent turn before switching: %v", err))
	}

	oldConfig := session.Metadata.CodeAgentConfig
	oldOverrides := session.Metadata.CodeAgentOverrides
	session.Metadata.CodeAgentConfig = config
	session.Metadata.CodeAgentOverrides = nil
	if _, err := s.Store.UpdateSession(ctx, *session); err != nil {
		session.Metadata.CodeAgentConfig = oldConfig
		session.Metadata.CodeAgentOverrides = oldOverrides
		return false, false, system.NewHTTPError500(fmt.Sprintf("failed to save code-agent config: %v", err))
	}
	rollback := func() {
		session.Metadata.CodeAgentConfig = oldConfig
		session.Metadata.CodeAgentOverrides = oldOverrides
		if _, err := s.Store.UpdateSession(ctx, *session); err != nil {
			log.Error().Err(err).Str("session_id", session.ID).Msg("Failed to roll back session code-agent config")
		}
	}

	if switchErr := s.switchAgentInPlaceForNextTurn(
		ctx, session, config.Runtime, session.ParentApp, agentSwitchOptions{
			createHandoff: live,
			handoffReason: handoffReason,
			deliverLive:   live,
		},
	); switchErr != nil {
		rollback()
		return false, false, switchErr
	}
	return true, live, nil
}

// cancelTurnsForSwitch drains every in-flight turn so the agent is idle before
// its configuration is swapped underneath it.
func (s *HelixAPIServer) cancelTurnsForSwitch(ctx context.Context, sessionID string) error {
	const maxTurns = 100
	for i := 0; i < maxTurns; i++ {
		status, err := s.cancelActiveTurn(ctx, sessionID)
		if err != nil {
			return err
		}
		if status == "noop" {
			return nil
		}
		if status == "pending" {
			return fmt.Errorf("external agent cancellation is pending acknowledgement")
		}
	}
	return fmt.Errorf("more than %d active turns remain", maxTurns)
}

func (s *HelixAPIServer) hasRunningAgentContainer(ctx context.Context, sessionID string) bool {
	return s.externalAgentExecutor != nil && s.externalAgentExecutor.HasRunningContainer(ctx, sessionID)
}

// cancelOfflineTurnsForSwitch clears queued rows without contacting an agent.
// HasRunningContainer is authoritative: when it is false, no sandbox process
// can acknowledge a cancellation and leaving a Waiting row would make the UI
// look active indefinitely.
func (s *HelixAPIServer) cancelOfflineTurnsForSwitch(ctx context.Context, session *types.Session) error {
	interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    session.ID,
		GenerationID: session.GenerationID,
		PerPage:      10_000,
	})
	if err != nil {
		return fmt.Errorf("list queued interactions: %w", err)
	}
	for _, interaction := range interactions {
		if interaction == nil || interaction.State != types.InteractionStateWaiting {
			continue
		}
		if _, err := s.interruptWaitingInteraction(ctx, session, interaction); err != nil {
			return err
		}
	}
	return nil
}

func (s *HelixAPIServer) codeAgentRuntimeForApp(ctx context.Context, appID string) (types.CodeAgentRuntime, error) {
	app, err := s.Store.GetApp(ctx, appID)
	if err != nil {
		return "", fmt.Errorf("failed to load agent: %w", err)
	}
	assistant := external_agent.FindZedExternalAssistant(app)
	if assistant == nil {
		return "", fmt.Errorf("agent has no coding assistant")
	}
	if assistant.CodeAgentRuntime == "" {
		return types.CodeAgentRuntimeZedAgent, nil
	}
	return assistant.CodeAgentRuntime, nil
}

// codeAgentConfigTarget describes an App-backed chat surface whose coding
// identity is being changed.
type codeAgentConfigTarget struct {
	// surface names the caller in user-facing errors ("spec tasks", "sessions").
	surface string
	// requiredAgentKind, when set, restricts which Agents this surface may
	// switch to. Org chart chat accepts any Agent with a coding assistant.
	requiredAgentKind string
	// session runs the agent. Nil when the surface has not started one yet, in
	// which case the new configuration simply applies on first start.
	session *types.Session
	// agentID and overrides are the currently persisted identity, used for the
	// no-op check and for rolling back a failed switch.
	agentID   string
	overrides *types.CodeAgentOverrides
	// persist writes the new App-backed identity to its owning session row.
	persist func(ctx context.Context, agentID string, overrides *types.CodeAgentOverrides) error
	// handoffReason is shown to the freshly started agent thread.
	handoffReason string
}

// persistSessionCodeAgentConfig writes a coding identity onto a session row.
// The agent id is ignored on purpose: switchAgentInPlaceForNextTurn repoints
// ParentApp itself, and it owns that binding.
func (s *HelixAPIServer) persistSessionCodeAgentConfig(
	session *types.Session,
) func(context.Context, string, *types.CodeAgentOverrides) error {
	return func(ctx context.Context, _ string, overrides *types.CodeAgentOverrides) error {
		session.Metadata.CodeAgentOverrides = overrides
		_, err := s.Store.UpdateSession(ctx, *session)
		return err
	}
}

// applyCodeAgentExecutionConfig validates the requested identity, persists it,
// and — when a session is live — cancels the in-flight turn and starts a fresh
// ACP thread seeded with the prior transcript. Returns whether anything changed
// and whether a new agent thread was started. Persistence is rolled back if the
// switch fails, so the stored identity always matches the running agent.
func (s *HelixAPIServer) applyCodeAgentExecutionConfig(
	ctx context.Context,
	user *types.User,
	target codeAgentConfigTarget,
	req types.SessionExecutionConfigUpdateRequest,
) (changed bool, restarted bool, httpErr *system.HTTPError) {
	targetAgentID := target.agentID
	// Only a genuine change of Agent is re-checked: re-sending the surface's
	// current agent id alongside a model edit is not an agent switch, and must
	// not be held to the switch-target rules.
	if req.AgentID != "" && req.AgentID != target.agentID {
		targetAgentID = req.AgentID
		targetApp, appErr := s.Store.GetApp(ctx, targetAgentID)
		if appErr != nil {
			return false, false, system.NewHTTPError400("selected agent not found")
		}
		if authErr := s.authorizeUserToApp(ctx, user, targetApp, types.ActionGet); authErr != nil {
			return false, false, system.NewHTTPError403(authErr.Error())
		}
		if target.requiredAgentKind != "" {
			if kindErr := requireAgentKind(targetApp, target.requiredAgentKind, target.surface); kindErr != nil {
				return false, false, system.NewHTTPError400(kindErr.Error())
			}
		}
	}
	if err := s.validateCodeAgentOverrides(ctx, targetAgentID, req.CodeAgentOverrides, user.ID); err != nil {
		return false, false, system.NewHTTPError400(err.Error())
	}
	if targetAgentID == target.agentID && reflect.DeepEqual(target.overrides, req.CodeAgentOverrides) {
		return false, false, nil
	}

	live := target.session != nil && s.hasRunningAgentContainer(ctx, target.session.ID)
	if target.session != nil {
		var cancelErr error
		if live {
			cancelErr = s.cancelTurnsForSwitch(ctx, target.session.ID)
		} else {
			cancelErr = s.cancelOfflineTurnsForSwitch(ctx, target.session)
		}
		if cancelErr != nil {
			return false, false, system.NewHTTPError409(
				fmt.Sprintf("failed to stop the current agent turn before switching: %v", cancelErr))
		}
	}

	if err := target.persist(ctx, targetAgentID, req.CodeAgentOverrides); err != nil {
		return false, false, system.NewHTTPError500(fmt.Sprintf("failed to save code-agent overrides: %v", err))
	}
	rollback := func() {
		if err := target.persist(ctx, target.agentID, target.overrides); err != nil {
			log.Error().Err(err).
				Str("surface", target.surface).
				Str("agent_id", target.agentID).
				Msg("Failed to roll back code-agent overrides after a failed switch")
		}
	}

	if target.session == nil {
		return true, false, nil
	}

	runtime, err := s.codeAgentRuntimeForApp(ctx, targetAgentID)
	if err != nil {
		rollback()
		return false, false, system.NewHTTPError400(err.Error())
	}
	if switchErr := s.switchAgentInPlaceForNextTurn(
		ctx, target.session, runtime, targetAgentID, agentSwitchOptions{
			createHandoff: live,
			handoffReason: target.handoffReason,
			deliverLive:   live,
		},
	); switchErr != nil {
		rollback()
		return false, false, switchErr
	}
	return true, live, nil
}
