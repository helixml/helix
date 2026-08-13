package server

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func (s *WebSocketSyncSuite) TestRecordACPUsage_RecordsSubscriptionTurn() {
	created := time.Now().Add(-2 * time.Second)
	session := &types.Session{
		ID:             "ses_123",
		Owner:          "user_123",
		ParentApp:      "app_123",
		ProjectID:      "prj_123",
		OrganizationID: "org_123",
		Metadata:       types.SessionMetadata{SpecTaskID: "task_123"},
	}
	interaction := &types.Interaction{
		ID:        "int_123",
		Created:   created,
		Completed: created.Add(2 * time.Second),
	}
	app := &types.App{ID: "app_123", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			Model:                   "gpt-5.3-codex",
		}},
	}}}
	syncMsg := &types.SyncMessage{Data: map[string]interface{}{
		"usage": map[string]interface{}{
			"total_tokens":        float64(175),
			"input_tokens":        float64(100),
			"output_tokens":       float64(75),
			"cached_read_tokens":  float64(40),
			"cached_write_tokens": float64(10),
		},
	}}

	s.store.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(&types.SpecTask{
		ID:         "task_123",
		HelixAppID: "app_123",
		CodeAgentOverrides: &types.CodeAgentOverrides{
			Model: "gpt-5.6-luna",
		},
	}, nil)
	s.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)
	s.store.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.Equal(s.T(), types.UsageMetricSourceACP, metric.Source)
			assert.Equal(s.T(), "ses_123:int_123", metric.SourceID)
			assert.True(s.T(), metric.UsageKnown)
			assert.Equal(s.T(), "openai", metric.Provider)
			assert.Equal(s.T(), "gpt-5.6-luna", metric.Model)
			assert.Equal(s.T(), 100, metric.PromptTokens)
			assert.Equal(s.T(), 75, metric.CompletionTokens)
			assert.Equal(s.T(), 175, metric.TotalTokens)
			assert.Equal(s.T(), 40, metric.CacheReadTokens)
			assert.Equal(s.T(), 10, metric.CacheWriteTokens)
			assert.Equal(s.T(), 2000, metric.DurationMs)
			assert.Equal(s.T(), "task_123", metric.SpecTaskID)
			assert.Equal(s.T(), "user_123", metric.UserID)
			return metric, nil
		},
	)

	err := s.server.recordACPUsage(context.Background(), session, interaction, syncMsg)
	require.NoError(s.T(), err)
}

func TestApplyACPInteractionUsage(t *testing.T) {
	interaction := &types.Interaction{}
	syncMsg := &types.SyncMessage{Data: map[string]interface{}{
		"usage": map[string]interface{}{
			"total_tokens":  float64(175),
			"input_tokens":  float64(100),
			"output_tokens": float64(75),
		},
		"context_usage": map[string]interface{}{
			"used_tokens": float64(120_000),
			"max_tokens":  float64(200_000),
		},
	}}

	err := applyACPInteractionUsage(interaction, syncMsg)
	require.NoError(t, err)
	assert.Equal(t, 100, interaction.Usage.PromptTokens)
	assert.Equal(t, 75, interaction.Usage.CompletionTokens)
	assert.Equal(t, 175, interaction.Usage.TotalTokens)
	assert.Equal(t, 120_000, interaction.Usage.ContextTokens)
	assert.Equal(t, 200_000, interaction.Usage.ContextLength)
}

func (s *WebSocketSyncSuite) TestRecordACPUsage_RecordsUnknownUsageAsActivity() {
	app := &types.App{ID: "app_123", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
		}},
	}}}
	s.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)
	s.store.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.False(s.T(), metric.UsageKnown)
			assert.Zero(s.T(), metric.TotalTokens)
			assert.Equal(s.T(), "anthropic", metric.Provider)
			assert.Equal(s.T(), "claude-subscription", metric.Model)
			return metric, nil
		},
	)

	err := s.server.recordACPUsage(
		context.Background(),
		&types.Session{ID: "ses_123", ParentApp: "app_123"},
		&types.Interaction{ID: "int_123"},
		&types.SyncMessage{Data: map[string]interface{}{"agent_name": "claude-acp"}},
	)
	require.NoError(s.T(), err)
}

