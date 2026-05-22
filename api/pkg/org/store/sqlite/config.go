package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
)

type configRow struct {
	Key       string `gorm:"primaryKey;type:text"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
	UpdatedBy string `gorm:"type:text"`
}

func (configRow) TableName() string { return "configs" }

type configsRepo struct {
	db *gorm.DB
}

// Set upserts a config row by key. The `/ui/settings` admin page is
// the intended caller (helix-org/server/ui — see handleSettingsSet);
// there is no MCP tool path in.
func (r *configsRepo) Set(ctx context.Context, cfg domain.Config) error {
	row := configRow{
		Key:       cfg.Key,
		Value:     cfg.Value,
		UpdatedAt: cfg.UpdatedAt,
		UpdatedBy: string(cfg.UpdatedBy),
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at", "updated_by"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("set config %q: %w", cfg.Key, err)
	}
	return nil
}

func (r *configsRepo) Get(ctx context.Context, key string) (domain.Config, error) {
	var row configRow
	err := r.db.WithContext(ctx).First(&row, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Config{}, fmt.Errorf("config %q: %w", key, store.ErrNotFound)
		}
		return domain.Config{}, fmt.Errorf("get config %q: %w", key, err)
	}
	return rowToConfig(row), nil
}

// List returns every config row whose key starts with prefix, ordered
// by key. An empty prefix returns everything.
func (r *configsRepo) List(ctx context.Context, prefix string) ([]domain.Config, error) {
	var rows []configRow
	q := r.db.WithContext(ctx).Order("key")
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	out := make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToConfig(row))
	}
	return out, nil
}

func (r *configsRepo) Delete(ctx context.Context, key string) error {
	res := r.db.WithContext(ctx).Delete(&configRow{}, "key = ?", key)
	if res.Error != nil {
		return fmt.Errorf("delete config %q: %w", key, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("config %q: %w", key, store.ErrNotFound)
	}
	return nil
}

func rowToConfig(row configRow) domain.Config {
	return domain.Config{
		Key:       row.Key,
		Value:     row.Value,
		UpdatedAt: row.UpdatedAt,
		UpdatedBy: worker.ID(row.UpdatedBy),
	}
}
