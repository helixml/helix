package knowledge

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/robfig/cron/v3"
)

func Validate(k *types.AssistantKnowledge) error {
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
		if secondRun.Sub(nextRun) < 10*time.Minute {
			return fmt.Errorf("refresh schedule must not run more than once per 10 minutes")
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
