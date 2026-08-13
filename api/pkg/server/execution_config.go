package server

// Shared code-agent execution configuration. A SpecTask and a plain chat
// session (org bot chat, project chat) both run the same Zed/ACP machinery, so
// both resolve their coding identity and apply model/reasoning changes through
// the helpers here. The only difference between the two surfaces is WHERE the
// overrides are persisted, which each caller supplies as codeAgentConfigTarget.

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

// effectiveCodeAgentOverrides picks the overrides that apply to a session's
// next turn. A SpecTask owns the configuration of the sessions it drives, so
// its overrides win; every other session carries its own.
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
	}
	return fmt.Errorf("more than %d active turns remain", maxTurns)
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

// codeAgentConfigTarget is the surface whose coding identity is being changed:
// its current identity, where to persist a new one, and the live session (if
// any) that has to be restarted onto it.
type codeAgentConfigTarget struct {
	// surface names the caller in user-facing errors ("spec tasks", "sessions").
	surface string
	// requiredAgentKind, when set, restricts which Agents this surface may
	// switch TO. Spec tasks take coding agents only; a chat session takes any
	// Agent with a coding assistant, which includes an org chart's bots.
	requiredAgentKind string
	// session runs the agent. Nil when the surface has not started one yet, in
	// which case the new configuration simply applies on first start.
	session *types.Session
	// agentID and overrides are the currently persisted identity, used for the
	// no-op check and for rolling back a failed switch.
	agentID   string
	overrides *types.CodeAgentOverrides
	// persist writes a new identity to whichever record owns it (a SpecTask row
	// for task surfaces, the session row otherwise).
	persist func(ctx context.Context, agentID string, overrides *types.CodeAgentOverrides) error
	// handoffReason is shown to the freshly started agent thread.
	handoffReason string
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

	if target.session != nil {
		if cancelErr := s.cancelTurnsForSwitch(ctx, target.session.ID); cancelErr != nil {
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
		ctx, target.session, runtime, targetAgentID, true, target.handoffReason,
	); switchErr != nil {
		rollback()
		return false, false, switchErr
	}
	return true, true, nil
}
