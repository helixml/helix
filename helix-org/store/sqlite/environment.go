package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

type environmentRow struct {
	WorkerID  string `gorm:"primaryKey;type:text"`
	Path      string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (environmentRow) TableName() string { return "environments" }

type environmentsRepo struct {
	db *gorm.DB
}

func (r *environmentsRepo) Create(ctx context.Context, env domain.Environment) error {
	row := environmentRow{
		WorkerID:  string(env.WorkerID),
		Path:      env.Path,
		CreatedAt: env.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create environment: %w", err)
	}
	return nil
}

func (r *environmentsRepo) Get(ctx context.Context, workerID domain.WorkerID) (domain.Environment, error) {
	var row environmentRow
	err := r.db.WithContext(ctx).First(&row, "worker_id = ?", string(workerID)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Environment{}, fmt.Errorf("environment for worker %q: %w", workerID, store.ErrNotFound)
		}
		return domain.Environment{}, fmt.Errorf("get environment for worker %q: %w", workerID, err)
	}
	return domain.NewEnvironment(domain.WorkerID(row.WorkerID), row.Path, row.CreatedAt)
}
