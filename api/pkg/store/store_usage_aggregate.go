package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// GroupedUsageQuery is the shared filter+pagination shape used by every
// /api/v1/usage/aggregate/* endpoint. All filters AND together; empty
// fields are not filtered. From/To are required and must be set by the
// caller (the handler defaults them to the last 7 days).
type GroupedUsageQuery struct {
	From, To time.Time

	OrganizationID string
	UserID         string
	ProjectID      string
	AppID          string
	SessionID      string
	Provider       string
	Model          string

	// SortBy is one of the column aliases produced by the query
	// (e.g. "total_cost", "total_tokens", "request_count", "last_activity").
	// Invalid values fall back to a sensible default per endpoint.
	SortBy  string
	SortDir string // "asc" or "desc"; defaults to "desc"

	Page     int // 1-indexed
	PageSize int
}

// applyUsageFilters adds the WHERE clauses common to every aggregate
// query to a *gorm.DB selecting from usage_metrics.
func applyUsageFilters(db *gorm.DB, q *GroupedUsageQuery) *gorm.DB {
	db = db.Where("created >= ? AND created <= ?", q.From, q.To)
	if q.OrganizationID != "" {
		db = db.Where("organization_id = ?", q.OrganizationID)
	}
	if q.UserID != "" {
		db = db.Where("user_id = ?", q.UserID)
	}
	if q.ProjectID != "" {
		db = db.Where("project_id = ?", q.ProjectID)
	}
	if q.AppID != "" {
		db = db.Where("app_id = ?", q.AppID)
	}
	if q.SessionID != "" {
		// usage_metrics has interaction_id, not session_id. Resolve via
		// the sessions table - one session has many interactions.
		db = db.Where("interaction_id IN (?)",
			db.Session(&gorm.Session{NewDB: true}).
				Table("interactions").
				Select("id").
				Where("session_id = ?", q.SessionID))
	}
	if q.Provider != "" {
		db = db.Where("provider = ?", q.Provider)
	}
	if q.Model != "" {
		db = db.Where("model = ?", q.Model)
	}
	return db
}

// usageTotalsSelect is the SELECT fragment producing every UsageTotals
// column. Used as a base for the GROUP BY queries.
const usageTotalsSelect = `
	COALESCE(SUM(prompt_tokens), 0)        AS prompt_tokens,
	COALESCE(SUM(completion_tokens), 0)    AS completion_tokens,
	COALESCE(SUM(cache_read_tokens), 0)    AS cache_read_tokens,
	COALESCE(SUM(cache_write_tokens), 0)   AS cache_write_tokens,
	COALESCE(SUM(total_tokens), 0)         AS total_tokens,
	COALESCE(SUM(prompt_cost), 0)          AS prompt_cost,
	COALESCE(SUM(completion_cost), 0)      AS completion_cost,
	COALESCE(SUM(cache_read_cost), 0)      AS cache_read_cost,
	COALESCE(SUM(cache_write_cost), 0)     AS cache_write_cost,
	COALESCE(SUM(total_cost), 0)           AS total_cost,
	COUNT(DISTINCT interaction_id)         AS request_count
`

func paginate(page, pageSize int) (offset, limit int) {
	if pageSize <= 0 {
		pageSize = 25
	}
	if pageSize > 200 {
		pageSize = 200
	}
	if page <= 0 {
		page = 1
	}
	return (page - 1) * pageSize, pageSize
}

func orderClause(sortBy, sortDir string, allowed map[string]string, fallback string) string {
	col, ok := allowed[sortBy]
	if !ok {
		col = allowed[fallback]
	}
	dir := "DESC"
	if sortDir == "asc" || sortDir == "ASC" {
		dir = "ASC"
	}
	return col + " " + dir
}

// GetUsageSummary returns whole-set totals plus a daily time-series.
// The handler is responsible for whichever scoping (cross-org, org-locked)
// is appropriate to the caller's role.
func (s *PostgresStore) GetUsageSummary(ctx context.Context, q *GroupedUsageQuery) (*types.UsageSummary, error) {
	totals, err := s.usageTotals(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("get usage totals: %w", err)
	}

	// Distinct counts for active users / sessions / projects within the
	// filtered window. Cheap on indexed columns.
	var counts struct {
		ActiveUsers    int
		ActiveSessions int
		ActiveProjects int
	}
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q)
	if err := base.
		Select(`
			COUNT(DISTINCT user_id) AS active_users,
			COUNT(DISTINCT (
				SELECT i.session_id FROM interactions i
				WHERE i.id = usage_metrics.interaction_id
			)) AS active_sessions,
			COUNT(DISTINCT project_id) AS active_projects
		`).
		Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("get usage counts: %w", err)
	}

	// Time series: reuse the existing aggregate that already handles
	// fill-in-missing-dates and the various aggregation levels.
	series, err := s.GetAggregatedUsageMetrics(ctx, &GetAggregatedUsageMetricsQuery{
		From:           q.From,
		To:             q.To,
		OrganizationID: q.OrganizationID,
		UserID:         q.UserID,
		ProjectID:      q.ProjectID,
	})
	if err != nil {
		return nil, fmt.Errorf("get usage time series: %w", err)
	}

	return &types.UsageSummary{
		From:           q.From,
		To:             q.To,
		UsageTotals:    *totals,
		ActiveUsers:    counts.ActiveUsers,
		ActiveSessions: counts.ActiveSessions,
		ActiveProjects: counts.ActiveProjects,
		TimeSeries:     series,
	}, nil
}

