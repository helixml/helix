package gorm

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/environment"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

type environmentRow struct {
	OrgID     string `gorm:"primaryKey;type:text;index"`
	WorkerID  string `gorm:"primaryKey;type:text"`
	Path      string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (environmentRow) TableName() string { return "org_environments" }

type environmentMapper struct{}

func (environmentMapper) ToRow(env environment.Environment) (environmentRow, error) {
	return environmentRow{
		OrgID:     env.OrganizationID,
		WorkerID:  string(env.WorkerID),
		Path:      env.Path,
		CreatedAt: env.CreatedAt,
	}, nil
}

func (environmentMapper) ToDomain(row environmentRow) (environment.Environment, error) {
	return environment.New(orgchart.WorkerID(row.WorkerID), row.Path, row.CreatedAt, row.OrgID)
}

type environmentsRepo struct {
	*Repository[environment.Environment, environmentRow]
}

func newEnvironmentsRepo(db *gorm.DB) *environmentsRepo {
	return &environmentsRepo{Repository: NewRepository[environment.Environment, environmentRow](db, environmentMapper{}, "environment")}
}

func (r *environmentsRepo) Get(ctx context.Context, orgID string, workerID orgchart.WorkerID) (environment.Environment, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
	)
}

func (r *environmentsRepo) Delete(ctx context.Context, orgID string, workerID orgchart.WorkerID) error {
	// Pre-existing behaviour: Delete swallows ErrNotFound (returns
	// nil when no row matches). The Repository.Delete returns
	// ErrNotFound when rows-affected is 0; map that back to nil so
	// downstream callers (worker lifecycle teardown) stay
	// idempotent.
	err := r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
	)
	if err != nil && errors.Is(err, store.ErrNotFound) {
		return nil
	}
	return err
}
