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
		Error: genericACPAbortMarker, CodeAgentConfigSnapshot: &types.InteractionCodeAgentConfigSnapshot{
			Provider: "helixcyber", Model: "qwen3.8-27b",
		},
	}, nil)
	st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{{
		InteractionID: interactionID,
	}}, int64(2), nil)

	recovered, err := server.recoverProviderFailedActivationAfterConfigChange(context.Background(), session)
	require.NoError(t, err)
	require.False(t, recovered)
}

func TestProviderFailureForInteractionUsesMatchingUnlinkedCall(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		calls []*types.LLMCall
		want  bool
	}{
		{
			name: "matching failure",
			calls: []*types.LLMCall{{
				InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash",
				Created: now.Add(-20 * time.Second), Updated: now.Add(-5 * time.Second), Error: "http2 GOAWAY",
			}},
			want: true,
		},
		{
			name: "long request failing near attempt end",
			calls: []*types.LLMCall{{
				InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash",
				Created: now.Add(-5 * time.Minute), DurationMs: int64((5*time.Minute - 5*time.Second) / time.Millisecond), Error: "http2 GOAWAY",
			}},
			want: true,
		},
		{
			name: "different model",
			calls: []*types.LLMCall{{
				InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "qwen",
				Created: now.Add(-20 * time.Second), Error: "http2 GOAWAY",
			}},
		},
		{
			name: "stale failure",
			calls: []*types.LLMCall{{
				InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash",
				Created: now.Add(-5 * time.Minute), Updated: now.Add(-4 * time.Minute), Error: "http2 GOAWAY",
			}},
		},
		{
			name: "newer success",
			calls: []*types.LLMCall{
				{InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash", Created: now.Add(-5 * time.Second)},
				{InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash", Created: now.Add(-20 * time.Second), Error: "http2 GOAWAY"},
			},
		},
		{
			name: "later turn failure",
			calls: []*types.LLMCall{{
				InteractionID: "n/a", SessionID: "ses_1", Provider: "pe_1", Model: "glm-5.3-flash",
				Created: now.Add(time.Second), Error: "http2 GOAWAY",
			}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
			interaction := &types.Interaction{
				ID: "int_1", SessionID: "ses_1", State: types.InteractionStateError,
				Created: now.Add(-10 * time.Minute), Updated: now,
				CodeAgentConfigSnapshot: &types.InteractionCodeAgentConfigSnapshot{
					Provider: "pe_1", Model: "glm-5.3-flash",
				},
			}
			gomock.InOrder(
				st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil),
				st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return(test.calls, int64(len(test.calls)), nil),
			)

			failure, err := server.providerFailureForInteraction(context.Background(), interaction)
			require.NoError(t, err)
			require.Equal(t, test.want, failure != nil)
		})
	}
}

func TestProviderFailureForInteractionLinkedSuccessSuppressesOlderUnlinkedFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
	interaction := &types.Interaction{
		ID: "int_1", SessionID: "ses_1", State: types.InteractionStateError,
		CodeAgentConfigSnapshot: &types.InteractionCodeAgentConfigSnapshot{Provider: "pe_1", Model: "glm-5.3-flash"},
	}
	st.EXPECT().ListLLMCalls(gomock.Any(), gomock.Any()).Return([]*types.LLMCall{{
		InteractionID: interaction.ID, Provider: "pe_1", Model: "glm-5.3-flash",
	}}, int64(1), nil)

	failure, err := server.providerFailureForInteraction(context.Background(), interaction)
	require.NoError(t, err)
	require.Nil(t, failure)
}

func TestClearProviderRecoveryAfterCompletedTurn(t *testing.T) {
	for _, test := range []struct {
		name        string
		promptID    string
		latestID    string
		state       types.InteractionState
		error       string
		clearResult bool
		wantCleared bool
	}{
		{name: "successful replacement attempt", promptID: "prompt_1", latestID: "int_success", state: types.InteractionStateComplete, clearResult: true, wantCleared: true},
		{name: "unrelated successful prompt", promptID: "prompt_2", state: types.InteractionStateComplete},
		{name: "stale successful attempt", promptID: "prompt_1", latestID: "int_newer", state: types.InteractionStateComplete},
		{name: "newer recovery wins compare and clear race", promptID: "prompt_1", latestID: "int_success", state: types.InteractionStateComplete},
		{name: "interrupted trailing completion", promptID: "prompt_1", state: types.InteractionStateInterrupted},
		{name: "error state", promptID: "prompt_1", state: types.InteractionStateError, error: "provider unavailable"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: st}
			session := &types.Session{ID: "ses_1", Metadata: types.SessionMetadata{Recovery: &types.ProviderRecovery{
				PromptID: "prompt_1", InteractionID: "int_failed",
			}}}
			interaction := &types.Interaction{
				ID: "int_success", SessionID: session.ID, PromptID: test.promptID, State: test.state, Error: test.error,
			}
			if test.promptID == "prompt_1" && test.state == types.InteractionStateComplete && test.error == "" {
				st.EXPECT().GetLatestInteractionByPromptID(gomock.Any(), "prompt_1").Return(&types.Interaction{ID: test.latestID}, nil)
			}
			if test.latestID == interaction.ID {
				st.EXPECT().ClearSessionProviderRecovery(gomock.Any(), session.ID, "prompt_1", "int_failed").Return(test.clearResult, nil)
			}

			cleared, err := server.clearProviderRecoveryAfterCompletedTurn(context.Background(), session, interaction)
			require.NoError(t, err)
			require.Equal(t, test.wantCleared, cleared)
		})
	}
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

func TestProviderFailureMatchesInteractionSnapshot(t *testing.T) {
	require.True(t, providerFailureMatchesSnapshot(
		&types.LLMCall{Provider: "bunker", Model: "bunker/glm-5.3-flash"},
		&types.InteractionCodeAgentConfigSnapshot{Provider: "bunker", Model: "glm-5.3-flash"},
	))
	require.False(t, providerFailureMatchesSnapshot(
		&types.LLMCall{Provider: "bunker", Model: "glm-5.3-flash"},
		&types.InteractionCodeAgentConfigSnapshot{Provider: "helixcyber", Model: "qwen3.8-27b"},
	))
	require.True(t, providerFailureMatchesSnapshot(
		&types.LLMCall{Provider: "anthropic", Model: "claude-opus-4-7"},
		&types.InteractionCodeAgentConfigSnapshot{Provider: "global/anthropic", Model: "claude-opus-4-7"},
	))
	require.False(t, providerFailureMatchesSnapshot(
		&types.LLMCall{Provider: "anthropic", Model: "claude-opus-4-7"},
		&types.InteractionCodeAgentConfigSnapshot{Provider: "user/anthropic", Model: "claude-opus-4-7"},
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
