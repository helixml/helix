package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) GetWebPage(ctx context.Context, id string) (*types.WebPage, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}
	return s.getWebPage(ctx, "id = ?", id)
}

func (s *PostgresStore) GetWebPageByURL(ctx context.Context, url string) (*types.WebPage, error) {
	if url == "" {
		return nil, fmt.Errorf("url not specified")
	}
	return s.getWebPage(ctx, "url = ?", url)
}

func (s *PostgresStore) getWebPage(ctx context.Context, query string, arg string) (*types.WebPage, error) {
	var page types.WebPage
	if err := s.gdb.WithContext(ctx).Where(query, arg).First(&page).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &page, nil
}

// UpsertWebPage stores a fetched page, replacing any earlier fetch of it.
func (s *PostgresStore) UpsertWebPage(ctx context.Context, page *types.WebPage) error {
	if page.ID == "" {
		return fmt.Errorf("id not specified")
	}
	if page.URL == "" {
		return fmt.Errorf("url not specified")
	}
	now := time.Now()
	page.Created = now
	page.Updated = now
	return s.gdb.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"final_url", "status_code", "content_type", "body", "fetched_at", "updated"}),
	}).Create(page).Error
}
