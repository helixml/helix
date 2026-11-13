package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRecoverProviderFailedSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	now := time.Now()
	session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			Provider: "pe_1", Model: "glm-5.3-flash", PromptID: "prompt_1", FailedAt: now.Add(-2 * time.Second), NextCheckAt: now.Add(-time.Second),
		},
	}}
	st.EXPECT().RecoverProviderPrompt(gomock.Any(), "ses_1", "prompt_1", gomock.Any(), gomock.Any(), maxProviderRecoveryAttempts).Return(true, nil)

	recovered, err := server.recoverProviderFailedSession(context.Background(), session, []types.OpenAIModel{{ID: "glm-5.3-flash"}}, now, now)
	require.NoError(t, err)
	require.True(t, recovered)
}

func TestRecoverProviderFailedWorkersMatchesGlobalProviderAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	st.EXPECT().ListSessions(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query store.ListSessionsQuery) ([]*types.Session, int64, error) {
			require.ElementsMatch(t, []string{"anthropic", "global/anthropic"}, query.ProviderRecoveryProviders)
			return nil, 0, nil
		},
	)

	server.recoverProviderFailedWorkers(context.Background(), &types.ProviderEndpoint{
		Name: "anthropic", Owner: string(types.OwnerTypeSystem),
	}, ProviderModels{})
}

func TestRecoverProviderFailedWorkersDoesNotAliasDatabaseEndpoint(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	st.EXPECT().ListSessions(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, query store.ListSessionsQuery) ([]*types.Session, int64, error) {
			require.ElementsMatch(t, []string{"anthropic", "pe_org"}, query.ProviderRecoveryProviders)
			return nil, 0, nil
		},
	)

	server.recoverProviderFailedWorkers(context.Background(), &types.ProviderEndpoint{
		ID: "pe_org", Name: "anthropic", Owner: "org_1",
	}, ProviderModels{})
}

func TestRecoverProviderFailedSessionWaitsForModel(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	now := time.Now()
	session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			Provider: "pe_1", Model: "glm-5.3-flash", PromptID: "prompt_1", FailedAt: now.Add(-2 * time.Second), NextCheckAt: now.Add(-time.Second),
		},
	}}

	recovered, err := server.recoverProviderFailedSession(context.Background(), session, []types.OpenAIModel{{ID: "qwen"}}, now, now)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverProviderFailedSessionIgnoresModelsCachedBeforeFailure(t *testing.T) {
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	now := time.Now()
	session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			Provider: "pe_1", Model: "glm-5.3-flash", PromptID: "prompt_1", FailedAt: now.Add(-time.Minute), NextCheckAt: now.Add(-time.Second),
		},
	}}

	recovered, err := server.recoverProviderFailedSession(context.Background(), session, []types.OpenAIModel{{ID: "glm-5.3-flash"}}, now.Add(-2*time.Minute), now)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverProviderFailedSessionStopsAtAttemptLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	now := time.Now()
	session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			Provider: "pe_1", Model: "glm-5.3-flash", PromptID: "prompt_1", Attempts: maxProviderRecoveryAttempts,
			FailedAt: now.Add(-2 * time.Second), NextCheckAt: now.Add(-time.Second),
		},
	}}

	recovered, err := server.recoverProviderFailedSession(context.Background(), session, []types.OpenAIModel{{ID: "glm-5.3-flash"}}, now, now)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverExplicitProviderFailedActivationAfterRepeatedConfigSwitch(t *testing.T) {
	tests := []struct {
		name     string
		workerID string
		error    string
	}{
		{
			name:     "promoter transport timeout",
			workerID: "promoter",
			error:    `agent turn aborted: Internal error: Internal Server Error: Post "https://node06.lukemarsden.net/v1/chat/completions": dial tcp 212.82.90.212:443: i/o timeout: { "service": "session", "errorName": "APIError" }`,
		},
		{
			name:     "chief of staff gateway timeout",
			workerID: "chief-of-staff",
			error:    "agent turn aborted: Internal error: status code 524: {\n  \"service\": \"session\",\n  \"errorName\": \"APIError\"\n}",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
			now := time.Now()
			promptID := "prompt_activation_" + test.workerID
			interactionID := "int_" + test.workerID
			session := &types.Session{ID: "ses_" + test.workerID, Metadata: types.SessionMetadata{
				OrgWorkerID:        test.workerID,
				AutoRestartOnCrash: true,
				CodeAgentConfig: &types.CodeAgentExecutionConfig{
					ProviderRef: "helixcyber",
					Model:       "qwen3.8-27b",
				},
			}}
			st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
				ID: promptID, SessionID: session.ID, Status: "sent", CreatedAt: now,
			}, nil)
			st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
				ID: interactionID, SessionID: session.ID, PromptID: promptID, State: types.InteractionStateError, Error: test.error,
			}, nil)
			st.EXPECT().RecoverFailedPrompt(gomock.Any(), session.ID, promptID, interactionID).Return(true, nil)

			recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
			require.NoError(t, err)
			require.True(t, recovered)
		})
	}
}

