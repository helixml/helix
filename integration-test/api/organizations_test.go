package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsTestSuite))
}

type OrganizationsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *store.PostgresStore
}

func (suite *OrganizationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	store, err := getStoreClient()
	suite.Require().NoError(err)
	suite.db = store
}

func (suite *OrganizationsTestSuite) TestCreateOrganization() {
	// Create a user
	userEmail := fmt.Sprintf("test-create-org-%s@test.com", uuid.New().String())

	user, apiKey, err := createUser(suite.db, userEmail)
	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Require().NotNil(apiKey)

	client, err := getApiClient(apiKey)
	suite.Require().NoError(err)

	name := "test-org-" + time.Now().Format("20060102150405")

	org, err := client.CreateOrganization(suite.ctx, &types.Organization{
		Name: name,
	})
	suite.Require().NoError(err)
	suite.Require().NotNil(org)
	suite.Require().Equal(name, org.Name)

	suite.T().Cleanup(func() {
		err := client.DeleteOrganization(suite.ctx, org.ID)
		suite.Require().NoError(err)
	})
}
