package server

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

type proxyUsageAttribution struct {
	SessionID        string
	AppID            string
	CodeAgentRuntime types.CodeAgentRuntime
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
	if session.ParentApp != "" {
		attribution.AppID = session.ParentApp
	}
	return attribution, nil
}
