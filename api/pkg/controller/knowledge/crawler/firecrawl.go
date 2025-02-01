package crawler

import (
	"context"
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
		apiURL = k.Source.Web.Crawler.Firecrawl.APIURL
	)

	app, err := firecrawl.NewFirecrawlApp(apiKey, apiURL)
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

func (f *Firecrawl) GetStatus() types.KnowledgeProgress {
	// TODO
	return types.KnowledgeProgress{
		Step:           "Crawling",
		Progress:       0,
		ElapsedSeconds: 0,
	}
}

func (f *Firecrawl) Crawl(_ context.Context) ([]*types.CrawledDocument, error) {
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
		return nil, fmt.Errorf("failed to crawl url: %w", err)
	}

	docs, ok := result.([]*firecrawl.FirecrawlDocument)
	if !ok {
		return nil, fmt.Errorf("failed to convert result to FirecrawlDocument")
	}

	log.Info().
		Str("knowledge_id", f.knowledge.ID).
		Str("knowledge_name", f.knowledge.Name).
		Int("num_docs", len(docs)).
		Msg("crawling completed")

	crawledDocs := make([]*types.CrawledDocument, 0, len(docs))
	for _, doc := range docs {
		crawledDocs = append(crawledDocs, &types.CrawledDocument{
			ID:          doc.ID,
			Title:       doc.Metadata.Title,
			Description: doc.Metadata.Description,
			SourceURL:   doc.Metadata.SourceURL,
			Content:     doc.Content,
		})
	}

	return crawledDocs, nil
}
