package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCodeAgentModelCompatibility(t *testing.T) {
	tests := []struct {
		name      string
		assistant AssistantConfig
		wantError string
	}{
		{name: "codex current model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI, Model: "gpt-5.6-sol"}},
		{name: "codex provider-prefixed model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI, GenerationModel: "openai/gpt-5.3-codex-spark"}},
		{name: "codex legacy model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI, Model: "codex-mini-latest"}},
		{name: "codex default model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI}},
		{name: "codex rejects claude", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI, Model: "claude-opus-4-8"}, wantError: "requires a Codex model"},
		{name: "codex rejects generic model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeCodexCLI, Model: "llama-4"}, wantError: "requires a Codex model"},
		{name: "claude current model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, Model: "claude-opus-4-8"}},
		{name: "claude provider-prefixed model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, GenerationModel: "anthropic/claude-sonnet-4-6"}},
		{name: "claude bedrock model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, Model: "us.anthropic.claude-opus-4-6-v1"}},
		{name: "claude subscription alias", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, ClaudeSubscriptionModel: "opus[1m]"}},
		{name: "claude sonnet alias", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, ClaudeSubscriptionModel: "sonnet"}},
		{name: "claude default model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode}},
		{name: "claude rejects codex", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, Model: "gpt-5.6-sol"}, wantError: "requires an Anthropic Claude model"},
		{name: "claude rejects generic model", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeClaudeCode, Model: "llama-4"}, wantError: "requires an Anthropic Claude model"},
		{name: "qwen remains permissive", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeQwenCode, Model: "claude-opus-4-8"}},
		{name: "zed remains permissive", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeZedAgent, Model: "llama-4"}},
		{name: "goose remains permissive", assistant: AssistantConfig{CodeAgentRuntime: CodeAgentRuntimeGooseCode, Model: "custom-model"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCodeAgentModelCompatibility(tt.assistant)
			if tt.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantError)
		})
	}
}
