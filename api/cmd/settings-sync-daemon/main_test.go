package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectLanguageModelAPIKey(t *testing.T) {
	tests := []struct {
		name           string
		apiToken       string
		helixSettings  map[string]interface{}
		wantAPIKeySet  bool
		wantProviders  []string // providers that should have api_key
	}{
		{
			name:     "injects api_key into single provider",
			apiToken: "test-token",
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantAPIKeySet: true,
			wantProviders: []string{"openai"},
		},
		{
			name:     "injects api_key into multiple providers",
			apiToken: "test-token",
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
					"anthropic": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantAPIKeySet: true,
			wantProviders: []string{"openai", "anthropic"},
		},
		{
			name:     "does nothing when apiToken is empty",
			apiToken: "",
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantAPIKeySet: false,
			wantProviders: []string{},
		},
		{
			name:     "does nothing when language_models is missing",
			apiToken: "test-token",
			helixSettings: map[string]interface{}{
				"theme": "dark",
			},
			wantAPIKeySet: false,
			wantProviders: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &SettingsDaemon{
				apiToken:      tt.apiToken,
				helixSettings: tt.helixSettings,
			}

			d.injectLanguageModelAPIKey()

			// Check if api_key was set in the expected providers
			if languageModels, ok := d.helixSettings["language_models"].(map[string]interface{}); ok {
				for _, provider := range tt.wantProviders {
					if providerConfig, ok := languageModels[provider].(map[string]interface{}); ok {
						if tt.wantAPIKeySet {
							assert.Equal(t, tt.apiToken, providerConfig["api_key"], "api_key should be set for %s", provider)
						} else {
							assert.Nil(t, providerConfig["api_key"], "api_key should not be set for %s", provider)
						}
					}
				}
			}
		})
	}
}

func TestInjectAvailableModels(t *testing.T) {
	tests := []struct {
		name            string
		codeAgentConfig *CodeAgentConfig
		helixSettings   map[string]interface{}
		wantModel       string
		wantProvider    string
	}{
		{
			name: "adds model to existing openai provider",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "helix/qwen3:8b",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantModel:    "helix/qwen3:8b",
			wantProvider: "openai",
		},
		{
			name: "creates provider config if missing",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "helix/qwen3:8b",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{},
			},
			wantModel:    "helix/qwen3:8b",
			wantProvider: "openai",
		},
		{
			name: "defaults to openai provider when APIType is empty",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "custom-model",
				APIType: "",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{},
			},
			wantModel:    "custom-model",
			wantProvider: "openai",
		},
		{
			name: "adds to anthropic provider",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "claude-custom",
				APIType: "anthropic",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"anthropic": map[string]interface{}{
						"api_url": "http://localhost:8080/v1",
					},
				},
			},
			wantModel:    "claude-custom",
			wantProvider: "anthropic",
		},
		{
			name:            "does nothing when codeAgentConfig is nil",
			codeAgentConfig: nil,
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "",
			wantProvider: "",
		},
		{
			name: "does nothing when model is empty",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "",
			wantProvider: "",
		},
		{
			name: "does not duplicate model if already exists",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "existing-model",
				APIType: "openai",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{
						"available_models": []interface{}{
							map[string]interface{}{
								"name":              "existing-model",
								"display_name":      "existing-model",
								"max_tokens":        131072,
								"max_output_tokens": 16384,
							},
						},
					},
				},
			},
			wantModel:    "existing-model",
			wantProvider: "openai",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &SettingsDaemon{
				codeAgentConfig: tt.codeAgentConfig,
				helixSettings:   tt.helixSettings,
			}

			d.injectAvailableModels()

			// Verify the model was added to the correct provider
			if tt.wantModel == "" || tt.wantProvider == "" {
				// Expected no changes
				return
			}

			languageModels := d.helixSettings["language_models"].(map[string]interface{})
			providerConfig := languageModels[tt.wantProvider].(map[string]interface{})
			availableModels := providerConfig["available_models"].([]interface{})

			// Find the model
			found := false
			for _, m := range availableModels {
				model := m.(map[string]interface{})
				if model["name"] == tt.wantModel {
					found = true
					assert.Equal(t, tt.wantModel, model["display_name"], "display_name should match model name")
					assert.NotNil(t, model["max_tokens"], "max_tokens should be set")
					assert.NotNil(t, model["max_output_tokens"], "max_output_tokens should be set")
					break
				}
			}
			assert.True(t, found, "model %s should be in available_models", tt.wantModel)

			// For the duplicate test, ensure there's only one entry
			if tt.name == "does not duplicate model if already exists" {
				count := 0
				for _, m := range availableModels {
					model := m.(map[string]interface{})
					if model["name"] == tt.wantModel {
						count++
					}
				}
				assert.Equal(t, 1, count, "should not duplicate model")
			}
		})
	}
}
