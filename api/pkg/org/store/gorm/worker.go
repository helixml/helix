package gorm

import (
	"context"
	"encoding/json"
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

type workerMapper struct{}

func (workerMapper) ToRow(w domain.Worker) (workerRow, error) {
	pos := w.Position()
	encoded, err := json.Marshal([]position.ID{pos})
	if err != nil {
		return workerRow{}, fmt.Errorf("marshal position: %w", err)
	}
	return workerRow{
		ID:              string(w.ID()),
		OrgID:           w.OrganizationID(),
		Kind:            string(w.Kind()),
		Positions:       string(encoded),
		IdentityContent: w.IdentityContent(),
	}, nil
}

func (workerMapper) ToDomain(row workerRow) (domain.Worker, error) {
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

type workersRepo struct {
	*Repository[domain.Worker, workerRow]
}

func newWorkersRepo(db *gorm.DB) *workersRepo {
	return &workersRepo{Repository: NewRepository[domain.Worker, workerRow](db, workerMapper{}, "worker")}
}

func (r *workersRepo) Get(ctx context.Context, orgID string, id worker.ID) (domain.Worker, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *workersRepo) List(ctx context.Context, orgID string) ([]domain.Worker, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *workersRepo) Delete(ctx context.Context, orgID string, id worker.ID) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *workersRepo) Update(ctx context.Context, w domain.Worker) error {
	row, err := workerMapper{}.ToRow(w)
	if err != nil {
		return err
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"identity_content": row.IdentityContent,
			"positions":        row.Positions,
			"kind":             row.Kind,
		}),
	)
}
