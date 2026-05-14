package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// usageInteractionsJoin is a per-row LATERAL lookup that fetches the
// session/app/user/timestamps for the interaction referenced by a usage_metrics
// row. It avoids the previous approach of aggregating the entire `interactions`
// table on every breakdown query — with the (id, generation_id) PK index this
// is an O(log N) index seek per matched usage_metrics row, so it scales with
// the size of the org+date window instead of the whole interactions table.
const usageInteractionsJoin = `
	LEFT JOIN LATERAL (
		SELECT
			MAX(session_id) AS session_id,
			MAX(app_id)     AS app_id,
			MAX(user_id)    AS user_id,
			MIN(created)    AS created,
			MAX(updated)    AS updated,
			MAX(completed)  AS completed
		FROM interactions
		WHERE interactions.id = usage_metrics.interaction_id
		  AND usage_metrics.interaction_id <> ''
	) usage_interactions ON true
`

func (s *PostgresStore) CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
	if metric.ID == "" {
		metric.ID = system.GenerateUsageMetricID()
	}

	// For tests we supply custom time
	if metric.Created.IsZero() {
		metric.Created = time.Now()
	}
	// Set the date field to just the date part (truncate time portion)
	metric.Date = metric.Created.Truncate(24 * time.Hour)

	err := s.gdb.WithContext(ctx).Create(metric).Error
	if err != nil {
		return nil, err
	}
	return metric, nil
}

func (s *PostgresStore) GetAppUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.UsageMetric, error) {
	if appID == "" {
		return nil, errors.New("app_id is required")
	}

	var metrics []*types.UsageMetric
	err := s.gdb.WithContext(ctx).
		Where("app_id = ? AND created >= ? AND created <= ?", appID, from, to).
		Order("created DESC").
		Find(&metrics).Error

	return metrics, err
}

func (s *PostgresStore) GetAppDailyUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.AggregatedUsageMetric, error) {
	var metrics []*types.AggregatedUsageMetric
	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			date,
			app_id,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(total_tokens) as total_tokens,
			SUM(cache_read_tokens) as cache_read_tokens,
			SUM(cache_write_tokens) as cache_write_tokens,
			SUM(total_cost) as total_cost,
			SUM(cache_read_cost) as cache_read_cost,
			SUM(cache_write_cost) as cache_write_cost,
			AVG(duration_ms) as latency_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT interaction_id) as total_requests
		`).
		Where("app_id = ? AND date >= ? AND date <= ?", appID, from, to).
		Group("date, app_id").
		Order("date ASC").
		Find(&metrics).Error

	if err != nil {
		return nil, err
	}

	completeMetrics := fillInMissingDates(metrics, from, to)

	return completeMetrics, nil
}

func (s *PostgresStore) GetProviderDailyUsageMetrics(ctx context.Context, providerID string, from time.Time, to time.Time) ([]*types.AggregatedUsageMetric, error) {
	var metrics []*types.AggregatedUsageMetric
	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			date,
			provider,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(total_tokens) as total_tokens,
			SUM(cache_read_tokens) as cache_read_tokens,
			SUM(cache_write_tokens) as cache_write_tokens,
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(cache_read_cost) as cache_read_cost,
			SUM(cache_write_cost) as cache_write_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as latency_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT interaction_id) as total_requests
		`).
		Where("provider = ? AND date >= ? AND date <= ?", providerID, from, to).
		Group("date, provider").
		Order("date ASC").
		Find(&metrics).Error

	if err != nil {
		return nil, err
	}

	completeMetrics := fillInMissingDates(metrics, from, to)

	return completeMetrics, nil
}

