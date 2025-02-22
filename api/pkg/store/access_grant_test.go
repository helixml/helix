package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestAccessGrantTestSuite(t *testing.T) {
	suite.Run(t, new(AccessGrantTestSuite))
}

type AccessGrantTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
	org *types.Organization // We need an organization for access grant tests
}

func (suite *AccessGrantTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)
	suite.db = store

	// Create a test organization for all access grant tests
	orgID := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.org = createdOrg
}

func (suite *AccessGrantTestSuite) TearDownTest() {
	// Cleanup the test organization
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}
}

func (suite *AccessGrantTestSuite) TestCreateAccessGrant() {
	// Create test roles
	roles := []*types.Role{
		{
			ID:             system.GenerateUUID(),
			OrganizationID: suite.org.ID,
			Name:           "TestRole1",
			Description:    "Test Role 1",
		},
		{
			ID:             system.GenerateUUID(),
			OrganizationID: suite.org.ID,
			Name:           "TestRole2",
			Description:    "Test Role 2",
		},
	}

	// Test successful creation with user
	accessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceApplication,
		ResourceID:     "test-dataset-1",
		UserID:         "test-user",
	}

	created, err := suite.db.CreateAccessGrant(suite.ctx, accessGrant, roles)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(accessGrant.OrganizationID, created.OrganizationID)
	suite.Equal(accessGrant.ResourceType, created.ResourceType)
	suite.Equal(accessGrant.ResourceID, created.ResourceID)
	suite.Equal(accessGrant.UserID, created.UserID)
	suite.False(created.CreatedAt.IsZero())
	suite.False(created.UpdatedAt.IsZero())

	// Test successful creation with team
	teamAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "test-dataset-2",
		TeamID:         "test-team",
	}

	created, err = suite.db.CreateAccessGrant(suite.ctx, teamAccessGrant, roles)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(teamAccessGrant.TeamID, created.TeamID)

	// Test validation errors
	invalidCases := []struct {
		name        string
		accessGrant *types.AccessGrant
	}{
		{
			name: "missing organization ID",
			accessGrant: &types.AccessGrant{
				ResourceType: types.ResourceTypeDataset,
				ResourceID:   "test-dataset",
				UserID:       "test-user",
			},
		},
		{
			name: "missing resource ID",
			accessGrant: &types.AccessGrant{
				OrganizationID: suite.org.ID,
				ResourceType:   types.ResourceTypeDataset,
				UserID:         "test-user",
			},
		},
		{
			name: "missing resource type",
			accessGrant: &types.AccessGrant{
				OrganizationID: suite.org.ID,
				ResourceID:     "test-dataset",
				UserID:         "test-user",
			},
		},
		{
			name: "missing both user and team ID",
			accessGrant: &types.AccessGrant{
				OrganizationID: suite.org.ID,
				ResourceType:   types.ResourceTypeDataset,
				ResourceID:     "test-dataset",
			},
		},
		{
			name: "both user and team ID specified",
			accessGrant: &types.AccessGrant{
				OrganizationID: suite.org.ID,
				ResourceType:   types.ResourceTypeDataset,
				ResourceID:     "test-dataset",
				UserID:         "test-user",
				TeamID:         "test-team",
			},
		},
	}

	for _, tc := range invalidCases {
		suite.T().Run(tc.name, func(_ *testing.T) {
			_, err := suite.db.CreateAccessGrant(suite.ctx, tc.accessGrant, roles)
			suite.Error(err)
		})
	}
}

