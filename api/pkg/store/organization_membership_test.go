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

func TestOrganizationMembershipTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationMembershipTestSuite))
}

type OrganizationMembershipTestSuite struct {
	suite.Suite
	ctx  context.Context
	db   *PostgresStore
	org  *types.Organization // We need an organization for membership tests
	user *types.User
}

func (suite *OrganizationMembershipTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	suite.db = GetTestDB()

	// Create a test organization for all membership tests
	orgID := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.org = createdOrg

	userID := system.GenerateUserID()
	user := &types.User{
		ID:        userID,
		Email:     userID + "@example.com",
		Username:  userID,
		FullName:  "Test User",
		CreatedAt: time.Now(),
	}
	createdUser, err := suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)
	suite.user = createdUser
}

func (suite *OrganizationMembershipTestSuite) TearDownTest() {
	// Cleanup the test organization
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}
}

func (suite *OrganizationMembershipTestSuite) TestCreateOrganizationMembership() {
	membership := &types.OrganizationMembership{
		UserID:         suite.user.ID,
		OrganizationID: suite.org.ID,
		Role:           types.OrganizationRoleMember,
	}

	created, err := suite.db.CreateOrganizationMembership(suite.ctx, membership)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(membership.UserID, created.UserID)
	suite.Equal(membership.OrganizationID, created.OrganizationID)
	suite.Equal(membership.Role, created.Role)
	suite.False(created.CreatedAt.IsZero())
	suite.False(created.UpdatedAt.IsZero())

	// Test validation
	invalidMembership := &types.OrganizationMembership{}
	_, err = suite.db.CreateOrganizationMembership(suite.ctx, invalidMembership)
	suite.Error(err)
}

func (suite *OrganizationMembershipTestSuite) TestGetOrganizationMembership() {
	membership := &types.OrganizationMembership{
		UserID:         suite.user.ID,
		OrganizationID: suite.org.ID,
		Role:           types.OrganizationRoleMember,
	}
	created, err := suite.db.CreateOrganizationMembership(suite.ctx, membership)
	suite.Require().NoError(err)

	// Test successful get
	found, err := suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: created.OrganizationID,
		UserID:         created.UserID,
	})
	suite.Require().NoError(err)
	suite.NotNil(found)
	suite.Equal(created.UserID, found.UserID)
	suite.Equal(created.OrganizationID, found.OrganizationID)
	suite.Equal(created.Role, found.Role)

	// Check whether user is preloaded
	suite.NotNil(found.User.ID)

	// Test not found
	_, err = suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: "non-existent",
		UserID:         "non-existent",
	})
	suite.ErrorIs(err, ErrNotFound)

	// Test validation
	_, err = suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{})
	suite.Error(err)
}

func (suite *OrganizationMembershipTestSuite) TestListOrganizationMemberships() {
	// Create multiple memberships
	memberships := []*types.OrganizationMembership{
		{
			UserID:         system.GenerateUserID(),
			OrganizationID: suite.org.ID,
			Role:           types.OrganizationRoleMember,
		},
		{
			UserID:         system.GenerateUserID(),
			OrganizationID: suite.org.ID,
			Role:           types.OrganizationRoleOwner,
		},
	}

	for _, m := range memberships {

		user := &types.User{
			ID:        m.UserID,
			Email:     m.UserID + "@example.com",
			Username:  m.UserID,
			FullName:  "Test User",
			CreatedAt: time.Now(),
		}
		_, err := suite.db.CreateUser(suite.ctx, user)
		suite.Require().NoError(err)

		_, err = suite.db.CreateOrganizationMembership(suite.ctx, m)
		suite.Require().NoError(err)
	}

	// Test list all
	found, err := suite.db.ListOrganizationMemberships(suite.ctx, &ListOrganizationMembershipsQuery{
		OrganizationID: suite.org.ID,
	})
	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(found), 2)

	// Test list by user
	found, err = suite.db.ListOrganizationMemberships(suite.ctx, &ListOrganizationMembershipsQuery{
		UserID: memberships[0].UserID,
	})
	suite.Require().NoError(err)
	suite.Len(found, 1)
	suite.Equal(memberships[0].UserID, found[0].UserID)

	// Test list with no results
	found, err = suite.db.ListOrganizationMemberships(suite.ctx, &ListOrganizationMembershipsQuery{
		OrganizationID: "non-existent",
	})
	suite.Require().NoError(err)
	suite.Empty(found)
}

func (suite *OrganizationMembershipTestSuite) TestUpdateOrganizationMembership() {
	membership := &types.OrganizationMembership{
		UserID:         suite.user.ID,
		OrganizationID: suite.org.ID,
		Role:           types.OrganizationRoleMember,
	}
	created, err := suite.db.CreateOrganizationMembership(suite.ctx, membership)
	suite.Require().NoError(err)

	// Update role
	created.Role = types.OrganizationRoleOwner
	updated, err := suite.db.UpdateOrganizationMembership(suite.ctx, created)
	suite.Require().NoError(err)
	suite.Equal(types.OrganizationRoleOwner, updated.Role)

	// Test validation
	invalidMembership := &types.OrganizationMembership{}
	_, err = suite.db.UpdateOrganizationMembership(suite.ctx, invalidMembership)
	suite.Error(err)
}

func (suite *OrganizationMembershipTestSuite) TestDeleteOrganizationMembership() {
	membership := &types.OrganizationMembership{
		UserID:         suite.user.ID,
		OrganizationID: suite.org.ID,
		Role:           types.OrganizationRoleMember,
	}
	created, err := suite.db.CreateOrganizationMembership(suite.ctx, membership)
	suite.Require().NoError(err)

	// Test successful delete
	err = suite.db.DeleteOrganizationMembership(suite.ctx, created.OrganizationID, created.UserID)
	suite.Require().NoError(err)

	// Verify deletion
	_, err = suite.db.GetOrganizationMembership(suite.ctx, &GetOrganizationMembershipQuery{
		OrganizationID: created.OrganizationID,
		UserID:         created.UserID,
	})
	suite.ErrorIs(err, ErrNotFound)

	// Test validation
	err = suite.db.DeleteOrganizationMembership(suite.ctx, "", "")
	suite.Error(err)
}
