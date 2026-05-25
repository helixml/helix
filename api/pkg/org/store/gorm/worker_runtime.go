package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/worker"
)

// workerRuntimeStateRow stores one (workerID, backend, key) → value
// triple. The composite primary key is the natural key — there is no
// synthetic ID. Backends own the key namespace inside their backend
// label; helix-org core never reads or writes here.
type workerRuntimeStateRow struct {
	WorkerID  string    `gorm:"primaryKey;type:text"`
	Backend   string    `gorm:"primaryKey;type:text"`
	Key       string    `gorm:"primaryKey;type:text"`
	Value     string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (workerRuntimeStateRow) TableName() string { return "org_worker_runtime_state" }

type workerRuntimeStateRepo struct {
	db *gorm.DB
}

func (r *workerRuntimeStateRepo) Get(ctx context.Context, workerID worker.ID, backend string) (map[string]string, error) {
	if workerID == "" || backend == "" {
		return nil, errors.New("worker_runtime_state: workerID and backend are required")
	}
	var rows []workerRuntimeStateRow
	err := r.db.WithContext(ctx).
		Where("worker_id = ? AND backend = ?", string(workerID), backend).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("worker_runtime_state get %s/%s: %w", workerID, backend, err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

func (r *workerRuntimeStateRepo) Set(ctx context.Context, workerID worker.ID, backend, key, value string) error {
	return r.SetMany(ctx, workerID, backend, map[string]string{key: value})
}

func (r *workerRuntimeStateRepo) SetMany(ctx context.Context, workerID worker.ID, backend string, kv map[string]string) error {
	if workerID == "" || backend == "" {
		return errors.New("worker_runtime_state: workerID and backend are required")
	}
	if len(kv) == 0 {
		return nil
	}
	rows := make([]workerRuntimeStateRow, 0, len(kv))
	for k, v := range kv {
		if k == "" {
			return errors.New("worker_runtime_state: key is empty")
		}
		rows = append(rows, workerRuntimeStateRow{
			WorkerID: string(workerID),
			Backend:  backend,
			Key:      k,
			Value:    v,
		})
	}
	// Upsert on the natural key — preserves any keys not in kv.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "worker_id"}, {Name: "backend"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&rows).Error
	if err != nil {
		return fmt.Errorf("worker_runtime_state set %s/%s: %w", workerID, backend, err)
	}
	return nil
}

func (r *workerRuntimeStateRepo) Clear(ctx context.Context, workerID worker.ID, backend string) error {
	if workerID == "" || backend == "" {
		return errors.New("worker_runtime_state: workerID and backend are required")
	}
	err := r.db.WithContext(ctx).
		Where("worker_id = ? AND backend = ?", string(workerID), backend).
		Delete(&workerRuntimeStateRow{}).Error
	if err != nil {
		return fmt.Errorf("worker_runtime_state clear %s/%s: %w", workerID, backend, err)
	}
	return nil
}