func (s *PostgresStore) usageTotals(ctx context.Context, q *GroupedUsageQuery) (*types.UsageTotals, error) {
	var t types.UsageTotals
	err := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Select(usageTotalsSelect).
		Scan(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

var byOrgSortCols = map[string]string{
	"total_cost":    "total_cost",
	"total_tokens":  "total_tokens",
	"request_count": "request_count",
	"last_activity": "last_activity",
	"":              "total_cost",
}

// GetUsageGroupedByOrg aggregates usage by organization. Global admin only;
// the handler must enforce that.
func (s *PostgresStore) GetUsageGroupedByOrg(ctx context.Context, q *GroupedUsageQuery) ([]*types.UsageByOrg, int, error) {
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Where("organization_id != ''")

	var totalRows int64
	if err := base.Session(&gorm.Session{}).
		Distinct("organization_id").
		Count(&totalRows).Error; err != nil {
		return nil, 0, fmt.Errorf("count orgs: %w", err)
	}

	offset, limit := paginate(q.Page, q.PageSize)
	order := orderClause(q.SortBy, q.SortDir, byOrgSortCols, "")

	var rows []*types.UsageByOrg
	err := base.
		Select(`
			usage_metrics.organization_id          AS organization_id,
			COALESCE(orgs.display_name, orgs.name) AS organization_name,
			` + usageTotalsSelect + `,
			COUNT(DISTINCT user_id)    AS user_count,
			COUNT(DISTINCT (SELECT i.session_id FROM interactions i WHERE i.id = usage_metrics.interaction_id)) AS session_count,
			MAX(created) AS last_activity
		`).
		Joins("LEFT JOIN organizations orgs ON orgs.id = usage_metrics.organization_id").
		Group("usage_metrics.organization_id, organization_name").
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list orgs: %w", err)
	}
	return rows, int(totalRows), nil
}

var byUserSortCols = map[string]string{
	"total_cost":    "total_cost",
	"total_tokens":  "total_tokens",
	"request_count": "request_count",
	"last_activity": "last_activity",
	"":              "total_cost",
}

func (s *PostgresStore) GetUsageGroupedByUser(ctx context.Context, q *GroupedUsageQuery) ([]*types.UsageByUser, int, error) {
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Where("user_id != ''")

	var totalRows int64
	if err := base.Session(&gorm.Session{}).
		Distinct("user_id").
		Count(&totalRows).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	offset, limit := paginate(q.Page, q.PageSize)
	order := orderClause(q.SortBy, q.SortDir, byUserSortCols, "")

	var rows []*types.UsageByUser
	err := base.
		Select(`
			usage_metrics.user_id              AS user_id,
			COALESCE(users.email, '')          AS email,
			usage_metrics.organization_id      AS organization_id,
			` + usageTotalsSelect + `,
			COUNT(DISTINCT (SELECT i.session_id FROM interactions i WHERE i.id = usage_metrics.interaction_id)) AS session_count,
			MAX(created) AS last_activity
		`).
		Joins("LEFT JOIN users ON users.id = usage_metrics.user_id").
		Group("usage_metrics.user_id, users.email, usage_metrics.organization_id").
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return rows, int(totalRows), nil
}

var byProjectSortCols = map[string]string{
	"total_cost":    "total_cost",
	"total_tokens":  "total_tokens",
	"request_count": "request_count",
	"":              "total_cost",
}

func (s *PostgresStore) GetUsageGroupedByProject(ctx context.Context, q *GroupedUsageQuery) ([]*types.UsageByProject, int, error) {
	// Group by (project_id, app_id) so both project-level and
	// app-level rows surface. The handler can fan out by Kind.
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Where("project_id != '' OR app_id != ''")

	var totalRows int64
	if err := base.Session(&gorm.Session{}).
		Distinct("project_id", "app_id").
		Count(&totalRows).Error; err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	offset, limit := paginate(q.Page, q.PageSize)
	order := orderClause(q.SortBy, q.SortDir, byProjectSortCols, "")

	var rows []*types.UsageByProject
	err := base.
		Select(`
			usage_metrics.project_id           AS project_id,
			usage_metrics.app_id               AS app_id,
			COALESCE(NULLIF(apps.id, ''), projects.id, '') AS _ignore_id,
			COALESCE(NULLIF(apps.id, ''), projects.name, '') AS name,
			CASE WHEN usage_metrics.app_id != '' THEN 'app' ELSE 'project' END AS kind,
			COALESCE(NULLIF(apps.owner, ''), projects.owner, '') AS owner_user_id,
			usage_metrics.organization_id      AS organization_id,
			` + usageTotalsSelect + `,
			COUNT(DISTINCT (SELECT i.session_id FROM interactions i WHERE i.id = usage_metrics.interaction_id)) AS session_count
		`).
		Joins("LEFT JOIN projects ON projects.id = usage_metrics.project_id").
		Joins("LEFT JOIN apps ON apps.id = usage_metrics.app_id").
		Group("usage_metrics.project_id, usage_metrics.app_id, apps.id, apps.owner, projects.id, projects.name, projects.owner, usage_metrics.organization_id").
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list projects: %w", err)
	}
	return rows, int(totalRows), nil
}

