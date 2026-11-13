package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// ListAPIKeys filters by project in the query, and UserManaged leaves out the
// keys minted for a session or spec task.
func (suite *PostgresStoreTestSuite) TestListAPIKeysByProjectUserManaged() {
	owner := "test-" + system.GenerateUUID()
	mk := func(project, session, task string) *types.ApiKey {
		key, err := system.GenerateAPIKey()
		suite.Require().NoError(err)
		created, err := suite.db.CreateAPIKey(suite.ctx, &types.ApiKey{
			Key: key, Name: "k", Owner: owner, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI,
			OrganizationID: "org_x", ProjectID: project, SessionID: session, SpecTaskID: task,
		})
		suite.Require().NoError(err)
		suite.T().Cleanup(func() { _ = suite.db.DeleteAPIKey(suite.ctx, key) })
		return created
	}
	want := mk("prj_x", "", "")
	mk("prj_x", "ses_1", "")
	mk("prj_x", "ses_2", "spt_1")
	mk("prj_other", "", "")
	mk("", "", "")

	got, err := suite.db.ListAPIKeys(suite.ctx, &ListAPIKeysQuery{
		Owner: owner, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI,
		OrganizationID: "org_x", ProjectID: "prj_x", UserManaged: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(got, 1)
	suite.Equal(want.Key, got[0].Key)

	all, err := suite.db.ListAPIKeys(suite.ctx, &ListAPIKeysQuery{Owner: owner, ProjectID: "prj_x"})
	suite.Require().NoError(err)
	suite.Len(all, 3)
}
