package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestUsageAggregateTestSuite(t *testing.T) {
	suite.Run(t, new(UsageAggregateTestSuite))
}

type UsageAggregateTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (s *UsageAggregateTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.db = GetTestDB()
}

func (s *UsageAggregateTestSuite) TearDownTestSuite() {
	_ = s.db.Close()
}

// seedRow creates a usage_metrics row plus an interaction tied to a
// session so the SUBSELECT-based session-distinct counts in the
// aggregate queries can find a value.
func (s *UsageAggregateTestSuite) seedRow(orgID, userID, projectID, appID, sessionID, model string, promptTokens int, cost float64) {
	interactionID := "int_" + system.GenerateID()

	_, err := s.db.CreateUsageMetric(s.ctx, &types.UsageMetric{
		OrganizationID:   orgID,
		UserID:           userID,
		ProjectID:        projectID,
		AppID:            appID,
		InteractionID:    interactionID,
		Created:          time.Now(),
		Date:             time.Now().Truncate(24 * time.Hour),
		Provider:         string(types.ProviderAnthropic),
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: promptTokens / 4,
		CacheReadTokens:  promptTokens / 10,
		CacheWriteTokens: promptTokens / 20,
		TotalTokens:      promptTokens + promptTokens/4 + promptTokens/10 + promptTokens/20,
		PromptCost:       cost,
		CompletionCost:   cost * 5,
		CacheReadCost:    cost * 0.1,
		CacheWriteCost:   cost * 1.25,
		TotalCost:        cost + cost*5 + cost*0.1 + cost*1.25,
	})
	s.Require().NoError(err)

	if sessionID != "" {
		// Seed a minimal interaction row so the SUBSELECT for session_id
		// resolves. Using GORM Create directly to avoid the heavyweight
		// CreateInteraction logic.
		err := s.db.gdb.WithContext(s.ctx).Exec(
			"INSERT INTO interactions (id, session_id, created_at, updated_at) VALUES (?, ?, NOW(), NOW())",
			interactionID, sessionID,
		).Error
		s.Require().NoError(err)
	}
}

func (s *UsageAggregateTestSuite) buildQuery(orgID string) *GroupedUsageQuery {
	return &GroupedUsageQuery{
		From:           time.Now().Add(-24 * time.Hour),
		To:             time.Now().Add(24 * time.Hour),
		OrganizationID: orgID,
		Page:           1,
		PageSize:       50,
	}
}

