package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

type proxyUsageAttribution struct {
	SessionID          string
	AppID              string
	CodeAgentRuntime   types.CodeAgentRuntime
	CodeAgentOverrides *types.CodeAgentOverrides
	hasExecutionConfig bool
}

func (s *HelixAPIServer) resolveProxyUsageAttribution(ctx context.Context, user *types.User, defaultSessionID string) (*proxyUsageAttribution, error) {
	attribution := &proxyUsageAttribution{
		SessionID: defaultSessionID,
		AppID:     user.AppID,
	}
	if user.SessionID == "" {
		return attribution, nil
	}

	session, err := s.Store.GetSession(ctx, user.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load API key session %q: %w", user.SessionID, err)
	}
	if session.OrganizationID != "" && user.OrganizationID != "" && session.OrganizationID != user.OrganizationID {
		return nil, fmt.Errorf("API key session %q belongs to a different organization", user.SessionID)
	}
	if user.SpecTaskID != "" && session.Metadata.SpecTaskID != "" && session.Metadata.SpecTaskID != user.SpecTaskID {
		return nil, fmt.Errorf("API key session %q belongs to a different task", user.SessionID)
	}
	if attribution.AppID != "" && session.ParentApp != "" && attribution.AppID != session.ParentApp {
		return nil, fmt.Errorf("API key app %q does not match session app %q", attribution.AppID, session.ParentApp)
	}

	attribution.SessionID = session.ID
	attribution.CodeAgentRuntime = session.Metadata.CodeAgentRuntime
	attribution.CodeAgentOverrides = session.Metadata.CodeAgentOverrides
	if session.Metadata.CodeAgentConfig != nil {
		attribution.hasExecutionConfig = true
		attribution.CodeAgentRuntime = session.Metadata.CodeAgentConfig.Runtime
		attribution.CodeAgentOverrides = &types.CodeAgentOverrides{
			ProviderRef:     session.Metadata.CodeAgentConfig.ProviderRef,
			Model:           session.Metadata.CodeAgentConfig.Model,
			ReasoningEffort: session.Metadata.CodeAgentConfig.ReasoningEffort,
			ServiceTier:     session.Metadata.CodeAgentConfig.ServiceTier,
		}
	}
	if session.ParentApp != "" {
		attribution.AppID = session.ParentApp
	}
	if session.Metadata.SpecTaskID != "" {
		task, err := s.Store.GetSpecTask(ctx, session.Metadata.SpecTaskID)
		if err != nil {
			return nil, fmt.Errorf("failed to load API key spec task %q: %w", session.Metadata.SpecTaskID, err)
		}
		if task.PlanningSessionID != "" && task.PlanningSessionID != session.ID {
			return nil, fmt.Errorf("API key session %q does not own spec task %q", session.ID, task.ID)
		}
		if task.CodeAgentConfig != nil {
			attribution.hasExecutionConfig = true
			attribution.AppID = ""
			attribution.CodeAgentRuntime = task.CodeAgentConfig.Runtime
			attribution.CodeAgentOverrides = &types.CodeAgentOverrides{
				ProviderRef:     task.CodeAgentConfig.ProviderRef,
				Model:           task.CodeAgentConfig.Model,
				ReasoningEffort: task.CodeAgentConfig.ReasoningEffort,
				ServiceTier:     task.CodeAgentConfig.ServiceTier,
			}
		} else {
			attribution.CodeAgentOverrides = task.CodeAgentOverrides
		}
		if task.CodeAgentConfig == nil && task.HelixAppID != "" {
			attribution.AppID = task.HelixAppID
		}
	}
	return attribution, nil
}
