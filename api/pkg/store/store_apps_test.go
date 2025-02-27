package store

import (
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *PostgresStoreTestSuite) TestCreateApp() {
	ownerID := "test-" + system.GenerateUUID()

	app := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.NoError(err)
	suite.NotNil(createdApp)
	suite.Equal(app.Owner, createdApp.Owner)
	suite.Equal(app.OwnerType, createdApp.OwnerType)
	suite.NotEmpty(createdApp.ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestGetApp() {
	ownerID := "test-" + system.GenerateUUID()

	app := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.NoError(err)
	suite.NotNil(createdApp)
	suite.Equal(app.Owner, createdApp.Owner)
	suite.Equal(app.OwnerType, createdApp.OwnerType)
	suite.NotEmpty(createdApp.ID)

	// Now, getting the app
	fetchedApp, err := suite.db.GetApp(suite.ctx, createdApp.ID)
	suite.NoError(err)
	suite.NotNil(fetchedApp)
	suite.Equal(createdApp.ID, fetchedApp.ID)
	suite.Equal(createdApp.Owner, fetchedApp.Owner)
	suite.Equal(createdApp.OwnerType, fetchedApp.OwnerType)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestListApps() {
	ownerID := "test-" + system.GenerateUUID()

	app := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.NoError(err)
	suite.NotNil(createdApp)
	suite.Equal(app.Owner, createdApp.Owner)
	suite.Equal(app.OwnerType, createdApp.OwnerType)
	suite.NotEmpty(createdApp.ID)

	// Now, listing all apps for the owner
	apps, err := suite.db.ListApps(suite.ctx, &ListAppsQuery{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
	})
	suite.NoError(err)
	suite.Equal(1, len(apps))
	suite.Equal(createdApp.ID, apps[0].ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestListOrganizationApps() {
	ownerID := "test-" + system.GenerateUUID()
	orgID := "test-org-" + system.GenerateUUID()

	orgApp := &types.App{
		Owner:          ownerID,
		OwnerType:      types.OwnerTypeUser,
		Config:         types.AppConfig{},
		OrganizationID: orgID,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, orgApp)
	suite.NoError(err)
	suite.NotNil(createdApp)

	// Now, listing all apps for the owner
	apps, err := suite.db.ListApps(suite.ctx, &ListAppsQuery{
		OrganizationID: orgID,
	})
	suite.NoError(err)
	suite.Equal(1, len(apps))
	suite.Equal(createdApp.ID, apps[0].ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestListNonOrganizationApps() {
	ownerID := "test-" + system.GenerateUUID()
	orgID := "test-org-" + system.GenerateUUID()

	orgApp := &types.App{
		Owner:          ownerID,
		OwnerType:      types.OwnerTypeUser,
		Config:         types.AppConfig{},
		OrganizationID: orgID,
	}

	// Creating a non-org app
	nonOrgApp := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdOrgApp, err := suite.db.CreateApp(suite.ctx, orgApp)
	suite.NoError(err)
	suite.NotNil(createdOrgApp)
	suite.NotEmpty(createdOrgApp.ID)
	suite.Equal(orgID, createdOrgApp.OrganizationID)

	createdNonOrgApp, err := suite.db.CreateApp(suite.ctx, nonOrgApp)
	suite.NoError(err)

	// Now, listing all apps for the owner
	apps, err := suite.db.ListApps(suite.ctx, &ListAppsQuery{
		OrganizationID: orgID,
	})
	suite.NoError(err)
	suite.Equal(1, len(apps))
	suite.Equal(createdOrgApp.ID, apps[0].ID)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdOrgApp.ID)
		suite.NoError(err)

		err = suite.db.DeleteApp(suite.ctx, createdNonOrgApp.ID)
		suite.NoError(err)
	})
}

func (suite *PostgresStoreTestSuite) TestDeleteApp() {

	ownerID := "test-" + system.GenerateUUID()

	app := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.NoError(err)
	suite.NotEmpty(createdApp.ID)

	// Delete it
	err = suite.db.DeleteApp(suite.ctx, createdApp.ID)
	suite.NoError(err)

	// Now, listing all tools for the owner
	tools, err := suite.db.ListApps(suite.ctx, &ListAppsQuery{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
	})
	suite.NoError(err)
	suite.Equal(0, len(tools))
}

func (suite *PostgresStoreTestSuite) TestRectifyApp() {
	testCases := []struct {
		name          string
		app           *types.App
		validateAfter func(*testing.T, *types.App)
	}{
		{
			name: "convert tools to apis",
			app: &types.App{
				Owner:     "test-owner",
				OwnerType: types.OwnerTypeUser,
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{
							{
								ID:    "test-assistant",
								Name:  "Test Assistant",
								Model: "gpt-4",
								Tools: []*types.Tool{
									{
										Name:        "test-api",
										Description: "Test API",
										ToolType:    types.ToolTypeAPI,
										Config: types.ToolConfig{
											API: &types.ToolAPIConfig{
												URL:    "http://example.com/api",
												Schema: "openapi: 3.0.0\ninfo:\n  title: Test API\n  version: 1.0.0",
												Headers: map[string]string{
													"Authorization": "Bearer test",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			validateAfter: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				// Tools should be empty after rectification
				assert.Empty(t, assistant.Tools, "Tools should be empty after rectification")

				// APIs should contain the converted tool
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "test-api", api.Name)
				assert.Equal(t, "http://example.com/api", api.URL)
				assert.Equal(t, "Test API", api.Description)
			},
		},
		{
			name: "preserve existing apis",
			app: &types.App{
				Owner:     "test-owner",
				OwnerType: types.OwnerTypeUser,
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{
							{
								ID:    "test-assistant",
								Name:  "Test Assistant",
								Model: "gpt-4",
								APIs: []types.AssistantAPI{
									{
										Name:        "existing-api",
										Description: "Existing API",
										URL:         "http://example.com/existing",
										Schema:      "openapi: 3.0.0",
									},
								},
							},
						},
					},
				},
			},
			validateAfter: func(t *testing.T, app *types.App) {
				require.Len(t, app.Config.Helix.Assistants, 1)
				assistant := app.Config.Helix.Assistants[0]

				// Tools should be empty
				assert.Empty(t, assistant.Tools)

				// Existing API should be preserved
				require.Len(t, assistant.APIs, 1)
				api := assistant.APIs[0]
				assert.Equal(t, "existing-api", api.Name)
				assert.Equal(t, "http://example.com/existing", api.URL)
			},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Create the app
			createdApp, err := suite.db.CreateApp(suite.ctx, tc.app)
			suite.NoError(err)
			suite.NotNil(createdApp)

			// Validate the rectified app
			tc.validateAfter(suite.T(), createdApp)

			// Clean up
			suite.T().Cleanup(func() {
				err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
				suite.NoError(err)
			})
		})
	}
}
