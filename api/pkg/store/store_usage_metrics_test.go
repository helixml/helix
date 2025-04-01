package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestUsageMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(UsageMetricsTestSuite))
}

type UsageMetricsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *UsageMetricsTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)
	suite.db = store

	suite.T().Cleanup(func() {
		_ = suite.db.Close()
	})
}

func (suite *UsageMetricsTestSuite) TestCreateAndGetUsageMetrics() {
	appID := "test-" + system.GenerateAppID()
	now := time.Now()

	// Create metrics for 3 days
	for i := 0; i < 3; i++ {
		date := now.AddDate(0, 0, -i)
		// Create multiple metrics per day
		for j := 0; j < 5; j++ {
			metric := &types.UsageMetric{
				AppID:             appID,
				Created:           date.Add(time.Duration(j) * time.Hour),
				PromptTokens:      100 + j,
				CompletionTokens:  200 + j,
				TotalTokens:       300 + j,
				LatencyMs:         50 + j,
				RequestSizeBytes:  1000 + j,
				ResponseSizeBytes: 2000 + j,
			}
			_, err := suite.db.CreateUsageMetric(suite.ctx, metric)
			suite.NoError(err)
		}
	}

	// Test getting metrics for a specific day
	startTime := now.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	endTime := startTime.Add(24 * time.Hour)
	metrics, err := suite.db.GetUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)
	suite.Len(metrics, 5, "Should have 5 metrics for the specific day")

	// Test getting metrics for all 3 days
	startTime = now.AddDate(0, 0, -3).Truncate(24 * time.Hour)
	endTime = now.Add(24 * time.Hour)
	metrics, err = suite.db.GetUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)
	suite.Len(metrics, 15, "Should have 15 metrics total")
}

func (suite *UsageMetricsTestSuite) TestGetDailyUsageMetrics() {
	appID := "test-" + system.GenerateAppID()
	now := time.Now()

	// Create metrics for 3 days
	for i := 0; i < 3; i++ {
		date := now.AddDate(0, 0, -i)
		// Create multiple metrics per day with known values
		for j := 0; j < 5; j++ {
			metric := &types.UsageMetric{
				AppID:             appID,
				Created:           date.Add(time.Duration(j) * time.Hour),
				PromptTokens:      100,
				CompletionTokens:  200,
				TotalTokens:       300,
				LatencyMs:         50,
				RequestSizeBytes:  1000,
				ResponseSizeBytes: 2000,
			}
			_, err := suite.db.CreateUsageMetric(suite.ctx, metric)
			suite.NoError(err)
		}
	}

	// Test daily aggregation
	startTime := now.AddDate(0, 0, -3).Truncate(24 * time.Hour)
	endTime := now.Add(24 * time.Hour)
	dailyMetrics, err := suite.db.GetDailyUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)
	suite.Len(dailyMetrics, 3, "Should have 3 daily aggregations")

	// Verify aggregation for each day
	for _, metric := range dailyMetrics {
		suite.Equal(500, metric.PromptTokens, "Daily prompt tokens should be 100 * 5")
		suite.Equal(1000, metric.CompletionTokens, "Daily completion tokens should be 200 * 5")
		suite.Equal(1500, metric.TotalTokens, "Daily total tokens should be 300 * 5")
		suite.Equal(float64(50), metric.LatencyMs, "Daily latency should be average of 50")
		suite.Equal(5000, metric.RequestSizeBytes, "Daily request size should be 1000 * 5")
		suite.Equal(10000, metric.ResponseSizeBytes, "Daily response size should be 2000 * 5")
	}
}

func (suite *UsageMetricsTestSuite) TestDailyUsageMetricsWithGaps() {
	appID := "test-" + system.GenerateAppID()

	metric1 := &types.UsageMetric{
		AppID:             appID,
		Created:           time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC), // March 5th
		PromptTokens:      100,
		CompletionTokens:  200,
		TotalTokens:       300,
		LatencyMs:         50,
		RequestSizeBytes:  1000,
		ResponseSizeBytes: 2000,
	}
	_, err := suite.db.CreateUsageMetric(suite.ctx, metric1)
	suite.NoError(err)

	metric2 := &types.UsageMetric{
		AppID:            appID,
		Created:          time.Date(2025, 3, 7, 0, 0, 0, 0, time.UTC), // March 7th
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
	}
	_, err = suite.db.CreateUsageMetric(suite.ctx, metric2)
	suite.NoError(err)

	// Test daily aggregation including the gap day
	startTime := time.Date(2025, 3, 4, 0, 0, 0, 0, time.UTC) // March 4th
	endTime := time.Date(2025, 3, 8, 0, 0, 0, 0, time.UTC)   // March 8th
	dailyMetrics, err := suite.db.GetDailyUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)

	// We should have 5 days of data, from march 4th to march 8th
	suite.Require().Len(dailyMetrics, 5, "Should have 5 daily aggregations (days with data)")

	// Check prompt tokens for the days
	suite.Equal(0, dailyMetrics[0].PromptTokens)   // March 8th
	suite.Equal(100, dailyMetrics[1].PromptTokens) // March 7th
	suite.Equal(0, dailyMetrics[2].PromptTokens)   // March 6th
	suite.Equal(100, dailyMetrics[3].PromptTokens) // March 5th
	suite.Equal(0, dailyMetrics[4].PromptTokens)   // March 4th
}
