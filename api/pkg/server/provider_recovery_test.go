package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
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
