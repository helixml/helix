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

// reportingLineRow is one edge of the org's reporting graph: ReportID
// reports to ManagerID. Composite PK (org_id, manager_id, report_id).
// Both (org_id, manager_id) and (org_id, report_id) carry an
// ON DELETE CASCADE FK to org_bots(org_id, id), installed in
// OpenWithDB — so deleting either endpoint Bot drops the line with
// no app code involved.
type reportingLineRow struct {
	OrgID     string `gorm:"primaryKey;type:text;index"`
	ManagerID string `gorm:"primaryKey;type:text;index"`
	ReportID  string `gorm:"primaryKey;type:text;index"`
	CreatedAt time.Time
}

func (reportingLineRow) TableName() string { return "org_reporting_lines" }

type reportingLinesRepo struct {
	db *gorm.DB
}

func newReportingLinesRepo(db *gorm.DB) *reportingLinesRepo {
	return &reportingLinesRepo{db: db}
}

func (r *reportingLinesRepo) Add(ctx context.Context, line orgchart.ReportingLine) error {
	row := reportingLineRow{
		OrgID:     line.OrgID,
		ManagerID: string(line.ManagerID),
		ReportID:  string(line.ReportID),
	}
	// Idempotent: re-adding an existing (manager, report) edge is a
	// no-op rather than a PK-violation error.
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error; err != nil {
		return fmt.Errorf("add reporting line: %w", err)
	}
	return nil
}

func (r *reportingLinesRepo) Remove(ctx context.Context, orgID string, reportID, managerID orgchart.BotID) error {
	res := r.db.WithContext(ctx).
		Where("org_id = ? AND manager_id = ? AND report_id = ?", orgID, string(managerID), string(reportID)).
		Delete(&reportingLineRow{})
	if res.Error != nil {
		return fmt.Errorf("remove reporting line: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("reporting line: %w", store.ErrNotFound)
	}
	return nil
}

func (r *reportingLinesRepo) List(ctx context.Context, orgID string) ([]orgchart.ReportingLine, error) {
	var rows []reportingLineRow
	if err := r.db.WithContext(ctx).
		Where("org_id = ?", orgID).
		Order("manager_id ASC, report_id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list reporting lines: %w", err)
	}
	out := make([]orgchart.ReportingLine, 0, len(rows))
	for _, row := range rows {
		out = append(out, orgchart.ReportingLine{
			OrgID:     row.OrgID,
			ManagerID: orgchart.BotID(row.ManagerID),
			ReportID:  orgchart.BotID(row.ReportID),
		})
	}
	return out, nil
}

func (r *reportingLinesRepo) ListManagers(ctx context.Context, orgID string, reportID orgchart.BotID) ([]orgchart.BotID, error) {
	var rows []reportingLineRow
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND report_id = ?", orgID, string(reportID)).
		Order("manager_id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list managers: %w", err)
	}
	out := make([]orgchart.BotID, 0, len(rows))
	for _, row := range rows {
		out = append(out, orgchart.BotID(row.ManagerID))
	}
	return out, nil
}

func (r *reportingLinesRepo) ListReports(ctx context.Context, orgID string, managerID orgchart.BotID) ([]orgchart.BotID, error) {
	var rows []reportingLineRow
	if err := r.db.WithContext(ctx).
		Where("org_id = ? AND manager_id = ?", orgID, string(managerID)).
		Order("report_id ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	out := make([]orgchart.BotID, 0, len(rows))
	for _, row := range rows {
		out = append(out, orgchart.BotID(row.ReportID))
	}
	return out, nil
}