// GetUsersAggregatedUsageMetrics returns a list of users and their aggregated usage metrics for a given provider. Usage aggregated per day
func (s *PostgresStore) GetUsersAggregatedUsageMetrics(ctx context.Context, provider string, from time.Time, to time.Time) ([]*types.UsersAggregatedUsageMetric, error) {
	metrics := []*types.UsersAggregatedUsageMetric{}

	// First get the aggregated metrics per user
	var userMetrics []struct {
		UserID            string    `gorm:"column:user_id"`
		Date              time.Time `gorm:"column:date"`
		PromptTokens      int       `gorm:"column:prompt_tokens"`
		CompletionTokens  int       `gorm:"column:completion_tokens"`
		TotalTokens       int       `gorm:"column:total_tokens"`
		CacheReadTokens   int       `gorm:"column:cache_read_tokens"`
		CacheWriteTokens  int       `gorm:"column:cache_write_tokens"`
		PromptCost        float64   `gorm:"column:prompt_cost"`
		CompletionCost    float64   `gorm:"column:completion_cost"`
		CacheReadCost     float64   `gorm:"column:cache_read_cost"`
		CacheWriteCost    float64   `gorm:"column:cache_write_cost"`
		TotalCost         float64   `gorm:"column:total_cost"`
		DurationMs        float64   `gorm:"column:duration_ms"`
		RequestSizeBytes  int       `gorm:"column:request_size_bytes"`
		ResponseSizeBytes int       `gorm:"column:response_size_bytes"`
		TotalRequests     int       `gorm:"column:total_requests"`
	}

	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			user_id,
			date,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(total_tokens) as total_tokens,
			SUM(cache_read_tokens) as cache_read_tokens,
			SUM(cache_write_tokens) as cache_write_tokens,
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(cache_read_cost) as cache_read_cost,
			SUM(cache_write_cost) as cache_write_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT id) as total_requests
		`).
		Where("provider = ? AND date >= ? AND date <= ?", provider, from, to).
		Group("user_id, date").
		Order("user_id ASC, date ASC").
		Find(&userMetrics).Error

	if err != nil {
		return nil, err
	}

	// Create a map to group metrics by user_id
	userMetricsMap := make(map[string][]*types.AggregatedUsageMetric)
	userIDs := make(map[string]bool)

	for _, m := range userMetrics {
		userIDs[m.UserID] = true
		userMetricsMap[m.UserID] = append(userMetricsMap[m.UserID], &types.AggregatedUsageMetric{
			Date:              m.Date,
			PromptTokens:      m.PromptTokens,
			CompletionTokens:  m.CompletionTokens,
			TotalTokens:       m.TotalTokens,
			CacheReadTokens:   m.CacheReadTokens,
			CacheWriteTokens:  m.CacheWriteTokens,
			PromptCost:        m.PromptCost,
			CompletionCost:    m.CompletionCost,
			CacheReadCost:     m.CacheReadCost,
			CacheWriteCost:    m.CacheWriteCost,
			TotalCost:         m.TotalCost,
			LatencyMs:         m.DurationMs,
			RequestSizeBytes:  m.RequestSizeBytes,
			ResponseSizeBytes: m.ResponseSizeBytes,
			TotalRequests:     m.TotalRequests,
		})
	}

	// Get user information for all users that have metrics
	var users []types.User
	if len(userIDs) > 0 {
		userIDList := make([]string, 0, len(userIDs))
		for userID := range userIDs {
			userIDList = append(userIDList, userID)
		}

		err = s.gdb.WithContext(ctx).
			Model(&types.User{}).
			Where("id IN ?", userIDList).
			Find(&users).Error

		if err != nil {
			return nil, err
		}
	}

	// Create final response combining user info with their metrics
	for _, user := range users {
		userMetrics := userMetricsMap[user.ID]
		if userMetrics == nil {
			userMetrics = []*types.AggregatedUsageMetric{}
		}

		// Fill in missing dates
		completeMetrics := fillInMissingDates(userMetrics, from, to)

		// Convert []*AggregatedUsageMetric to []AggregatedUsageMetric
		convertedMetrics := make([]types.AggregatedUsageMetric, len(completeMetrics))
		for i, m := range completeMetrics {
			convertedMetrics[i] = *m
		}

		metrics = append(metrics, &types.UsersAggregatedUsageMetric{
			User:    user,
			Metrics: convertedMetrics,
		})
	}

	return metrics, nil
}

func (s *PostgresStore) GetAggregatedUsageMetrics(ctx context.Context, q *GetAggregatedUsageMetricsQuery) ([]*types.AggregatedUsageMetric, error) {
	metrics := []*types.AggregatedUsageMetric{}

	aggregationLevel := q.AggregationLevel
	if aggregationLevel == "" {
		aggregationLevel = AggregationLevelDaily
	}

	var dateExpr string
	var groupBy string
	switch aggregationLevel {
	case AggregationLevel5Min:
		dateExpr = "date_trunc('hour', usage_metrics.created) + INTERVAL '5 min' * FLOOR(EXTRACT(MINUTE FROM usage_metrics.created) / 5) as date"
		groupBy = "date_trunc('hour', usage_metrics.created) + INTERVAL '5 min' * FLOOR(EXTRACT(MINUTE FROM usage_metrics.created) / 5)"
	case AggregationLevelHourly:
		dateExpr = "date_trunc('hour', usage_metrics.created) as date"
		groupBy = "date_trunc('hour', usage_metrics.created)"
	default:
		dateExpr = "usage_metrics.date as date"
		groupBy = "usage_metrics.date"
	}

	query := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			` + dateExpr + `,
			SUM(usage_metrics.prompt_tokens) as prompt_tokens,
			SUM(usage_metrics.completion_tokens) as completion_tokens,
			SUM(usage_metrics.cache_read_tokens) as cache_read_tokens,
			SUM(usage_metrics.cache_write_tokens) as cache_write_tokens,
			SUM(usage_metrics.prompt_cost) as prompt_cost,
			SUM(usage_metrics.completion_cost) as completion_cost,
			SUM(usage_metrics.cache_read_cost) as cache_read_cost,
			SUM(usage_metrics.cache_write_cost) as cache_write_cost,
			SUM(usage_metrics.total_tokens) as total_tokens,
			SUM(usage_metrics.total_cost) as total_cost,
			AVG(usage_metrics.duration_ms) as latency_ms,
			SUM(usage_metrics.request_size_bytes) as request_size_bytes,
			SUM(usage_metrics.response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT usage_metrics.id) as total_requests
		`)

	if q.SessionID != "" || q.AppID != "" {
		query = query.Joins(usageInteractionsJoin)
	}
	if q.ProjectID != "" {
		query = query.Where("usage_metrics.project_id = ?", q.ProjectID)
	}
	if q.SpecTaskID != "" {
		query = query.Where("usage_metrics.spec_task_id = ?", q.SpecTaskID)
	}
	if q.UserID != "" {
		query = query.Where("usage_metrics.user_id = ?", q.UserID)
	}
	if q.OrganizationID != "" {
		query = query.Where("usage_metrics.organization_id = ?", q.OrganizationID)
	}
	if q.AppID != "" {
		query = query.Where("COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id) = ?", q.AppID)
	}
	if q.SessionID != "" {
		query = query.Where("usage_interactions.session_id = ?", q.SessionID)
	}
	if q.Provider != "" {
		query = query.Where("usage_metrics.provider = ?", q.Provider)
	}
	if q.Model != "" {
		query = query.Where("usage_metrics.model = ?", q.Model)
	}

	query = query.Where("usage_metrics.created >= ? AND usage_metrics.created <= ?", q.From, q.To)

	err := query.Group(groupBy).
		Order("date ASC").
		Find(&metrics).Error
	if err != nil {
		return nil, err
	}

	var completeMetrics []*types.AggregatedUsageMetric
	switch aggregationLevel {
	case AggregationLevel5Min:
		completeMetrics = fillInMissing5Minutes(metrics, q.From, q.To)
	case AggregationLevelHourly:
		completeMetrics = fillInMissingHours(metrics, q.From, q.To)
	default:
		completeMetrics = fillInMissingDates(metrics, q.From, q.To)
	}

	return completeMetrics, nil
}

