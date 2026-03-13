package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInjectAvailableModels(t *testing.T) {
	tests := []struct {
		name            string
		codeAgentConfig *CodeAgentConfig
		helixSettings   map[string]interface{}
		wantModel       string
		wantProvider    string
		wantSkipped     bool // true if injection should be skipped (e.g. anthropic built-in)
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
			name: "skips injection for anthropic provider — Zed has built-in definitions",
			codeAgentConfig: &CodeAgentConfig{
				Model:   "claude-opus-4-6",
				APIType: "anthropic",
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"anthropic": map[string]interface{}{
						"api_url": "http://localhost:8080",
					},
				},
			},
			wantSkipped: true,
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
		{
			name: "uses 200K fallback when MaxTokens is 0",
			codeAgentConfig: &CodeAgentConfig{
				Model:     "nebius/some-model",
				APIType:   "openai",
				MaxTokens: 0,
			},
			helixSettings: map[string]interface{}{
				"language_models": map[string]interface{}{
					"openai": map[string]interface{}{},
				},
			},
			wantModel:    "nebius/some-model",
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

			if tt.wantSkipped {
				// Anthropic models should NOT be injected — verify no available_models added
				languageModels, ok := d.helixSettings["language_models"].(map[string]interface{})
				if ok {
					if providerConfig, ok := languageModels["anthropic"].(map[string]interface{}); ok {
						availableModels, exists := providerConfig["available_models"]
						if exists {
							assert.Nil(t, availableModels, "available_models should not be set for anthropic provider")
						}
					}
				}
				return
			}

			// Expected no changes
			if tt.wantModel == "" || tt.wantProvider == "" {
				return
			}

			languageModels := d.helixSettings["language_models"].(map[string]interface{})
			providerConfig := languageModels[tt.wantProvider].(map[string]interface{})
			availableModels := providerConfig["available_models"].([]interface{})

			// Helper to get model name from either map or struct
			getModelName := func(m interface{}) string {
				if model, ok := m.(map[string]interface{}); ok {
					return model["name"].(string)
				}
				if model, ok := m.(AvailableModel); ok {
					return model.Name
				}
				return ""
			}

			// Helper to get max_tokens from either map or struct
			getMaxTokens := func(m interface{}) int {
				if model, ok := m.(map[string]interface{}); ok {
					if v, ok := model["max_tokens"].(int); ok {
						return v
					}
				}
				if model, ok := m.(AvailableModel); ok {
					return model.MaxTokens
				}
				return 0
			}

			// Find the model
			found := false
			for _, m := range availableModels {
				if getModelName(m) == tt.wantModel {
					found = true
					// Check fields based on type
					if model, ok := m.(AvailableModel); ok {
						assert.Equal(t, tt.wantModel, model.DisplayName, "display_name should match model name")
						assert.NotZero(t, model.MaxTokens, "max_tokens should be set")
					} else if model, ok := m.(map[string]interface{}); ok {
						assert.Equal(t, tt.wantModel, model["display_name"], "display_name should match model name")
						assert.NotNil(t, model["max_tokens"], "max_tokens should be set")
					}
					break
				}
			}
			assert.True(t, found, "model %s should be in available_models", tt.wantModel)

			// For the duplicate test, ensure there's only one entry
			if tt.name == "does not duplicate model if already exists" {
				count := 0
				for _, m := range availableModels {
					if getModelName(m) == tt.wantModel {
						count++
					}
				}
				assert.Equal(t, 1, count, "should not duplicate model")
			}

			// For the 200K fallback test, verify the default is applied
			if tt.name == "uses 200K fallback when MaxTokens is 0" {
				for _, m := range availableModels {
					if getModelName(m) == tt.wantModel {
						maxTokens := getMaxTokens(m)
						assert.Equal(t, 200000, maxTokens, "should use 200K fallback when MaxTokens is 0")
						break
					}
				}
			}
		})
	}
}
