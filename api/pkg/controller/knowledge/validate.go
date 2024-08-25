package knowledge

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func Validate(k *types.AssistantKnowledge) error {
	if k.Source.Web != nil {
		if len(k.Source.Web.URLs) == 0 {
			return fmt.Errorf("at least one url is required")
		}

		if k.Source.Web.Crawler.Firecrawl != nil {
			if k.Source.Web.Crawler.Firecrawl.APIKey == "" {
				return fmt.Errorf("firecrawl api key is required")
			}
		}
	}

	return nil
}
