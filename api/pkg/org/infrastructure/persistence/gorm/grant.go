package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
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

func (grantMapper) ToRow(g orgchart.ToolGrant) (grantRow, error) {
	return grantRow{
		ID:       string(g.ID),
		OrgID:    g.OrganizationID,
		WorkerID: string(g.WorkerID),
		ToolName: string(g.ToolName),
	}, nil
}

func (grantMapper) ToDomain(row grantRow) (orgchart.ToolGrant, error) {
	return orgchart.NewToolGrant(
		orgchart.GrantID(row.ID),
		orgchart.WorkerID(row.WorkerID),
		tool.Name(row.ToolName),
		row.OrgID,
	)
}

type grantsRepo struct {
	*Repository[orgchart.ToolGrant, grantRow]
}

func newGrantsRepo(db *gorm.DB) *grantsRepo {
	return &grantsRepo{Repository: NewRepository[orgchart.ToolGrant, grantRow](db, grantMapper{}, "grant")}
}

func (r *grantsRepo) Get(ctx context.Context, orgID string, id orgchart.GrantID) (orgchart.ToolGrant, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *grantsRepo) ListByWorker(ctx context.Context, orgID string, workerID orgchart.WorkerID) ([]orgchart.ToolGrant, error) {
	return r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithOrderAsc("id"),
	)
}

func (r *grantsRepo) FindForWorkerAndTool(ctx context.Context, orgID string, workerID orgchart.WorkerID, toolName tool.Name) (orgchart.ToolGrant, error) {
	return r.FindOne(ctx,
		store.WithOrg(orgID),
		store.WithCondition("worker_id", string(workerID)),
		store.WithCondition("tool_name", string(toolName)),
	)
}

func (r *grantsRepo) Delete(ctx context.Context, orgID string, id orgchart.GrantID) error {
	return r.Repository.Delete(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}
