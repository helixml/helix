package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/store"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/worker"
)

type grantRow struct {
	ID        string `gorm:"primaryKey;type:text"`
	OrgID     string `gorm:"primaryKey;type:text;index"`
	WorkerID  string `gorm:"not null;index"`
	ToolName  string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (grantRow) TableName() string { return "org_grants" }

type grantMapper struct{}

func (grantMapper) ToRow(g domain.ToolGrant) (grantRow, error) {
	return grantRow{
		ID:       string(g.ID),
		OrgID:    g.OrganizationID,
		WorkerID: string(g.WorkerID),
		ToolName: string(g.ToolName),
	}, nil
}

func (grantMapper) ToDomain(row grantRow) (domain.ToolGrant, error) {
	return domain.NewToolGrant(
		grant.ID(row.ID),
		worker.ID(row.WorkerID),
		tool.Name(row.ToolName),
		row.OrgID,
	)
}

type grantsRepo struct {
	*Repository[domain.ToolGrant, grantRow]
}

func newGrantsRepo(db *gorm.DB) *grantsRepo {
	return &grantsRepo{Repository: NewRepository[domain.ToolGrant, grantRow](db, grantMapper{}, "grant")}
}

func (r *grantsRepo) Get(ctx context.Context, orgID string, id grant.ID) (domain.ToolGrant, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *grantsRepo) ListByWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.ToolGrant, error) {
	return r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithOrderAsc("id"),
	)
}

func (r *grantsRepo) FindForWorkerAndTool(ctx context.Context, orgID string, workerID worker.ID, toolName tool.Name) (domain.ToolGrant, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("tool_name", string(toolName)),
	)
}

func (r *grantsRepo) Delete(ctx context.Context, orgID string, id grant.ID) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}
