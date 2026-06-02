package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

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

type configMapper struct{}

func (configMapper) ToRow(cfg domain.Config) (configRow, error) {
	return configRow{
		OrgID:     cfg.OrganizationID,
		Key:       cfg.Key,
		Value:     cfg.Value,
		UpdatedAt: cfg.UpdatedAt,
		UpdatedBy: string(cfg.UpdatedBy),
	}, nil
}

func (configMapper) ToDomain(row configRow) (domain.Config, error) {
	return domain.Config{
		OrganizationID: row.OrgID,
		Key:            row.Key,
		Value:          row.Value,
		UpdatedAt:      row.UpdatedAt,
		UpdatedBy:      worker.ID(row.UpdatedBy),
	}, nil
}

type configsRepo struct {
	*Repository[domain.Config, configRow]
}

func newConfigsRepo(db *gorm.DB) *configsRepo {
	return &configsRepo{Repository: NewRepository[domain.Config, configRow](db, configMapper{}, "config")}
}

func (r *configsRepo) Set(ctx context.Context, cfg domain.Config) error {
	// gorm.Save issues INSERT … ON CONFLICT DO UPDATE on Postgres
	// when the row has its full composite PK set — matches the
	// original explicit clause.OnConflict behaviour.
	return r.Save(ctx, cfg)
}

func (r *configsRepo) Get(ctx context.Context, orgID, key string) (domain.Config, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("key", key),
	)
}

func (r *configsRepo) List(ctx context.Context, orgID, prefix string) ([]domain.Config, error) {
	opts := []store.Option{
		store.WithOrg(orgID),
		store.WithOrderAsc("key"),
	}
	if prefix != "" {
		opts = append(opts, store.WithWhere("key LIKE ?", prefix+"%"))
	}
	return r.Repository.Find(ctx, opts...)
}

func (r *configsRepo) Delete(ctx context.Context, orgID, key string) error {
	return r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("key", key),
	)
}
