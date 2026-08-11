package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	claudeLoginSessionName     = "Claude Login"
	claudeLoginSessionProvider = "anthropic"
	codexLoginSessionName      = "Codex Login"
	codexLoginSessionProvider  = "openai"
)

func (apiServer *HelixAPIServer) cleanupSubscriptionLoginSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return errors.New("login session ID is required")
	}
	if apiServer.externalAgentExecutor == nil {
		return errors.New("external agent executor is not configured")
	}

	cleanupCtx := context.WithoutCancel(ctx)
	if err := apiServer.externalAgentExecutor.StopDesktop(cleanupCtx, sessionID); err != nil {
		return fmt.Errorf("stop login desktop: %w", err)
	}
	if _, err := apiServer.Store.DeleteSession(cleanupCtx, sessionID); err != nil {
		return fmt.Errorf("delete login session: %w", err)
	}
	return nil
}

func isTemporarySubscriptionLoginSession(session *types.Session, name, provider string) bool {
	return session != nil &&
		session.Name == name &&
		session.Provider == provider &&
		session.ModelName == "external_agent" &&
		session.Metadata.AgentType == "zed_external" &&
		session.Metadata.SessionRole == "exploratory"
}

func (apiServer *HelixAPIServer) cleanupSubscriptionLoginSessionsForOwner(ctx context.Context, ownerID, name, provider string) error {
	sessions, err := apiServer.Store.ListSessionsByOwner(context.WithoutCancel(ctx), ownerID)
	if err != nil {
		return fmt.Errorf("list login sessions: %w", err)
	}

	var cleanupErrors []error
	for _, session := range sessions {
		if !isTemporarySubscriptionLoginSession(session, name, provider) {
			continue
		}
		if err := apiServer.cleanupSubscriptionLoginSession(ctx, session.ID); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("session %s: %w", session.ID, err))
		}
	}
	return errors.Join(cleanupErrors...)
}
