package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestIsDeferredNativeHarnessProjectConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *types.CodeAgentExecutionConfig
		want   bool
	}{
		{
			name: "codex",
			config: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			want: true,
		},
		{
			name: "claude",
			config: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeClaudeCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			want: true,
		},
		{
			name: "complete codex config",
			config: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
				ProviderRef:    "openai",
				Model:          "gpt-5.6-sol",
			},
		},
		{
			name: "generic harness",
			config: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeOpenCode,
				CredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			want: true,
		},
		{
			name: "subscription",
			config: &types.CodeAgentExecutionConfig{
				Runtime:        types.CodeAgentRuntimeCodexCLI,
				CredentialType: types.CodeAgentCredentialTypeSubscription,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isDeferredNativeHarnessProjectConfig(tt.config))
		})
	}
}

func TestApplySpecTaskExecutionConfigDoesNotRestartInactivePhase(t *testing.T) {
	server, memoryStore := newForkTestServer(t)
	planner := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeClaudeCode, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "planner",
	}
	oldImplementer := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeZedAgent, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "old-implementer",
	}
	newImplementer := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeCodexCLI, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "new-implementer",
	}
	task := &types.SpecTask{
		ID: "task_1", Status: types.TaskStatusSpecGeneration,
		PlanningCodeAgentConfig: planner, CodeAgentConfig: oldImplementer,
	}
	session := newTestParentSession("user_a")
	session.Metadata.CodeAgentRuntime = planner.Runtime
	memoryStore.SeedSpecTask(task)

	changed, restarted, httpErr := server.applySpecTaskExecutionConfig(
		context.Background(), &types.User{ID: "user_a"}, task, session,
		types.SpecTaskPhaseImplementation, newImplementer, "test handoff",
	)

	require.Nil(t, httpErr)
	require.True(t, changed)
	require.False(t, restarted)
	require.Equal(t, planner.Runtime, session.Metadata.CodeAgentRuntime)
	updated, err := memoryStore.GetSpecTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, newImplementer, updated.CodeAgentConfig)
	require.Equal(t, planner, updated.PlanningCodeAgentConfig)
}

func TestApplyPlanningConfigPreservesLegacyImplementationSource(t *testing.T) {
	server, memoryStore := newForkTestServer(t)
	planner := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeClaudeCode, CredentialType: types.CodeAgentCredentialTypeSubscription, Model: "planner",
	}
	legacyOverrides := &types.CodeAgentOverrides{Model: "legacy-implementer"}
	task := &types.SpecTask{
		ID: "task_legacy", Status: types.TaskStatusBacklog,
		HelixAppID: "legacy_app", CodeAgentOverrides: legacyOverrides,
	}
	memoryStore.SeedSpecTask(task)

	changed, restarted, httpErr := server.applySpecTaskExecutionConfig(
		context.Background(), &types.User{ID: "user_a"}, task, nil,
		types.SpecTaskPhasePlanning, planner, "test handoff",
	)

	require.Nil(t, httpErr)
	require.True(t, changed)
	require.False(t, restarted)
	updated, err := memoryStore.GetSpecTask(context.Background(), task.ID)
	require.NoError(t, err)
	require.Equal(t, planner, updated.PlanningCodeAgentConfig)
	require.Equal(t, "legacy_app", updated.HelixAppID)
	require.Equal(t, legacyOverrides, updated.CodeAgentOverrides)
}
