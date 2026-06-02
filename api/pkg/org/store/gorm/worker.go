package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/worker"
)

// workerRow has composite PK (id, org_id) so short readable handles
// (`w-owner`, `w-mark`) can repeat across helix tenants. OrgID
// additionally carries a FK to organizations(id) ON DELETE CASCADE —
// added out-of-band in OpenWithDB because GORM tag-driven FK creation
// to a table owned by another package is fragile.
type workerRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	OrgID           string `gorm:"primaryKey;type:text;index"`
	Kind            string `gorm:"not null"` // "human" or "ai"
	Positions       string // JSON array of position ids
	IdentityContent string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (workerRow) TableName() string { return "org_workers" }

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

func (r *workersRepo) Get(ctx context.Context, orgID string, id worker.ID) (domain.Worker, error) {
	var row workerRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND id = ?", orgID, string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("worker %q in org %q: %w", id, orgID, store.ErrNotFound)
		}
		return nil, fmt.Errorf("get worker %q in org %q: %w", id, orgID, err)
	}
	return rowToWorker(row)
}

func (r *workersRepo) List(ctx context.Context, orgID string) ([]domain.Worker, error) {
	var rows []workerRow
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list workers in org %q: %w", orgID, err)
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

func (r *workersRepo) Delete(ctx context.Context, orgID string, id worker.ID) error {
	res := r.db.WithContext(ctx).Delete(&workerRow{}, "org_id = ? AND id = ?", orgID, string(id))
	if res.Error != nil {
		return fmt.Errorf("delete worker %q in org %q: %w", id, orgID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	return nil
}

func (r *workersRepo) Update(ctx context.Context, worker domain.Worker) error {
	row, err := workerToRow(worker)
	if err != nil {
		return err
	}
	res := r.db.WithContext(ctx).
		Model(&workerRow{}).
		Where("org_id = ? AND id = ?", row.OrgID, row.ID).
		Updates(map[string]any{
			"identity_content": row.IdentityContent,
			"positions":        row.Positions,
			"kind":             row.Kind,
		})
	if res.Error != nil {
		return fmt.Errorf("update worker %q in org %q: %w", row.ID, row.OrgID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("worker %q in org %q: %w", worker.ID(), row.OrgID, store.ErrNotFound)
	}
	return nil
}

func workerToRow(worker domain.Worker) (workerRow, error) {
	pos := worker.Position()
	encoded, err := json.Marshal([]position.ID{pos})
	if err != nil {
		return workerRow{}, fmt.Errorf("marshal position: %w", err)
	}
	return workerRow{
		ID:              string(worker.ID()),
		OrgID:           worker.OrganizationID(),
		Kind:            string(worker.Kind()),
		Positions:       string(encoded),
		IdentityContent: worker.IdentityContent(),
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
	switch worker.Kind(row.Kind) {
	case worker.KindHuman:
		return domain.NewHumanWorker(worker.ID(row.ID), pos, row.IdentityContent, row.OrgID)
	case worker.KindAI:
		return domain.NewAIWorker(worker.ID(row.ID), pos, row.IdentityContent, row.OrgID)
	default:
		return nil, fmt.Errorf("unknown worker kind %q", row.Kind)
	}
}
