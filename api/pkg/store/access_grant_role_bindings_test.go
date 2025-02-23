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

func TestAccessGrantRoleBindingTestSuite(t *testing.T) {
	suite.Run(t, new(AccessGrantRoleBindingTestSuite))
}

type AccessGrantRoleBindingTestSuite struct {
	suite.Suite
	ctx   context.Context
	db    *PostgresStore
	org   *types.Organization
	grant *types.AccessGrant
	role  *types.Role
}

func (suite *AccessGrantRoleBindingTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)
	suite.db = store

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

	// Create a test access grant
	accessGrant := &types.AccessGrant{
		OrganizationID: suite.org.ID,
		ResourceType:   types.ResourceTypeDataset,
		ResourceID:     "test-dataset",
		UserID:         "test-user",
	}

	// Create access grant with no roles
	createdGrant, err := suite.db.CreateAccessGrant(suite.ctx, accessGrant, []*types.Role{})
	suite.Require().NoError(err)
	suite.grant = createdGrant
}

func (suite *AccessGrantRoleBindingTestSuite) TearDownTest() {
	// Clean up in reverse order of creation
	if suite.grant != nil {
		err := suite.db.DeleteAccessGrant(suite.ctx, suite.grant.ID)
		suite.NoError(err)
	}
	if suite.role != nil {
		err := suite.db.DeleteRole(suite.ctx, suite.role.ID)
		suite.NoError(err)
	}
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}
}

func (suite *AccessGrantRoleBindingTestSuite) TestCreateAccessGrantRoleBinding() {
	// Test successful creation with user
	binding := &types.AccessGrantRoleBinding{
		AccessGrantID:  suite.grant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
		UserID:         "test-user",
	}

	created, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, binding)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(binding.AccessGrantID, created.AccessGrantID)
	suite.Equal(binding.RoleID, created.RoleID)
	suite.Equal(binding.OrganizationID, created.OrganizationID)
	suite.Equal(binding.UserID, created.UserID)
	suite.False(created.CreatedAt.IsZero())
	suite.False(created.UpdatedAt.IsZero())

	// Test successful creation with team
	teamBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  suite.grant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
		TeamID:         "test-team",
	}

	created, err = suite.db.CreateAccessGrantRoleBinding(suite.ctx, teamBinding)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(teamBinding.TeamID, created.TeamID)

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
				UserID:         "test-user",
			},
		},
		{
			name: "missing role ID",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID:  suite.grant.ID,
				OrganizationID: suite.org.ID,
				UserID:         "test-user",
			},
		},
		{
			name: "missing organization ID",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID: suite.grant.ID,
				RoleID:        suite.role.ID,
				UserID:        "test-user",
			},
		},
		{
			name: "missing both user and team ID",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID:  suite.grant.ID,
				RoleID:         suite.role.ID,
				OrganizationID: suite.org.ID,
			},
		},
		{
			name: "both user and team ID specified",
			binding: &types.AccessGrantRoleBinding{
				AccessGrantID:  suite.grant.ID,
				RoleID:         suite.role.ID,
				OrganizationID: suite.org.ID,
				UserID:         "test-user",
				TeamID:         "test-team",
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

func (suite *AccessGrantRoleBindingTestSuite) TestGetAccessGrantRoleBindings() {
	// Create test bindings
	userBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  suite.grant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
		UserID:         "test-user-get",
	}

	teamBinding := &types.AccessGrantRoleBinding{
		AccessGrantID:  suite.grant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
		TeamID:         "test-team-get",
	}

	_, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, userBinding)
	suite.Require().NoError(err)

	_, err = suite.db.CreateAccessGrantRoleBinding(suite.ctx, teamBinding)
	suite.Require().NoError(err)

	// Test getting by access grant ID
	bindings, err := suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		AccessGrantID: suite.grant.ID,
	})
	suite.NoError(err)
	suite.Len(bindings, 2)

	// Test getting by role ID
	bindings, err = suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		RoleID: suite.role.ID,
	})
	suite.NoError(err)
	suite.Len(bindings, 2)

	// Test getting by user ID
	bindings, err = suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		UserID: "test-user-get",
	})
	suite.NoError(err)
	suite.Len(bindings, 1)
	suite.Equal("test-user-get", bindings[0].UserID)

	// Test getting by team ID
	bindings, err = suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		TeamID: "test-team-get",
	})
	suite.NoError(err)
	suite.Len(bindings, 1)
	suite.Equal("test-team-get", bindings[0].TeamID)

	// Test getting by organization ID
	bindings, err = suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		OrganizationID: suite.org.ID,
	})
	suite.NoError(err)
	suite.NotEmpty(bindings)

	// Test not found case
	bindings, err = suite.db.GetAccessGrantRoleBindings(suite.ctx, &GetAccessGrantRoleBindingsQuery{
		UserID: "non-existent-user",
	})
	suite.NoError(err)
	suite.Empty(bindings)
}

func (suite *AccessGrantRoleBindingTestSuite) TestDeleteAccessGrantRoleBinding() {
	// Create a test binding
	binding := &types.AccessGrantRoleBinding{
		AccessGrantID:  suite.grant.ID,
		RoleID:         suite.role.ID,
		OrganizationID: suite.org.ID,
		UserID:         "test-user-delete",
	}

	created, err := suite.db.CreateAccessGrantRoleBinding(suite.ctx, binding)
	suite.Require().NoError(err)

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

	err = suite.db.DeleteAccessGrantRoleBinding(suite.ctx, suite.grant.ID, "")
	suite.Error(err)
}
