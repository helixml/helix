package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// GetOrgComputeUsageQuery selects the sandbox-compute spend to summarise.
//
// Only the date range and the project narrow the result. The token-shaped
// filters on the usage page (model, provider, session, user) have no analogue
// for a container, so compute intentionally ignores them rather than
// silently returning zero when one is set.
type GetOrgComputeUsageQuery struct {
	OrganizationID string
	ProjectID      string
	From           time.Time
	To             time.Time
	// SandboxLimit caps the per-sandbox breakdown. Defaults to 10.
	SandboxLimit int
}

// GetOrgComputeUsage summarises what an org spent on sandbox runtime.
//
// The source of truth is the wallet ledger, not the `sandboxes` table: charges
// are immutable transactions, so a desktop that has since been torn down still
// accounts for what it cost. Sandbox rows are LEFT JOINed only to decorate the
// breakdown with a name, size and owning task, all of which may be missing for
// a hard-deleted row.
func (s *PostgresStore) GetOrgComputeUsage(ctx context.Context, q *GetOrgComputeUsageQuery) (*types.OrgComputeUsage, error) {
	if q == nil {
		return nil, errors.New("query is required")
	}
	if q.OrganizationID == "" {
		return nil, errors.New("organization_id is required")
	}
	limit := q.SandboxLimit
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	usage := &types.OrgComputeUsage{
		Daily:     []types.UsageComputeDailyPoint{},
		Sandboxes: []types.UsageComputeBreakdownRow{},
	}

	// Charges are recorded as negative wallet deltas; flip the sign so the
	// page reads in spend, matching the token cost columns next to it.
	base := func() *gorm.DB {
		query := s.gdb.WithContext(ctx).
			Table("transactions").
			Joins("JOIN wallets ON wallets.id = transactions.wallet_id").
			Joins("LEFT JOIN sandboxes ON sandboxes.id = transactions.sandbox_id").
			Where("wallets.org_id = ?", q.OrganizationID).
			Where("transactions.type = ?", types.TransactionTypeUsage).
			Where("transactions.sandbox_id <> ''").
			Where("transactions.created_at >= ? AND transactions.created_at <= ?", q.From, q.To)
		if q.ProjectID != "" {
			query = query.Where("sandboxes.project_id = ?", q.ProjectID)
		}
		return query
	}

	var totals []struct {
		PricingType string  `gorm:"column:pricing_type"`
		Credits     float64 `gorm:"column:credits"`
	}
	if err := base().
		Select("COALESCE(NULLIF(transactions.sandbox_pricing_type, ''), 'headless') as pricing_type, COALESCE(SUM(-transactions.amount), 0) as credits").
		Group("COALESCE(NULLIF(transactions.sandbox_pricing_type, ''), 'headless')").
		Scan(&totals).Error; err != nil {
		return nil, err
	}
	for _, row := range totals {
		usage.TotalCredits += row.Credits
		if row.PricingType == sandboxPricingTypeDesktop {
			usage.DesktopCredits += row.Credits
		} else {
			usage.HeadlessCredits += row.Credits
		}
	}

	var daily []struct {
		Date        time.Time `gorm:"column:date"`
		PricingType string    `gorm:"column:pricing_type"`
		Credits     float64   `gorm:"column:credits"`
	}
	if err := base().
		Select(`date_trunc('day', transactions.created_at) as date,
			COALESCE(NULLIF(transactions.sandbox_pricing_type, ''), 'headless') as pricing_type,
			COALESCE(SUM(-transactions.amount), 0) as credits`).
		Group("date_trunc('day', transactions.created_at), COALESCE(NULLIF(transactions.sandbox_pricing_type, ''), 'headless')").
		Order("date ASC").
		Scan(&daily).Error; err != nil {
		return nil, err
	}
	usage.Daily = buildComputeDailySeries(daily, q.From, q.To)

	var sandboxes []struct {
		SandboxID   string  `gorm:"column:sandbox_id"`
		Name        string  `gorm:"column:name"`
		Runtime     string  `gorm:"column:runtime"`
		PricingType string  `gorm:"column:pricing_type"`
		SpecTaskID  string  `gorm:"column:spec_task_id"`
		ProjectID   string  `gorm:"column:project_id"`
		VCPUs       int     `gorm:"column:vcpus"`
		Credits     float64 `gorm:"column:credits"`
	}
	if err := base().
		Select(`transactions.sandbox_id as sandbox_id,
			COALESCE(MAX(sandboxes.name), '') as name,
			COALESCE(MAX(sandboxes.runtime), MAX(transactions.sandbox_runtime), '') as runtime,
			COALESCE(MAX(NULLIF(transactions.sandbox_pricing_type, '')), 'headless') as pricing_type,
			COALESCE(MAX(sandboxes.spec_task_id), '') as spec_task_id,
			COALESCE(MAX(sandboxes.project_id), '') as project_id,
			COALESCE(MAX(sandboxes.v_cpus), 0) as vcpus,
			COALESCE(SUM(-transactions.amount), 0) as credits`).
		Group("transactions.sandbox_id").
		Order("credits DESC").
		Limit(limit).
		Scan(&sandboxes).Error; err != nil {
		return nil, err
	}
	for _, row := range sandboxes {
		usage.Sandboxes = append(usage.Sandboxes, types.UsageComputeBreakdownRow{
			SandboxID:   row.SandboxID,
			Name:        row.Name,
			Runtime:     row.Runtime,
			PricingType: row.PricingType,
			SpecTaskID:  row.SpecTaskID,
			ProjectID:   row.ProjectID,
			VCPUs:       row.VCPUs,
			Credits:     row.Credits,
		})
	}

	var running int64
	runningQuery := s.gdb.WithContext(ctx).
		Model(&types.Sandbox{}).
		Where("organization_id = ? AND deleted_at IS NULL AND status = ?", q.OrganizationID, types.SandboxStatusRunning)
	if q.ProjectID != "" {
		runningQuery = runningQuery.Where("project_id = ?", q.ProjectID)
	}
	if err := runningQuery.Count(&running).Error; err != nil {
		return nil, err
	}
	usage.RunningSandboxes = int(running)

	settings, err := s.GetSystemSettings(ctx)
	if err != nil {
		return nil, err
	}
	usage.BillingEnabled = settings.SandboxBillingEnabled

	return usage, nil
}

