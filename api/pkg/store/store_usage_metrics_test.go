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

func (suite *UsageMetricsTestSuite) TestGetAggregatedUsageMetrics_IncludesCacheFields() {
	// Regression test: the aggregate SELECT used to omit cache_read_*,
	// cache_write_*, prompt_cost, completion_cost; rows came back with
	// zeros in those columns even though the underlying data had them.
	// Anthropic traffic shows the bug most clearly because Anthropic
	// reports cache_read/cache_write as separate buckets.
	orgID := "org_" + system.GenerateID()
	userID := "user_" + system.GenerateID()
	day := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)

	metric := &types.UsageMetric{
		OrganizationID:   orgID,
		UserID:           userID,
		Created:          day.Add(2 * time.Hour),
		Date:             day,
		Provider:         string(types.ProviderAnthropic),
		PromptTokens:     1000,
		CompletionTokens: 500,
		CacheReadTokens:  4000,
		CacheWriteTokens: 800,
		TotalTokens:      6300,
		PromptCost:       0.01,
		CompletionCost:   0.03,
		CacheReadCost:    0.002,
		CacheWriteCost:   0.004,
		TotalCost:        0.046,
	}
	_, err := suite.db.CreateUsageMetric(suite.ctx, metric)
	suite.Require().NoError(err)

	from := day.Add(-24 * time.Hour)
	to := day.Add(24 * time.Hour)
	metrics, err := suite.db.GetAggregatedUsageMetrics(suite.ctx, &GetAggregatedUsageMetricsQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
	})
	suite.Require().NoError(err)
	suite.Require().NotEmpty(metrics)

	// Locate the row for our seeded day. fillInMissingDates pads other
	// days with zeros, which is fine - we just need our day to land.
	var got *types.AggregatedUsageMetric
	for _, m := range metrics {
		if m.Date.Year() == day.Year() && m.Date.YearDay() == day.YearDay() {
			got = m
			break
		}
	}
	suite.Require().NotNil(got, "expected an aggregate row for the seeded day")

	suite.Equal(1000, got.PromptTokens)
	suite.Equal(500, got.CompletionTokens)
	suite.Equal(4000, got.CacheReadTokens, "cache_read_tokens must be summed")
	suite.Equal(800, got.CacheWriteTokens, "cache_write_tokens must be summed")
	suite.Equal(6300, got.TotalTokens)
	suite.InDelta(0.01, got.PromptCost, 1e-9)
	suite.InDelta(0.03, got.CompletionCost, 1e-9)
	suite.InDelta(0.002, got.CacheReadCost, 1e-9, "cache_read_cost must be summed")
	suite.InDelta(0.004, got.CacheWriteCost, 1e-9, "cache_write_cost must be summed")
	suite.InDelta(0.046, got.TotalCost, 1e-9)
}

