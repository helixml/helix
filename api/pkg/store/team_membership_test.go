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

func TestTeamMembershipTestSuite(t *testing.T) {
	suite.Run(t, new(TeamMembershipTestSuite))
}

type TeamMembershipTestSuite struct {
	suite.Suite
	ctx  context.Context
	db   *PostgresStore
	org  *types.Organization
	team *types.Team
	user *types.User
}

func (suite *TeamMembershipTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var storeCfg config.Store
	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

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

	// Create a test team
	teamID := system.GenerateTeamID()
	team := &types.Team{
		ID:             teamID,
		Name:           "Test Team " + teamID,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.team = createdTeam

	// Create a test user
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

func (suite *TeamMembershipTestSuite) TearDownTestSuite() {
	// Cleanup the test team
	if suite.team != nil {
		err := suite.db.DeleteTeam(suite.ctx, suite.team.ID)
		suite.NoError(err)
	}

	// Cleanup the test organization
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}

	_ = suite.db.Close()
}

func (suite *TeamMembershipTestSuite) TestCreateTeamMembership() {
	membership := &types.TeamMembership{
		OrganizationID: suite.org.ID,
		UserID:         suite.user.ID,
		TeamID:         suite.team.ID,
	}

	created, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Require().NoError(err)
	suite.NotNil(created)
	suite.Equal(membership.UserID, created.UserID)
	suite.Equal(membership.TeamID, created.TeamID)
	suite.False(created.CreatedAt.IsZero())
	suite.False(created.UpdatedAt.IsZero())

	// Test validation
	invalidMembership := &types.TeamMembership{}
	_, err = suite.db.CreateTeamMembership(suite.ctx, invalidMembership)
	suite.Error(err)
}

func (suite *TeamMembershipTestSuite) TestCreateTeamMembership_UserID_NotSpecified() {
	membership := &types.TeamMembership{
		OrganizationID: suite.org.ID,
		TeamID:         suite.team.ID,
	}

	_, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Error(err)
	suite.Contains(err.Error(), "user_id not specified")
}

func (suite *TeamMembershipTestSuite) TestCreateTeamMembership_TeamID_NotSpecified() {
	membership := &types.TeamMembership{
		OrganizationID: suite.org.ID,
		UserID:         suite.user.ID,
	}

	_, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Error(err)
	suite.Contains(err.Error(), "team_id not specified")
}

func (suite *TeamMembershipTestSuite) TestCreateTeamMembership_OrganizationID_NotSpecified() {
	membership := &types.TeamMembership{
		UserID: suite.user.ID,
		TeamID: suite.team.ID,
	}

	_, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Error(err)
	suite.Contains(err.Error(), "organization_id not specified")
}

func (suite *TeamMembershipTestSuite) TestGetTeamMembership() {
	membership := &types.TeamMembership{
		OrganizationID: suite.org.ID,
		UserID:         suite.user.ID,
		TeamID:         suite.team.ID,
	}
	created, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Require().NoError(err)

	// Test successful get
	found, err := suite.db.GetTeamMembership(suite.ctx, &GetTeamMembershipQuery{
		TeamID: created.TeamID,
		UserID: created.UserID,
	})
	suite.Require().NoError(err)
	suite.NotNil(found)
	suite.Equal(created.UserID, found.UserID)
	suite.Equal(created.TeamID, found.TeamID)

	// Check whether user and team are preloaded
	suite.NotNil(found.User)
	suite.NotNil(found.Team)

	// Test not found
	_, err = suite.db.GetTeamMembership(suite.ctx, &GetTeamMembershipQuery{
		TeamID: "non-existent",
		UserID: "non-existent",
	})
	suite.ErrorIs(err, ErrNotFound)

	// Test validation
	_, err = suite.db.GetTeamMembership(suite.ctx, &GetTeamMembershipQuery{})
	suite.Error(err)
}

func (suite *TeamMembershipTestSuite) TestListTeamMemberships() {
	// Create multiple memberships
	memberships := []*types.TeamMembership{
		{
			OrganizationID: suite.org.ID,
			UserID:         system.GenerateUserID(),
			TeamID:         suite.team.ID,
		},
		{
			OrganizationID: suite.org.ID,
			UserID:         system.GenerateUserID(),
			TeamID:         suite.team.ID,
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

		_, err = suite.db.CreateTeamMembership(suite.ctx, m)
		suite.Require().NoError(err)
	}

	// Test list all
	found, err := suite.db.ListTeamMemberships(suite.ctx, &ListTeamMembershipsQuery{
		TeamID: suite.team.ID,
	})
	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(found), 2)

	// Test list by user
	found, err = suite.db.ListTeamMemberships(suite.ctx, &ListTeamMembershipsQuery{
		UserID: memberships[0].UserID,
	})
	suite.Require().NoError(err)
	suite.Len(found, 1)
	suite.Equal(memberships[0].UserID, found[0].UserID)

	// Test list with no results
	found, err = suite.db.ListTeamMemberships(suite.ctx, &ListTeamMembershipsQuery{
		TeamID: "non-existent",
	})
	suite.Require().NoError(err)
	suite.Empty(found)
}

func (suite *TeamMembershipTestSuite) TestDeleteTeamMembership() {
	membership := &types.TeamMembership{
		OrganizationID: suite.org.ID,
		UserID:         suite.user.ID,
		TeamID:         suite.team.ID,
	}
	created, err := suite.db.CreateTeamMembership(suite.ctx, membership)
	suite.Require().NoError(err)

	// Test successful delete
	err = suite.db.DeleteTeamMembership(suite.ctx, created.TeamID, created.UserID)
	suite.Require().NoError(err)

	// Verify deletion
	_, err = suite.db.GetTeamMembership(suite.ctx, &GetTeamMembershipQuery{
		TeamID: created.TeamID,
		UserID: created.UserID,
	})
	suite.ErrorIs(err, ErrNotFound)

	// Test validation
	err = suite.db.DeleteTeamMembership(suite.ctx, "", "")
	suite.Error(err)
}
