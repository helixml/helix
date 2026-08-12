package server

import (
	"context"
	"errors"
	"testing"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCleanupSubscriptionLoginSessionStopsDesktopBeforeDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	storeMock := store.NewMockStore(ctrl)
	executor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: storeMock, externalAgentExecutor: executor}
	session := &types.Session{ID: "ses_login"}

	gomock.InOrder(
		executor.EXPECT().StopDesktop(gomock.Any(), session.ID).Return(nil),
		storeMock.EXPECT().DeleteSession(gomock.Any(), session.ID).Return(session, nil),
	)

	require.NoError(t, server.cleanupSubscriptionLoginSession(context.Background(), session.ID))
}

func TestCleanupSubscriptionLoginSessionKeepsRowWhenDesktopStopFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	storeMock := store.NewMockStore(ctrl)
	executor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: storeMock, externalAgentExecutor: executor}

	executor.EXPECT().StopDesktop(gomock.Any(), "ses_login").Return(errors.New("stop failed"))

	err := server.cleanupSubscriptionLoginSession(context.Background(), "ses_login")
	require.ErrorContains(t, err, "stop login desktop")
}

func TestCleanupSubscriptionLoginSessionsForOwnerOnlyDeletesTemporaryLoginSessions(t *testing.T) {
	ctrl := gomock.NewController(t)
	storeMock := store.NewMockStore(ctrl)
	executor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: storeMock, externalAgentExecutor: executor}
	loginSession := &types.Session{
		ID:        "ses_claude_login",
		Name:      claudeLoginSessionName,
		Provider:  claudeLoginSessionProvider,
		ModelName: "external_agent",
		Metadata: types.SessionMetadata{
			AgentType:   "zed_external",
			SessionRole: "exploratory",
		},
	}
	ordinarySessionWithSameName := &types.Session{
		ID:        "ses_ordinary",
		Name:      claudeLoginSessionName,
		ModelName: "gpt-5",
	}

	storeMock.EXPECT().ListSessionsByOwner(gomock.Any(), "usr_1").Return(
		[]*types.Session{loginSession, ordinarySessionWithSameName}, nil,
	)
	executor.EXPECT().StopDesktop(gomock.Any(), loginSession.ID).Return(nil)
	storeMock.EXPECT().DeleteSession(gomock.Any(), loginSession.ID).Return(loginSession, nil)

	require.NoError(t, server.cleanupSubscriptionLoginSessionsForOwner(
		context.Background(), "usr_1", claudeLoginSessionName, claudeLoginSessionProvider,
	))
}
