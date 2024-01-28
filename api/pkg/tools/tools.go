package tools

import openai "github.com/sashabaranov/go-openai"

type Config struct {
	OpenAIApiKey  string `envconfig:"OPENAI_API_KEY"`
	OpenAIBaseURL string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`
	Model         string `envconfig:"OPENAI_MODEL" default:""`
}

type ChainStrategy struct {
	apiClient *openai.Client
}

func NewChainStrategy(cfg *Config) (*ChainStrategy, error) {
	config := openai.DefaultConfig(cfg.OpenAIApiKey)
	config.BaseURL = cfg.OpenAIBaseURL

	return &ChainStrategy{
		apiClient: openai.NewClientWithConfig(config),
	}, nil
}
