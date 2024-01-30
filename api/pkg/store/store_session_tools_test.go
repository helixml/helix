package store

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/system"
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

	// TODO: move server options to envconfig
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}

	store, err := NewPostgresStore(StoreOptions{
		Host:        host,
		Port:        5432,
		Username:    "postgres",
		Password:    "postgres",
		Database:    "postgres",
		AutoMigrate: true,
	})
	suite.NoError(err)

	suite.db = store
}

func (suite *PostgresStoreTestSuite) Test_ListSessionTools() {
	ownerID := "test-" + system.GenerateUUID()

	tool := &types.Tool{
		Name:      "test",
		Owner:     ownerID,
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

	sessionID := "session-test" + system.GenerateUUID()

	// Now, creating a session tool binding
	err = suite.db.CreateSessionToolBinding(suite.ctx, sessionID, createdTool.ID)
	suite.NoError(err)

	// Now, listing all tools for the session
	tools, err := suite.db.ListSessionTools(suite.ctx, sessionID)
	suite.NoError(err)

	suite.Equal(1, len(tools))
	suite.Equal(createdTool.ID, tools[0].ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTool(suite.ctx, createdTool.ID)
		suite.NoError(err)
	})
}
