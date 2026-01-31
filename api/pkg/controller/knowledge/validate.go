package knowledge

import (
	"fmt"
	"net/url"
	"strings"
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

	// At least one knowledge source must be specified
	if k.Source.Web == nil && k.Source.Filestore == nil && k.Source.Text == nil && k.Source.SharePoint == nil {
		return fmt.Errorf("at least one knowledge source must be specified")
	}

	// Validate SharePoint configuration
	if k.Source.SharePoint != nil {
		if k.Source.SharePoint.SiteID == "" {
			return fmt.Errorf("sharepoint site_id is required")
		}
		if k.Source.SharePoint.OAuthProviderID == "" {
			return fmt.Errorf("sharepoint oauth_provider_id is required")
		}
	}

	if k.Source.Web != nil {
		if len(k.Source.Web.URLs) == 0 {
			return fmt.Errorf("at least one url is required")
		}

		// Validate the URLs
		for _, u := range k.Source.Web.URLs {
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				return fmt.Errorf("url must start with http:// or https://")
			}
			// Should be a valid URL
			if _, err := url.Parse(u); err != nil {
				return fmt.Errorf("invalid url '%s': %w", u, err)
			}
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
		}
	}

	return nil
}
