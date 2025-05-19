package tests

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	OpenAIAPIKey string `envconfig:"OPENAI_API_KEY"`
	BaseURL      string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`

	ReasoningModel  string `envconfig:"REASONING_MODEL" default:"o3-mini"`
	GenerationModel string `envconfig:"GENERATION_MODEL" default:"gpt-4o"`

	SmallReasoningModel  string `envconfig:"SMALL_REASONING_MODEL" default:"o3-mini"`
	SmallGenerationModel string `envconfig:"SMALL_GENERATION_MODEL" default:"gpt-4o-mini"`
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
