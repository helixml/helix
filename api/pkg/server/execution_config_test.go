package server

import (
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
