package gorm

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
)

// chartPositionRow stores free-placed canvas coordinates for one
// org-chart node. Composite PK (org_id, kind, id) so the same entity
// handle can exist under different kinds without collision (unlikely
// but cheap insurance) and so short ids can repeat across orgs.
type chartPositionRow struct {
	OrgID     string  `gorm:"primaryKey;type:text;index"`
	Kind      string  `gorm:"primaryKey;type:text"`
	ID        string  `gorm:"primaryKey;type:text"`
	X         float64 `gorm:"not null"`
	Y         float64 `gorm:"not null"`
	UpdatedAt time.Time
}

func (chartPositionRow) TableName() string { return "org_chart_positions" }

type chartPositionMapper struct{}

func (chartPositionMapper) ToRow(p orgchart.ChartPosition) (chartPositionRow, error) {
	return chartPositionRow{
		OrgID:     p.OrganizationID,
		Kind:      p.Kind,
		ID:        p.ID,
		X:         p.X,
		Y:         p.Y,
		UpdatedAt: p.UpdatedAt,
	}, nil
}

func (chartPositionMapper) ToDomain(row chartPositionRow) (orgchart.ChartPosition, error) {
	return orgchart.ChartPosition{
		OrganizationID: row.OrgID,
		Kind:           row.Kind,
		ID:             row.ID,
		X:              row.X,
		Y:              row.Y,
		UpdatedAt:      row.UpdatedAt,
	}, nil
}

type chartPositionsRepo struct {
	*Repository[orgchart.ChartPosition, chartPositionRow]
	db *gorm.DB
}

func newChartPositionsRepo(db *gorm.DB) *chartPositionsRepo {
	return &chartPositionsRepo{
		Repository: NewRepository[orgchart.ChartPosition, chartPositionRow](db, chartPositionMapper{}, "chart_position"),
		db:         db,
	}
}

func (r *chartPositionsRepo) List(ctx context.Context, orgID string) ([]orgchart.ChartPosition, error) {
	return r.Find(ctx, store.WithOrg(orgID), store.WithOrderAsc("kind"), store.WithOrderAsc("id"))
}

func (r *chartPositionsRepo) Upsert(ctx context.Context, pos orgchart.ChartPosition) error {
	row, err := chartPositionMapper{}.ToRow(pos)
	if err != nil {
		return fmt.Errorf("map chart_position: %w", err)
	}
	// Explicit ON CONFLICT so re-drags replace x/y rather than 23505.
	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "kind"}, {Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"x", "y", "updated_at"}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("upsert chart_position: %w", err)
	}
	return nil
}

func (r *chartPositionsRepo) UpsertMany(ctx context.Context, positions []orgchart.ChartPosition) error {
	for _, p := range positions {
		if err := r.Upsert(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func (r *chartPositionsRepo) Delete(ctx context.Context, orgID, kind, id string) error {
	return r.Repository.Delete(ctx,
		store.WithOrg(orgID),
		store.WithCondition("kind", kind),
		store.WithCondition("id", id),
	)
}

func (r *chartPositionsRepo) Clear(ctx context.Context, orgID string) error {
	// Clear is intentionally a no-op when empty: the chart's "reset
	// layout" affordance should not 404 on a fresh org.
	res := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Delete(&chartPositionRow{})
	if res.Error != nil {
		return fmt.Errorf("clear chart_positions: %w", res.Error)
	}
	return nil
}
