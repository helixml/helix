package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// GetOrgComputeUsageQuery selects the sandbox compute to summarise.
//
// Date, project, and task narrow the result. The remaining token-shaped
// filters (model, provider, session, user) have no container analogue.
type GetOrgComputeUsageQuery struct {
	OrganizationID string
	ProjectID      string
	TaskID         string
	From           time.Time
	To             time.Time
	// SandboxLimit caps the per-sandbox breakdown. Defaults to 10.
	SandboxLimit int
}

const computeTaskIDExpr = `COALESCE(
	(
		SELECT trigger_executions.trigger_configuration_id
		FROM trigger_executions
		WHERE trigger_executions.session_id = sandboxes.session_id
		  AND sandboxes.session_id <> ''
		ORDER BY trigger_executions.created DESC
		LIMIT 1
	),
	NULLIF(sandboxes.spec_task_id, ''),
	''
)`

// GetOrgComputeUsage summarises an org's sandbox compute: what it was charged
// and how long its containers ran.
//
// Charges come from the wallet ledger and runtime from the sandbox run
// ledger, never from live `sandboxes` rows: both are append-only, so a desktop
// that has since been torn down still accounts for what it cost and how long
// it ran, and runtime is there even when billing is off. Sandbox rows are
// LEFT JOINed only to decorate the breakdown and apply the project and task
// filters; they may be missing for a hard-deleted row.
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

	args := map[string]any{
		"org":     q.OrganizationID,
		"from":    q.From,
		"to":      q.To,
		"start":   q.From.UTC().Truncate(24 * time.Hour),
		"now":     time.Now(),
		"usage":   types.TransactionTypeUsage,
		"desktop": types.SandboxRuntimeUbuntuDesktop,
		"project": q.ProjectID,
		"task":    q.TaskID,
		"limit":   limit,
	}
	filters := ""
	if q.ProjectID != "" {
		filters += " AND sandboxes.project_id = @project"
	}
	if q.TaskID != "" {
		filters += " AND " + computeTaskIDExpr + " = @task"
	}

	// Charges are recorded as negative wallet deltas; flip the sign so the
	// page reads in spend, matching the token cost columns next to it.
	charges := `SELECT transactions.sandbox_id,
			COALESCE(NULLIF(transactions.sandbox_pricing_type, ''), 'headless') AS pricing_type,
			date_trunc('day', transactions.created_at AT TIME ZONE 'UTC') AS date,
			-transactions.amount AS credits
		FROM transactions
		JOIN wallets ON wallets.id = transactions.wallet_id
		LEFT JOIN sandboxes ON sandboxes.id = transactions.sandbox_id
		WHERE wallets.org_id = @org
		  AND transactions.type = @usage
		  AND transactions.sandbox_id <> ''
		  AND transactions.created_at >= @from AND transactions.created_at <= @to` + filters

	// Each run is clipped to the range and, for the daily series, to each day
	// it overlaps; a run still open counts up to now. Days are naive UTC
	// timestamps, like the charge days, so neither the UNION nor the day
	// arithmetic depends on the server's TimeZone or its DST rules.
	runs := `FROM sandbox_runs r
		LEFT JOIN sandboxes ON sandboxes.id = r.sandbox_id
		WHERE r.organization_id = @org
		  AND r.started_at < @to
		  AND COALESCE(r.stopped_at, @now) > @from` + filters
	runPricingType := `CASE WHEN r.runtime = @desktop THEN 'desktop' ELSE 'headless' END`
	dayStart := `(d.day AT TIME ZONE 'UTC')`
	dayEnd := `((d.day + interval '1 day') AT TIME ZONE 'UTC')`

	var daily []computeDailyRow
	if err := s.gdb.WithContext(ctx).Raw(`
		SELECT date, pricing_type, SUM(credits) AS credits, 0 AS hours
		FROM (`+charges+`) c
		GROUP BY date, pricing_type
		UNION ALL
		SELECT d.day AS date, `+runPricingType+` AS pricing_type, 0 AS credits,
			SUM(GREATEST(0, EXTRACT(EPOCH FROM
				LEAST(COALESCE(r.stopped_at, @now), `+dayEnd+`, @to) -
				GREATEST(r.started_at, `+dayStart+`, @from)))) / 3600 AS hours
		FROM generate_series(CAST(@start AS timestamptz) AT TIME ZONE 'UTC', CAST(@to AS timestamptz) AT TIME ZONE 'UTC', interval '1 day') AS d(day)
		JOIN sandbox_runs r
		  ON r.started_at < LEAST(`+dayEnd+`, @to)
		 AND COALESCE(r.stopped_at, @now) > GREATEST(`+dayStart+`, @from)
		LEFT JOIN sandboxes ON sandboxes.id = r.sandbox_id
		WHERE r.organization_id = @org`+filters+`
		GROUP BY d.day, 2`, args).
		Scan(&daily).Error; err != nil {
		return nil, fmt.Errorf("query daily compute usage: %w", err)
	}
	usage.Daily = buildComputeDailySeries(daily, q.From, q.To)
	for _, point := range usage.Daily {
		usage.TotalCredits += point.Total
		usage.DesktopCredits += point.Desktop
		usage.HeadlessCredits += point.Headless
		usage.TotalHours += point.TotalHours
		usage.DesktopHours += point.DesktopHours
		usage.HeadlessHours += point.HeadlessHours
	}

	var sandboxes []struct {
		SandboxID   string  `gorm:"column:sandbox_id"`
		Name        string  `gorm:"column:name"`
		Runtime     string  `gorm:"column:runtime"`
		PricingType string  `gorm:"column:pricing_type"`
		SpecTaskID  string  `gorm:"column:spec_task_id"`
		ProjectID   string  `gorm:"column:project_id"`
		VCPUs       int     `gorm:"column:vcpus"`
		Credits     float64 `gorm:"column:credits"`
		Hours       float64 `gorm:"column:hours"`
	}
	if err := s.gdb.WithContext(ctx).Raw(`
		WITH charged AS (
			SELECT sandbox_id, (array_agg(pricing_type ORDER BY credits DESC))[1] AS pricing_type, SUM(credits) AS credits
			FROM (`+charges+`) c
			GROUP BY sandbox_id
		), run_hours AS (
			SELECT r.sandbox_id, `+runPricingType+` AS pricing_type, r.runtime,
				GREATEST(0, EXTRACT(EPOCH FROM LEAST(COALESCE(r.stopped_at, @now), @to) - GREATEST(r.started_at, @from))) / 3600 AS hours
			`+runs+`
		), ran AS (
			SELECT sandbox_id, (array_agg(pricing_type ORDER BY hours DESC))[1] AS pricing_type,
				(array_agg(runtime ORDER BY hours DESC))[1] AS runtime, SUM(hours) AS hours
			FROM run_hours
			GROUP BY sandbox_id
		)
		SELECT COALESCE(ran.sandbox_id, charged.sandbox_id) AS sandbox_id,
			COALESCE(sandboxes.name, '') AS name,
			COALESCE(sandboxes.runtime, ran.runtime, '') AS runtime,
			COALESCE(charged.pricing_type, ran.pricing_type, 'headless') AS pricing_type,
			COALESCE(sandboxes.spec_task_id, '') AS spec_task_id,
			COALESCE(sandboxes.project_id, '') AS project_id,
			COALESCE(sandboxes.v_cpus, 0) AS vcpus,
			COALESCE(charged.credits, 0) AS credits,
			COALESCE(ran.hours, 0) AS hours
		FROM ran
		FULL JOIN charged ON charged.sandbox_id = ran.sandbox_id
		LEFT JOIN sandboxes ON sandboxes.id = COALESCE(ran.sandbox_id, charged.sandbox_id)
		ORDER BY credits DESC, hours DESC
		LIMIT @limit`, args).
		Scan(&sandboxes).Error; err != nil {
		return nil, fmt.Errorf("query compute usage by sandbox: %w", err)
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
			Hours:       row.Hours,
		})
	}

	var running int64
	runningQuery := s.gdb.WithContext(ctx).
		Model(&types.Sandbox{}).
		Where("organization_id = ? AND deleted_at IS NULL AND status = ?", q.OrganizationID, types.SandboxStatusRunning)
	if q.ProjectID != "" {
		runningQuery = runningQuery.Where("project_id = ?", q.ProjectID)
	}
	if q.TaskID != "" {
		runningQuery = runningQuery.Where(computeTaskIDExpr+" = ?", q.TaskID)
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

// computeDailyRow is one (day, pricing class) bucket of charges or runtime.
type computeDailyRow struct {
	Date        time.Time `gorm:"column:date"`
	PricingType string    `gorm:"column:pricing_type"`
	Credits     float64   `gorm:"column:credits"`
	Hours       float64   `gorm:"column:hours"`
}

const sandboxPricingTypeDesktop = "desktop"

// buildComputeDailySeries pivots (day, pricing type) rows into one point per
// day and fills the gaps, so a day with no sandboxes plots as zero instead of
// collapsing the x-axis onto the days that happened to have usage.
func buildComputeDailySeries(rows []computeDailyRow, from, to time.Time) []types.UsageComputeDailyPoint {
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
			point.DesktopHours += row.Hours
		} else {
			point.Headless += row.Credits
			point.HeadlessHours += row.Hours
		}
		point.Total += row.Credits
		point.TotalHours += row.Hours
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
