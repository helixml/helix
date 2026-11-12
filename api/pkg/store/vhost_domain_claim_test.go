package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// A claim outlives its route: deleting the route releases it, verifying the
// route stamps it, and the latest claim is what a re-claim consults.
func (suite *PostgresStoreTestSuite) TestVHostDomainClaimFollowsRouteLifecycle() {
	hostname := "claim-" + system.GenerateID() + ".customer.example"

	route := &types.VHostRoute{
		Hostname: hostname, TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_first", VerificationToken: "tok1",
	}
	suite.Require().NoError(suite.db.CreateVHostRoute(suite.ctx, route))
	suite.Require().NoError(suite.db.CreateVHostDomainClaim(suite.ctx, &types.VHostDomainClaim{
		Hostname: hostname, TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_first", OrganizationID: "org_a",
	}))

	suite.Require().NoError(suite.db.MarkVHostRouteVerified(suite.ctx, route.ID))
	claim, err := suite.db.GetLatestVHostDomainClaim(suite.ctx, hostname)
	suite.Require().NoError(err)
	suite.Equal("prj_first", claim.TargetID)
	suite.NotNil(claim.VerifiedAt)
	suite.Nil(claim.ReleasedAt)

	suite.Require().NoError(suite.db.DeleteVHostRoute(suite.ctx, route.ID))
	_, err = suite.db.GetVHostRouteByHostname(suite.ctx, hostname)
	suite.ErrorIs(err, ErrNotFound)
	claim, err = suite.db.GetLatestVHostDomainClaim(suite.ctx, hostname)
	suite.Require().NoError(err)
	suite.NotNil(claim.ReleasedAt, "deleting the route releases the claim")
	suite.False(claim.RequiresOwnershipProof("prj_first", "org_a"))
	suite.True(claim.RequiresOwnershipProof("prj_second", "org_a"))
	suite.True(claim.RequiresOwnershipProof("prj_first", "org_b"))

	// A second holder's claim becomes the latest one; deleting by target
	// releases it too.
	suite.Require().NoError(suite.db.CreateVHostRoute(suite.ctx, &types.VHostRoute{
		Hostname: hostname, TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_second", VerificationToken: "tok2", OwnershipProofRequired: true,
	}))
	suite.Require().NoError(suite.db.CreateVHostDomainClaim(suite.ctx, &types.VHostDomainClaim{
		Hostname: hostname, TargetKind: types.VHostTargetProjectWebService, TargetID: "prj_second", OrganizationID: "org_b",
	}))
	claim, err = suite.db.GetLatestVHostDomainClaim(suite.ctx, hostname)
	suite.Require().NoError(err)
	suite.Equal("prj_second", claim.TargetID)
	suite.Nil(claim.ReleasedAt)

	suite.Require().NoError(suite.db.DeleteVHostRoutesByTarget(suite.ctx, types.VHostTargetProjectWebService, "prj_second"))
	claim, err = suite.db.GetLatestVHostDomainClaim(suite.ctx, hostname)
	suite.Require().NoError(err)
	suite.Equal("prj_second", claim.TargetID)
	suite.NotNil(claim.ReleasedAt)

	_, err = suite.db.GetLatestVHostDomainClaim(suite.ctx, "never-"+hostname)
	suite.ErrorIs(err, ErrNotFound)
}
