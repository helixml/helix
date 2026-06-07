package gorm

import (
	"context"
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
//
// RoleID is the live capability binding. ParentID is the reporting
// line (nullable; the owner has no parent).
type workerRow struct {
	ID              string  `gorm:"primaryKey;type:text"`
	OrgID           string  `gorm:"primaryKey;type:text;index"`
	Kind            string  `gorm:"not null"` // "human" or "ai"
	RoleID          string  `gorm:"not null;index"`
	ParentID        *string `gorm:"index"`
	IdentityContent string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (workerRow) TableName() string { return "org_workers" }

type workerMapper struct{}

func (workerMapper) ToRow(w orgchart.Worker) (workerRow, error) {
	var parent *string
	if p := w.ParentID(); p != nil {
		s := string(*p)
		parent = &s
	}
	return workerRow{
		ID:              string(w.ID()),
		OrgID:           w.OrganizationID(),
		Kind:            string(w.Kind()),
		RoleID:          string(w.RoleID()),
		ParentID:        parent,
		IdentityContent: w.IdentityContent(),
	}, nil
}

func (workerMapper) ToDomain(row workerRow) (orgchart.Worker, error) {
	var parent *orgchart.WorkerID
	if row.ParentID != nil {
		p := orgchart.WorkerID(*row.ParentID)
		parent = &p
	}
	switch orgchart.WorkerKind(row.Kind) {
	case orgchart.WorkerKindHuman:
		return orgchart.NewHumanWorker(orgchart.WorkerID(row.ID), orgchart.RoleID(row.RoleID), parent, row.IdentityContent, row.OrgID)
	case orgchart.WorkerKindAI:
		return orgchart.NewAIWorker(orgchart.WorkerID(row.ID), orgchart.RoleID(row.RoleID), parent, row.IdentityContent, row.OrgID)
	default:
		return nil, errUnknownWorkerKind(row.Kind)
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

// Delete removes the worker row and structurally cascades the two
// relationships that reference it, so deletion can never leave a
// dangling pointer:
//
//   - direct reports: any worker whose parent_id is this worker has
//     it cleared to NULL (they become top-level). A composite
//     self-referential FK with ON DELETE SET NULL can't express this
//     — Postgres would also null the org_id half of (org_id,
//     parent_id), which is a NOT NULL PK column — so the cascade
//     lives here in the store, the one place every delete funnels
//     through.
//   - subscriptions: worker-anchored rows for this worker are
//     dropped so none outlive the worker row.
//
// All three writes run in one transaction so a partial cascade can't
// leave the graph half-torn.
func (r *workersRepo) Delete(ctx context.Context, orgID string, id orgchart.WorkerID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&workerRow{}).
			Where("org_id = ? AND parent_id = ?", orgID, string(id)).
			Update("parent_id", gorm.Expr("NULL")).Error; err != nil {
			return fmt.Errorf("delete worker: clear child parent_id: %w", err)
		}
		if err := tx.Where("org_id = ? AND worker_id = ?", orgID, string(id)).
			Delete(&subscriptionRow{}).Error; err != nil {
			return fmt.Errorf("delete worker: drop subscriptions: %w", err)
		}
		res := tx.Where("org_id = ? AND id = ?", orgID, string(id)).Delete(&workerRow{})
		if res.Error != nil {
			return fmt.Errorf("delete worker: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("worker: %w", store.ErrNotFound)
		}
		return nil
	})
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
			"role_id":          row.RoleID,
			"parent_id":        row.ParentID,
			"kind":             row.Kind,
		}),
	)
}

type unknownWorkerKindErr struct{ kind string }

func (e unknownWorkerKindErr) Error() string { return "unknown worker kind " + e.kind }
func errUnknownWorkerKind(k string) error    { return unknownWorkerKindErr{kind: k} }
