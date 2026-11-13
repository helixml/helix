// Package pagecache stores pages fetched for web search in Postgres. Pages
// are public web content keyed by normalised URL, so the cache is shared
// across organizations.
package pagecache

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/helixml/helix/api/pkg/websearch/native"
)

type Cache struct {
	store store.Store
}

var _ native.PageCache = (*Cache)(nil)

func New(store store.Store) *Cache {
	return &Cache{store: store}
}

func (c *Cache) Get(ctx context.Context, url string) (*native.RawPage, error) {
	normalized, err := websearch.NormalizeURL(url)
	if err != nil {
		return nil, fmt.Errorf("normalise %q: %w", url, err)
	}
	return toRawPage(c.store.GetWebPageByURL(ctx, normalized))
}

func (c *Cache) GetByID(ctx context.Context, id string) (*native.RawPage, error) {
	return toRawPage(c.store.GetWebPage(ctx, id))
}

func (c *Cache) Put(ctx context.Context, page *native.RawPage) error {
	normalized, err := websearch.NormalizeURL(page.URL)
	if err != nil {
		return fmt.Errorf("normalise %q: %w", page.URL, err)
	}
	return c.store.UpsertWebPage(ctx, &types.WebPage{
		ID:          websearch.DocumentID(normalized),
		URL:         normalized,
		FinalURL:    page.FinalURL,
		StatusCode:  page.StatusCode,
		ContentType: page.ContentType,
		Body:        page.Body,
		FetchedAt:   page.FetchedAt,
	})
}

func toRawPage(row *types.WebPage, err error) (*native.RawPage, error) {
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &native.RawPage{
		URL:         row.URL,
		FinalURL:    row.FinalURL,
		StatusCode:  row.StatusCode,
		ContentType: row.ContentType,
		Body:        row.Body,
		FetchedAt:   row.FetchedAt,
	}, nil
}