func TestRecoverProviderFailedActivationRequiresLatestFailedLLMCall(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	promptID := "prompt_activation_cos"
	interactionID := "int_cos"
	session := &types.Session{ID: "ses_cos", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
	}}
	st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
		ID: promptID, SessionID: session.ID, Status: "sent",
	}, nil)
	st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
		ID: interactionID, SessionID: session.ID, PromptID: promptID, State: types.InteractionStateError,
		Error: genericACPAbortMarker,
	}, nil)
	st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{{
		InteractionID: interactionID,
	}}, int64(2), nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestRecoverProviderFailedActivationDoesNotReplayOlderFailureAfterCancellation(t *testing.T) {
	ctx := context.Background()
	memory := memorystore.New()
	now := time.Now()
	deletedAt := now
	session := &types.Session{ID: "ses_cancelled", Metadata: types.SessionMetadata{
		OrgWorkerID: "chief-of-staff", AutoRestartOnCrash: true,
	}}
	_, err := memory.CreateSession(ctx, *session)
	require.NoError(t, err)
	require.NoError(t, memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: "prompt_activation_old_failure", SessionID: session.ID, Status: "sent", CreatedAt: now.Add(-time.Minute),
	}))
	require.NoError(t, memory.CreatePromptHistoryEntry(ctx, &types.PromptHistoryEntry{
		ID: "prompt_activation_cancelled", SessionID: session.ID, Status: "sent", CreatedAt: now, DeletedAt: &deletedAt,
	}))
	_, err = memory.CreateInteraction(ctx, &types.Interaction{
		ID: "int_old_failure", SessionID: session.ID, PromptID: "prompt_activation_old_failure",
		State: types.InteractionStateError, Error: `agent turn aborted: status code 500`,
	})
	require.NoError(t, err)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: memory}
	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(ctx, session)
	require.NoError(t, err)
	require.False(t, recovered)
	oldPrompt, err := memory.GetPromptHistoryEntry(ctx, "prompt_activation_old_failure")
	require.NoError(t, err)
	require.Equal(t, "sent", oldPrompt.Status)
}

func TestRecoverOpaqueProviderFailureWithMatchingRecoveryMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	promptID := "prompt_activation_metadata"
	interactionID := "int_metadata"
	session := &types.Session{ID: "ses_metadata", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			PromptID: promptID, InteractionID: interactionID,
		},
	}}
	st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
		ID: promptID, SessionID: session.ID, Status: "sent",
	}, nil)
	st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
		ID: interactionID, SessionID: session.ID, PromptID: promptID,
		State: types.InteractionStateError, Error: genericACPAbortMarker,
	}, nil)
	st.EXPECT().RecoverFailedPrompt(gomock.Any(), session.ID, promptID, interactionID).Return(true, nil)
	st.EXPECT().ClearSessionProviderRecovery(gomock.Any(), session.ID, promptID, interactionID).Return(true, nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.True(t, recovered)
	require.Nil(t, session.Metadata.Recovery)
}

func TestSuccessfulRequeuePreservesRecoveryWhenCompareAndClearLosesRace(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	promptID := "prompt_activation_race"
	interactionID := "int_race"
	state := &types.ProviderRecovery{PromptID: promptID, InteractionID: interactionID}
	session := &types.Session{ID: "ses_race", Metadata: types.SessionMetadata{
		OrgWorkerID: "chief-of-staff", AutoRestartOnCrash: true, Recovery: state,
	}}
	st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
		ID: promptID, SessionID: session.ID, Status: "sent",
	}, nil)
	st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
		ID: interactionID, SessionID: session.ID, PromptID: promptID,
		State: types.InteractionStateError, Error: genericACPAbortMarker,
	}, nil)
	st.EXPECT().RecoverFailedPrompt(gomock.Any(), session.ID, promptID, interactionID).Return(true, nil)
	st.EXPECT().ClearSessionProviderRecovery(gomock.Any(), session.ID, promptID, interactionID).Return(false, nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.True(t, recovered)
	require.Same(t, state, session.Metadata.Recovery)
}

