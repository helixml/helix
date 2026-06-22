package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/config"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type configRow struct {
	OrgID     string `gorm:"primaryKey;type:text;index"`
	Key       string `gorm:"primaryKey;type:text"`
	Value     string `gorm:"not null"`
	UpdatedAt time.Time
}

func (configRow) TableName() string { return "org_configs" }

type configMapper struct{}

func (configMapper) ToRow(cfg config.Config) (configRow, error) {
	return configRow{
		OrgID:     cfg.OrganizationID,
		Key:       cfg.Key,
		Value:     cfg.Value,
		UpdatedAt: cfg.UpdatedAt,
	}, nil
}

func (configMapper) ToDomain(row configRow) (config.Config, error) {
	return config.Config{
		OrganizationID: row.OrgID,
		Key:            row.Key,
		Value:          row.Value,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

type configsRepo struct {
	*Repository[config.Config, configRow]
}

func newConfigsRepo(db *gorm.DB) *configsRepo {
	return &configsRepo{Repository: NewRepository[config.Config, configRow](db, configMapper{}, "config")}
}

func (r *configsRepo) Set(ctx context.Context, cfg config.Config) error {
	// gorm.Save issues INSERT … ON CONFLICT DO UPDATE on Postgres
	// when the row has its full composite PK set — matches the
	// original explicit clause.OnConflict behaviour.
	return r.Save(ctx, cfg)
}

func (r *configsRepo) Get(ctx context.Context, orgID, key string) (config.Config, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("key", key),
	)
}

func (r *configsRepo) List(ctx context.Context, orgID, prefix string) ([]config.Config, error) {
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
