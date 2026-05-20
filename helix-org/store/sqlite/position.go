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

type positionRow struct {
	ID        string  `gorm:"primaryKey;type:text"`
	RoleID    string  `gorm:"not null;index"`
	ParentID  *string `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (positionRow) TableName() string { return "positions" }

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

func (r *positionsRepo) Get(ctx context.Context, id domain.PositionID) (domain.Position, error) {
	var row positionRow
	err := r.db.WithContext(ctx).First(&row, "id = ?", string(id)).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.Position{}, fmt.Errorf("position %q: %w", id, store.ErrNotFound)
		}
		return domain.Position{}, fmt.Errorf("get position %q: %w", id, err)
	}
	return rowToPosition(row)
}

func (r *positionsRepo) List(ctx context.Context) ([]domain.Position, error) {
	var rows []positionRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list positions: %w", err)
	}
	return rowsToPositions(rows)
}

func (r *positionsRepo) ListChildren(ctx context.Context, parent domain.PositionID) ([]domain.Position, error) {
	var rows []positionRow
	if err := r.db.WithContext(ctx).Where("parent_id = ?", string(parent)).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list children of %q: %w", parent, err)
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
		RoleID:   string(pos.RoleID),
		ParentID: parent,
	}
}

func rowToPosition(row positionRow) (domain.Position, error) {
	var parent *domain.PositionID
	if row.ParentID != nil {
		p := domain.PositionID(*row.ParentID)
		parent = &p
	}
	return domain.NewPosition(domain.PositionID(row.ID), domain.RoleID(row.RoleID), parent)
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
