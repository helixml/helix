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

	suite.db = GetTestDB()

}

func (suite *UsageMetricsTestSuite) TearDownTestSuite() {
	_ = suite.db.Close()
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
				DurationMs:        50 + j,
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
	metrics, err := suite.db.GetAppUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)
	suite.Len(metrics, 5, "Should have 5 metrics for the specific day")

	// Test getting metrics for all 3 days
	startTime = now.AddDate(0, 0, -3).Truncate(24 * time.Hour)
	endTime = now.Add(24 * time.Hour)
	metrics, err = suite.db.GetAppUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)
	suite.Len(metrics, 15, "Should have 15 metrics total")
}

func (suite *UsageMetricsTestSuite) TestDailyUsageMetricsWithGaps() {
	appID := "test-" + system.GenerateAppID()

	metric1 := &types.UsageMetric{
		AppID:             appID,
		Created:           time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC), // March 5th
		PromptTokens:      100,
		CompletionTokens:  200,
		TotalTokens:       300,
		DurationMs:        50,
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
	dailyMetrics, err := suite.db.GetAppDailyUsageMetrics(suite.ctx, appID, startTime, endTime)
	suite.NoError(err)

	// We should have 5 days of data, from march 4th to march 8th
	suite.Require().Len(dailyMetrics, 5, "Should have 5 daily aggregations (days with data)")

	// Check dates for ascending order
	suite.Equal(4, dailyMetrics[0].Date.Day())
	suite.Equal(5, dailyMetrics[1].Date.Day())
	suite.Equal(6, dailyMetrics[2].Date.Day())
	suite.Equal(7, dailyMetrics[3].Date.Day())
	suite.Equal(8, dailyMetrics[4].Date.Day())

	// Check prompt tokens for the days
	suite.Equal(0, dailyMetrics[0].PromptTokens)   // March 4th
	suite.Equal(100, dailyMetrics[1].PromptTokens) // March 5th
	suite.Equal(0, dailyMetrics[2].PromptTokens)   // March 6th
	suite.Equal(100, dailyMetrics[3].PromptTokens) // March 7th
	suite.Equal(0, dailyMetrics[4].PromptTokens)   // March 8th
}

func (suite *UsageMetricsTestSuite) TestGetUserMonthlyTokenUsage_User() {
	appID := "test-" + system.GenerateAppID()
	userID := system.GenerateID()

	metric1 := &types.UsageMetric{
		AppID:             appID,
		UserID:            userID,
		Created:           time.Now(),
		Date:              time.Now().Truncate(24 * time.Hour),
		PromptTokens:      100,
		CompletionTokens:  200,
		TotalTokens:       300,
		DurationMs:        50,
		RequestSizeBytes:  1000,
		ResponseSizeBytes: 2000,
		Provider:          string(types.ProviderOpenAI),
	}
	_, err := suite.db.CreateUsageMetric(suite.ctx, metric1)
	suite.NoError(err)

	metric2 := &types.UsageMetric{
		AppID:            appID,
		Created:          time.Now(),
		Date:             time.Now().Truncate(24 * time.Hour),
		UserID:           userID,
		PromptTokens:     100,
		CompletionTokens: 200,
		TotalTokens:      300,
		Provider:         string(types.ProviderAnthropic),
	}
	_, err = suite.db.CreateUsageMetric(suite.ctx, metric2)
	suite.NoError(err)

	// Test getting monthly usage for the user, should be combined
	monthlyTokens, err := suite.db.GetUserMonthlyTokenUsage(suite.ctx, userID, types.GlobalProviders)
	suite.NoError(err)
	suite.Equal(600, monthlyTokens)

	// Test getting monthly usage for the user with a filter by provider, should be combined too
	// as we are fetching all providers
	monthlyTokens, err = suite.db.GetUserMonthlyTokenUsage(suite.ctx, userID, []string{})
	suite.NoError(err)
	suite.Equal(600, monthlyTokens)

	// Test getting monthly usage for the user with a filter by provider
	monthlyTokens, err = suite.db.GetUserMonthlyTokenUsage(suite.ctx, userID, []string{string(types.ProviderOpenAI)})
	suite.NoError(err)
	suite.Equal(300, monthlyTokens)
}
