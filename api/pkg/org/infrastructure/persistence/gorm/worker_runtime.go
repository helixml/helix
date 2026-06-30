package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type workerRuntimeStateRow struct {
	OrgID     string    `gorm:"primaryKey;type:text;index"`
	WorkerID  string    `gorm:"primaryKey;type:text"`
	Backend   string    `gorm:"primaryKey;type:text"`
	Key       string    `gorm:"primaryKey;type:text"`
	Value     string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (workerRuntimeStateRow) TableName() string { return "org_worker_runtime_state" }

// workerRuntimeStateEntry is the domain-level shape of a single
// runtime-state row. The store interface exposes Get as
// map[string]string; the entry is internal scaffolding so the generic
// Repository[D, R] can carry the type-pair around.
type workerRuntimeStateEntry struct {
	OrgID    string
	WorkerID string
	Backend  string
	Key      string
	Value    string
}

type workerRuntimeStateMapper struct{}

func (workerRuntimeStateMapper) ToRow(e workerRuntimeStateEntry) (workerRuntimeStateRow, error) {
	return workerRuntimeStateRow{
		OrgID:    e.OrgID,
		WorkerID: e.WorkerID,
		Backend:  e.Backend,
		Key:      e.Key,
		Value:    e.Value,
	}, nil
}

func (workerRuntimeStateMapper) ToDomain(row workerRuntimeStateRow) (workerRuntimeStateEntry, error) {
	return workerRuntimeStateEntry{
		OrgID:    row.OrgID,
		WorkerID: row.WorkerID,
		Backend:  row.Backend,
		Key:      row.Key,
		Value:    row.Value,
	}, nil
}

// workerRuntimeStateRepo embeds the generic Repository for the
// shared Get / Delete code paths and falls through to a bespoke
// gorm.OnConflict batch upsert for SetMany. Single-row Save() would
// work but would issue one SQL statement per pair; the map shape of
// the public Set/SetMany API makes a batched VALUES(…),(…) write the
// natural fit.
type workerRuntimeStateRepo struct {
	*Repository[workerRuntimeStateEntry, workerRuntimeStateRow]
	db *gorm.DB
}

func newWorkerRuntimeStateRepo(db *gorm.DB) *workerRuntimeStateRepo {
	return &workerRuntimeStateRepo{
		Repository: NewRepository[workerRuntimeStateEntry, workerRuntimeStateRow](db, workerRuntimeStateMapper{}, "worker_runtime_state"),
		db:         db,
	}
}

func (r *workerRuntimeStateRepo) Get(ctx context.Context, orgID string, workerID orgchart.BotID, backend string) (map[string]string, error) {
	if orgID == "" || workerID == "" || backend == "" {
		return nil, errors.New("worker_runtime_state: orgID, workerID, and backend are required")
	}
	entries, err := r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("backend", backend),
	)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		out[e.Key] = e.Value
	}
	return out, nil
}

func (r *workerRuntimeStateRepo) Set(ctx context.Context, orgID string, workerID orgchart.BotID, backend, key, value string) error {
	return r.SetMany(ctx, orgID, workerID, backend, map[string]string{key: value})
}

// SetMany batches upserts in a single VALUES(…) write. Keeping the
// raw gorm path here (rather than going through Repository.Save in
// a loop) avoids N round-trips on hot calls — see
// activation-time multi-key writes in the runtime/helix adapter.
func (r *workerRuntimeStateRepo) SetMany(ctx context.Context, orgID string, workerID orgchart.BotID, backend string, kv map[string]string) error {
	if orgID == "" || workerID == "" || backend == "" {
		return errors.New("worker_runtime_state: orgID, workerID, and backend are required")
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
			OrgID:    orgID,
			WorkerID: string(workerID),
			Backend:  backend,
			Key:      k,
			Value:    v,
		})
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "org_id"}, {Name: "worker_id"}, {Name: "backend"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&rows).Error
	if err != nil {
		return fmt.Errorf("worker_runtime_state set %s/%s/%s: %w", orgID, workerID, backend, err)
	}
	return nil
}

func (r *workerRuntimeStateRepo) Clear(ctx context.Context, orgID string, workerID orgchart.BotID, backend string) error {
	if orgID == "" || workerID == "" || backend == "" {
		return errors.New("worker_runtime_state: orgID, workerID, and backend are required")
	}
	err := r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("backend", backend),
	)
	// Clearing a non-existent runtime-state set is idempotent —
	// match the memory impl and the prior gorm behaviour (raw DELETE
	// with no rows is success, not 404).
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}
