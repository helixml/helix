package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

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
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
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
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
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
		PromptCost        float64   `gorm:"column:prompt_cost"`
		CompletionCost    float64   `gorm:"column:completion_cost"`
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
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT interaction_id) as total_requests
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

	query := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			date,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(total_tokens) as total_tokens,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT interaction_id) as total_requests
		`)

	if q.ProjectID != "" {
		query = query.Where("project_id = ?", q.ProjectID)
	}
	if q.SpecTaskID != "" {
		query = query.Where("spec_task_id = ?", q.SpecTaskID)
	}
	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}
	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	query = query.Where("date >= ? AND date <= ?", q.From, q.To)

	err := query.Group("date").
		Order("date ASC").
		Find(&metrics).Error
	if err != nil {
		return nil, err
	}

	// Fill in missing dates
	completeMetrics := fillInMissingDates(metrics, q.From, q.To)

	return completeMetrics, nil
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
		PromptCost        float64   `gorm:"column:prompt_cost"`
		CompletionCost    float64   `gorm:"column:completion_cost"`
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
			SUM(prompt_cost) as prompt_cost,
			SUM(completion_cost) as completion_cost,
			SUM(total_cost) as total_cost,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes,
			COUNT(DISTINCT interaction_id) as total_requests
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
			TotalCost:         m.TotalCost,
			PromptCost:        m.PromptCost,
			CompletionCost:    m.CompletionCost,
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

	existingDates := make(map[metricDate]bool)
	metricsMap := make(map[metricDate]*types.AggregatedUsageMetric)
	for _, metric := range metrics {
		date := metricDate{
			Year:  metric.Date.Year(),
			Month: int(metric.Date.Month()),
			Day:   metric.Date.Day(),
		}
		existingDates[date] = true
		metricsMap[date] = metric
	}

	// Start from 'from' date and move forward to 'to' date
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

func (s *PostgresStore) DeleteUsageMetrics(ctx context.Context, appID string) error {
	if appID == "" {
		return errors.New("app_id is required")
	}

	return s.gdb.WithContext(ctx).Where("app_id = ?", appID).Delete(&types.UsageMetric{}).Error
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