func (suite *UsageMetricsTestSuite) TestGetOrgUsageSummary_PaginatesAndExportsAllBreakdowns() {
	orgID := "org_" + system.GenerateID()
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC)

	users := []types.User{
		{ID: "user_" + system.GenerateID(), Email: "alice.usage@example.com", Username: "alice-usage", FullName: "Alice Usage"},
		{ID: "user_" + system.GenerateID(), Email: "bob.usage@example.com", Username: "bob-usage", FullName: "Bob Usage"},
		{ID: "user_" + system.GenerateID(), Email: "carol.usage@example.com", Username: "carol-usage", FullName: "Carol Usage"},
	}
	projects := []types.Project{
		{ID: "prj_" + system.GenerateID(), Name: "High Tokens", OrganizationID: orgID, UserID: users[0].ID},
		{ID: "prj_" + system.GenerateID(), Name: "Middle Tokens", OrganizationID: orgID, UserID: users[1].ID},
		{ID: "prj_" + system.GenerateID(), Name: "Low Tokens", OrganizationID: orgID, UserID: users[2].ID},
	}
	apps := []types.App{
		{ID: "app_" + system.GenerateID(), OrganizationID: orgID, Owner: users[0].ID, Config: types.AppConfig{Helix: types.AppHelixConfig{Name: "Agent High"}}},
		{ID: "app_" + system.GenerateID(), OrganizationID: orgID, Owner: users[1].ID, Config: types.AppConfig{Helix: types.AppHelixConfig{Name: "Agent Middle"}}},
		{ID: "app_" + system.GenerateID(), OrganizationID: orgID, Owner: users[2].ID, Config: types.AppConfig{Helix: types.AppHelixConfig{Name: "Agent Low"}}},
	}
	tasks := []types.SpecTask{
		{ID: "spt_" + system.GenerateID(), Name: "High task", ProjectID: projects[0].ID, UserID: users[0].ID, OrganizationID: orgID},
		{ID: "spt_" + system.GenerateID(), Name: "Middle task", ProjectID: projects[1].ID, UserID: users[1].ID, OrganizationID: orgID},
		{ID: "spt_" + system.GenerateID(), Name: "Low task", ProjectID: projects[2].ID, UserID: users[2].ID, OrganizationID: orgID},
	}
	sessions := []types.Session{
		{ID: "ses_" + system.GenerateID(), Name: "High session", Created: from.Add(2 * time.Hour), Updated: from.Add(3 * time.Hour), OrganizationID: orgID, ProjectID: projects[0].ID, ParentApp: apps[0].ID, Owner: users[0].ID},
		{ID: "ses_" + system.GenerateID(), Name: "Middle session", Created: from.AddDate(0, 0, 1), Updated: from.AddDate(0, 0, 1).Add(time.Hour), OrganizationID: orgID, ProjectID: projects[1].ID, ParentApp: apps[1].ID, Owner: users[1].ID},
		{ID: "ses_" + system.GenerateID(), Name: "Low session", Created: from.AddDate(0, 0, 2), Updated: from.AddDate(0, 0, 2).Add(time.Hour), OrganizationID: orgID, ProjectID: projects[2].ID, ParentApp: apps[2].ID, Owner: users[2].ID},
	}
	suite.insertOrgUsageDimensions(users, projects, apps, tasks, sessions)

	rows := []struct {
		day          time.Time
		user         types.User
		project      types.Project
		app          types.App
		task         types.SpecTask
		session      types.Session
		provider     string
		model        string
		promptTokens int
		totalTokens  int
	}{
		{from.AddDate(0, 0, 1), users[0], projects[0], apps[0], tasks[0], sessions[0], string(types.ProviderOpenAI), "gpt-4o", 500, 900},
		{from.AddDate(0, 0, 2), users[1], projects[1], apps[1], tasks[1], sessions[1], string(types.ProviderAnthropic), "claude-sonnet-4", 350, 600},
		{from.AddDate(0, 0, 3), users[2], projects[2], apps[2], tasks[2], sessions[2], string(types.ProviderOpenAI), "gpt-4o-mini", 150, 300},
	}
	for index, row := range rows {
		interactionID := "int_" + system.GenerateID()
		suite.insertInteraction(interactionID, row.day, row.user.ID, row.app.ID, row.session.ID)
		suite.insertUsageMetric(&types.UsageMetric{
			OrganizationID:   orgID,
			UserID:           row.user.ID,
			ProjectID:        row.project.ID,
			AppID:            row.app.ID,
			SpecTaskID:       row.task.ID,
			InteractionID:    interactionID,
			Created:          row.day.Add(time.Duration(index) * time.Hour),
			Provider:         row.provider,
			Model:            row.model,
			PromptTokens:     row.promptTokens,
			CompletionTokens: row.totalTokens - row.promptTokens,
			TotalTokens:      row.totalTokens,
			TotalCost:        float64(row.totalTokens) / 1_000_000,
			DurationMs:       100 + index,
		})
	}

	resp, err := suite.db.GetOrgUsageSummary(suite.ctx, &GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		ProjectLimit:   1,
		ProjectOffset:  1,
		TaskLimit:      1,
		SessionLimit:   2,
		UserLimit:      2,
	})
	suite.Require().NoError(err)

	suite.Equal(1800, sumAggregatedTokens(resp.Metrics))
	suite.Equal(3, resp.ActiveUsers)
	suite.Equal(3, resp.ActiveProjects)
	suite.Equal(3, resp.ActiveApps)
	suite.Equal(3, resp.ActiveSessions)

	suite.EqualValues(3, resp.ProjectsTotal)
	suite.Require().Len(resp.Projects, 1)
	suite.Equal(projects[1].ID, resp.Projects[0].ID, "project pagination should be ordered by total tokens")
	suite.Len(resp.ExportProjects, 3, "project exports should include all filtered rows, not just the current page")

	suite.EqualValues(3, resp.TasksTotal)
	suite.Require().Len(resp.Tasks, 1)
	suite.Len(resp.ExportTasks, 3)

	suite.EqualValues(3, resp.SessionsTotal)
	suite.Require().Len(resp.Sessions, 2)
	suite.Len(resp.ExportSessions, 3)

	suite.EqualValues(3, resp.UsersTotal)
	suite.Require().Len(resp.Users, 2)
	suite.Len(resp.ExportUsers, 3)
}