var bySessionSortCols = map[string]string{
	"total_cost":   "total_cost",
	"total_tokens": "total_tokens",
	"call_count":   "call_count",
	"started_at":   "started_at",
	"ended_at":     "ended_at",
	"":             "ended_at",
}

func (s *PostgresStore) GetUsageGroupedBySession(ctx context.Context, q *GroupedUsageQuery) ([]*types.UsageBySession, int, error) {
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Joins("INNER JOIN interactions ON interactions.id = usage_metrics.interaction_id")

	var totalRows int64
	if err := base.Session(&gorm.Session{}).
		Distinct("interactions.session_id").
		Count(&totalRows).Error; err != nil {
		return nil, 0, fmt.Errorf("count sessions: %w", err)
	}

	offset, limit := paginate(q.Page, q.PageSize)
	order := orderClause(q.SortBy, q.SortDir, bySessionSortCols, "")

	var rows []*types.UsageBySession
	err := base.
		Select(`
			interactions.session_id        AS session_id,
			COALESCE(sessions.name, '')    AS name,
			usage_metrics.user_id          AS user_id,
			usage_metrics.project_id       AS project_id,
			usage_metrics.organization_id  AS organization_id,
			MAX(usage_metrics.provider)    AS provider,
			MAX(usage_metrics.model)       AS model,
			COALESCE(SUM(usage_metrics.prompt_tokens), 0)      AS prompt_tokens,
			COALESCE(SUM(usage_metrics.completion_tokens), 0)  AS completion_tokens,
			COALESCE(SUM(usage_metrics.cache_read_tokens), 0)  AS cache_read_tokens,
			COALESCE(SUM(usage_metrics.cache_write_tokens), 0) AS cache_write_tokens,
			COALESCE(SUM(usage_metrics.total_tokens), 0)       AS total_tokens,
			COALESCE(SUM(usage_metrics.prompt_cost), 0)        AS prompt_cost,
			COALESCE(SUM(usage_metrics.completion_cost), 0)    AS completion_cost,
			COALESCE(SUM(usage_metrics.cache_read_cost), 0)    AS cache_read_cost,
			COALESCE(SUM(usage_metrics.cache_write_cost), 0)   AS cache_write_cost,
			COALESCE(SUM(usage_metrics.total_cost), 0)         AS total_cost,
			COUNT(DISTINCT usage_metrics.interaction_id)       AS request_count,
			COUNT(DISTINCT usage_metrics.interaction_id)       AS call_count,
			MIN(usage_metrics.created)     AS started_at,
			MAX(usage_metrics.created)     AS ended_at
		`).
		Joins("LEFT JOIN sessions ON sessions.id = interactions.session_id").
		Group("interactions.session_id, sessions.name, usage_metrics.user_id, usage_metrics.project_id, usage_metrics.organization_id").
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list sessions: %w", err)
	}
	return rows, int(totalRows), nil
}

var byModelSortCols = map[string]string{
	"total_cost":    "total_cost",
	"total_tokens":  "total_tokens",
	"request_count": "request_count",
	"":              "total_cost",
}

func (s *PostgresStore) GetUsageGroupedByModel(ctx context.Context, q *GroupedUsageQuery) ([]*types.UsageByModel, int, error) {
	base := applyUsageFilters(s.gdb.WithContext(ctx).Model(&types.UsageMetric{}), q).
		Where("provider != '' AND model != ''")

	var totalRows int64
	if err := base.Session(&gorm.Session{}).
		Distinct("provider", "model").
		Count(&totalRows).Error; err != nil {
		return nil, 0, fmt.Errorf("count models: %w", err)
	}

	offset, limit := paginate(q.Page, q.PageSize)
	order := orderClause(q.SortBy, q.SortDir, byModelSortCols, "")

	var rows []*types.UsageByModel
	err := base.
		Select(`
			provider,
			model,
			` + usageTotalsSelect + `,
			COUNT(DISTINCT user_id) AS unique_users,
			COUNT(DISTINCT (SELECT i.session_id FROM interactions i WHERE i.id = usage_metrics.interaction_id)) AS unique_sessions,
			COUNT(DISTINCT project_id) AS unique_projects
		`).
		Group("provider, model").
		Order(order).
		Offset(offset).
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("list models: %w", err)
	}
	return rows, int(totalRows), nil
}
