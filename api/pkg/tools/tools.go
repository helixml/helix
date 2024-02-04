package tools

import (
	"context"
	"errors"
	"net/http"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
)

// TODO: probably move planner into a separate package so we can decide when we want to call APIs, when to go with RAG, etc.
type Planner interface {
	IsActionable(ctx context.Context, tools []*types.Tool, history []*types.Interaction, currentMessage string) (*IsActionableResponse, error)
	// TODO: RAG lookup
	RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error)
}

// Static check
var _ Planner = &ChainStrategy{}

type ChainStrategy struct {
	cfg        *config.ServerConfig
	apiClient  openai.Client
	httpClient *http.Client
}

func NewChainStrategy(cfg *config.ServerConfig) (*ChainStrategy, error) {
	var apiClient openai.Client

	switch cfg.Tools.Provider {
	case config.ProviderOpenAI:
		if cfg.Providers.OpenAI.APIKey == "" {
			return nil, errors.New("OpenAI API key (OPENAI_API_KEY) is required")
		}

		// TODO: validate tool model

		// goopenai.GPT3Dot5Turbo

		apiClient = openai.New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL)
	case config.ProviderTogetherAI:
		if cfg.Providers.TogetherAI.APIKey == "" {
			return nil, errors.New("TogetherAI API key (TOGETHER_API_KEY) is required")
		}
		apiClient = openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL)
	}

	return &ChainStrategy{
		cfg:        cfg,
		apiClient:  apiClient,
		httpClient: http.DefaultClient,
	}, nil
}
