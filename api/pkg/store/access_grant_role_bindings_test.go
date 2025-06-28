package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestAccessGrantRoleBindingTestSuite(t *testing.T) {
	suite.Run(t, new(AccessGrantRoleBindingTestSuite))
}

type AccessGrantRoleBindingTestSuite struct {
	suite.Suite
	ctx  context.Context
	db   *PostgresStore
	org  *types.Organization
	role *types.Role
}

func (suite *AccessGrantRoleBindingTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()

	// Create a test organization
	orgID := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	}
	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.org = createdOrg

	// Create a test role
	role := &types.Role{
		ID:             system.GenerateUUID(),
		OrganizationID: suite.org.ID,
		Name:           "TestRole",
		Description:    "Test Role",
	}
	createdRole, err := suite.db.CreateRole(suite.ctx, role)
	suite.Require().NoError(err)
	suite.role = createdRole
}

func (suite *AccessGrantRoleBindingTestSuite) TearDownTestSuite() {
	// Clean up in reverse order of creation
	if suite.role != nil {
		err := suite.db.DeleteRole(suite.ctx, suite.role.ID)
		suite.NoError(err)
	}
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}

	// No need to close the database connection here as it's managed by TestMain
}

func (suite *AccessGrantRoleBindingTestSuite) TestCreateAccessGrantRoleBinding() {
	// Create a test access grant
	userAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     "test-dataset",
		UserID:         "test-user",
	}

	// Create access grant with no roles
	createdUserGrant, err := suite.db.CreateAccessGrant(suite.ctx, userAccessGrant, []*types.Role{})
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteAccessGrant(suite.ctx, createdUserGrant.ID)
		suite.NoError(err)
	})

	binding := &types.AccessGrantRoleBinding{
		AccessGrantID:  createdUserGrant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
	}

	created, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, binding)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(binding.AccessGrantID, created.AccessGrantID)
	suite.Equal(binding.RoleID, created.RoleID)
	suite.Equal(binding.OrganizationID, created.OrganizationID)
	suite.False(created.CreatedAt.IsZero())
	suite.False(created.UpdatedAt.IsZero())

	// Test successful creation with team

	teamAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     "test-dataset",
		TeamID:         "test-TestCreateAccessGrantRoleBinding",
	}

	// Create access grant with no roles
	createdTeamGrant, err := suite.db.CreateAccessGrant(suite.ctx, teamAccessGrant, []*types.Role{})
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteAccessGrant(suite.ctx, createdTeamGrant.ID)
		suite.NoError(err)
	})

	teamBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  createdTeamGrant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
	}

	created, err = suite.db.CreateAccessGrantRoleBinding(suite.ctx, teamBinding)
	suite.Require().NoError(err)
	suite.NotNil(created)

	// Should have role ID and access grant ID
	suite.Equal(teamBinding.RoleID, created.RoleID)
	suite.Equal(teamBinding.AccessGrantID, created.AccessGrantID)
}

func (suite *AccessGrantRoleBindingTestSuite) TestCreateAccessGrant_Validation() {
	// Test validation errors
	invalidCases := []struct {
		name    string
		binding *types.AccessGrantRoleBinding
	}{
		{
			name: "missing access grant ID",
			binding: &types.AccessGrantRoleBinding{
				RoleID:         suite.role.ID,
				OrganizationID: suite.org.ID,
			},
		},
		{
			name: "missing role ID",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID:  "access-grant-id",
				OrganizationID: suite.org.ID,
			},
		},
		{
			name: "missing organization ID",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID: "access-grant-id",
				RoleID:        suite.role.ID,
			},
		},
	}

	for _, tc := range invalidCases {
		suite.T().Run(tc.name, func(_ *testing.T) {
			_, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, tc.binding)
			suite.Error(err)
		})
	}
}

func (suite *AccessGrantRoleBindingTestSuite) TestGetAccessGrantRoleBindings_EmptyID() {

	_, err := suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		AccessGrantID:  "",
		OrganizationID: suite.org.ID,
	})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "access_grant_id must be specified")
}

func (suite *AccessGrantRoleBindingTestSuite) TestGetAccessGrantRoleBindings() {
	userAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     "test-dataset",
		UserID:         "test-user",
	}

	// Create access grant with no roles
	createdUserGrant, err := suite.db.CreateAccessGrant(suite.ctx, userAccessGrant, []*types.Role{})
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteAccessGrant(suite.ctx, createdUserGrant.ID)
		suite.NoError(err)
	})

	userBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  createdUserGrant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
	}

	teamAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     "test-dataset",
		TeamID:         "test-TestCreateAccessGrantRoleBinding",
	}

	// Create access grant with no roles
	createdTeamGrant, err := suite.db.CreateAccessGrant(suite.ctx, teamAccessGrant, []*types.Role{})
	suite.Require().NoError(err)

	teamBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  createdTeamGrant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
	}

	_, err = suite.db.CreateAccessGrantRoleBinding(suite.ctx, userBinding)
	suite.Require().NoError(err)

	_, err = suite.db.CreateAccessGrantRoleBinding(suite.ctx, teamBinding)
	suite.Require().NoError(err)

	// Test getting by access grant ID
	bindings, err := suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		AccessGrantID:  createdUserGrant.ID,
		OrganizationID: suite.org.ID,
	})
	suite.NoError(err)
	suite.Len(bindings, 1, "should have 1 binding, team should have another one")
	suite.Equal(userBinding.RoleID, bindings[0].RoleID, "role ID should match")

	teamBindings, err := suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		AccessGrantID: createdTeamGrant.ID,
	})
	suite.NoError(err)
	suite.Len(teamBindings, 1, "should have 1 binding, user should have another one")
	suite.Equal(teamBinding.RoleID, teamBindings[0].RoleID, "role ID should match")
}

func (suite *AccessGrantRoleBindingTestSuite) TestDeleteAccessGrantRoleBinding() {
	userAccessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceID:     "test-app-id",
		UserID:         "test-TestDeleteAccessGrantRoleBinding",
	}

	// Create access grant with no roles
	createdUserGrant, err := suite.db.CreateAccessGrant(suite.ctx, userAccessGrant, []*types.Role{})
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteAccessGrant(suite.ctx, createdUserGrant.ID)
		suite.NoError(err)
	})

	binding := &types.AccessGrantRoleBinding{
		AccessGrantID:  createdUserGrant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
	}

	created, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, binding)
	suite.Require().NoError(err)

	// Get the access grant and check whether the binding is present
	accessGrants, err := suite.db.ListAccessGrants(suite.ctx, &ListAccessGrantsQuery{
		OrganizationID: suite.org.ID,
		ResourceID:     userAccessGrant.ResourceID,
		UserID:         userAccessGrant.UserID,
	})
	suite.NoError(err)
	suite.Len(accessGrants, 1)

	// Check that the binding is present
	suite.Equal(suite.role.ID, accessGrants[0].Roles[0].ID, "role ID should match")

	// Test successful deletion
	err = suite.db.DeleteAccessGrantRoleBinding(suite.ctx, created.AccessGrantID, created.RoleID)
	suite.NoError(err)

	// Verify deletion
	bindings, err := suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		AccessGrantID: created.AccessGrantID,
		RoleID:        created.RoleID,
	})
	suite.NoError(err)
	suite.Empty(bindings)

	// Test validation errors
	err = suite.db.DeleteAccessGrantRoleBinding(suite.ctx, "", suite.role.ID)
	suite.Error(err)

	err = suite.db.DeleteAccessGrantRoleBinding(suite.ctx, createdUserGrant.ID, "")
	suite.Error(err)
}
