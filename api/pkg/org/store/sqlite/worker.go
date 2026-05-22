package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/store"
)

type workerRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	Kind            string `gorm:"not null"` // "human" or "ai"
	Positions       string // JSON array of position ids
	IdentityContent string // markdown body — domain-owned persona/profile, projected by the spawner
	// OrgID is the helix.Organization the Worker belongs to. Empty
	// in single-tenant deployments (today's alpha). H5.2+ adds the
	// composite (org_id, id) index and tenant-scoped query
	// predicates; H5.1 lands the column as additive scaffolding so
	// existing rows round-trip cleanly.
	OrgID     string `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (workerRow) TableName() string { return "workers" }

type workersRepo struct {
	db *gorm.DB
}

func (r *workersRepo) Create(ctx context.Context, worker domain.Worker) error {
	row, err := workerToRow(worker)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create worker: %w", err)
	}
	return nil
}

func (r *workersRepo) Get(ctx context.Context, id worker.ID) (domain.Worker, error) {
	var row workerRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("worker %q: %w", id, store.ErrNotFound)
		}
		return nil, fmt.Errorf("get worker %q: %w", id, err)
	}
	return rowToWorker(row)
}

func (r *workersRepo) List(ctx context.Context) ([]domain.Worker, error) {
	var rows []workerRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list workers: %w", err)
	}
	out := make([]domain.Worker, 0, len(rows))
	for _, row := range rows {
		w, err := rowToWorker(row)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// Update rewrites the mutable fields of an existing worker row.
// Positions and Kind are not user-editable and are kept aligned with
// the existing record on save — only IdentityContent is intended to
// change today, but we write all mutable fields for forward-compat.
func (r *workersRepo) Update(ctx context.Context, worker domain.Worker) error {
	row, err := workerToRow(worker)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&workerRow{}).
		Where("id = ?", row.ID).
		Updates(map[string]any{
			"identity_content": row.IdentityContent,
			"positions":        row.Positions,
			"kind":             row.Kind,
			"org_id":           row.OrgID,
		})
	if res.Error != nil {
		return fmt.Errorf("update worker %q: %w", row.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q: %w", worker.ID(), store.ErrNotFound)
	}
	return nil
}

func workerToRow(worker domain.Worker) (workerRow, error) {
	// Positions column is preserved as JSON-array for backward compat
	// with existing rows; we encode the single Position into a one-
	// element array on write, and unwrap it on read. A future migration
	// can flatten this to a scalar column.
	pos := worker.Position()
	encoded, err := json.Marshal([]position.ID{pos})
	if err != nil {
		return workerRow{}, fmt.Errorf("marshal position: %w", err)
	}
	return workerRow{
		ID:              string(worker.ID()),
		Kind:            string(worker.Kind()),
		Positions:       string(encoded),
		IdentityContent: worker.IdentityContent(),
		OrgID:           worker.OrganizationID(),
	}, nil
}

func rowToWorker(row workerRow) (domain.Worker, error) {
	var positions []position.ID
	if row.Positions != "" {
		if err := json.Unmarshal([]byte(row.Positions), &positions); err != nil {
			return nil, fmt.Errorf("unmarshal positions: %w", err)
		}
	}
	var pos position.ID
	if len(positions) > 0 {
		pos = positions[0]
	}
	var built domain.Worker
	switch worker.Kind(row.Kind) {
	case worker.KindHuman:
		w, err := domain.NewHumanWorker(worker.ID(row.ID), pos, row.IdentityContent)
		if err != nil {
			return nil, err
		}
		built = w
	case worker.KindAI:
		w, err := domain.NewAIWorker(worker.ID(row.ID), pos, row.IdentityContent)
		if err != nil {
			return nil, err
		}
		built = w
	default:
		return nil, fmt.Errorf("unknown worker kind %q", row.Kind)
	}
	if row.OrgID != "" {
		built = built.WithOrgID(row.OrgID)
	}
	return built, nil
}
