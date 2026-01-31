package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationCreate() {

	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	triggerConfig := &types.TriggerConfiguration{
		Name:      "Test Cron Trigger",
		Owner:     createdApp.Owner,
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Enabled:   true,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
				Input:    "test input",
			},
		},
	}

	createdTriggerConfig, err := suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.NoError(suite.T(), err)
	assert.NotEmpty(suite.T(), createdTriggerConfig.ID)
	assert.Equal(suite.T(), triggerConfig.Name, createdTriggerConfig.Name)
	assert.Equal(suite.T(), triggerConfig.Owner, createdTriggerConfig.Owner)
	assert.Equal(suite.T(), triggerConfig.OwnerType, createdTriggerConfig.OwnerType)
	assert.Equal(suite.T(), triggerConfig.AppID, createdTriggerConfig.AppID)
	assert.Equal(suite.T(), triggerConfig.Enabled, createdTriggerConfig.Enabled)
	assert.Equal(suite.T(), types.TriggerTypeCron, createdTriggerConfig.TriggerType)
	assert.NotZero(suite.T(), createdTriggerConfig.Created)
	assert.NotZero(suite.T(), createdTriggerConfig.Updated)
	assert.NotNil(suite.T(), createdTriggerConfig.Trigger.Cron)
	assert.Equal(suite.T(), triggerConfig.Trigger.Cron.Schedule, createdTriggerConfig.Trigger.Cron.Schedule)
	assert.Equal(suite.T(), triggerConfig.Trigger.Cron.Input, createdTriggerConfig.Trigger.Cron.Input)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteTriggerConfiguration(suite.ctx, createdTriggerConfig.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationCreateWithAzureDevOps() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	triggerConfig := &types.TriggerConfiguration{
		Name:      "Test Azure DevOps Trigger",
		Owner:     createdApp.Owner,
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Enabled:   true,
		Trigger: types.Trigger{
			AzureDevOps: &types.AzureDevOpsTrigger{
				Enabled: true,
			},
		},
	}

	createdTriggerConfig, err := suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), types.TriggerTypeAzureDevOps, createdTriggerConfig.TriggerType)
	assert.NotNil(suite.T(), createdTriggerConfig.Trigger.AzureDevOps)
	assert.Equal(suite.T(), triggerConfig.Trigger.AzureDevOps.Enabled, createdTriggerConfig.Trigger.AzureDevOps.Enabled)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteTriggerConfiguration(suite.ctx, createdTriggerConfig.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationCreateValidation() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	// Test missing owner
	triggerConfig := &types.TriggerConfiguration{
		Name:  "Test Trigger",
		AppID: createdApp.ID,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
			},
		},
	}

	_, err = suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "owner not specified")

	// Test missing name
	triggerConfig.Owner = "test-owner"
	triggerConfig.Name = ""
	_, err = suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "name not specified")

	// Test missing app_id
	triggerConfig.Name = "Test Trigger"
	triggerConfig.AppID = ""
	_, err = suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "app_id not specified")

	// Test missing trigger type
	triggerConfig.AppID = "test-app"
	triggerConfig.Trigger = types.Trigger{}
	_, err = suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "trigger type not specified")
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationUpdate() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	// Create initial trigger config
	triggerConfig := &types.TriggerConfiguration{
		Name:      "Original Trigger",
		Owner:     createdApp.Owner,
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Enabled:   true,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
				Input:    "original input",
			},
		},
	}

	createdTriggerConfig, err := suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.NoError(suite.T(), err)

	// Update the trigger config
	updatedTriggerConfig := &types.TriggerConfiguration{
		ID:        createdTriggerConfig.ID,
		Name:      "Updated Trigger",
		Owner:     createdTriggerConfig.Owner,
		OwnerType: createdTriggerConfig.OwnerType,
		AppID:     createdTriggerConfig.AppID,
		Enabled:   false,
		Archived:  true,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  false,
				Schedule: "0 12 * * *",
				Input:    "updated input",
			},
		},
	}

	updated, err := suite.db.UpdateTriggerConfiguration(suite.ctx, updatedTriggerConfig)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), updatedTriggerConfig.Name, updated.Name)
	assert.Equal(suite.T(), updatedTriggerConfig.Enabled, updated.Enabled)
	assert.Equal(suite.T(), updatedTriggerConfig.Archived, updated.Archived)
	assert.Equal(suite.T(), updatedTriggerConfig.Trigger.Cron.Schedule, updated.Trigger.Cron.Schedule)
	assert.Equal(suite.T(), updatedTriggerConfig.Trigger.Cron.Input, updated.Trigger.Cron.Input)
	assert.Equal(suite.T(), updatedTriggerConfig.Trigger.Cron.Enabled, updated.Trigger.Cron.Enabled)
	assert.NotZero(suite.T(), updated.Updated)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteTriggerConfiguration(suite.ctx, createdTriggerConfig.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationUpdateValidation() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	// Test missing ID
	triggerConfig := &types.TriggerConfiguration{
		Name:      "Test Trigger",
		Owner:     "test-owner",
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
			},
		},
	}

	_, err = suite.db.UpdateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "id not specified")

	// Test missing owner
	triggerConfig.ID = "test-id"
	triggerConfig.Owner = ""
	_, err = suite.db.UpdateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "owner not specified")

	// Test missing name
	triggerConfig.Owner = "test-owner"
	triggerConfig.Name = ""
	_, err = suite.db.UpdateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "name not specified")

	// Test missing app_id
	triggerConfig.Name = "Test Trigger"
	triggerConfig.AppID = ""
	_, err = suite.db.UpdateTriggerConfiguration(suite.ctx, triggerConfig)
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "app_id not specified")
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationGet() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	// Create a trigger config
	triggerConfig := &types.TriggerConfiguration{
		Name:      "Test Get Trigger",
		Owner:     createdApp.Owner,
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Enabled:   true,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
			},
		},
	}

	createdTriggerConfig, err := suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.NoError(suite.T(), err)

	// Get by ID
	fetchedTriggerConfig, err := suite.db.GetTriggerConfiguration(suite.ctx, &GetTriggerConfigurationQuery{
		ID: createdTriggerConfig.ID,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), createdTriggerConfig.ID, fetchedTriggerConfig.ID)
	assert.Equal(suite.T(), createdTriggerConfig.Name, fetchedTriggerConfig.Name)
	assert.Equal(suite.T(), createdTriggerConfig.Owner, fetchedTriggerConfig.Owner)

	// Get by owner
	fetchedByOwner, err := suite.db.GetTriggerConfiguration(suite.ctx, &GetTriggerConfigurationQuery{
		Owner: createdTriggerConfig.Owner,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), createdTriggerConfig.ID, fetchedByOwner.ID)

	// Get by owner and owner type
	fetchedByOwnerAndType, err := suite.db.GetTriggerConfiguration(suite.ctx, &GetTriggerConfigurationQuery{
		Owner:     createdTriggerConfig.Owner,
		OwnerType: createdTriggerConfig.OwnerType,
	})
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), createdTriggerConfig.ID, fetchedByOwnerAndType.ID)

	// Test not found
	_, err = suite.db.GetTriggerConfiguration(suite.ctx, &GetTriggerConfigurationQuery{
		ID: "non-existent-id",
	})
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrNotFound, err)

	// Clean up
	suite.T().Cleanup(func() {
		err := suite.db.DeleteTriggerConfiguration(suite.ctx, createdTriggerConfig.ID)
		assert.NoError(suite.T(), err)
	})
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationList() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})

	orgID := "test-org-" + system.GenerateUUID()

	// Create multiple trigger configs
	triggerConfigs := []*types.TriggerConfiguration{
		{
			Name:           "Cron Trigger 1",
			Owner:          createdApp.Owner,
			OwnerType:      types.OwnerTypeUser,
			AppID:          createdApp.ID,
			OrganizationID: orgID,
			Enabled:        true,
			Trigger: types.Trigger{
				Cron: &types.CronTrigger{
					Enabled:  true,
					Schedule: "0 0 * * *",
				},
			},
		},
		{
			Name:           "Slack Trigger 1",
			Owner:          createdApp.Owner,
			OwnerType:      types.OwnerTypeUser,
			AppID:          createdApp.ID,
			OrganizationID: orgID,
			Enabled:        false,
			Trigger: types.Trigger{
				Slack: &types.SlackTrigger{
					Enabled:  false,
					AppToken: "xapp-test-token",
					BotToken: "xoxb-test-bot-token",
					Channels: []string{"#general"},
				},
			},
		},
		{
			Name:           "Azure DevOps Trigger 1",
			Owner:          createdApp.Owner,
			OwnerType:      types.OwnerTypeUser,
			AppID:          createdApp.ID,
			OrganizationID: orgID,
			Enabled:        true,
			Trigger: types.Trigger{
				AzureDevOps: &types.AzureDevOpsTrigger{
					Enabled: true,
				},
			},
		},
		{
			Name:           "Cron Trigger 2",
			Owner:          createdApp.Owner,
			OwnerType:      types.OwnerTypeUser,
			AppID:          createdApp.ID,
			OrganizationID: orgID,
			Enabled:        true,
			Trigger: types.Trigger{
				Cron: &types.CronTrigger{
					Enabled:  true,
					Schedule: "0 12 * * *",
				},
			},
		},
	}

	for _, config := range triggerConfigs {
		created, err := suite.db.CreateTriggerConfiguration(suite.ctx, config)
		require.NoError(suite.T(), err)

		suite.T().Cleanup(func() {
			err := suite.db.DeleteTriggerConfiguration(suite.ctx, created.ID)
			assert.NoError(suite.T(), err)
		})
	}

	// Test listing by organization
	orgConfigs, err := suite.db.ListTriggerConfigurations(suite.ctx, &ListTriggerConfigurationsQuery{
		OrganizationID: createdApp.OrganizationID,
		AppID:          createdApp.ID,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), orgConfigs, 4, "should list all configs for the organization")

	// Test listing by trigger type
	cronConfigs, err := suite.db.ListTriggerConfigurations(suite.ctx, &ListTriggerConfigurationsQuery{
		TriggerType: types.TriggerTypeCron,
		AppID:       createdApp.ID,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), cronConfigs, 2, "should list all cron configs")

	slackConfigs, err := suite.db.ListTriggerConfigurations(suite.ctx, &ListTriggerConfigurationsQuery{
		TriggerType: types.TriggerTypeSlack,
		AppID:       createdApp.ID,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), slackConfigs, 1, "should list all slack configs")

	azureConfigs, err := suite.db.ListTriggerConfigurations(suite.ctx, &ListTriggerConfigurationsQuery{
		TriggerType: types.TriggerTypeAzureDevOps,
		AppID:       createdApp.ID,
	})
	require.NoError(suite.T(), err)
	assert.Len(suite.T(), azureConfigs, 1, "should list all azure devops configs")

	// Test listing enabled configs
	enabledConfigs, err := suite.db.ListTriggerConfigurations(suite.ctx, &ListTriggerConfigurationsQuery{
		Owner: createdApp.Owner,
		AppID: createdApp.ID,
	})
	require.NoError(suite.T(), err)
	enabledCount := 0
	for _, config := range enabledConfigs {
		if config.Enabled {
			enabledCount++
		}
	}
	assert.Equal(suite.T(), 3, enabledCount, "should have 3 enabled configs for the owner")
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationDelete() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})
	// Create a trigger config
	triggerConfig := &types.TriggerConfiguration{
		Name:      "Test Delete Trigger",
		Owner:     createdApp.Owner,
		OwnerType: types.OwnerTypeUser,
		AppID:     createdApp.ID,
		Enabled:   true,
		Trigger: types.Trigger{
			Cron: &types.CronTrigger{
				Enabled:  true,
				Schedule: "0 0 * * *",
			},
		},
	}

	createdTriggerConfig, err := suite.db.CreateTriggerConfiguration(suite.ctx, triggerConfig)
	require.NoError(suite.T(), err)

	// Delete the trigger config
	err = suite.db.DeleteTriggerConfiguration(suite.ctx, createdTriggerConfig.ID)
	require.NoError(suite.T(), err)

	// Verify the trigger config is deleted
	_, err = suite.db.GetTriggerConfiguration(suite.ctx, &GetTriggerConfigurationQuery{
		ID: createdTriggerConfig.ID,
	})
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), ErrNotFound, err)
}

func (suite *PostgresStoreTestSuite) TestTriggerConfigurationDeleteValidation() {
	app := &types.App{
		Owner:     "test-owner-" + system.GenerateUUID(),
		OwnerType: types.OwnerTypeUser,
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteApp(suite.ctx, createdApp.ID)
		suite.Require().NoError(err)
	})
	// Test missing ID
	err = suite.db.DeleteTriggerConfiguration(suite.ctx, "")
	require.Error(suite.T(), err)
	assert.Contains(suite.T(), err.Error(), "id not specified")
}