func (s *UsageAggregateTestSuite) TestGetUsageSummary() {
	orgID := "org_" + system.GenerateID()
	userA := "user_" + system.GenerateID()
	userB := "user_" + system.GenerateID()
	sessionA := "ses_" + system.GenerateID()
	sessionB := "ses_" + system.GenerateID()

	s.seedRow(orgID, userA, "prj_x", "", sessionA, "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgID, userA, "prj_x", "", sessionA, "claude-opus-4-7", 2000, 0.02)
	s.seedRow(orgID, userB, "prj_y", "", sessionB, "claude-sonnet-4-6", 500, 0.005)

	sum, err := s.db.GetUsageSummary(s.ctx, s.buildQuery(orgID))
	s.Require().NoError(err)
	s.Equal(3500, sum.PromptTokens, "prompt tokens")
	s.InDelta(0.035, sum.PromptCost, 1e-9, "prompt cost")
	s.Equal(3, sum.RequestCount, "request count")
	s.Equal(2, sum.ActiveUsers)
	s.Equal(2, sum.ActiveSessions)
	s.Equal(2, sum.ActiveProjects)
	s.NotEmpty(sum.TimeSeries)
}

func (s *UsageAggregateTestSuite) TestGetUsageGroupedByUser() {
	orgID := "org_" + system.GenerateID()
	userA := "user_" + system.GenerateID()
	userB := "user_" + system.GenerateID()

	s.seedRow(orgID, userA, "prj_x", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgID, userA, "prj_x", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 2000, 0.02)
	s.seedRow(orgID, userB, "prj_y", "", "ses_"+system.GenerateID(), "claude-sonnet-4-6", 500, 0.005)

	rows, total, err := s.db.GetUsageGroupedByUser(s.ctx, s.buildQuery(orgID))
	s.Require().NoError(err)
	s.Equal(2, total)
	s.Len(rows, 2)
	// Sorted by total_cost desc by default -> userA first
	s.Equal(userA, rows[0].UserID)
	s.Equal(3000, rows[0].PromptTokens)
	s.InDelta(0.03, rows[0].PromptCost, 1e-9)
	s.Equal(userB, rows[1].UserID)
}

func (s *UsageAggregateTestSuite) TestGetUsageGroupedByModel() {
	orgID := "org_" + system.GenerateID()
	user := "user_" + system.GenerateID()
	s.seedRow(orgID, user, "prj_x", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgID, user, "prj_x", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 2000, 0.02)
	s.seedRow(orgID, user, "prj_x", "", "ses_"+system.GenerateID(), "claude-sonnet-4-6", 500, 0.005)

	rows, total, err := s.db.GetUsageGroupedByModel(s.ctx, s.buildQuery(orgID))
	s.Require().NoError(err)
	s.Equal(2, total)
	s.Len(rows, 2)
	s.Equal("claude-opus-4-7", rows[0].Model)
	s.Equal(3000, rows[0].PromptTokens)
	s.Equal("claude-sonnet-4-6", rows[1].Model)
}

func (s *UsageAggregateTestSuite) TestGetUsageGroupedBySession() {
	orgID := "org_" + system.GenerateID()
	user := "user_" + system.GenerateID()
	sessA := "ses_" + system.GenerateID()
	sessB := "ses_" + system.GenerateID()

	s.seedRow(orgID, user, "prj_x", "", sessA, "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgID, user, "prj_x", "", sessA, "claude-opus-4-7", 2000, 0.02)
	s.seedRow(orgID, user, "prj_x", "", sessB, "claude-opus-4-7", 500, 0.005)

	rows, total, err := s.db.GetUsageGroupedBySession(s.ctx, s.buildQuery(orgID))
	s.Require().NoError(err)
	s.Equal(2, total)
	s.Len(rows, 2)
	// ordered by ended_at desc, both seeded "now"; assert presence
	ids := map[string]int{rows[0].SessionID: rows[0].PromptTokens, rows[1].SessionID: rows[1].PromptTokens}
	s.Equal(3000, ids[sessA])
	s.Equal(500, ids[sessB])
}

func (s *UsageAggregateTestSuite) TestGetUsageGroupedByOrg() {
	orgA := "org_" + system.GenerateID()
	orgB := "org_" + system.GenerateID()
	s.seedRow(orgA, "u1", "prj_x", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgB, "u2", "prj_y", "", "ses_"+system.GenerateID(), "claude-opus-4-7", 2000, 0.02)

	// Wide query (no org filter) so we see both orgs.
	q := &GroupedUsageQuery{
		From:     time.Now().Add(-24 * time.Hour),
		To:       time.Now().Add(24 * time.Hour),
		Page:     1,
		PageSize: 50,
	}
	rows, total, err := s.db.GetUsageGroupedByOrg(s.ctx, q)
	s.Require().NoError(err)
	s.GreaterOrEqual(total, 2)
	// orgA + orgB must be present.
	found := map[string]bool{}
	for _, r := range rows {
		found[r.OrganizationID] = true
	}
	s.True(found[orgA], "orgA must be in by-org rows")
	s.True(found[orgB], "orgB must be in by-org rows")
}

func (s *UsageAggregateTestSuite) TestGetUsageGroupedByProject() {
	orgID := "org_" + system.GenerateID()
	prjA := "prj_" + system.GenerateID()
	prjB := "prj_" + system.GenerateID()
	user := "user_" + system.GenerateID()

	s.seedRow(orgID, user, prjA, "", "ses_"+system.GenerateID(), "claude-opus-4-7", 1000, 0.01)
	s.seedRow(orgID, user, prjA, "", "ses_"+system.GenerateID(), "claude-opus-4-7", 2000, 0.02)
	s.seedRow(orgID, user, prjB, "", "ses_"+system.GenerateID(), "claude-opus-4-7", 500, 0.005)

	rows, total, err := s.db.GetUsageGroupedByProject(s.ctx, s.buildQuery(orgID))
	s.Require().NoError(err)
	s.Equal(2, total)
	s.Len(rows, 2)
	// Sorted by total_cost desc -> prjA first
	s.Equal(prjA, rows[0].ProjectID)
	s.Equal(3000, rows[0].PromptTokens)
	s.Equal(prjB, rows[1].ProjectID)
}
