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

	var storeCfg config.Store

	err := envconfig.Process("", &storeCfg)
	suite.NoError(err)

	store, err := NewPostgresStore(storeCfg)
	suite.Require().NoError(err)

	suite.db = store
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

}
