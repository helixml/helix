package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestLLMCallsTestSuite(t *testing.T) {
	suite.Run(t, new(LLMCallsTestSuite))
}

type LLMCallsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *LLMCallsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()
}

func (suite *LLMCallsTestSuite) TearDownTestSuite() {
	_ = suite.db.Close()
}

// TestListLLMCalls_OrganizationIDFilter verifies the new OrganizationID
// query field scopes results. Without this filter the admin endpoint
// listed across orgs; the Usage dashboard drill-down relies on it to
// stop an org member seeing other orgs' traces.
func (suite *LLMCallsTestSuite) TestListLLMCalls_OrganizationIDFilter() {
	orgA := "org_" + system.GenerateID()
	orgB := "org_" + system.GenerateID()

	// Two rows in orgA, one row in orgB.
	for i := range 2 {
		_, err := suite.db.CreateLLMCall(suite.ctx, &types.LLMCall{
			OrganizationID: orgA,
			Model:          "claude-opus-4-7",
			Provider:       string(types.ProviderAnthropic),
			TotalTokens:    int64(100 + i),
		})
		suite.Require().NoError(err)
	}
	_, err := suite.db.CreateLLMCall(suite.ctx, &types.LLMCall{
		OrganizationID: orgB,
		Model:          "claude-opus-4-7",
		Provider:       string(types.ProviderAnthropic),
		TotalTokens:    999,
	})
	suite.Require().NoError(err)

	// orgA filter sees only orgA rows.
	got, total, err := suite.db.ListLLMCalls(suite.ctx, &ListLLMCallsQuery{
		OrganizationID: orgA,
		Page:           1,
		PerPage:        50,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(2), total, "orgA total")
	suite.Len(got, 2)
	for _, c := range got {
		suite.Equal(orgA, c.OrganizationID)
	}

	// orgB filter sees only the orgB row.
	got, total, err = suite.db.ListLLMCalls(suite.ctx, &ListLLMCallsQuery{
		OrganizationID: orgB,
		Page:           1,
		PerPage:        50,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total, "orgB total")
	suite.Len(got, 1)
	suite.Equal(orgB, got[0].OrganizationID)

	// Empty OrganizationID does NOT add a where clause (caller-side
	// admin scoping is what we rely on). Both orgA rows + orgB row
	// surface, plus any pre-existing rows from other tests.
	_, total, err = suite.db.ListLLMCalls(suite.ctx, &ListLLMCallsQuery{
		Page:    1,
		PerPage: 50,
	})
	suite.Require().NoError(err)
	suite.GreaterOrEqual(total, int64(3), "no-filter must include all seeded rows")
}
