package crawler

import (
	"context"

	"github.com/gocolly/colly/v2"

	"github.com/helixml/helix/api/pkg/types"
)

type Colly struct {
	knowledge *types.Knowledge
}

func NewColly(k *types.Knowledge) (*Colly, error) {
	return &Colly{
		knowledge: k,
	}, nil
}

func (c *Colly) Crawl(ctx context.Context) ([]*types.CrawledDocument, error) {
	collector := colly.NewCollector(
		// Visit only domains: hackerspaces.org, wiki.hackerspaces.org
		colly.AllowedDomains(c.knowledge.Source.Web.URLs...),
	)
}
