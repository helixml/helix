package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsTestSuite))
}

type OrganizationsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *OrganizationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()
}

func (suite *OrganizationsTestSuite) TearDownTestSuite() {
	// No need to close the database connection here as it's managed by TestMain
}

func (suite *OrganizationsTestSuite) TestCreateOrganization() {
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)
	suite.Equal(org.ID, createdOrg.ID)
	suite.Equal(org.Name, createdOrg.Name)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestGetOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Test getting by ID
	fetchedOrg, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.NoError(err)
	suite.NotNil(fetchedOrg)
	suite.Equal(createdOrg.ID, fetchedOrg.ID)
	suite.Equal(createdOrg.Name, fetchedOrg.Name)

	// Test getting by Name
	fetchedByName, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{Name: createdOrg.Name})
	suite.NoError(err)
	suite.NotNil(fetchedByName)
	suite.Equal(createdOrg.ID, fetchedByName.ID)

	// Test getting non-existent organization
	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: "non-existent-id"})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestUpdateOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Update the organization
	updatedName := "Updated Organization " + id
	createdOrg.Name = updatedName
	updatedOrg, err := suite.db.UpdateOrganization(suite.ctx, createdOrg)
	suite.NoError(err)
	suite.NotNil(updatedOrg)
	suite.Equal(updatedName, updatedOrg.Name)

	// Verify the update
	fetchedOrg, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.NoError(err)
	suite.Equal(updatedName, fetchedOrg.Name)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestDeleteOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Delete the organization
	err = suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
	suite.NoError(err)

	// Verify the deletion
	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test deleting non-existent organization
	err = suite.db.DeleteOrganization(suite.ctx, "non-existent-id")
	suite.NoError(err) // Should not return error as delete is idempotent
}

func (suite *OrganizationsTestSuite) TestListOrganizations() {
	// Create multiple test organizations
	owner1 := "test-user-1"
	owner2 := "test-user-2"
	orgsToCreate := []*types.Organization{
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 1",
			Owner: owner1,
		},
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 2",
			Owner: owner1,
		},
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 3",
			Owner: owner2,
		},
	}

	for _, org := range orgsToCreate {
		_, err := suite.db.CreateOrganization(suite.ctx, org)
		suite.Require().NoError(err)
	}

	// Test listing all organizations
	allOrgs, err := suite.db.ListOrganizations(suite.ctx, nil)
	suite.NoError(err)
	suite.GreaterOrEqual(len(allOrgs), len(orgsToCreate))

	// Test listing by owner
	owner1Orgs, err := suite.db.ListOrganizations(suite.ctx, &ListOrganizationsQuery{Owner: owner1})
	suite.NoError(err)
	suite.Equal(2, len(owner1Orgs))
	for _, org := range owner1Orgs {
		suite.Equal(owner1, org.Owner)
	}

	// Cleanup
	suite.T().Cleanup(func() {
		for _, org := range orgsToCreate {
			err := suite.db.DeleteOrganization(suite.ctx, org.ID)
			suite.NoError(err)
		}
	})
}
