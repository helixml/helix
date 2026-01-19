package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestBuildCodeAgentConfigFromAssistant(t *testing.T) {
	helixURL := "http://localhost:8080"

	tests := []struct {
		name      string
		assistant *types.AssistantConfig
		want      *types.CodeAgentConfig
	}{
		{
			name: "anthropic provider with zed_agent runtime",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-20250514",
				CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "openai provider with zed_agent runtime uses prefixed model",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "openai",
				GenerationModel:         "gpt-4o",
				CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
			},
			want: &types.CodeAgentConfig{
				Provider:  "openai",
				Model:     "openai/gpt-4o",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "openai",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "helix provider with qwen_code runtime",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "helix",
				GenerationModel:         "qwen3:8b",
				CodeAgentRuntime:        types.CodeAgentRuntimeQwenCode,
			},
			want: &types.CodeAgentConfig{
				Provider:  "helix",
				Model:     "helix/qwen3:8b",
				AgentName: "qwen",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "openai",
				Runtime:   types.CodeAgentRuntimeQwenCode,
			},
		},
		{
			name: "azure_openai provider",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "azure_openai",
				GenerationModel:         "gpt-4o",
				CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
			},
			want: &types.CodeAgentConfig{
				Provider:  "azure_openai",
				Model:     "gpt-4o",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/openai",
				APIType:   "azure_openai",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "defaults to zed_agent runtime when not specified",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-20250514",
				// CodeAgentRuntime not set
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "falls back to Provider/Model when GenerationModel fields empty",
			assistant: &types.AssistantConfig{
				Provider:         "anthropic",
				Model:            "claude-sonnet-4-20250514",
				CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "returns nil when no provider specified",
			assistant: &types.AssistantConfig{
				GenerationModel:  "claude-sonnet-4-20250514",
				CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
			},
			want: nil,
		},
		{
			name: "returns nil when no model specified",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "anthropic",
				CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCodeAgentConfigFromAssistant(tt.assistant, helixURL)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCodeAgentConfig(t *testing.T) {
	helixURL := "http://localhost:8080"

	tests := []struct {
		name string
		app  *types.App
		want *types.CodeAgentConfig
	}{
		{
			name: "returns config for zed_external assistant",
			app: &types.App{
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{
							{
								AgentType:               types.AgentTypeZedExternal,
								GenerationModelProvider: "anthropic",
								GenerationModel:         "claude-sonnet-4-20250514",
								CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
							},
						},
					},
				},
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "zed-agent",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeZedAgent,
			},
		},
		{
			name: "returns nil when no zed_external assistant",
			app: &types.App{
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{
							{
								AgentType: types.AgentTypeHelixBasic,
								Provider:  "anthropic",
								Model:     "claude-sonnet-4-20250514",
							},
						},
					},
				},
			},
			want: nil,
		},
		{
			name: "returns nil when app has no assistants",
			app: &types.App{
				Config: types.AppConfig{},
			},
			want: nil,
		},
		{
			name: "finds zed_external among multiple assistants",
			app: &types.App{
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{
							{
								AgentType: types.AgentTypeHelixBasic,
								Provider:  "openai",
								Model:     "gpt-4o",
							},
							{
								AgentType:               types.AgentTypeZedExternal,
								GenerationModelProvider: "helix",
								GenerationModel:         "qwen3:8b",
								CodeAgentRuntime:        types.CodeAgentRuntimeQwenCode,
							},
						},
					},
				},
			},
			want: &types.CodeAgentConfig{
				Provider:  "helix",
				Model:     "helix/qwen3:8b",
				AgentName: "qwen",
				BaseURL:   "http://localhost:8080/v1",
				APIType:   "openai",
				Runtime:   types.CodeAgentRuntimeQwenCode,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCodeAgentConfig(tt.app, helixURL)
			assert.Equal(t, tt.want, got)
		})
	}
}