func (suite *AccessGrantTestSuite) TestGetAccessGrant() {
	// Create test access grant
	accessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "test-dataset-get",
		UserID:         "test-user-get",
	}

	roles := []*types.Role{
		{
			ID:             system.GenerateUUID(),
			OrganizationID: suite.org.ID,
			Name:           "TestRole",
			Description:    "Test Role",
		},
	}

	// Create role
	role, err := suite.db.CreateRole(suite.ctx, roles[0])
	suite.Require().NoError(err)
	suite.NotNil(role)

	created, err := suite.db.CreateAccessGrant(suite.ctx, accessGrant, roles)
	suite.Require().NoError(err)

	// Test successful get by user ID
	found, err := suite.db.GetAccessGrant(suite.ctx, &GetAccessGrantQuery{
		OrganizationID: suite.org.ID,
		ResourceType:   created.ResourceType,
		ResourceID:     created.ResourceID,
		UserID:         created.UserID,
	})
	suite.Require().NoError(err)
	suite.Require().Len(found, 1)
	suite.Equal(created.ID, found[0].ID)
	suite.Equal(created.UserID, found[0].UserID)
	suite.Require().Len(found[0].Roles, 1)
	suite.Equal(roles[0].ID, found[0].Roles[0].ID)

	// Test get by team ID
	teamAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "test-dataset-get-team",
		TeamID:         "test-team-get",
	}

	created, err = suite.db.CreateAccessGrant(suite.ctx, teamAccessGrant, roles)
	suite.Require().NoError(err)

	found, err = suite.db.GetAccessGrant(suite.ctx, &GetAccessGrantQuery{
		OrganizationID: suite.org.ID,
		ResourceType:   created.ResourceType,
		ResourceID:     created.ResourceID,
		TeamIDs:        []string{created.TeamID},
	})
	suite.Require().NoError(err)
	suite.Require().Len(found, 1)
	suite.Equal(created.TeamID, found[0].TeamID)

	// Test not found
	found, err = suite.db.GetAccessGrant(suite.ctx, &GetAccessGrantQuery{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "non-existent",
		UserID:         "non-existent",
	})
	suite.NoError(err)
	suite.Empty(found)

	// Test validation errors
	invalidQueries := []struct {
		name  string
		query *GetAccessGrantQuery
	}{
		{
			name: "missing resource type",
			query: &GetAccessGrantQuery{
				ResourceID: "test-dataset",
				UserID:     "test-user",
			},
		},
		{
			name: "missing resource ID",
			query: &GetAccessGrantQuery{
				ResourceType: types.ResourceTypeDataset,
				UserID:       "test-user",
			},
		},
		{
			name: "missing both user and team IDs",
			query: &GetAccessGrantQuery{
				ResourceType: types.ResourceTypeDataset,
				ResourceID:   "test-dataset",
			},
		},
	}

	for _, tc := range invalidQueries {
		suite.T().Run(tc.name, func(_ *testing.T) {
			_, err := suite.db.GetAccessGrant(suite.ctx, tc.query)
			suite.Error(err)
		})
	}
}

func (suite *AccessGrantTestSuite) TestDeleteAccessGrant() {
	// Create test access grant
	accessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "test-dataset-delete",
		UserID:         "test-user-delete",
	}

	roles := []*types.Role{
		{
			ID:             system.GenerateUUID(),
			OrganizationID: suite.org.ID,
			Name:           "TestRole",
			Description:    "Test Role",
		},
	}

	// Create org role
	orgRole, err := suite.db.CreateRole(suite.ctx, roles[0])
	suite.Require().NoError(err)
	suite.NotNil(orgRole)

	created, err := suite.db.CreateAccessGrant(suite.ctx, accessGrant, roles)
	suite.Require().NoError(err)

	// Test successful delete
	err = suite.db.DeleteAccessGrant(suite.ctx, created.ID)
	suite.Require().NoError(err)

	// Verify deletion
	found, err := suite.db.GetAccessGrant(suite.ctx, &GetAccessGrantQuery{
		OrganizationID: suite.org.ID,
		ResourceType:   created.ResourceType,
		ResourceID:     created.ResourceID,
		UserID:         created.UserID,
	})
	suite.NoError(err)
	suite.Empty(found)

	// Test validation
	err = suite.db.DeleteAccessGrant(suite.ctx, "")
	suite.Error(err)
}