func (suite *UsageMetricsTestSuite) TestGetOrgUsageSummary_FiltersAcrossMonths() {
	orgID := "org_" + system.GenerateID()
	otherOrgID := "org_" + system.GenerateID()
	jan := time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC)

	alice := types.User{ID: "user_" + system.GenerateID(), Email: "alice.months@example.com", Username: "alice-months", FullName: "Alice Months"}
	bob := types.User{ID: "user_" + system.GenerateID(), Email: "bob.months@example.com", Username: "bob-months", FullName: "Bob Months"}
	projectA := types.Project{ID: "prj_" + system.GenerateID(), Name: "Months Project A", OrganizationID: orgID, UserID: alice.ID}
	projectB := types.Project{ID: "prj_" + system.GenerateID(), Name: "Months Project B", OrganizationID: orgID, UserID: bob.ID}
	appA := types.App{ID: "app_" + system.GenerateID(), OrganizationID: orgID, Owner: alice.ID, Config: types.AppConfig{Helix: types.AppHelixConfig{Name: "Months Agent A"}}}
	appB := types.App{ID: "app_" + system.GenerateID(), OrganizationID: orgID, Owner: bob.ID, Config: types.AppConfig{Helix: types.AppHelixConfig{Name: "Months Agent B"}}}
	taskA := types.SpecTask{ID: "spt_" + system.GenerateID(), Name: "Months task A", ProjectID: projectA.ID, UserID: alice.ID, OrganizationID: orgID}
	taskB := types.SpecTask{ID: "spt_" + system.GenerateID(), Name: "Months task B", ProjectID: projectB.ID, UserID: bob.ID, OrganizationID: orgID}
	sessionJan := types.Session{ID: "ses_" + system.GenerateID(), Name: "January session", Created: jan, Updated: jan.Add(time.Hour), OrganizationID: orgID, ProjectID: projectA.ID, ParentApp: appA.ID, Owner: alice.ID}
	sessionFeb := types.Session{ID: "ses_" + system.GenerateID(), Name: "February session", Created: feb, Updated: feb.Add(time.Hour), OrganizationID: orgID, ProjectID: projectA.ID, ParentApp: appA.ID, Owner: alice.ID}
	sessionBob := types.Session{ID: "ses_" + system.GenerateID(), Name: "Bob session", Created: feb, Updated: feb.Add(time.Hour), OrganizationID: orgID, ProjectID: projectB.ID, ParentApp: appB.ID, Owner: bob.ID}
	suite.insertOrgUsageDimensions([]types.User{alice, bob}, []types.Project{projectA, projectB}, []types.App{appA, appB}, []types.SpecTask{taskA, taskB}, []types.Session{sessionJan, sessionFeb, sessionBob})

	janInteractionID := "int_" + system.GenerateID()
	febInteractionID := "int_" + system.GenerateID()
	bobInteractionID := "int_" + system.GenerateID()
	otherInteractionID := "int_" + system.GenerateID()
	suite.insertInteraction(janInteractionID, jan, alice.ID, appA.ID, sessionJan.ID)
	suite.insertInteraction(febInteractionID, feb, alice.ID, appA.ID, sessionFeb.ID)
	suite.insertInteraction(bobInteractionID, feb.Add(time.Hour), bob.ID, appB.ID, sessionBob.ID)
	suite.insertInteraction(otherInteractionID, feb, alice.ID, appA.ID, sessionFeb.ID)

	suite.insertUsageMetric(&types.UsageMetric{
		OrganizationID: orgID, UserID: alice.ID, ProjectID: projectA.ID, AppID: appA.ID, SpecTaskID: taskA.ID, InteractionID: janInteractionID,
		Created: jan, Provider: string(types.ProviderOpenAI), Model: "gpt-4o-mini", PromptTokens: 60, CompletionTokens: 40, TotalTokens: 100, TotalCost: 0.001, DurationMs: 120,
	})
	suite.insertUsageMetric(&types.UsageMetric{
		OrganizationID: orgID, UserID: alice.ID, ProjectID: projectA.ID, AppID: appA.ID, SpecTaskID: taskA.ID, InteractionID: febInteractionID,
		Created: feb, Provider: string(types.ProviderOpenAI), Model: "gpt-4o-mini", PromptTokens: 130, CompletionTokens: 70, TotalTokens: 200, TotalCost: 0.002, DurationMs: 140,
	})
	suite.insertUsageMetric(&types.UsageMetric{
		OrganizationID: orgID, UserID: bob.ID, ProjectID: projectB.ID, AppID: appB.ID, SpecTaskID: taskB.ID, InteractionID: bobInteractionID,
		Created: feb.Add(time.Hour), Provider: string(types.ProviderAnthropic), Model: "claude-sonnet-4", PromptTokens: 300, CompletionTokens: 200, TotalTokens: 500, TotalCost: 0.005, DurationMs: 180,
	})
	suite.insertUsageMetric(&types.UsageMetric{
		OrganizationID: otherOrgID, UserID: alice.ID, ProjectID: projectA.ID, AppID: appA.ID, SpecTaskID: taskA.ID, InteractionID: otherInteractionID,
		Created: feb, Provider: string(types.ProviderOpenAI), Model: "gpt-4o-mini", PromptTokens: 700, CompletionTokens: 299, TotalTokens: 999, TotalCost: 0.009, DurationMs: 100,
	})

	openAIResp, err := suite.db.GetOrgUsageSummary(suite.ctx, &GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		Provider:       string(types.ProviderOpenAI),
		Model:          "gpt-4o-mini",
	})
	suite.Require().NoError(err)
	suite.Equal(300, sumAggregatedTokens(openAIResp.Metrics))
	suite.Equal(1, openAIResp.ActiveUsers)
	suite.Equal(1, openAIResp.ActiveProjects)
	suite.Require().Len(openAIResp.Models, 1)
	suite.Equal("gpt-4o-mini", openAIResp.Models[0].Model)
	suite.Len(openAIResp.ExportSessions, 2)

	projectResp, err := suite.db.GetOrgUsageSummary(suite.ctx, &GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		ProjectID:      projectA.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(300, sumAggregatedTokens(projectResp.Metrics))
	suite.Equal(1, projectResp.ActiveUsers)

	sessionResp, err := suite.db.GetOrgUsageSummary(suite.ctx, &GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		SessionID:      sessionFeb.ID,
	})
	suite.Require().NoError(err)
	suite.Equal(200, sumAggregatedTokens(sessionResp.Metrics))
	suite.Require().Len(sessionResp.Sessions, 1)
	suite.Equal(sessionFeb.ID, sessionResp.Sessions[0].SessionID)

	userSearchResp, err := suite.db.GetOrgUsageSummary(suite.ctx, &GetOrgUsageSummaryQuery{
		OrganizationID: orgID,
		From:           from,
		To:             to,
		UserSearch:     "alice.months",
	})
	suite.Require().NoError(err)
	suite.Equal(800, sumAggregatedTokens(userSearchResp.Metrics), "user table search should not filter the dashboard totals")
	suite.EqualValues(1, userSearchResp.UsersTotal)
	suite.Require().Len(userSearchResp.Users, 1)
	suite.Equal(alice.ID, userSearchResp.Users[0].ID)
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

