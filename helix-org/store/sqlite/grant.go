package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/store"
)

type grantRow struct {
	ID        string `gorm:"primaryKey;type:text"`
	WorkerID  string `gorm:"not null;index"`
	ToolName  string `gorm:"not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (grantRow) TableName() string { return "grants" }

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

func (r *grantsRepo) Get(ctx context.Context, id domain.GrantID) (domain.ToolGrant, error) {
	var row grantRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ToolGrant{}, fmt.Errorf("grant %q: %w", id, store.ErrNotFound)
		}
		return domain.ToolGrant{}, fmt.Errorf("get grant %q: %w", id, err)
	}
	return rowToGrant(row)
}

func (r *grantsRepo) ListByWorker(ctx context.Context, workerID domain.WorkerID) ([]domain.ToolGrant, error) {
	var rows []grantRow
	if err := r.db.WithContext(ctx).Where("worker_id = ?", string(workerID)).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list grants for worker %q: %w", workerID, err)
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

func (r *grantsRepo) FindForWorkerAndTool(ctx context.Context, workerID domain.WorkerID, toolName domain.ToolName) (domain.ToolGrant, error) {
	var row grantRow
	err := r.db.WithContext(ctx).Where("worker_id = ? AND tool_name = ?", string(workerID), string(toolName)).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.ToolGrant{}, fmt.Errorf("grant for worker %q tool %q: %w", workerID, toolName, store.ErrNotFound)
		}
		return domain.ToolGrant{}, fmt.Errorf("find grant for worker %q tool %q: %w", workerID, toolName, err)
	}
	return rowToGrant(row)
}

func (r *grantsRepo) Delete(ctx context.Context, id domain.GrantID) error {
	res := r.db.WithContext(ctx).Delete(&grantRow{}, "id = ?", string(id))
	if res.Error != nil {
		return fmt.Errorf("delete grant %q: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("grant %q: %w", id, store.ErrNotFound)
	}
	return nil
}

func grantToRow(g domain.ToolGrant) grantRow {
	return grantRow{
		ID:       string(g.ID),
		WorkerID: string(g.WorkerID),
		ToolName: string(g.ToolName),
	}
}

func rowToGrant(row grantRow) (domain.ToolGrant, error) {
	return domain.NewToolGrant(
		domain.GrantID(row.ID),
		domain.WorkerID(row.WorkerID),
		domain.ToolName(row.ToolName),
	)
}