func (s *PostgresStore) GetOrgUsageSummary(ctx context.Context, q *GetOrgUsageSummaryQuery) (*types.OrgUsageSummaryResponse, error) {
	if q == nil {
		return nil, errors.New("query is required")
	}
	if q.OrganizationID == "" {
		return nil, errors.New("organization_id is required")
	}
	userLimit := q.UserLimit
	if userLimit <= 0 {
		userLimit = 10
	}
	if userLimit > 100 {
		userLimit = 100
	}
	userOffset := q.UserOffset
	if userOffset < 0 {
		userOffset = 0
	}
	projectLimit := boundedUsageLimit(q.ProjectLimit)
	projectOffset := q.ProjectOffset
	if projectOffset < 0 {
		projectOffset = 0
	}
	taskLimit := boundedUsageLimit(q.TaskLimit)
	taskOffset := q.TaskOffset
	if taskOffset < 0 {
		taskOffset = 0
	}
	sessionLimit := q.SessionLimit
	if sessionLimit <= 0 {
		sessionLimit = 10
	}
	if sessionLimit > 100 {
		sessionLimit = 100
	}
	sessionOffset := q.SessionOffset
	if sessionOffset < 0 {
		sessionOffset = 0
	}

	resp := &types.OrgUsageSummaryResponse{}

	// Phase 1: fan out independent breakdown queries. Each call into
	// orgUsageBreakdownQuery / orgUsageBaseQuery starts a fresh gorm chain
	// from s.gdb, so the goroutines do not share builder state; the pgx
	// connection pool isolates them per-query. We cap concurrency to keep
	// the connection pool from being monopolised by one summary request.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(6)

	g.Go(func() error {
		metrics, err := s.GetAggregatedUsageMetrics(gctx, &GetAggregatedUsageMetricsQuery{
			AggregationLevel: AggregationLevelDaily,
			OrganizationID:   q.OrganizationID,
			From:             q.From,
			To:               q.To,
			UserID:           q.UserID,
			ProjectID:        q.ProjectID,
			AppID:            q.AppID,
			SessionID:        q.SessionID,
			Provider:         q.Provider,
			Model:            q.Model,
		})
		if err != nil {
			return err
		}
		resp.Metrics = metrics
		return nil
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "project").Limit(projectLimit).Offset(projectOffset).Find(&resp.Projects).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "project_model").Find(&resp.ProjectModels).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "app").Limit(10).Find(&resp.Apps).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "task_model").Limit(taskLimit).Offset(taskOffset).Find(&resp.Tasks).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "session").Limit(sessionLimit).Offset(sessionOffset).Find(&resp.Sessions).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "model").Limit(10).Find(&resp.Models).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "user").Limit(userLimit).Offset(userOffset).Find(&resp.Users).Error
	})
	g.Go(func() error {
		countQuery := s.orgUsageBaseQuery(gctx, q).
			Joins("LEFT JOIN users ON users.id = usage_metrics.user_id").
			Where("usage_metrics.user_id <> ''")
		if q.UserSearch != "" {
			search := "%" + q.UserSearch + "%"
			countQuery = countQuery.Where(
				"users.email ILIKE ? OR users.username ILIKE ? OR users.full_name ILIKE ? OR usage_metrics.user_id ILIKE ?",
				search, search, search, search,
			)
		}
		return countQuery.Distinct("usage_metrics.user_id").Count(&resp.UsersTotal).Error
	})
	g.Go(func() error {
		count, err := s.countOrgUsageBreakdownRows(gctx, q, "project")
		if err != nil {
			return err
		}
		resp.ProjectsTotal = count
		return nil
	})
	g.Go(func() error {
		count, err := s.countOrgUsageBreakdownRows(gctx, q, "task_model")
		if err != nil {
			return err
		}
		resp.TasksTotal = count
		return nil
	})
	g.Go(func() error {
		count, err := s.countOrgUsageBreakdownRows(gctx, q, "session")
		if err != nil {
			return err
		}
		resp.SessionsTotal = count
		return nil
	})
	g.Go(func() error {
		return s.orgUsageActiveCounts(gctx, q, resp)
	})
	g.Go(func() error {
		return s.orgUsageFilterOptions(gctx, q, resp)
	})
	g.Go(func() error {
		return s.orgUsageExportRows(gctx, q, resp)
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Phase 2: model time series depends on which models came out on top.
	modelSeries, err := s.getOrgUsageModelTimeSeries(ctx, q, resp.Models)
	if err != nil {
		return nil, err
	}
	resp.ModelTimeSeries = modelSeries

	return resp, nil
}

func boundedUsageLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (s *PostgresStore) countOrgUsageBreakdownRows(ctx context.Context, q *GetOrgUsageSummaryQuery, dimension string) (int64, error) {
	var count int64
	subquery := s.orgUsageBreakdownQuery(ctx, q, dimension).Limit(-1).Offset(-1)
	err := s.gdb.WithContext(ctx).Table("(?) as usage_breakdown_rows", subquery).Count(&count).Error
	return count, err
}

func (s *PostgresStore) orgUsageBreakdownQuery(ctx context.Context, q *GetOrgUsageSummaryQuery, dimension string) *gorm.DB {
	selectFields := `
		SUM(usage_metrics.prompt_tokens) as prompt_tokens,
		SUM(usage_metrics.completion_tokens) as completion_tokens,
		SUM(usage_metrics.cache_read_tokens) as cache_read_tokens,
		SUM(usage_metrics.cache_write_tokens) as cache_write_tokens,
		SUM(usage_metrics.prompt_cost) as prompt_cost,
		SUM(usage_metrics.completion_cost) as completion_cost,
		SUM(usage_metrics.cache_read_cost) as cache_read_cost,
		SUM(usage_metrics.cache_write_cost) as cache_write_cost,
		SUM(usage_metrics.total_tokens) as total_tokens,
		SUM(usage_metrics.total_cost) as total_cost,
		AVG(usage_metrics.duration_ms) as latency_ms,
		SUM(usage_metrics.request_size_bytes) as request_size_bytes,
		SUM(usage_metrics.response_size_bytes) as response_size_bytes,
		COUNT(DISTINCT usage_metrics.id) as total_requests,
		COUNT(DISTINCT NULLIF(usage_interactions.session_id, '')) as session_count,
		COUNT(DISTINCT NULLIF(usage_metrics.user_id, '')) as unique_users,
		COUNT(DISTINCT NULLIF(usage_interactions.session_id, '')) as unique_sessions,
		COUNT(DISTINCT NULLIF(usage_metrics.project_id, '')) as unique_projects,
		COUNT(DISTINCT NULLIF(COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id), '')) as unique_apps,
		MAX(usage_metrics.created) as last_activity_at
	`

	query := s.orgUsageBaseQuery(ctx, q)

	switch dimension {
	case "project":
		return query.
			Joins("LEFT JOIN projects ON projects.id = usage_metrics.project_id").
			Select("usage_metrics.project_id as id, COALESCE(NULLIF(projects.name, ''), NULLIF(usage_metrics.project_id, ''), 'Unassigned') as name, " + selectFields).
			Group("usage_metrics.project_id, projects.name").
			Order("total_tokens DESC")
	case "project_model":
		return query.
			Joins("LEFT JOIN projects ON projects.id = usage_metrics.project_id").
			Where("usage_metrics.model <> ''").
			Select("usage_metrics.project_id as id, COALESCE(NULLIF(projects.name, ''), NULLIF(usage_metrics.project_id, ''), 'Unassigned') as name, usage_metrics.provider, usage_metrics.model, " + selectFields).
			Group("usage_metrics.project_id, projects.name, usage_metrics.provider, usage_metrics.model").
			Order("total_tokens DESC")
	case "app":
		appIDExpr := "COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id)"
		appNameExpr := "COALESCE(MAX(NULLIF(apps.config->'helix'->>'name', '')), NULLIF(apps.id, ''), 'Unassigned')"
		return query.
			Joins("LEFT JOIN apps ON apps.id = " + appIDExpr).
			Select(appIDExpr + " as id, " + appNameExpr + " as name, " + selectFields).
			Group(appIDExpr + ", apps.id").
			Order("total_tokens DESC")
	case "task_model":
		return query.
			Joins("LEFT JOIN spec_tasks ON spec_tasks.id = usage_metrics.spec_task_id").
			Where("usage_metrics.spec_task_id <> ''").
			Select("usage_metrics.spec_task_id || ':' || usage_metrics.provider || ':' || usage_metrics.model as id, COALESCE(NULLIF(spec_tasks.name, ''), usage_metrics.spec_task_id) as name, usage_metrics.provider, usage_metrics.model, " + selectFields).
			Group("usage_metrics.spec_task_id, spec_tasks.name, usage_metrics.provider, usage_metrics.model").
			Order("total_tokens DESC")
	case "session":
		return query.
			Joins("LEFT JOIN sessions ON sessions.id = usage_interactions.session_id").
			Where("usage_interactions.session_id <> ''").
			Select(`
				usage_interactions.session_id as id,
				usage_interactions.session_id as session_id,
				COALESCE(NULLIF(sessions.name, ''), usage_interactions.session_id) as name,
				MIN(usage_interactions.created) as started_at,
				MAX(usage_interactions.completed) as ended_at,
				` + selectFields).
			Group("usage_interactions.session_id, sessions.name").
			Order("total_tokens DESC")
	case "model":
		return query.
			Where("usage_metrics.model <> ''").
			Select("usage_metrics.provider || ':' || usage_metrics.model as id, usage_metrics.model as name, usage_metrics.provider, usage_metrics.model, " + selectFields).
			Group("usage_metrics.provider, usage_metrics.model").
			Order("total_tokens DESC")
	case "user":
		query = query.
			Joins("LEFT JOIN users ON users.id = usage_metrics.user_id").
			Where("usage_metrics.user_id <> ''")
		if q.UserSearch != "" {
			search := "%" + q.UserSearch + "%"
			query = query.Where(
				"users.email ILIKE ? OR users.username ILIKE ? OR users.full_name ILIKE ? OR usage_metrics.user_id ILIKE ?",
				search, search, search, search,
			)
		}
		return query.
			Select("usage_metrics.user_id as id, COALESCE(NULLIF(users.full_name, ''), NULLIF(users.username, ''), NULLIF(users.email, ''), usage_metrics.user_id) as name, users.email, users.username, " + selectFields).
			Group("usage_metrics.user_id, users.full_name, users.username, users.email").
			Order("total_tokens DESC")
	default:
		return query.Select(selectFields).Order("total_tokens DESC")
	}
}

func (s *PostgresStore) orgUsageBaseQuery(ctx context.Context, q *GetOrgUsageSummaryQuery) *gorm.DB {
	query := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Joins(usageInteractionsJoin).
		Where("usage_metrics.organization_id = ? AND usage_metrics.created >= ? AND usage_metrics.created <= ?", q.OrganizationID, q.From, q.To)

	if q.UserID != "" {
		query = query.Where("usage_metrics.user_id = ?", q.UserID)
	}
	if q.ProjectID != "" {
		query = query.Where("usage_metrics.project_id = ?", q.ProjectID)
	}
	if q.AppID != "" {
		query = query.Where("COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id) = ?", q.AppID)
	}
	if q.SessionID != "" {
		query = query.Where("usage_interactions.session_id = ?", q.SessionID)
	}
	if q.Provider != "" {
		query = query.Where("usage_metrics.provider = ?", q.Provider)
	}
	if q.Model != "" {
		query = query.Where("usage_metrics.model = ?", q.Model)
	}

	return query
}

func (s *PostgresStore) orgUsageActiveCounts(ctx context.Context, q *GetOrgUsageSummaryQuery, resp *types.OrgUsageSummaryResponse) error {
	var row struct {
		ActiveUsers    int `gorm:"column:active_users"`
		ActiveSessions int `gorm:"column:active_sessions"`
		ActiveProjects int `gorm:"column:active_projects"`
		ActiveApps     int `gorm:"column:active_apps"`
	}
	err := s.orgUsageBaseQuery(ctx, q).
		Select(`
			COUNT(DISTINCT NULLIF(usage_metrics.user_id, '')) as active_users,
			COUNT(DISTINCT NULLIF(usage_interactions.session_id, '')) as active_sessions,
			COUNT(DISTINCT NULLIF(usage_metrics.project_id, '')) as active_projects,
			COUNT(DISTINCT NULLIF(COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id), '')) as active_apps
		`).
		Scan(&row).Error
	if err != nil {
		return err
	}
	resp.ActiveUsers = row.ActiveUsers
	resp.ActiveSessions = row.ActiveSessions
	resp.ActiveProjects = row.ActiveProjects
	resp.ActiveApps = row.ActiveApps
	return nil
}

func (s *PostgresStore) orgUsageFilterOptions(ctx context.Context, q *GetOrgUsageSummaryQuery, resp *types.OrgUsageSummaryResponse) error {
	optionQuery := *q
	optionQuery.UserID = ""
	optionQuery.ProjectID = ""
	optionQuery.AppID = ""
	optionQuery.SessionID = ""
	optionQuery.Provider = ""
	optionQuery.Model = ""
	optionQuery.UserSearch = ""

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)

	g.Go(func() error {
		return s.orgUsageBaseQuery(gctx, &optionQuery).
			Joins("LEFT JOIN users ON users.id = usage_metrics.user_id").
			Where("usage_metrics.user_id <> ''").
			Select(`
				usage_metrics.user_id as id,
				COALESCE(NULLIF(users.full_name, ''), NULLIF(users.username, ''), NULLIF(users.email, ''), usage_metrics.user_id) as name,
				users.email,
				users.username
			`).
			Group("usage_metrics.user_id, users.full_name, users.username, users.email").
			Order("name ASC").
			Limit(1000).
			Find(&resp.FilterUsers).Error
	})

	g.Go(func() error {
		return s.orgUsageBaseQuery(gctx, &optionQuery).
			Joins("LEFT JOIN projects ON projects.id = usage_metrics.project_id").
			Where("usage_metrics.project_id <> ''").
			Select("usage_metrics.project_id as id, COALESCE(NULLIF(projects.name, ''), usage_metrics.project_id) as name").
			Group("usage_metrics.project_id, projects.name").
			Order("name ASC").
			Limit(1000).
			Find(&resp.FilterProjects).Error
	})

	g.Go(func() error {
		appIDExpr := "COALESCE(NULLIF(usage_metrics.app_id, ''), usage_interactions.app_id)"
		appNameExpr := "COALESCE(MAX(NULLIF(apps.config->'helix'->>'name', '')), NULLIF(apps.id, ''), " + appIDExpr + ")"
		return s.orgUsageBaseQuery(gctx, &optionQuery).
			Joins("LEFT JOIN apps ON apps.id = " + appIDExpr).
			Where(appIDExpr + " <> ''").
			Select(appIDExpr + " as id, " + appNameExpr + " as name").
			Group(appIDExpr + ", apps.id").
			Order("name ASC").
			Limit(1000).
			Find(&resp.FilterApps).Error
	})

	g.Go(func() error {
		return s.orgUsageBaseQuery(gctx, &optionQuery).
			Where("usage_metrics.model <> ''").
			Select("usage_metrics.provider || ':' || usage_metrics.model as id, usage_metrics.model as name, usage_metrics.provider, usage_metrics.model").
			Group("usage_metrics.provider, usage_metrics.model").
			Order("usage_metrics.provider ASC, usage_metrics.model ASC").
			Limit(1000).
			Find(&resp.FilterModels).Error
	})

	return g.Wait()
}

