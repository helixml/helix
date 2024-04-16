package tools

import (
	"context"
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TODO: probably move planner into a separate package so we can decide when we want to call APIs, when to go with RAG, etc.
type Planner interface {
	IsActionable(ctx context.Context, tools []*types.Tool, history []*types.Interaction, currentMessage string) (*IsActionableResponse, error)
	// TODO: RAG lookup
	RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error)
	// Validation and defaulting
	ValidateAndDefault(ctx context.Context, tool *types.Tool) (*types.Tool, error)
}

// Static check
var _ Planner = &ChainStrategy{}

type ChainStrategy struct {
	cfg        *config.ServerConfig
	apiClient  openai.Client
	httpClient *http.Client
	Local      bool // run locally for tests XXX security risk, never set this to true in production
}

func NewChainStrategy(cfg *config.ServerConfig, ps pubsub.PubSub, controller openai.Controller) (*ChainStrategy, error) {
	var apiClient openai.Client

	switch cfg.Tools.Provider {
	case config.ProviderOpenAI:
		if cfg.Providers.OpenAI.APIKey == "" {
			return nil, errors.New("OpenAI API key (OPENAI_API_KEY) is required")
		}

		log.Info().Msg("using OpenAI provider for tools")

		apiClient = openai.New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL)
	case config.ProviderTogetherAI:
		if cfg.Providers.TogetherAI.APIKey != "" {

			log.Info().
				Str("base_url", cfg.Providers.TogetherAI.BaseURL).
				Msg("using TogetherAI provider for tools")

			apiClient = openai.New(
				cfg.Providers.TogetherAI.APIKey,
				cfg.Providers.TogetherAI.BaseURL)
		} else {
			// gptscript server case
			log.Info().Msg("no explicit tools provider LLM configured (gptscript server will still work if OPENAI_API_KEY is set)")
		}
	case config.ProviderHelix:
		if controller != nil {
			log.Info().Msg("using Helix provider for tools")

			apiClient = openai.NewInternalHelixClient(cfg, ps, controller)
		}

	default:
		log.Warn().Msg("no tools provider configured")
	}

	retryClient := system.NewRetryClient(3)
	return &ChainStrategy{
		cfg:        cfg,
		apiClient:  apiClient,
		httpClient: retryClient.StandardClient(),
	}, nil
}
