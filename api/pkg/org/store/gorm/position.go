package gorm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store"
)

type positionRow struct {
	ID        string  `gorm:"primaryKey;type:text"`
	OrgID     string  `gorm:"primaryKey;type:text;index"`
	RoleID    string  `gorm:"not null;index"`
	ParentID  *string `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (positionRow) TableName() string { return "org_positions" }

type positionsRepo struct {
	db *gorm.DB
}

func (r *positionsRepo) Create(ctx context.Context, pos domain.Position) error {
	row := positionToRow(pos)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create position: %w", err)
	}
	return nil
}

func (r *positionsRepo) Get(ctx context.Context, orgID string, id position.ID) (domain.Position, error) {
	var row positionRow
	err := r.db.WithContext(ctx).First(&row, "org_id = ? AND id = ?", orgID, string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Position{}, fmt.Errorf("position %q in org %q: %w", id, orgID, store.ErrNotFound)
		}
		return domain.Position{}, fmt.Errorf("get position %q in org %q: %w", id, orgID, err)
	}
	return rowToPosition(row)
}

func (r *positionsRepo) List(ctx context.Context, orgID string) ([]domain.Position, error) {
	var rows []positionRow
	if err := r.db.WithContext(ctx).Where("org_id = ?", orgID).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list positions in org %q: %w", orgID, err)
	}
	return rowsToPositions(rows)
}

func (r *positionsRepo) Update(ctx context.Context, pos domain.Position) error {
	row := positionToRow(pos)
	res := r.db.WithContext(ctx).
		Model(&positionRow{}).
		Where("org_id = ? AND id = ?", row.OrgID, row.ID).
		Updates(map[string]any{
			"role_id":   row.RoleID,
			"parent_id": row.ParentID,
		})
	if res.Error != nil {
		return fmt.Errorf("update position %q in org %q: %w", row.ID, row.OrgID, res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("position %q in org %q: %w", row.ID, row.OrgID, store.ErrNotFound)
	}
	return nil
}

func (r *positionsRepo) ListChildren(ctx context.Context, orgID string, parent position.ID) ([]domain.Position, error) {
	var rows []positionRow
	if err := r.db.WithContext(ctx).Where("org_id = ? AND parent_id = ?", orgID, string(parent)).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list children of %q in org %q: %w", parent, orgID, err)
	}
	return rowsToPositions(rows)
}

func positionToRow(pos domain.Position) positionRow {
	var parent *string
	if pos.ParentID != nil {
		s := string(*pos.ParentID)
		parent = &s
	}
	return positionRow{
		ID:       string(pos.ID),
		OrgID:    pos.OrganizationID,
		RoleID:   string(pos.RoleID),
		ParentID: parent,
	}
}

func rowToPosition(row positionRow) (domain.Position, error) {
	var parent *position.ID
	if row.ParentID != nil {
		p := position.ID(*row.ParentID)
		parent = &p
	}
	return domain.NewPosition(position.ID(row.ID), role.ID(row.RoleID), parent, row.OrgID)
}

func rowsToPositions(rows []positionRow) ([]domain.Position, error) {
	out := make([]domain.Position, 0, len(rows))
	for _, row := range rows {
		pos, err := rowToPosition(row)
		if err != nil {
			return nil, err
		}
		out = append(out, pos)
	}
	return out, nil
}
