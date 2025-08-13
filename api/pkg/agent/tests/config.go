package tests

import (
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"

	helix_openai "github.com/helixml/helix/api/pkg/openai"
)

type Config struct {
	OpenAIAPIKey string `envconfig:"OPENAI_API_KEY"`
	BaseURL      string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`

	ReasoningModelProvider string `envconfig:"REASONING_MODEL_PROVIDER" default:"openai"`
	ReasoningModel         string `envconfig:"REASONING_MODEL" default:"o3-mini"`
	ReasoningModelEffort   string `envconfig:"REASONING_MODEL_EFFORT" default:"none"`

	GenerationModelProvider string `envconfig:"GENERATION_MODEL_PROVIDER" default:"openai"`
	GenerationModel         string `envconfig:"GENERATION_MODEL" default:"gpt-4o"`

	SmallReasoningModelProvider string `envconfig:"SMALL_REASONING_MODEL_PROVIDER" default:"openai"`
	SmallReasoningModel         string `envconfig:"SMALL_REASONING_MODEL" default:"o3-mini"`
	SmallReasoningModelEffort   string `envconfig:"SMALL_REASONING_MODEL_EFFORT" default:"none"`

	SmallGenerationModelProvider string `envconfig:"SMALL_GENERATION_MODEL_PROVIDER" default:"openai"`
	SmallGenerationModel         string `envconfig:"SMALL_GENERATION_MODEL" default:"gpt-4o-mini"`

	DisableAgentTests bool `envconfig:"DISABLE_AGENT_TESTS" default:"false"`

	TestUserCreate bool   `envconfig:"TEST_USER_CREATE" default:"true"`
	TestUserAPIKey string `envconfig:"TEST_USER_API_KEY" default:""`
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	cfg := Config{}

	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func getLLM(config *Config) *agent.LLM {
	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL, false)

	reasoning := agent.LLMModelConfig{
		Client: client,
		Model:  config.ReasoningModel,
	}
	generation := agent.LLMModelConfig{
		Client: client,
		Model:  config.GenerationModel,
	}
	smallReasoning := agent.LLMModelConfig{
		Client: client,
		Model:  config.SmallReasoningModel,
	}
	smallGeneration := agent.LLMModelConfig{
		Client: client,
		Model:  config.SmallGenerationModel,
	}

	llm := agent.NewLLM(
		&reasoning,
		&generation,
		&smallReasoning,
		&smallGeneration,
	)

	return llm
}