func TestOpaqueProviderFailureIgnoresMismatchedRecoveryMetadata(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	promptID := "prompt_activation_current"
	interactionID := "int_current"
	session := &types.Session{ID: "ses_mismatch", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			PromptID: "prompt_activation_old", InteractionID: "int_old",
		},
	}}
	st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
		ID: promptID, SessionID: session.ID, Status: "sent",
	}, nil)
	st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
		ID: interactionID, SessionID: session.ID, PromptID: promptID,
		State: types.InteractionStateError, Error: genericACPAbortMarker,
	}, nil)
	st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
	st.EXPECT().ClearSessionProviderRecovery(gomock.Any(), session.ID, "prompt_activation_old", "int_old").Return(true, nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Nil(t, session.Metadata.Recovery)
}

func TestConfigChangeClearsStaleRecoveryWithoutReplayingCompletedActivation(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	now := time.Now()
	promptID := "prompt_activation_completed"
	session := &types.Session{ID: "ses_completed", Metadata: types.SessionMetadata{
		OrgWorkerID:        "chief-of-staff",
		AutoRestartOnCrash: true,
		Recovery: &types.ProviderRecovery{
			PromptID:      promptID,
			InteractionID: "int_completed",
		},
	}}
	st.EXPECT().GetLatestActivationPromptHistoryEntry(gomock.Any(), session.ID).Return(&types.PromptHistoryEntry{
		ID: promptID, SessionID: session.ID, Status: "sent", CreatedAt: now.Add(time.Second),
	}, nil)
	st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), promptID).Return(&types.Interaction{
		ID: "int_completed", SessionID: session.ID, PromptID: promptID, State: types.InteractionStateComplete,
	}, nil)
	st.EXPECT().ClearSessionProviderRecovery(gomock.Any(), session.ID, promptID, "int_completed").Return(true, nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.False(t, recovered)
	require.Nil(t, session.Metadata.Recovery)
}

func TestProviderFailureMatchesCurrentConfig(t *testing.T) {
	require.True(t, providerFailureMatchesConfig(
		&types.LLMCall{Provider: "bunker", Model: "bunker/glm-5.3-flash"},
		&types.CodeAgentExecutionConfig{ProviderRef: "bunker", Model: "glm-5.3-flash"},
	))
	require.False(t, providerFailureMatchesConfig(
		&types.LLMCall{Provider: "bunker", Model: "glm-5.3-flash"},
		&types.CodeAgentExecutionConfig{ProviderRef: "helixcyber", Model: "qwen3.8-27b"},
	))
	require.True(t, providerFailureMatchesConfig(
		&types.LLMCall{Provider: "anthropic", Model: "claude-opus-4-7"},
		&types.CodeAgentExecutionConfig{ProviderRef: "global/anthropic", Model: "claude-opus-4-7"},
	))
	require.False(t, providerFailureMatchesConfig(
		&types.LLMCall{Provider: "anthropic", Model: "claude-opus-4-7"},
		&types.CodeAgentExecutionConfig{ProviderRef: "user/anthropic", Model: "claude-opus-4-7"},
	))
}

func TestRecordProviderRecoveryOnlyForAutonomousOrgWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	started := time.Now().Add(-2 * time.Minute)
	completed := time.Now().Add(-time.Second)
	session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff", AutoRestartOnCrash: true}}
	st.EXPECT().GetSession(gomock.Any(), "ses_1").Return(session, nil)
	st.EXPECT().SetSessionProviderRecovery(gomock.Any(), "ses_1", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, recovery *types.ProviderRecovery) error {
			require.Equal(t, "pe_1", recovery.Provider)
			require.Equal(t, "prompt_1", recovery.PromptID)
			require.Equal(t, completed, recovery.FailedAt)
			require.NotEqual(t, started, recovery.FailedAt)
			return nil
		},
	)

	scheduled := server.recordProviderRecovery(context.Background(), &types.Interaction{
		ID: "int_1", SessionID: "ses_1", PromptID: "prompt_1",
	}, &types.LLMCall{Provider: "pe_1", Model: "glm-5.3-flash", Created: started, Updated: completed})
	require.True(t, scheduled)
}
