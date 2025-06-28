package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestTeamsTestSuite(t *testing.T) {
	suite.Run(t, new(TeamsTestSuite))
}

type TeamsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
	org *types.Organization // We need an organization for team tests
}

func (suite *TeamsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()

	// Create a test organization for all team tests
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

func (suite *TeamsTestSuite) TearDownTest() {
	// Cleanup the test organization
	if suite.org != nil {
		err := suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	}
}

func (suite *TeamsTestSuite) TestCreateTeam() {
	id := system.GenerateTeamID()
	team := &types.Team{
		ID:             id,
		Name:           "Test Team " + id,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.NotNil(createdTeam)
	suite.Equal(team.ID, createdTeam.ID)
	suite.Equal(team.Name, createdTeam.Name)
	suite.Equal(team.OrganizationID, createdTeam.OrganizationID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeam(suite.ctx, createdTeam.ID)
		suite.NoError(err)
	})
}

func (suite *TeamsTestSuite) TestCreateTeam_AlreadyExists() {
	id := system.GenerateTeamID()
	team := &types.Team{
		ID:             id,
		Name:           "Test Team " + id,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.NotNil(createdTeam)
	suite.Equal(team.ID, createdTeam.ID)
	suite.Equal(team.Name, createdTeam.Name)
	suite.Equal(team.OrganizationID, createdTeam.OrganizationID)

	_, err = suite.db.CreateTeam(suite.ctx, team)
	suite.Error(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeam(suite.ctx, createdTeam.ID)
		suite.NoError(err)
	})
}

func (suite *TeamsTestSuite) TestGetTeam() {
	// Create a test team first
	id := system.GenerateTeamID()
	team := &types.Team{
		ID:             id,
		Name:           "Test Team " + id,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.NotNil(createdTeam)

	// Test getting by ID
	fetchedTeam, err := suite.db.GetTeam(suite.ctx, &GetTeamQuery{ID: createdTeam.ID})
	suite.NoError(err)
	suite.NotNil(fetchedTeam)
	suite.Equal(createdTeam.ID, fetchedTeam.ID)
	suite.Equal(createdTeam.Name, fetchedTeam.Name)

	// Test getting by Organization ID and Name
	fetchedByOrgAndName, err := suite.db.GetTeam(suite.ctx, &GetTeamQuery{
		OrganizationID: suite.org.ID,
		Name:           createdTeam.Name,
	})
	suite.NoError(err)
	suite.NotNil(fetchedByOrgAndName)
	suite.Equal(createdTeam.ID, fetchedByOrgAndName.ID)

	// Test getting non-existent team
	_, err = suite.db.GetTeam(suite.ctx, &GetTeamQuery{ID: "non-existent-id"})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeam(suite.ctx, createdTeam.ID)
		suite.NoError(err)
	})
}

func (suite *TeamsTestSuite) TestUpdateTeam() {
	// Create a test team first
	id := system.GenerateTeamID()
	team := &types.Team{
		ID:             id,
		Name:           "Test Team " + id,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.NotNil(createdTeam)

	// Update the team
	updatedName := "Updated Team " + id
	createdTeam.Name = updatedName
	updatedTeam, err := suite.db.UpdateTeam(suite.ctx, createdTeam)
	suite.NoError(err)
	suite.NotNil(updatedTeam)
	suite.Equal(updatedName, updatedTeam.Name)

	// Verify the update
	fetchedTeam, err := suite.db.GetTeam(suite.ctx, &GetTeamQuery{ID: createdTeam.ID})
	suite.NoError(err)
	suite.Equal(updatedName, fetchedTeam.Name)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTeam(suite.ctx, createdTeam.ID)
		suite.NoError(err)
	})
}

func (suite *TeamsTestSuite) TestDeleteTeam() {
	// Create a test team first
	id := system.GenerateTeamID()
	team := &types.Team{
		ID:             id,
		Name:           "Test Team " + id,
		OrganizationID: suite.org.ID,
	}

	createdTeam, err := suite.db.CreateTeam(suite.ctx, team)
	suite.Require().NoError(err)
	suite.NotNil(createdTeam)

	// Delete the team
	err = suite.db.DeleteTeam(suite.ctx, createdTeam.ID)
	suite.NoError(err)

	// Verify the deletion
	_, err = suite.db.GetTeam(suite.ctx, &GetTeamQuery{ID: createdTeam.ID})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test deleting non-existent team
	err = suite.db.DeleteTeam(suite.ctx, "non-existent-id")
	suite.NoError(err) // Should not return error as delete is idempotent
}

