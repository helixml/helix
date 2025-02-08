package crawler

import (
	"context"

	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

//go:generate mockgen -source $GOFILE -destination crawler_mocks.go -package $GOPACKAGE

type Crawler interface {
	Crawl(ctx context.Context) ([]*types.CrawledDocument, error)
}

func NewCrawler(browserPool *browser.Browser, k *types.Knowledge, updateProgress func(progress types.KnowledgeProgress)) (Crawler, error) {
	switch {
	case k.Source.Web.Crawler.Firecrawl != nil:
		log.Info().
			Str("knowledge_id", k.ID).
			Str("knowledge_name", k.Name).
			Msgf("Using firecrawl crawler")
		return NewFirecrawl(k)
	default:
		log.Info().
			Str("knowledge_id", k.ID).
			Str("knowledge_name", k.Name).
			Msgf("Using default Helix crawler")
		return NewDefault(browserPool, k, updateProgress)
	}
}
