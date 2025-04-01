package store

import (
	"context"
	"errors"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (s *PostgresStore) CreateUsageMetric(ctx context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
	if metric.AppID == "" {
		return nil, errors.New("app_id is required")
	}

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

func (s *PostgresStore) GetUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.UsageMetric, error) {
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

func (s *PostgresStore) GetDailyUsageMetrics(ctx context.Context, appID string, from time.Time, to time.Time) ([]*types.AggregatedUsageMetric, error) {
	var metrics []*types.AggregatedUsageMetric
	err := s.gdb.WithContext(ctx).
		Model(&types.UsageMetric{}).
		Select(`
			date,
			app_id,
			SUM(prompt_tokens) as prompt_tokens,
			SUM(completion_tokens) as completion_tokens,
			SUM(total_tokens) as total_tokens,
			AVG(duration_ms) as duration_ms,
			SUM(request_size_bytes) as request_size_bytes,
			SUM(response_size_bytes) as response_size_bytes
		`).
		Where("app_id = ? AND date >= ? AND date <= ?", appID, from, to).
		Group("date, app_id").
		Order("date ASC").
		Find(&metrics).Error

	if err != nil {
		return nil, err
	}

	completeMetrics := fillInMissingDates(appID, metrics, from, to)

	return completeMetrics, nil
}

type metricDate struct {
	Year  int
	Month int
	Day   int
}

func fillInMissingDates(appID string, metrics []*types.AggregatedUsageMetric, from time.Time, to time.Time) []*types.AggregatedUsageMetric {
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
				Date:  date,
				AppID: appID,
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
