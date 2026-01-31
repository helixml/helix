package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsTestSuite))
}

type OrganizationsTestSuite struct {
	suite.Suite
	ctx           context.Context
	db            *store.PostgresStore
	authenticator auth.Authenticator
}

func (suite *OrganizationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store

	cfg := &config.ServerConfig{}
	authenticator, err := auth.NewHelixAuthenticator(cfg, suite.db, "test-secret", nil)
	suite.Require().NoError(err)

	suite.authenticator = authenticator
}

func (suite *OrganizationsTestSuite) TestCreateOrganization() {
	// Create a user
	emailID := uuid.New().String()
	userEmail := fmt.Sprintf("test-create-org-%s@test.com", emailID)

	user, apiKey, err := createUser(suite.T(), suite.db, suite.authenticator, userEmail)
	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Require().NotNil(apiKey)

	client, err := getAPIClient(apiKey)
	suite.Require().NoError(err)

	name := "test-org-" + time.Now().Format("20060102150405")

	org, err := client.CreateOrganization(suite.ctx, &types.Organization{
		Name: name,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(org)
	suite.Require().Equal(name, org.Name)

	// Get organization
	org, err = client.GetOrganization(suite.ctx, org.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(org)
	suite.Require().Equal(name, org.Name)

	// List organizations
	orgs, err := client.ListOrganizations(suite.ctx)
	suite.Require().NoError(err)
	suite.Require().NotNil(orgs)
	suite.Require().Greater(len(orgs), 0)

	// Find our organization in the list
	var foundOrg *types.Organization
	for _, o := range orgs {
		if o.ID == org.ID {
			foundOrg = o
		}
	}
	suite.Require().NotNil(foundOrg)
	suite.Require().Equal(org.ID, foundOrg.ID)
	suite.Require().Equal(org.Name, foundOrg.Name)

	// Update organization display name
	displayName := "test-org-display-name-" + time.Now().Format("20060102150405")
	org, err = client.UpdateOrganization(suite.ctx, org.ID, &types.Organization{
		DisplayName: displayName,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(org)

	// Get it again
	org, err = client.GetOrganization(suite.ctx, org.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(org)
	suite.Require().Equal(displayName, org.DisplayName)

	suite.T().Cleanup(func() {
		err := client.DeleteOrganization(suite.ctx, org.ID)
		suite.Require().NoError(err)
	})
}
