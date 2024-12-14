package knowledge

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

func Validate(cfg *config.ServerConfig, k *types.AssistantKnowledge) error {
	if k.Name == "" {
		return fmt.Errorf("knowledge name is required")
	}

	if k.RefreshSchedule != "" {
		cronSchedule, err := cron.ParseStandard(k.RefreshSchedule)
		if err != nil {
			return fmt.Errorf("invalid refresh schedule: %w", err)
		}

		// Check if the schedule runs more frequently than every 10 minutes
		nextRun := cronSchedule.Next(time.Now())
		secondRun := cronSchedule.Next(nextRun)
		if secondRun.Sub(nextRun) < cfg.RAG.Crawler.MaxFrequency {
			return fmt.Errorf("refresh schedule must not run more than once per %s", cfg.RAG.Crawler.MaxFrequency)
		}
	}

	if k.Source.Web != nil {
		if len(k.Source.Web.URLs) == 0 {
			return fmt.Errorf("at least one url is required")
		}

		if k.Source.Web.Crawler != nil && k.Source.Web.Crawler.Firecrawl != nil {
			if k.Source.Web.Crawler.Firecrawl.APIKey == "" {
				return fmt.Errorf("firecrawl api key is required")
			}
		}

		// Checking max depth and max pages
		if k.Source.Web.Crawler != nil {
			// If limits are set, we need to ensure they are not exceeded
			if cfg.RAG.Crawler.MaxDepth > 0 && k.Source.Web.Crawler.MaxDepth > cfg.RAG.Crawler.MaxDepth {
				k.Source.Web.Crawler.MaxDepth = cfg.RAG.Crawler.MaxDepth
				log.Warn().Msgf("max depth set to %d", k.Source.Web.Crawler.MaxDepth)
			}

			if cfg.RAG.Crawler.MaxPages > 0 && k.Source.Web.Crawler.MaxPages > cfg.RAG.Crawler.MaxPages {
				k.Source.Web.Crawler.MaxPages = cfg.RAG.Crawler.MaxPages
				log.Warn().Msgf("max pages set to %d", k.Source.Web.Crawler.MaxPages)
			}
		}
	}

	return nil
}
