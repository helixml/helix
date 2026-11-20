package store

import (
	"context"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func newPRProposal() *types.SpecTaskPRProposal {
	return &types.SpecTaskPRProposal{
		ID:           "prp_" + system.GenerateUUID(),
		ProjectID:    "prj_" + system.GenerateUUID(),
		RepositoryID: "code-" + system.GenerateUUID(),
		HeadBranch:   "feature/x",
		BaseBranch:   "main",
	}
}

// An org bot's proposal has no spec task; the store must accept it and the
// bot's list must find it.
func (suite *PostgresStoreTestSuite) TestCreateSpecTaskPRProposal_OrgBotOwner() {
	ctx := context.Background()
	p := newPRProposal()
	p.OrgBotID = "recruiter"
	p.OrganizationID = "org_" + system.GenerateUUID()

	suite.Require().NoError(suite.db.CreateSpecTaskPRProposal(ctx, p))

	got, err := suite.db.ListSpecTaskPRProposals(ctx, &types.SpecTaskPRProposalFilter{
		OrgBotID: p.OrgBotID, OrganizationID: p.OrganizationID,
	})
	suite.Require().NoError(err)
	suite.Require().Len(got, 1)
	suite.Equal(p.ID, got[0].ID)
	suite.Empty(got[0].SpecTaskID)
}

func (suite *PostgresStoreTestSuite) TestCreateSpecTaskPRProposal_SpecTaskOwner() {
	p := newPRProposal()
	p.SpecTaskID = "spt_" + system.GenerateUUID()
	suite.NoError(suite.db.CreateSpecTaskPRProposal(context.Background(), p))
}

func (suite *PostgresStoreTestSuite) TestCreateSpecTaskPRProposal_RequiresExactlyOneOwner() {
	ctx := context.Background()

	none := newPRProposal()
	suite.Error(suite.db.CreateSpecTaskPRProposal(ctx, none), "no owner")

	both := newPRProposal()
	both.SpecTaskID = "spt_x"
	both.OrgBotID = "recruiter"
	both.OrganizationID = "org_x"
	suite.Error(suite.db.CreateSpecTaskPRProposal(ctx, both), "two owners")

	botWithoutOrg := newPRProposal()
	botWithoutOrg.OrgBotID = "recruiter"
	suite.Error(suite.db.CreateSpecTaskPRProposal(ctx, botWithoutOrg), "bot id is only unique within an org")
}
