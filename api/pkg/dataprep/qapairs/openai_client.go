package qapairs

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
)

func NewClient(cfg *config.ServerConfig, ps pubsub.PubSub, controller openai.Controller) (openai.Client, error) {
	var apiClient openai.Client

	switch cfg.FineTuning.Provider {
	case config.ProviderOpenAI:
		if cfg.Providers.OpenAI.APIKey == "" {
			return nil, errors.New("OpenAI API key (OPENAI_API_KEY) is required")
		}

		log.Info().Msg("using OpenAI provider for tools")

		apiClient = openai.New(
			cfg.Providers.OpenAI.APIKey,
			cfg.Providers.OpenAI.BaseURL)
	case config.ProviderTogetherAI:
		if cfg.Providers.TogetherAI.APIKey == "" {
			return nil, fmt.Errorf("TogetherAI API key (TOGETHER_API_KEY) is required")
		}

		log.Info().
			Str("base_url", cfg.Providers.TogetherAI.BaseURL).
			Msg("using TogetherAI provider for tools")

		apiClient = openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL)

	case config.ProviderHelix:
		if controller == nil {
			return nil, errors.New("no controller provided for Helix provider")
		}

		if ps == nil {
			return nil, errors.New("no pubsub provided for Helix provider")
		}

		apiClient = openai.NewInternalHelixClient(cfg, ps, controller)

	default:
		log.Warn().Msg("no fine-tuning provider configured")
		return nil, errors.New("no fine-tuning provider configured")
	}

	return apiClient, nil
}
