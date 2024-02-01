package tools

import (
	"context"
	"net/http"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
)

// TODO: probably move planner into a separate package so we can decide when we want to call APIs, when to go with RAG, etc.
type Planner interface {
	IsActionable(ctx context.Context, tools []*types.Tool, history []*types.Interaction, currentMessage string) (*IsActionableResponse, error)
	RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error)
}

type Config struct {
	OpenAIApiKey  string `envconfig:"OPENAI_API_KEY"`
	OpenAIBaseURL string `envconfig:"OPENAI_BASE_URL" default:"https://api.together.xyz/v1"`

	ToolsModel string `envconfig:"TOOLS_MODEL" default:"mistralai/Mixtral-8x7B-Instruct-v0.1"`
}

type ChainStrategy struct {
	cfg        *Config
	apiClient  openai.Client
	httpClient *http.Client
}

func NewChainStrategy(cfg *Config) (*ChainStrategy, error) {
	return &ChainStrategy{
		cfg:        cfg,
		apiClient:  openai.New(cfg.OpenAIApiKey, cfg.OpenAIBaseURL),
		httpClient: http.DefaultClient,
	}, nil
}
