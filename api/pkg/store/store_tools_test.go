package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestCreateTool() {
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

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTool(suite.ctx, createdTool.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetTool() {
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

	// Now, getting the tool
	fetchedTool, err := suite.db.GetTool(suite.ctx, createdTool.ID)
	suite.NoError(err)
	suite.NotNil(fetchedTool)
	suite.Equal(createdTool.ID, fetchedTool.ID)
	suite.Equal(createdTool.Name, fetchedTool.Name)
	suite.Equal(createdTool.Owner, fetchedTool.Owner)
	suite.Equal(createdTool.OwnerType, fetchedTool.OwnerType)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTool(suite.ctx, createdTool.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestListTools() {
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

	// Now, listing all tools for the owner
	tools, err := suite.db.ListTools(suite.ctx, &ListToolsQuery{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
	})
	suite.NoError(err)
	suite.Equal(1, len(tools))
	suite.Equal(createdTool.ID, tools[0].ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteTool(suite.ctx, createdTool.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteTool() {

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
	suite.NotEmpty(createdTool.ID)

	// Delete it
	err = suite.db.DeleteTool(suite.ctx, createdTool.ID)
	suite.NoError(err)

	// Now, listing all tools for the owner
	tools, err := suite.db.ListTools(suite.ctx, &ListToolsQuery{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
	})
	suite.NoError(err)
	suite.Equal(0, len(tools))
}
