package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/worker"
)

type configRow struct {
	OrgID     string `gorm:"primaryKey;type:text;index"`
	Key       string `gorm:"primaryKey;type:text"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
	UpdatedBy string `gorm:"type:text"`
}

func (configRow) TableName() string { return "org_configs" }

type configsRepo struct {
	db *gorm.DB
}

func (r *configsRepo) Set(ctx context.Context, cfg domain.Config) error {
	row := configRow{
		OrgID:     cfg.OrganizationID,
		Key:       cfg.Key,
		Value:     cfg.Value,
		UpdatedAt: cfg.UpdatedAt,
		UpdatedBy: string(cfg.UpdatedBy),
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at", "updated_by"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("set config %q in org %q: %w", cfg.Key, cfg.OrganizationID, err)
	}
	return nil
}

func (r *configsRepo) Get(ctx context.Context, orgID, key string) (domain.Config, error) {
	var row configRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND key = ?", orgID, key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Config{}, fmt.Errorf("config %q in org %q: %w", key, orgID, store.ErrNotFound)
		}
		return domain.Config{}, fmt.Errorf("get config %q in org %q: %w", key, orgID, err)
	}
	return rowToConfig(row), nil
}

func (r *configsRepo) List(ctx context.Context, orgID, prefix string) ([]domain.Config, error) {
	var rows []configRow
	q := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("key")
	if prefix != "" {
		q = q.Where("key LIKE ?", prefix+"%")
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list configs in org %q: %w", orgID, err)
	}
	out := make([]domain.Config, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToConfig(row))
	}
	return out, nil
}

func (r *configsRepo) Delete(ctx context.Context, orgID, key string) error {
	res := r.db.WithContext(ctx).Delete(&configRow{}, "org_id = ? AND key = ?", orgID, key)
	if res.Error != nil {
		return fmt.Errorf("delete config %q in org %q: %w", key, orgID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("config %q in org %q: %w", key, orgID, store.ErrNotFound)
	}
	return nil
}

func rowToConfig(row configRow) domain.Config {
	return domain.Config{
		OrganizationID: row.OrgID,
		Key:            row.Key,
		Value:          row.Value,
		UpdatedAt:      row.UpdatedAt,
		UpdatedBy:      worker.ID(row.UpdatedBy),
	}
}
