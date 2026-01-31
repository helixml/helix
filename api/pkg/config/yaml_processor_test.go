package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessYAMLConfig_AgentMode(t *testing.T) {
	yamlContent := `
apiVersion: app.aispec.org/v1alpha1
kind: AIApp
metadata:
  name: Test Agent
spec:
  assistants:
    - name: Test Assistant
      agent_mode: true
      max_iterations: 15
      reasoning_model_provider: openai
      reasoning_model: o3-mini
      reasoning_model_effort: medium
      generation_model_provider: openai
      generation_model: gpt-4o
      small_reasoning_model_provider: openai
      small_reasoning_model: o3-mini
      small_reasoning_model_effort: low
      small_generation_model_provider: openai
      small_generation_model: gpt-4o-mini
      system_prompt: "You are a test assistant"
`

	config, err := ProcessYAMLConfig([]byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify the config was parsed correctly
	assert.Equal(t, "Test Agent", config.Name)
	require.Len(t, config.Assistants, 1)

	assistant := config.Assistants[0]
	assert.Equal(t, "Test Assistant", assistant.Name)
	assert.True(t, assistant.AgentMode, "agent_mode should be true")
	assert.Equal(t, 15, assistant.MaxIterations)
	assert.Equal(t, "openai", assistant.ReasoningModelProvider)
	assert.Equal(t, "o3-mini", assistant.ReasoningModel)
	assert.Equal(t, "medium", assistant.ReasoningModelEffort)
	assert.Equal(t, "openai", assistant.GenerationModelProvider)
	assert.Equal(t, "gpt-4o", assistant.GenerationModel)
	assert.Equal(t, "openai", assistant.SmallReasoningModelProvider)
	assert.Equal(t, "o3-mini", assistant.SmallReasoningModel)
	assert.Equal(t, "low", assistant.SmallReasoningModelEffort)
	assert.Equal(t, "openai", assistant.SmallGenerationModelProvider)
	assert.Equal(t, "gpt-4o-mini", assistant.SmallGenerationModel)
	assert.Equal(t, "You are a test assistant", assistant.SystemPrompt)
}

func TestProcessYAMLConfig_AgentModeFalse(t *testing.T) {
	yamlContent := `
apiVersion: app.aispec.org/v1alpha1
kind: AIApp
metadata:
  name: Test Agent
spec:
  assistants:
    - name: Test Assistant
      agent_mode: false
      model: gpt-4o
      provider: openai
      system_prompt: "You are a test assistant"
`

	config, err := ProcessYAMLConfig([]byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify the config was parsed correctly
	assert.Equal(t, "Test Agent", config.Name)
	require.Len(t, config.Assistants, 1)

	assistant := config.Assistants[0]
	assert.Equal(t, "Test Assistant", assistant.Name)
	assert.False(t, assistant.AgentMode, "agent_mode should be false")
	assert.Equal(t, "gpt-4o", assistant.Model)
	assert.Equal(t, "openai", assistant.Provider)
	assert.Equal(t, "You are a test assistant", assistant.SystemPrompt)
}

func TestProcessYAMLConfig_AgentModeDefault(t *testing.T) {
	yamlContent := `
apiVersion: app.aispec.org/v1alpha1
kind: AIApp
metadata:
  name: Test Agent
spec:
  assistants:
    - name: Test Assistant
      model: gpt-4o
      provider: openai
      system_prompt: "You are a test assistant"
`

	config, err := ProcessYAMLConfig([]byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Verify the config was parsed correctly
	assert.Equal(t, "Test Agent", config.Name)
	require.Len(t, config.Assistants, 1)

	assistant := config.Assistants[0]
	assert.Equal(t, "Test Assistant", assistant.Name)
	assert.False(t, assistant.AgentMode, "agent_mode should default to false")
	assert.Equal(t, "gpt-4o", assistant.Model)
	assert.Equal(t, "openai", assistant.Provider)
	assert.Equal(t, "You are a test assistant", assistant.SystemPrompt)
}

func TestProcessYAMLConfig_AllFields(t *testing.T) {
	yamlContent := `
apiVersion: app.aispec.org/v1alpha1
kind: AIApp
metadata:
  name: Comprehensive Test Agent
spec:
  assistants:
    - name: Test Assistant
      agent_mode: true
      max_iterations: 10
      reasoning_model_provider: openai
      reasoning_model: o3-mini
      reasoning_model_effort: high
      generation_model_provider: openai
      generation_model: gpt-4o
      small_reasoning_model_provider: openai
      small_reasoning_model: o3-mini
      small_reasoning_model_effort: low
      small_generation_model_provider: openai
      small_generation_model: gpt-4o-mini
      presence_penalty: 0.5
      frequency_penalty: 0.3
      top_p: 0.8
      max_tokens: 2000
      reasoning_effort: medium
  triggers:
    - cron:
        enabled: true
        schedule: "0 */6 * * *"
        input: "Daily check"
    - discord:
        server_name: "test-server"
`

	config, err := ProcessYAMLConfig([]byte(yamlContent))
	require.NoError(t, err)
	require.NotNil(t, config)

	// Check assistants
	require.Len(t, config.Assistants, 1)
	assistant := config.Assistants[0]

	// Check agent mode fields
	assert.True(t, assistant.AgentMode)
	assert.Equal(t, 10, assistant.MaxIterations)
	assert.Equal(t, "openai", assistant.ReasoningModelProvider)
	assert.Equal(t, "o3-mini", assistant.ReasoningModel)
	assert.Equal(t, "high", assistant.ReasoningModelEffort)
	assert.Equal(t, "openai", assistant.GenerationModelProvider)
	assert.Equal(t, "gpt-4o", assistant.GenerationModel)
	assert.Equal(t, "openai", assistant.SmallReasoningModelProvider)
	assert.Equal(t, "o3-mini", assistant.SmallReasoningModel)
	assert.Equal(t, "low", assistant.SmallReasoningModelEffort)
	assert.Equal(t, "openai", assistant.SmallGenerationModelProvider)
	assert.Equal(t, "gpt-4o-mini", assistant.SmallGenerationModel)

	// Check model parameter fields
	assert.Equal(t, float32(0.5), assistant.PresencePenalty)
	assert.Equal(t, float32(0.3), assistant.FrequencyPenalty)
	assert.Equal(t, float32(0.8), assistant.TopP)
	assert.Equal(t, 2000, assistant.MaxTokens)
	assert.Equal(t, "medium", assistant.ReasoningEffort)

	// Check triggers
	require.Len(t, config.Triggers, 2)

	// Check cron trigger
	cronTrigger := config.Triggers[0]
	require.NotNil(t, cronTrigger.Cron)
	assert.True(t, cronTrigger.Cron.Enabled)
	assert.Equal(t, "0 */6 * * *", cronTrigger.Cron.Schedule)
	assert.Equal(t, "Daily check", cronTrigger.Cron.Input)

	// Check discord trigger
	discordTrigger := config.Triggers[1]
	require.NotNil(t, discordTrigger.Discord)
	assert.Equal(t, "test-server", discordTrigger.Discord.ServerName)
}
