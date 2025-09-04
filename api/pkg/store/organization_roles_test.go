package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestRolesTestSuite(t *testing.T) {
	suite.Run(t, new(RolesTestSuite))
}

type RolesTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
	org *types.Organization // We need an organization for role tests
}

func (suite *RolesTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	suite.db = GetTestDB()

	// Create a test organization for all role tests
	orgID := uuid.New().String()
	org := &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.org = createdOrg
}

func (suite *RolesTestSuite) TearDownTestSuite() {
	_ = suite.db.Close()
}

func (suite *RolesTestSuite) TearDownTest() {
	// Cleanup the test organization
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}
}

func (suite *RolesTestSuite) TestCreateRole() {
	role := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role",
		Description:    "Test role description",
		Config: types.Config{
			Rules: []types.Rule{
				{
					Resources: []types.Resource{types.ResourceApplication},
					Actions:   []types.Action{types.ActionGet, types.ActionList},
					Effect:    types.EffectAllow,
				},
			},
		},
	}

	created, err := suite.db.CreateRole(suite.ctx, role)
	suite.Require().NoError(err)
	suite.Require().NotNil(created)

	suite.Equal(role.ID, created.ID)
	suite.Equal(role.OrganizationID, created.OrganizationID)
	suite.Equal(role.Name, created.Name)
	suite.Equal(role.Description, created.Description)
	suite.NotEmpty(created.CreatedAt)
	suite.NotEmpty(created.UpdatedAt)
	suite.Equal(role.Config, created.Config)

	// Cleanup
	suite.T().Cleanup(func() {
		err := suite.db.DeleteRole(suite.ctx, created.ID)
		suite.NoError(err)
	})
}

func (suite *RolesTestSuite) TestCreateRoleValidation() {
	// Test missing organization ID
	_, err := suite.db.CreateRole(suite.ctx, &types.Role{
		ID:   uuid.New().String(),
		Name: "test-role",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "organization_id not specified")

	// Test missing name
	_, err = suite.db.CreateRole(suite.ctx, &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
	})
	suite.Error(err)
	suite.Contains(err.Error(), "name not specified")
}

func (suite *RolesTestSuite) TestGetRole() {
	role := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role-get",
		Description:    "Test role for get operation",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	created, err := suite.db.CreateRole(suite.ctx, role)
	suite.Require().NoError(err)

	fetched, err := suite.db.GetRole(suite.ctx, created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched)

	suite.Equal(created.ID, fetched.ID)
	suite.Equal(created.OrganizationID, fetched.OrganizationID)
	suite.Equal(created.Name, fetched.Name)
	suite.Equal(created.Description, fetched.Description)

	// Cleanup
	suite.T().Cleanup(func() {
		err := suite.db.DeleteRole(suite.ctx, created.ID)
		suite.NoError(err)
	})
}

func (suite *RolesTestSuite) TestGetRoleNotFound() {
	_, err := suite.db.GetRole(suite.ctx, uuid.New().String())
	suite.Error(err)
}

func (suite *RolesTestSuite) TestListRoles() {
	// Create multiple roles
	role1 := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role-list-1",
		Description:    "Test role 1 for list operation",
	}
	role2 := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role-list-2",
		Description:    "Test role 2 for list operation",
	}

	created1, err := suite.db.CreateRole(suite.ctx, role1)
	suite.Require().NoError(err)
	created2, err := suite.db.CreateRole(suite.ctx, role2)
	suite.Require().NoError(err)

	roles, err := suite.db.ListRoles(suite.ctx, suite.org.ID)
	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(roles), 2)

	// Verify roles for different organization are not returned
	otherOrgID := uuid.New().String()
	otherOrgRoles, err := suite.db.ListRoles(suite.ctx, otherOrgID)
	suite.Require().NoError(err)
	suite.Empty(otherOrgRoles)

	// Cleanup
	suite.T().Cleanup(func() {
		err := suite.db.DeleteRole(suite.ctx, created1.ID)
		suite.NoError(err)
		err = suite.db.DeleteRole(suite.ctx, created2.ID)
		suite.NoError(err)
	})
}

func (suite *RolesTestSuite) TestUpdateRole() {
	role := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role-update",
		Description:    "Original description",
	}

	created, err := suite.db.CreateRole(suite.ctx, role)
	suite.Require().NoError(err)

	// Update the role
	created.Description = "Updated description"
	updated, err := suite.db.UpdateRole(suite.ctx, created)
	suite.Require().NoError(err)
	suite.Require().NotNil(updated)

	suite.Equal(created.ID, updated.ID)
	suite.Equal("Updated description", updated.Description)
	suite.True(updated.UpdatedAt.After(created.CreatedAt))

	// Cleanup
	suite.T().Cleanup(func() {
		err := suite.db.DeleteRole(suite.ctx, created.ID)
		suite.NoError(err)
	})
}

func (suite *RolesTestSuite) TestDeleteRole() {
	role := &types.Role{
		ID:             uuid.New().String(),
		OrganizationID: suite.org.ID,
		Name:           "test-role-delete",
		Description:    "Test role for delete operation",
	}

	created, err := suite.db.CreateRole(suite.ctx, role)
	suite.Require().NoError(err)

	// Delete the role
	err = suite.db.DeleteRole(suite.ctx, created.ID)
	suite.Require().NoError(err)

	// Verify role is deleted
	_, err = suite.db.GetRole(suite.ctx, created.ID)
	suite.Error(err)
}
