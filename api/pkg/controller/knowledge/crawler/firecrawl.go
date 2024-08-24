package crawler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mendableai/firecrawl-go"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func NewFirecrawl(k *types.Knowledge) (*Firecrawl, error) {
	if k.Source.Web == nil || k.Source.Web.Crawler == nil || k.Source.Web.Crawler.Firecrawl == nil {
		return nil, fmt.Errorf("firecrawl is not configured for this knowledge")
	}

	var (
		apiKey = k.Source.Web.Crawler.Firecrawl.APIKey
		apiUrl = k.Source.Web.Crawler.Firecrawl.APIURL
	)

	app, err := firecrawl.NewFirecrawlApp(apiKey, apiUrl)
	if err != nil {
		return nil, err
	}

	return &Firecrawl{
		app:       app,
		knowledge: k,
	}, nil
}

type Firecrawl struct {
	app       *firecrawl.FirecrawlApp
	knowledge *types.Knowledge
}

func (f *Firecrawl) Crawl(ctx context.Context) (string, error) {
	crawlParams := map[string]any{
		"crawlerOptions": map[string]any{
			"excludes": f.knowledge.Source.Web.Excludes,
		},
	}

	idempotencyKey := system.GenerateUUID()

	log.Info().
		Str("knowledge_id", f.knowledge.ID).
		Str("knowledge_name", f.knowledge.Name).
		Str("url", f.knowledge.Source.Web.URLs[0]).
		Str("idempotency_key", idempotencyKey).
		Msg("starting to crawl the website")

	result, err := f.app.CrawlURL(f.knowledge.Source.Web.URLs[0], crawlParams, true, 2, idempotencyKey)
	if err != nil {
		return "", fmt.Errorf("failed to crawl url: %w", err)
	}

	jsonCrawlResult, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal crawl result: %w", err)
	}

	fmt.Println(string(jsonCrawlResult))

	return string(jsonCrawlResult), nil
}
