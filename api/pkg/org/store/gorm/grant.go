package gorm

import (
	"context"
	"errors"
	"fmt"
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

type grantsRepo struct {
	db *gorm.DB
}

func (r *grantsRepo) Create(ctx context.Context, g domain.ToolGrant) error {
	row := grantToRow(g)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create grant: %w", err)
	}
	return nil
}

func (r *grantsRepo) Get(ctx context.Context, orgID string, id grant.ID) (domain.ToolGrant, error) {
	var row grantRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND id = ?", orgID, string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ToolGrant{}, fmt.Errorf("grant %q in org %q: %w", id, orgID, store.ErrNotFound)
		}
		return domain.ToolGrant{}, fmt.Errorf("get grant %q in org %q: %w", id, orgID, err)
	}
	return rowToGrant(row)
}

func (r *grantsRepo) ListByWorker(ctx context.Context, orgID string, workerID worker.ID) ([]domain.ToolGrant, error) {
	var rows []grantRow
	if err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ?", orgID, string(workerID)).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list grants for worker %q in org %q: %w", workerID, orgID, err)
	}
	out := make([]domain.ToolGrant, 0, len(rows))
	for _, row := range rows {
		g, err := rowToGrant(row)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *grantsRepo) FindForWorkerAndTool(ctx context.Context, orgID string, workerID worker.ID, toolName tool.Name) (domain.ToolGrant, error) {
	var row grantRow
	err := r.db.WithContext(ctx).Where("org_id = ? AND worker_id = ? AND tool_name = ?", orgID, string(workerID), string(toolName)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ToolGrant{}, fmt.Errorf("grant for worker %q tool %q in org %q: %w", workerID, toolName, orgID, store.ErrNotFound)
		}
		return domain.ToolGrant{}, fmt.Errorf("find grant for worker %q tool %q in org %q: %w", workerID, toolName, orgID, err)
	}
	return rowToGrant(row)
}

func (r *grantsRepo) Delete(ctx context.Context, orgID string, id grant.ID) error {
	res := r.db.WithContext(ctx).Delete(&grantRow{}, "org_id = ? AND id = ?", orgID, string(id))
	if res.Error != nil {
		return fmt.Errorf("delete grant %q in org %q: %w", id, orgID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("grant %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	return nil
}

func grantToRow(g domain.ToolGrant) grantRow {
	return grantRow{
		ID:       string(g.ID),
		OrgID:    g.OrganizationID,
		WorkerID: string(g.WorkerID),
		ToolName: string(g.ToolName),
	}
}

func rowToGrant(row grantRow) (domain.ToolGrant, error) {
	return domain.NewToolGrant(
		grant.ID(row.ID),
		worker.ID(row.WorkerID),
		tool.Name(row.ToolName),
		row.OrgID,
	)
}
