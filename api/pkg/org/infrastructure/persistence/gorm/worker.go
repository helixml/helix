package gorm

import (
	"context"
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
			"role_id":          row.RoleID,
			"parent_id":        row.ParentID,
			"kind":             row.Kind,
		}),
	)
}

type unknownWorkerKindErr struct{ kind string }

func (e unknownWorkerKindErr) Error() string { return "unknown worker kind " + e.kind }
func errUnknownWorkerKind(k string) error    { return unknownWorkerKindErr{kind: k} }
