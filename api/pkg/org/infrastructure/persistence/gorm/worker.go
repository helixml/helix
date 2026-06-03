package gorm

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
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

func (workerMapper) ToRow(w orgchart.Worker) (workerRow, error) {
	pos := w.Position()
	encoded, err := json.Marshal([]orgchart.PositionID{pos})
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

func (workerMapper) ToDomain(row workerRow) (orgchart.Worker, error) {
	var positions []orgchart.PositionID
	if row.Positions != "" {
		if err := json.Unmarshal([]byte(row.Positions), &positions); err != nil {
			return nil, fmt.Errorf("unmarshal positions: %w", err)
		}
	}
	var pos orgchart.PositionID
	if len(positions) > 0 {
		pos = positions[0]
	}
	switch orgchart.WorkerKind(row.Kind) {
	case orgchart.WorkerKindHuman:
		return orgchart.NewHumanWorker(orgchart.WorkerID(row.ID), pos, row.IdentityContent, row.OrgID)
	case orgchart.WorkerKindAI:
		return orgchart.NewAIWorker(orgchart.WorkerID(row.ID), pos, row.IdentityContent, row.OrgID)
	default:
		return nil, fmt.Errorf("unknown worker kind %q", row.Kind)
	}
}

type workersRepo struct {
	*Repository[orgchart.Worker, workerRow]
}

func newWorkersRepo(db *gorm.DB) *workersRepo {
	return &workersRepo{Repository: NewRepository[orgchart.Worker, workerRow](db, workerMapper{}, "worker")}
}

func (r *workersRepo) Get(ctx context.Context, orgID string, id orgchart.WorkerID) (orgchart.Worker, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *workersRepo) List(ctx context.Context, orgID string) ([]orgchart.Worker, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *workersRepo) Delete(ctx context.Context, orgID string, id orgchart.WorkerID) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *workersRepo) Update(ctx context.Context, w orgchart.Worker) error {
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
