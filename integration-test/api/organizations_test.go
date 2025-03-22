package api

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsTestSuite))
}

type OrganizationsTestSuite struct {
	suite.Suite
	ctx    context.Context
	db     *store.PostgresStore
	client *client.HelixClient
}

func (suite *OrganizationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store

	client, err := getApiClient()
	suite.Require().NoError(err)
	suite.client = client
}

func (suite *OrganizationsTestSuite) TestCreateOrganization() {
	name := "test-org-" + time.Now().Format("20060102150405")

	org, err := suite.client.CreateOrganization(suite.ctx, &types.Organization{
		Name: name,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(org)
	suite.Require().Equal(name, org.Name)

	suite.T().Cleanup(func() {
		err := suite.client.DeleteOrganization(suite.ctx, org.ID)
		suite.Require().NoError(err)
	})
}