func (s *PostgresStore) orgUsageExportRows(ctx context.Context, q *GetOrgUsageSummaryQuery, resp *types.OrgUsageSummaryResponse) error {
	const exportLimit = 10000

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(6)

	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "project").Limit(exportLimit).Find(&resp.ExportProjects).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "app").Limit(exportLimit).Find(&resp.ExportApps).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "task_model").Limit(exportLimit).Find(&resp.ExportTasks).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "session").Limit(exportLimit).Find(&resp.ExportSessions).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "model").Limit(exportLimit).Find(&resp.ExportModels).Error
	})
	g.Go(func() error {
		return s.orgUsageBreakdownQuery(gctx, q, "user").Limit(exportLimit).Find(&resp.ExportUsers).Error
	})

	return g.Wait()
}

func (s *PostgresStore) getOrgUsageModelTimeSeries(ctx context.Context, q *GetOrgUsageSummaryQuery, topModels []types.UsageBreakdownRow) ([]types.UsageModelTimeSeries, error) {
	if len(topModels) == 0 {
		return []types.UsageModelTimeSeries{}, nil
	}
	topModelKeys := make(map[string]struct{}, len(topModels))
	conditions := make([]string, 0, len(topModels))
	args := make([]interface{}, 0, len(topModels)*2)
	for _, model := range topModels {
		topModelKeys[model.Provider+":"+model.Model] = struct{}{}
		conditions = append(conditions, "(usage_metrics.provider = ? AND usage_metrics.model = ?)")
		args = append(args, model.Provider, model.Model)
	}

	var rows []struct {
		Date              time.Time `gorm:"column:date"`
		Provider          string    `gorm:"column:provider"`
		Model             string    `gorm:"column:model"`
		PromptTokens      int       `gorm:"column:prompt_tokens"`
		CompletionTokens  int       `gorm:"column:completion_tokens"`
		CacheReadTokens   int       `gorm:"column:cache_read_tokens"`
		CacheWriteTokens  int       `gorm:"column:cache_write_tokens"`
		TotalTokens       int       `gorm:"column:total_tokens"`
		PromptCost        float64   `gorm:"column:prompt_cost"`
		CompletionCost    float64   `gorm:"column:completion_cost"`
		CacheReadCost     float64   `gorm:"column:cache_read_cost"`
		CacheWriteCost    float64   `gorm:"column:cache_write_cost"`
		TotalCost         float64   `gorm:"column:total_cost"`
		LatencyMs         float64   `gorm:"column:latency_ms"`
		RequestSizeBytes  int       `gorm:"column:request_size_bytes"`
		ResponseSizeBytes int       `gorm:"column:response_size_bytes"`
		TotalRequests     int       `gorm:"column:total_requests"`
	}

	// Restrict to the (provider, model) pairs we actually need to chart.
	// Without this filter Postgres would aggregate every model in the org's
	// window and we'd then discard most of the result in Go.
	err := s.orgUsageBaseQuery(ctx, q).
		Select(`
			usage_metrics.date,
			usage_metrics.provider,
			usage_metrics.model,
			SUM(usage_metrics.prompt_tokens) as prompt_tokens,
			SUM(usage_metrics.completion_tokens) as completion_tokens,
			SUM(usage_metrics.cache_read_tokens) as cache_read_tokens,
			SUM(usage_metrics.cache_write_tokens) as cache_write_tokens,
			SUM(usage_metrics.total_tokens) as total_tokens,
			SUM(usage_metrics.prompt_cost) as prompt_cost,
			SUM(usage_metrics.completion_cost) as completion_cost,
			SUM(usage_metrics.cache_read_cost) as cache_read_cost,
			SUM(usage_metrics.cache_write_cost) as cache_write_cost,
			SUM(usage_metrics.total_cost) as total_cost,
			AVG(usage_metrics.duration_ms) as latency_ms,
			SUM(usage_metrics.request_size_bytes) as request_size_bytes,
			SUM(usage_metrics.response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT usage_metrics.id) as total_requests
		`).
		Where("usage_metrics.model <> ''").
		Where(strings.Join(conditions, " OR "), args...).
		Group("usage_metrics.date, usage_metrics.provider, usage_metrics.model").
		Order("usage_metrics.date ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	metricsByModel := make(map[string][]*types.AggregatedUsageMetric, len(topModels))
	for _, row := range rows {
		id := row.Provider + ":" + row.Model
		if _, ok := topModelKeys[id]; !ok {
			continue
		}
		metricsByModel[id] = append(metricsByModel[id], &types.AggregatedUsageMetric{
			Date:              row.Date,
			PromptTokens:      row.PromptTokens,
			CompletionTokens:  row.CompletionTokens,
			CacheReadTokens:   row.CacheReadTokens,
			CacheWriteTokens:  row.CacheWriteTokens,
			TotalTokens:       row.TotalTokens,
			PromptCost:        row.PromptCost,
			CompletionCost:    row.CompletionCost,
			CacheReadCost:     row.CacheReadCost,
			CacheWriteCost:    row.CacheWriteCost,
			TotalCost:         row.TotalCost,
			LatencyMs:         row.LatencyMs,
			RequestSizeBytes:  row.RequestSizeBytes,
			ResponseSizeBytes: row.ResponseSizeBytes,
			TotalRequests:     row.TotalRequests,
		})
	}

	series := make([]types.UsageModelTimeSeries, 0, len(topModels))
	for _, model := range topModels {
		id := model.Provider + ":" + model.Model
		complete := fillInMissingDates(metricsByModel[id], q.From, q.To)
		converted := make([]types.AggregatedUsageMetric, len(complete))
		for i, metric := range complete {
			converted[i] = *metric
		}
		series = append(series, types.UsageModelTimeSeries{
			ID:       id,
			Name:     model.Name,
			Provider: model.Provider,
			Model:    model.Model,
			Metrics:  converted,
		})
	}

	return series, nil
}

func (s *PostgresStore) GetSandboxUsageMetrics(ctx context.Context, q *GetAggregatedUsageMetricsQuery) ([]*types.AggregatedUsageMetric, error) {
	metrics := []*types.AggregatedUsageMetric{}
	if q == nil {
		return metrics, nil
	}
	if q.OrganizationID == "" {
		return fillInMissingDates(metrics, q.From, q.To), nil
	}

	aggregationLevel := q.AggregationLevel
	if aggregationLevel == "" {
		aggregationLevel = AggregationLevelDaily
	}

	var dateExpr string
	var groupBy string
	switch aggregationLevel {
	case AggregationLevel5Min:
		dateExpr = "date_trunc('hour', transactions.created_at) + INTERVAL '5 min' * FLOOR(EXTRACT(MINUTE FROM transactions.created_at) / 5) as date"
		groupBy = "date_trunc('hour', transactions.created_at) + INTERVAL '5 min' * FLOOR(EXTRACT(MINUTE FROM transactions.created_at) / 5)"
	case AggregationLevelHourly:
		dateExpr = "date_trunc('hour', transactions.created_at) as date"
		groupBy = "date_trunc('hour', transactions.created_at)"
	default:
		dateExpr = "date_trunc('day', transactions.created_at) as date"
		groupBy = "date_trunc('day', transactions.created_at)"
	}

	err := s.gdb.WithContext(ctx).
		Model(&types.Transaction{}).
		Joins("JOIN wallets ON wallets.id = transactions.wallet_id").
		Select(`
			`+dateExpr+`,
			COALESCE(SUM(-transactions.amount), 0) as sandbox_cost,
			COALESCE(SUM(-transactions.amount), 0) as total_cost,
			COUNT(DISTINCT transactions.id) as total_requests
		`).
		Where("wallets.org_id = ?", q.OrganizationID).
		Where("transactions.type = ?", types.TransactionTypeUsage).
		Where("transactions.sandbox_id <> ''").
		Where("transactions.created_at >= ? AND transactions.created_at <= ?", q.From, q.To).
		Group(groupBy).
		Order("date ASC").
		Find(&metrics).Error
	if err != nil {
		return nil, err
	}

	switch aggregationLevel {
	case AggregationLevel5Min:
		return fillInMissing5Minutes(metrics, q.From, q.To), nil
	case AggregationLevelHourly:
		return fillInMissingHours(metrics, q.From, q.To), nil
	default:
		return fillInMissingDates(metrics, q.From, q.To), nil
	}
}

func (s *PostgresStore) GetAppUsersAggregatedUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.UsersAggregatedUsageMetric, error) {
	metrics := []*types.UsersAggregatedUsageMetric{}

	// First get the aggregated metrics per user
	var userMetrics []struct {
		UserID            string    `gorm:"column:user_id"`
		Date              time.Time `gorm:"column:date"`
		PromptTokens      int       `gorm:"column:prompt_tokens"`
		CompletionTokens  int       `gorm:"column:completion_tokens"`
		TotalTokens       int       `gorm:"column:total_tokens"`
		CacheReadTokens   int       `gorm:"column:cache_read_tokens"`
		CacheWriteTokens  int       `gorm:"column:cache_write_tokens"`
		PromptCost        float64   `gorm:"column:prompt_cost"`
		CompletionCost    float64   `gorm:"column:completion_cost"`
		CacheReadCost     float64   `gorm:"column:cache_read_cost"`
		CacheWriteCost    float64   `gorm:"column:cache_write_cost"`
		TotalCost         float64   `gorm:"column:total_cost"`
		DurationMs        float64   `gorm:"column:duration_ms"`
		RequestSizeBytes  int       `gorm:"column:request_size_bytes"`
		ResponseSizeBytes int       `gorm:"column:response_size_bytes"`
		TotalRequests     int       `gorm:"column:total_requests"` // Grouped by interaction_id
	}

	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			user_id,
			date,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(total_tokens) as total_tokens,
			SUM(cache_read_tokens) as cache_read_tokens,
			SUM(cache_write_tokens) as cache_write_tokens,
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(cache_read_cost) as cache_read_cost,
			SUM(cache_write_cost) as cache_write_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT id) as total_requests
		`).
		Where("app_id = ? AND date >= ? AND date <= ?", appID, from, to).
		Group("user_id, date").
		Order("user_id ASC, date ASC").
		Find(&userMetrics).Error

	if err != nil {
		return nil, err
	}

	// Create a map to group metrics by user_id
	userMetricsMap := make(map[string][]*types.AggregatedUsageMetric)
	userIDs := make(map[string]bool)

	for _, m := range userMetrics {
		userIDs[m.UserID] = true
		userMetricsMap[m.UserID] = append(userMetricsMap[m.UserID], &types.AggregatedUsageMetric{
			Date:              m.Date,
			PromptTokens:      m.PromptTokens,
			CompletionTokens:  m.CompletionTokens,
			TotalTokens:       m.TotalTokens,
			CacheReadTokens:   m.CacheReadTokens,
			CacheWriteTokens:  m.CacheWriteTokens,
			TotalCost:         m.TotalCost,
			PromptCost:        m.PromptCost,
			CompletionCost:    m.CompletionCost,
			CacheReadCost:     m.CacheReadCost,
			CacheWriteCost:    m.CacheWriteCost,
			LatencyMs:         m.DurationMs,
			RequestSizeBytes:  m.RequestSizeBytes,
			ResponseSizeBytes: m.ResponseSizeBytes,
			TotalRequests:     m.TotalRequests,
		})
	}

	// Get user information for all users that have metrics
	var users []types.User
	if len(userIDs) > 0 {
		userIDList := make([]string, 0, len(userIDs))
		for userID := range userIDs {
			userIDList = append(userIDList, userID)
		}

		err = s.gdb.WithContext(ctx).
			Model(&types.User{}).
			Where("id IN ?", userIDList).
			Find(&users).Error

		if err != nil {
			return nil, err
		}
	}

	// Create final response combining user info with their metrics
	for _, user := range users {
		userMetrics := userMetricsMap[user.ID]
		if userMetrics == nil {
			userMetrics = []*types.AggregatedUsageMetric{}
		}

		// Fill in missing dates
		completeMetrics := fillInMissingDates(userMetrics, from, to)

		// Convert []*AggregatedUsageMetric to []AggregatedUsageMetric
		convertedMetrics := make([]types.AggregatedUsageMetric, len(completeMetrics))
		for i, m := range completeMetrics {
			convertedMetrics[i] = *m
		}

		metrics = append(metrics, &types.UsersAggregatedUsageMetric{
			User:    user,
			Metrics: convertedMetrics,
		})
	}

	return metrics, nil
}

type metricDate struct {
	Year  int
	Month int
	Day   int
}

func fillInMissingDates(metrics []*types.AggregatedUsageMetric, from time.Time, to time.Time) []*types.AggregatedUsageMetric {
	var completeMetrics []*types.AggregatedUsageMetric

	metricsMap := make(map[metricDate]*types.AggregatedUsageMetric)
	for _, metric := range metrics {
		date := metricDate{
			Year:  metric.Date.Year(),
			Month: int(metric.Date.Month()),
			Day:   metric.Date.Day(),
		}
		metricsMap[date] = metric
	}

	currentDate := from
	for !currentDate.After(to) {
		date := currentDate.Truncate(24 * time.Hour)
		mDate := metricDate{
			Year:  date.Year(),
			Month: int(date.Month()),
			Day:   date.Day(),
		}

		if metric, exists := metricsMap[mDate]; exists {
			completeMetrics = append(completeMetrics, metric)
		} else {
			completeMetrics = append(completeMetrics, &types.AggregatedUsageMetric{
				Date: date,
			})
		}
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	return completeMetrics
}

type metricHour struct {
	Year  int
	Month int
	Day   int
	Hour  int
}

func fillInMissingHours(metrics []*types.AggregatedUsageMetric, from time.Time, to time.Time) []*types.AggregatedUsageMetric {
	var completeMetrics []*types.AggregatedUsageMetric

	metricsMap := make(map[metricHour]*types.AggregatedUsageMetric)
	for _, metric := range metrics {
		hour := metricHour{
			Year:  metric.Date.Year(),
			Month: int(metric.Date.Month()),
			Day:   metric.Date.Day(),
			Hour:  metric.Date.Hour(),
		}
		metricsMap[hour] = metric
	}

	currentHour := from.Truncate(time.Hour)
	endHour := to.Truncate(time.Hour)
	for !currentHour.After(endHour) {
		mHour := metricHour{
			Year:  currentHour.Year(),
			Month: int(currentHour.Month()),
			Day:   currentHour.Day(),
			Hour:  currentHour.Hour(),
		}

		if metric, exists := metricsMap[mHour]; exists {
			completeMetrics = append(completeMetrics, metric)
		} else {
			completeMetrics = append(completeMetrics, &types.AggregatedUsageMetric{
				Date: currentHour,
			})
		}
		currentHour = currentHour.Add(time.Hour)
	}

	return completeMetrics
}

type metric5Min struct {
	Year   int
	Month  int
	Day    int
	Hour   int
	Minute int
}

func fillInMissing5Minutes(metrics []*types.AggregatedUsageMetric, from time.Time, to time.Time) []*types.AggregatedUsageMetric {
	var completeMetrics []*types.AggregatedUsageMetric

	metricsMap := make(map[metric5Min]*types.AggregatedUsageMetric)
	for _, metric := range metrics {
		m5 := metric5Min{
			Year:   metric.Date.Year(),
			Month:  int(metric.Date.Month()),
			Day:    metric.Date.Day(),
			Hour:   metric.Date.Hour(),
			Minute: (metric.Date.Minute() / 5) * 5,
		}
		metricsMap[m5] = metric
	}

	current := from.Truncate(5 * time.Minute)
	end := to.Truncate(5 * time.Minute)
	for !current.After(end) {
		m5 := metric5Min{
			Year:   current.Year(),
			Month:  int(current.Month()),
			Day:    current.Day(),
			Hour:   current.Hour(),
			Minute: current.Minute(),
		}

		if metric, exists := metricsMap[m5]; exists {
			completeMetrics = append(completeMetrics, metric)
		} else {
			completeMetrics = append(completeMetrics, &types.AggregatedUsageMetric{
				Date: current,
			})
		}
		current = current.Add(5 * time.Minute)
	}

	return completeMetrics
}

func (s *PostgresStore) DeleteUsageMetrics(ctx context.Context, appID string) error {
	if appID == "" {
		return errors.New("app_id is required")
	}

	return s.gdb.WithContext(ctx).Where("app_id = ?", appID).Delete(&types.UsageMetric{}).Error
}

// GetUserModelUsage returns per-(provider, model) aggregates for everything
// the given user has consumed. The result is ordered by total requests desc.
func (s *PostgresStore) GetUserModelUsage(ctx context.Context, userID string) ([]*types.UserModelUsage, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}

	var rows []*types.UserModelUsage
	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			provider,
			model,
			COUNT(DISTINCT id) as total_requests,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(prompt_tokens), 0) as prompt_tokens,
			COALESCE(SUM(completion_tokens), 0) as completion_tokens,
			COALESCE(SUM(cache_read_tokens), 0) as cache_read_tokens,
			COALESCE(SUM(cache_write_tokens), 0) as cache_write_tokens,
			COALESCE(SUM(total_cost), 0) as total_cost,
			MIN(created) as first_used,
			MAX(created) as last_used
		`).
		Where("user_id = ?", userID).
		Group("provider, model").
		Order("total_requests DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// GetUserLastUsage returns the most recent UsageMetric.created for a user, or
// zero time if the user has never triggered an inference request.
func (s *PostgresStore) GetUserLastUsage(ctx context.Context, userID string) (time.Time, error) {
	if userID == "" {
		return time.Time{}, errors.New("user_id is required")
	}
	var result struct {
		LastUsed *time.Time `gorm:"column:last_used"`
	}
	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select("MAX(created) as last_used").
		Where("user_id = ?", userID).
		Scan(&result).Error
	if err != nil {
		return time.Time{}, err
	}
	if result.LastUsed == nil {
		return time.Time{}, nil
	}
	return *result.LastUsed, nil
}

// GetUserMonthlyTokenUsage returns the total tokens used by a user in the current month
func (s *PostgresStore) GetUserMonthlyTokenUsage(ctx context.Context, userID string, providers []string) (int, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var result struct {
		TotalTokens int `gorm:"column:total_tokens"`
	}

	query := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select("COALESCE(SUM(total_tokens), 0) as total_tokens").
		Where("user_id = ? AND date >= ?", userID, startOfMonth)

	if len(providers) > 0 {
		query = query.Where("provider IN ?", providers)
	}

	err := query.Scan(&result).Error

	return result.TotalTokens, err
}
