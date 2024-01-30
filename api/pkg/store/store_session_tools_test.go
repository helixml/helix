package store

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestPostgresStoreSuite(t *testing.T) {
	suite.Run(t, new(PostgresStoreTestSuite))
}

type PostgresStoreTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *PostgresStoreTestSuite) SetupTest() {
	suite.ctx = context.Background()

	store, err := NewPostgresStore(StoreOptions{
		Host:     "localhost",
		Port:     5432,
		Username: "postgres",
		Password: "postgres",
		Database: "postgres",
	})
	suite.NoError(err)

	suite.db = store
}

func (suite *PostgresStoreTestSuite) TestCreateTool() {
	tool := &types.Tool{
		Name:      "test",
		Owner:     "test",
		OwnerType: types.OwnerTypeUser,
		Config: types.ToolConfig{
			API: &types.ToolApiConfig{
				URL: "http://test.com",
				Headers: map[string]string{
					"Authorization": "Bearer 123",
				},
				Schema: "123",
			},
		},
	}

	createdTool, err := suite.db.CreateTool(suite.ctx, tool)
	suite.NoError(err)
	suite.NotNil(createdTool)
	suite.Equal(tool.Name, createdTool.Name)
	suite.Equal(tool.Owner, createdTool.Owner)
	suite.Equal(tool.OwnerType, createdTool.OwnerType)
	suite.NotEmpty(createdTool.ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTool(suite.ctx, createdTool.ID)
		suite.NoError(err)
	})
}
