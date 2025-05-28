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

	ReasoningModel  string `envconfig:"REASONING_MODEL" default:"o3-mini"`
	GenerationModel string `envconfig:"GENERATION_MODEL" default:"gpt-4o"`

	SmallReasoningModel  string `envconfig:"SMALL_REASONING_MODEL" default:"o3-mini"`
	SmallGenerationModel string `envconfig:"SMALL_GENERATION_MODEL" default:"gpt-4o-mini"`

	DisableAgentTests bool `envconfig:"DISABLE_AGENT_TESTS" default:"false"`
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
	client := helix_openai.New(config.OpenAIAPIKey, config.BaseURL)

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