func (suite *UsageMetricsTestSuite) insertOrgUsageDimensions(users []types.User, projects []types.Project, apps []types.App, tasks []types.SpecTask, sessions []types.Session) {
	for _, user := range users {
		suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(&user).Error)
	}
	for _, project := range projects {
		suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(&project).Error)
	}
	for _, app := range apps {
		suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(&app).Error)
	}
	for _, task := range tasks {
		suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(&task).Error)
	}
	for _, session := range sessions {
		suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(&session).Error)
	}
}

func (suite *UsageMetricsTestSuite) insertInteraction(id string, created time.Time, userID, appID, sessionID string) {
	interaction := &types.Interaction{
		ID:           id,
		GenerationID: 0,
		Created:      created,
		Updated:      created.Add(time.Minute),
		Completed:    created.Add(2 * time.Minute),
		UserID:       userID,
		AppID:        appID,
		SessionID:    sessionID,
	}
	suite.Require().NoError(suite.db.gdb.WithContext(suite.ctx).Create(interaction).Error)
}

func (suite *UsageMetricsTestSuite) insertUsageMetric(metric *types.UsageMetric) {
	if metric.ID == "" {
		metric.ID = "um_" + system.GenerateID()
	}
	if metric.Date.IsZero() {
		metric.Date = time.Date(metric.Created.Year(), metric.Created.Month(), metric.Created.Day(), 0, 0, 0, 0, time.UTC)
	}
	_, err := suite.db.CreateUsageMetric(suite.ctx, metric)
	suite.Require().NoError(err)
}

func sumAggregatedTokens(metrics []*types.AggregatedUsageMetric) int {
	total := 0
	for _, metric := range metrics {
		total += metric.TotalTokens
	}
	return total
}
