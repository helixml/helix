package tools

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"

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

	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.RequestLogHook = func(_ retryablehttp.Logger, req *http.Request, attempt int) {
		switch {
		case req.Method == "POST":
			log.Trace().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("failed")
		default:
			// GET, PUT, DELETE, etc.
			log.Trace().
				Str(req.Method, req.URL.String()).
				Int("attempt", attempt).
				Msgf("")
		}
	}

	retryClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp == nil {
			return true, err
		}
		log.Trace().
			Str(resp.Request.Method, resp.Request.URL.String()).
			Int("code", resp.StatusCode).
			Msgf("")
		// don't retry for auth and bad request errors
		return resp.StatusCode >= 500, nil
	}

	return &ChainStrategy{
		cfg:        cfg,
		apiClient:  apiClient,
		httpClient: retryClient.StandardClient(),
	}, nil
}
