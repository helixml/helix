package gorm

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
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

type positionMapper struct{}

func (positionMapper) ToRow(p orgchart.Position) (positionRow, error) {
	var parent *string
	if p.ParentID != nil {
		s := string(*p.ParentID)
		parent = &s
	}
	return positionRow{
		ID:       string(p.ID),
		OrgID:    p.OrganizationID,
		RoleID:   string(p.RoleID),
		ParentID: parent,
	}, nil
}

func (positionMapper) ToDomain(row positionRow) (orgchart.Position, error) {
	var parent *orgchart.PositionID
	if row.ParentID != nil {
		p := orgchart.PositionID(*row.ParentID)
		parent = &p
	}
	return orgchart.NewPosition(orgchart.PositionID(row.ID), orgchart.RoleID(row.RoleID), parent, row.OrgID)
}

type positionsRepo struct {
	*Repository[orgchart.Position, positionRow]
}

func newPositionsRepo(db *gorm.DB) *positionsRepo {
	return &positionsRepo{Repository: NewRepository[orgchart.Position, positionRow](db, positionMapper{}, "position")}
}

func (r *positionsRepo) Get(ctx context.Context, orgID string, id orgchart.PositionID) (orgchart.Position, error) {
	return r.FindOne(ctx, store.WithOrg(orgID), store.WithID(string(id)))
}

func (r *positionsRepo) List(ctx context.Context, orgID string) ([]orgchart.Position, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("id"))
}

func (r *positionsRepo) ListChildren(ctx context.Context, orgID string, parent orgchart.PositionID) ([]orgchart.Position, error) {
	return r.Find(ctx,
		store.WithOrg(orgID),
		store.WithCondition("parent_id", string(parent)),
		store.WithOrderAsc("id"),
	)
}

func (r *positionsRepo) Update(ctx context.Context, pos orgchart.Position) error {
	row, err := positionMapper{}.ToRow(pos)
	if err != nil {
		return err
	}
	return r.Repository.Update(ctx,
		store.WithOrg(row.OrgID),
		store.WithID(row.ID),
		store.WithUpdates(map[string]any{
			"role_id":   row.RoleID,
			"parent_id": row.ParentID,
		}),
	)
}
