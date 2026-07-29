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

type nodeRuntimeStateRow struct {
	OrgID string `gorm:"primaryKey;type:text;index"`
	// bot_id is the legacy physical column name for the Node identifier.
	NodeID    string    `gorm:"column:bot_id;primaryKey;type:text"`
	Backend   string    `gorm:"primaryKey;type:text"`
	Key       string    `gorm:"primaryKey;type:text"`
	Value     string    `gorm:"type:text"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (nodeRuntimeStateRow) TableName() string { return "org_bot_runtime_state" }

// botRuntimeStateEntry is the domain-level shape of a single
// runtime-state row. The store interface exposes Get as
// map[string]string; the entry is internal scaffolding so the generic
// Repository[D, R] can carry the type-pair around.
type botRuntimeStateEntry struct {
	OrgID   string
	NodeID  string
	Backend string
	Key     string
	Value   string
}

type botRuntimeStateMapper struct{}

func (botRuntimeStateMapper) ToRow(e botRuntimeStateEntry) (nodeRuntimeStateRow, error) {
	return nodeRuntimeStateRow{
		OrgID:   e.OrgID,
		NodeID:  e.NodeID,
		Backend: e.Backend,
		Key:     e.Key,
		Value:   e.Value,
	}, nil
}

func (botRuntimeStateMapper) ToDomain(row nodeRuntimeStateRow) (botRuntimeStateEntry, error) {
	return botRuntimeStateEntry{
		OrgID:   row.OrgID,
		NodeID:  row.NodeID,
		Backend: row.Backend,
		Key:     row.Key,
		Value:   row.Value,
	}, nil
}

//	nodeRuntimeStateRepo embeds the generic Repository for the
//
// shared Get / Delete code paths and falls through to a bespoke
// gorm.OnConflict batch upsert for SetMany. Single-row Save() would
// work but would issue one SQL statement per pair; the map shape of
// the public Set/SetMany API makes a batched VALUES(…),(…) write the
// natural fit.
type nodeRuntimeStateRepo struct {
	*Repository[botRuntimeStateEntry, nodeRuntimeStateRow]
	db *gorm.DB
}

func newNodeRuntimeStateRepo(db *gorm.DB) *nodeRuntimeStateRepo {
	return &nodeRuntimeStateRepo{
		Repository: NewRepository[botRuntimeStateEntry, nodeRuntimeStateRow](db, botRuntimeStateMapper{}, "bot_runtime_state"),
		db:         db,
	}
}

func (r *nodeRuntimeStateRepo) Get(ctx context.Context, orgID string, botID orgchart.NodeID, backend string) (map[string]string, error) {
	if orgID == "" || botID == "" || backend == "" {
		return nil, errors.New("bot_runtime_state: orgID, botID, and backend are required")
	}
	entries, err := r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("bot_id", string(botID)),
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

func (r *nodeRuntimeStateRepo) Set(ctx context.Context, orgID string, botID orgchart.NodeID, backend, key, value string) error {
	return r.SetMany(ctx, orgID, botID, backend, map[string]string{key: value})
}

// SetMany batches upserts in a single VALUES(…) write. Keeping the
// raw gorm path here (rather than going through Repository.Save in
// a loop) avoids N round-trips on hot calls — see
// activation-time multi-key writes in the runtime/helix adapter.
func (r *nodeRuntimeStateRepo) SetMany(ctx context.Context, orgID string, botID orgchart.NodeID, backend string, kv map[string]string) error {
	if orgID == "" || botID == "" || backend == "" {
		return errors.New("bot_runtime_state: orgID, botID, and backend are required")
	}
	if len(kv) == 0 {
		return nil
	}
	rows := make([]nodeRuntimeStateRow, 0, len(kv))
	for k, v := range kv {
		if k == "" {
			return errors.New("bot_runtime_state: key is empty")
		}
		rows = append(rows, nodeRuntimeStateRow{
			OrgID:   orgID,
			NodeID:  string(botID),
			Backend: backend,
			Key:     k,
			Value:   v,
		})
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "org_id"}, {Name: "bot_id"}, {Name: "backend"}, {Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
		}).
		Create(&rows).Error
	if err != nil {
		return fmt.Errorf("bot_runtime_state set %s/%s/%s: %w", orgID, botID, backend, err)
	}
	return nil
}

func (r *nodeRuntimeStateRepo) Clear(ctx context.Context, orgID string, botID orgchart.NodeID, backend string) error {
	if orgID == "" || botID == "" || backend == "" {
		return errors.New("bot_runtime_state: orgID, botID, and backend are required")
	}
	err := r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("bot_id", string(botID)),
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