func (suite *TeamsTestSuite) TestListTeams() {
	// Create multiple test teams
	teamsToCreate := []*types.Team{
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 1",
			OrganizationID: suite.org.ID,
		},
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 2",
			OrganizationID: suite.org.ID,
		},
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 3",
			OrganizationID: suite.org.ID,
		},
	}

	for _, team := range teamsToCreate {
		_, err := suite.db.CreateTeam(suite.ctx, team)
		suite.Require().NoError(err)
	}

	// Test listing all teams for the organization
	orgTeams, err := suite.db.ListTeams(suite.ctx, &ListTeamsQuery{OrganizationID: suite.org.ID})
	suite.NoError(err)
	suite.Equal(len(teamsToCreate), len(orgTeams))

	// Create another organization and team to test filtering
	otherOrg, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		ID:    system.GenerateOrganizationID(),
		Name:  "Other Org",
		Owner: "test-user",
	})
	suite.NoError(err)

	otherTeam := &types.Team{
		ID:             system.GenerateTeamID(),
		Name:           "Other Team",
		OrganizationID: otherOrg.ID,
	}
	_, err = suite.db.CreateTeam(suite.ctx, otherTeam)
	suite.NoError(err)

	// Verify filtering works
	firstOrgTeams, err := suite.db.ListTeams(suite.ctx, &ListTeamsQuery{OrganizationID: suite.org.ID})
	suite.NoError(err)
	suite.Equal(len(teamsToCreate), len(firstOrgTeams))

	secondOrgTeams, err := suite.db.ListTeams(suite.ctx, &ListTeamsQuery{OrganizationID: otherOrg.ID})
	suite.NoError(err)
	suite.Equal(1, len(secondOrgTeams))

	// Cleanup
	suite.T().Cleanup(func() {
		for _, team := range teamsToCreate {
			err := suite.db.DeleteTeam(suite.ctx, team.ID)
			suite.NoError(err)
		}
		err := suite.db.DeleteTeam(suite.ctx, otherTeam.ID)
		suite.NoError(err)
		err = suite.db.DeleteOrganization(suite.ctx, otherOrg.ID)
		suite.NoError(err)
	})
}

func (suite *TeamsTestSuite) TestListTeams_ByUser() {
	// Create a test user
	userID := system.GenerateUserID()
	user := &types.User{
		ID:    userID,
		Email: "test-user@example.com",
	}
	_, err := suite.db.CreateUser(suite.ctx, user)
	suite.Require().NoError(err)

	// Create multiple test teams
	teamsToCreate := []*types.Team{
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 1",
			OrganizationID: suite.org.ID,
		},
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 2",
			OrganizationID: suite.org.ID,
		},
		{
			ID:             system.GenerateTeamID(),
			Name:           "Test Team 3",
			OrganizationID: suite.org.ID,
		},
	}

	for _, team := range teamsToCreate {
		_, err := suite.db.CreateTeam(suite.ctx, team)
		suite.Require().NoError(err)
	}

	// Add user to the second team
	_, err = suite.db.CreateTeamMembership(suite.ctx, &types.TeamMembership{
		TeamID:         teamsToCreate[1].ID,
		UserID:         userID,
		OrganizationID: suite.org.ID,
	})
	suite.NoError(err)

	// Test listing all teams for the organization
	orgTeams, err := suite.db.ListTeams(suite.ctx, &ListTeamsQuery{
		OrganizationID: suite.org.ID,
		UserID:         userID,
	})
	suite.NoError(err)
	suite.Equal(1, len(orgTeams))
	suite.Equal(teamsToCreate[1].ID, orgTeams[0].ID)

	// Cleanup
	suite.T().Cleanup(func() {
		for _, team := range teamsToCreate {
			err := suite.db.DeleteTeam(suite.ctx, team.ID)
			suite.NoError(err)
		}

		suite.NoError(err)
		err = suite.db.DeleteOrganization(suite.ctx, suite.org.ID)
		suite.NoError(err)
	})
}
