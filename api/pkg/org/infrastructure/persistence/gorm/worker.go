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
// RoleID is the live capability binding. Reporting lines (who reports
// to whom) are a separate many-to-many relation — see
// reportingLineRow — so a Worker carries no parent column.
type workerRow struct {
	ID              string `gorm:"primaryKey;type:text"`
	OrgID           string `gorm:"primaryKey;type:text;index"`
	Kind            string `gorm:"not null"` // "human" or "ai"
	RoleID          string `gorm:"not null;index"`
	IdentityContent string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (workerRow) TableName() string { return "org_workers" }

type workerMapper struct{}

func (workerMapper) ToRow(w orgchart.Worker) (workerRow, error) {
	return workerRow{
		ID:              string(w.ID()),
		OrgID:           w.OrganizationID(),
		Kind:            string(w.Kind()),
		RoleID:          string(w.RoleID()),
		IdentityContent: w.IdentityContent(),
	}, nil
}

func (workerMapper) ToDomain(row workerRow) (orgchart.Worker, error) {
	switch orgchart.WorkerKind(row.Kind) {
	case orgchart.WorkerKindHuman:
		return orgchart.NewHumanWorker(orgchart.BotID(row.ID), orgchart.BotID(row.RoleID), row.IdentityContent, row.OrgID)
	case orgchart.WorkerKindAI:
		return orgchart.NewAIWorker(orgchart.BotID(row.ID), orgchart.BotID(row.RoleID), row.IdentityContent, row.OrgID)
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

func (r *workersRepo) Get(ctx context.Context, orgID string, id orgchart.BotID) (orgchart.Worker, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *workersRepo) List(ctx context.Context, orgID string) ([]orgchart.Worker, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

// Delete removes the worker row and drops its worker-anchored
// subscriptions in the same transaction. The reporting lines that
// reference this worker (as manager or report) are removed by the
// ON DELETE CASCADE foreign keys on org_reporting_lines (installed in
// OpenWithDB), so no app code clears them — that's the whole point of
// the association table.
func (r *workersRepo) Delete(ctx context.Context, orgID string, id orgchart.BotID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			"kind":             row.Kind,
		}),
	)
}

type unknownWorkerKindErr struct{ kind string }

func (e unknownWorkerKindErr) Error() string { return "unknown worker kind " + e.kind }
func errUnknownWorkerKind(k string) error    { return unknownWorkerKindErr{kind: k} }
