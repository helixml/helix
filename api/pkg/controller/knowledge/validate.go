package knowledge

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/robfig/cron/v3"
)

func Validate(k *types.AssistantKnowledge) error {
	if k.RefreshSchedule != "" {
		cronSchedule, err := cron.ParseStandard(k.RefreshSchedule)
		if err != nil {
			return fmt.Errorf("invalid refresh schedule: %w", err)
		}

		// Validate that the cron schedule is setup to run not more than once per 10 minutes
		if cronSchedule.Next(time.Now()).Before(time.Now().Add(10 * time.Minute)) {
			return fmt.Errorf("refresh schedule must run more than once per 10 minutes")
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
	}

	return nil
}