const sandboxPricingTypeDesktop = "desktop"

// buildComputeDailySeries pivots (day, pricing type) rows into one point per
// day and fills the gaps, so a day with no sandboxes plots as zero instead of
// collapsing the x-axis onto the days that happened to have spend.
func buildComputeDailySeries(rows []struct {
	Date        time.Time `gorm:"column:date"`
	PricingType string    `gorm:"column:pricing_type"`
	Credits     float64   `gorm:"column:credits"`
}, from, to time.Time) []types.UsageComputeDailyPoint {
	byDay := make(map[time.Time]*types.UsageComputeDailyPoint)
	for _, row := range rows {
		day := row.Date.UTC().Truncate(24 * time.Hour)
		point, ok := byDay[day]
		if !ok {
			point = &types.UsageComputeDailyPoint{Date: day}
			byDay[day] = point
		}
		if row.PricingType == sandboxPricingTypeDesktop {
			point.Desktop += row.Credits
		} else {
			point.Headless += row.Credits
		}
		point.Total += row.Credits
	}

	series := []types.UsageComputeDailyPoint{}
	start := from.UTC().Truncate(24 * time.Hour)
	end := to.UTC().Truncate(24 * time.Hour)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if point, ok := byDay[day]; ok {
			series = append(series, *point)
			continue
		}
		series = append(series, types.UsageComputeDailyPoint{Date: day})
	}
	return series
}
