package tools

import (
	"net/http"

	openai "github.com/sashabaranov/go-openai"
)

type Config struct {
	OpenAIApiKey  string `envconfig:"OPENAI_API_KEY"`
	OpenAIBaseURL string `envconfig:"OPENAI_BASE_URL" default:"https://api.openai.com/v1"`

	ToolsModel string `envconfig:"TOOLS_MODEL" default:"gpt-4-1106-preview"`
}

type ChainStrategy struct {
	cfg        *Config
	apiClient  *openai.Client
	httpClient *http.Client
}

func NewChainStrategy(cfg *Config) (*ChainStrategy, error) {
	config := openai.DefaultConfig(cfg.OpenAIApiKey)
	config.BaseURL = cfg.OpenAIBaseURL

	return &ChainStrategy{
		cfg:        cfg,
		apiClient:  openai.NewClientWithConfig(config),
		httpClient: http.DefaultClient,
	}, nil
}