func (s *WebSocketSyncSuite) TestRecordACPUsage_UsesInteractionSnapshotAfterModelSwitch() {
	s.store.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.Equal(s.T(), "app_luna", metric.AppID)
			assert.Equal(s.T(), "openai", metric.Provider)
			assert.Equal(s.T(), "gpt-5.6-luna", metric.Model)
			return metric, nil
		},
	)

	err := s.server.recordACPUsage(
		context.Background(),
		&types.Session{
			ID:        "ses_123",
			ParentApp: "app_terra",
			Metadata:  types.SessionMetadata{SpecTaskID: "task_123"},
		},
		&types.Interaction{
			ID: "int_123",
			CodeAgentConfigSnapshot: &types.InteractionCodeAgentConfigSnapshot{
				AppID:          "app_luna",
				Provider:       "openai",
				Model:          "gpt-5.6-luna",
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeSubscription,
			},
		},
		&types.SyncMessage{Data: map[string]interface{}{
			"usage": map[string]interface{}{"total_tokens": float64(1)},
		}},
	)
	require.NoError(s.T(), err)
}

// Zed's message_completed carries neither `usage` nor `agent_name` when the
// binary predates the ACP turn-usage change. The turn must still be recorded so
// request counts and activity remain visible.
func (s *WebSocketSyncSuite) TestRecordACPUsage_RecordsTurnWithoutUsageOrAgentName() {
	app := &types.App{ID: "app_123", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			Model:                   "gpt-5.6-sol",
		}},
	}}}
	s.store.EXPECT().GetSpecTask(gomock.Any(), "task_123").Return(&types.SpecTask{
		ID:         "task_123",
		HelixAppID: "app_123",
	}, nil)
	s.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)
	s.store.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.Equal(s.T(), types.UsageMetricSourceACP, metric.Source)
			assert.Equal(s.T(), "ses_123:int_123", metric.SourceID)
			assert.False(s.T(), metric.UsageKnown)
			assert.Zero(s.T(), metric.TotalTokens)
			assert.Equal(s.T(), "prj_123", metric.ProjectID)
			assert.Equal(s.T(), "task_123", metric.SpecTaskID)
			return metric, nil
		},
	)

	err := s.server.recordACPUsage(
		context.Background(),
		&types.Session{
			ID:        "ses_123",
			ParentApp: "app_123",
			ProjectID: "prj_123",
			Metadata:  types.SessionMetadata{SpecTaskID: "task_123"},
		},
		&types.Interaction{ID: "int_123"},
		&types.SyncMessage{Data: map[string]interface{}{
			"acp_thread_id": "019fc6c4-33b7-7671-bfc0-0358c7d225ba",
			"message_id":    "0",
			"request_id":    "req_123",
		}},
	)
	require.NoError(s.T(), err)
}

func (s *WebSocketSyncSuite) TestRecordACPUsage_SkipsAPIKeyAgent() {
	app := &types.App{ID: "app_123", Config: types.AppConfig{Helix: types.AppHelixConfig{
		Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
		}},
	}}}
	s.store.EXPECT().GetApp(gomock.Any(), "app_123").Return(app, nil)

	err := s.server.recordACPUsage(
		context.Background(),
		&types.Session{ID: "ses_123", ParentApp: "app_123"},
		&types.Interaction{ID: "int_123"},
		&types.SyncMessage{Data: map[string]interface{}{"agent_name": "codex-acp"}},
	)
	require.NoError(s.T(), err)
}

func TestACPUsageProviderAndModel(t *testing.T) {
	provider, model := acpUsageProviderAndModel(&types.AssistantConfig{
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		ClaudeSubscriptionModel: "claude-opus-4-6",
	})
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "claude-opus-4-6", model)
}
