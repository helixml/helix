package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBuildCodeAgentConfigFromAssistant(t *testing.T) {
	helixURL := "http://localhost:8080"
	ctx := context.Background()
	// Create a minimal HelixAPIServer with nil modelInfoProvider
	// (token limits will be 0 but the rest of the config will be correct)
	apiServer := &HelixAPIServer{}

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
				ReasoningEffort:         types.ReasoningEffortNone,
			},
			want: &types.CodeAgentConfig{
				Provider:        "openai",
				Model:           "openai/gpt-4o",
				AgentName:       "zed-agent",
				BaseURL:         "http://localhost:8080/v1",
				APIType:         "openai",
				Runtime:         types.CodeAgentRuntimeZedAgent,
				ReasoningEffort: types.ReasoningEffortNone,
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
		{
			name: "claude_code subscription mode - defaults to 1M-context Opus",
			assistant: &types.AssistantConfig{
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			},
			want: &types.CodeAgentConfig{
				AgentName:        "claude",
				Model:            "claude-opus-5",
				Runtime:          types.CodeAgentRuntimeClaudeCode,
				UsesSubscription: true,
			},
		},
		{
			name: "claude_code subscription mode - honours ClaudeSubscriptionModel override",
			assistant: &types.AssistantConfig{
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
				ClaudeSubscriptionModel: "claude-haiku-4-5-latest",
			},
			want: &types.CodeAgentConfig{
				AgentName:        "claude",
				Model:            "claude-haiku-4-5-latest",
				Runtime:          types.CodeAgentRuntimeClaudeCode,
				UsesSubscription: true,
			},
		},
		{
			name: "claude_code subscription mode - ignores legacy Provider/Model fields",
			assistant: &types.AssistantConfig{
				Provider:                "anthropic",
				Model:                   "claude-sonnet-4-20250514",
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			},
			want: &types.CodeAgentConfig{
				AgentName:        "claude",
				Model:            "claude-opus-5",
				Runtime:          types.CodeAgentRuntimeClaudeCode,
				UsesSubscription: true,
			},
		},
		{
			name: "claude_code api_key mode - explicit credential type",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-20250514",
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "claude",
				BaseURL:   "http://localhost:8080",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeClaudeCode,
			},
		},
		{
			name: "claude_code api_key mode - generation model overrides stale top-level model",
			assistant: &types.AssistantConfig{
				Provider:                "anthropic",
				Model:                   "claude-opus-4-8",
				GenerationModelProvider: "scope-provider",
				GenerationModel:         "scope-e2e-model",
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			want: &types.CodeAgentConfig{
				Provider:  "scope-provider",
				Model:     "scope-e2e-model",
				AgentName: "claude",
				BaseURL:   "http://localhost:8080",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeClaudeCode,
			},
		},
		{
			name: "claude_code api_key mode - default when credential type empty",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-20250514",
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				// No CodeAgentCredentialType set = defaults to api_key
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-sonnet-4-20250514",
				AgentName: "claude",
				BaseURL:   "http://localhost:8080",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeClaudeCode,
			},
		},
		{
			name: "claude_code legacy empty credential - top-level model wins over stale generation model",
			assistant: &types.AssistantConfig{
				Provider:                "anthropic",
				Model:                   "claude-opus-4-8",
				GenerationModelProvider: "stale-provider",
				GenerationModel:         "stale-model",
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
			},
			want: &types.CodeAgentConfig{
				Provider:  "anthropic",
				Model:     "claude-opus-4-8",
				AgentName: "claude",
				BaseURL:   "http://localhost:8080",
				APIType:   "anthropic",
				Runtime:   types.CodeAgentRuntimeClaudeCode,
			},
		},
		{
			name: "codex_cli subscription mode",
			assistant: &types.AssistantConfig{
				CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			},
			want: &types.CodeAgentConfig{AgentName: "codex", Runtime: types.CodeAgentRuntimeCodexCLI, UsesSubscription: true},
		},
		{
			name: "codex_cli api key mode",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "openai",
				GenerationModel:         "gpt-5.3-codex",
				CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
				ReasoningEffort:         "high",
			},
			want: &types.CodeAgentConfig{
				Provider: "openai", Model: "gpt-5.3-codex", AgentName: "codex", ReasoningEffort: "high",
				BaseURL: "http://localhost:8080/v1", APIType: "openai", Runtime: types.CodeAgentRuntimeCodexCLI,
			},
		},
		{
			name: "codex_cli omits none reasoning effort",
			assistant: &types.AssistantConfig{
				GenerationModelProvider: "openai",
				GenerationModel:         "gpt-5.3-codex",
				CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
				ReasoningEffort:         types.ReasoningEffortNone,
			},
			want: &types.CodeAgentConfig{
				Provider: "openai", Model: "gpt-5.3-codex", AgentName: "codex",
				BaseURL: "http://localhost:8080/v1", APIType: "openai", Runtime: types.CodeAgentRuntimeCodexCLI,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiServer.buildCodeAgentConfigFromAssistant(ctx, tt.assistant, helixURL, nil)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCodeAgentConfig(t *testing.T) {
	helixURL := "http://localhost:8080"
	ctx := context.Background()
	// Create a minimal HelixAPIServer with nil modelInfoProvider
	apiServer := &HelixAPIServer{}

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
			got := apiServer.buildCodeAgentConfig(ctx, tt.app, helixURL, "")
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildCodeAgentConfigProviderAdvertisedContextLength(t *testing.T) {
	const (
		providerName          = "ds4-flash-node06"
		modelName             = "deepseek-v4-flash"
		providerBaseURL       = "http://ds4-loadbalancer/v1"
		catalogueContext      = 1048576
		catalogueOutputTokens = 131072
	)

	tests := []struct {
		name                 string
		advertisedContext    int
		catalogueUnavailable bool
		providerListErr      error
		providerModelsErr    error
		wantContext          int
		wantOutputTokens     int
	}{
		{
			name:              "advertised context overrides larger catalogue value",
			advertisedContext: 262144,
			wantContext:       262144,
			wantOutputTokens:  catalogueOutputTokens,
		},
		{
			name:                 "advertised context works without catalogue metadata",
			advertisedContext:    262144,
			catalogueUnavailable: true,
			wantContext:          262144,
		},
		{
			name:              "zero advertised context falls back to catalogue",
			advertisedContext: 0,
			wantContext:       catalogueContext,
			wantOutputTokens:  catalogueOutputTokens,
		},
		{
			name:             "provider endpoint lookup failure falls back to catalogue",
			providerListErr:  errors.New("provider registry unavailable"),
			wantContext:      catalogueContext,
			wantOutputTokens: catalogueOutputTokens,
		},
		{
			name:              "provider models failure falls back to catalogue",
			providerModelsErr: errors.New("models endpoint unavailable"),
			wantContext:       catalogueContext,
			wantOutputTokens:  catalogueOutputTokens,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			providerManager := manager.NewMockProviderManager(ctrl)
			providerClient := openai.NewMockClient(ctrl)
			modelInfoProvider := model.NewMockModelInfoProvider(ctrl)

			cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
				NumCounters: 1e3,
				MaxCost:     1 << 20,
				BufferItems: 64,
			})
			require.NoError(t, err)
			defer cache.Close()

			cfg := &config.ServerConfig{}
			cfg.WebServer.ModelsCacheTTL = time.Minute
			apiServer := &HelixAPIServer{
				Cfg:               cfg,
				providerManager:   providerManager,
				modelInfoProvider: modelInfoProvider,
				cache:             cache,
			}

			endpoint := &types.ProviderEndpoint{
				ID:      "pe_ds4",
				Name:    providerName,
				Owner:   "user-1",
				BaseURL: providerBaseURL,
			}

			providerManager.EXPECT().
				ListProviderEndpoints(gomock.Any(), "user-1").
				Return(func() []*types.ProviderEndpoint {
					if tt.providerListErr != nil {
						return nil
					}
					return []*types.ProviderEndpoint{endpoint}
				}(), tt.providerListErr)

			if tt.providerListErr == nil {
				providerManager.EXPECT().
					GetClient(gomock.Any(), &manager.GetClientRequest{Provider: providerName, Owner: "user-1"}).
					Return(providerClient, nil)
				providerClient.EXPECT().
					ListModels(gomock.Any()).
					Return(func() []types.OpenAIModel {
						if tt.providerModelsErr != nil {
							return nil
						}
						return []types.OpenAIModel{{ID: modelName, ContextLength: tt.advertisedContext}}
					}(), tt.providerModelsErr)
			}

			modelInfoProvider.EXPECT().
				GetModelInfo(gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ *model.ModelInfoRequest) (*types.ModelInfo, error) {
					if tt.catalogueUnavailable {
						return nil, errors.New("model not in catalogue")
					}
					return &types.ModelInfo{
						ContextLength:       catalogueContext,
						MaxCompletionTokens: catalogueOutputTokens,
					}, nil
				}).AnyTimes()

			app := &types.App{
				Owner: "user-1",
				Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
					AgentType:               types.AgentTypeZedExternal,
					GenerationModelProvider: providerName,
					GenerationModel:         modelName,
					CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
				}}}},
			}

			got := apiServer.buildCodeAgentConfig(context.Background(), app, "http://helix-api:8080", "")
			require.NotNil(t, got)
			assert.Equal(t, tt.wantContext, got.MaxTokens)
			assert.Equal(t, tt.wantOutputTokens, got.MaxOutputTokens)
			assert.Equal(t, providerName+"/"+modelName, got.Model)
		})
	}
}
