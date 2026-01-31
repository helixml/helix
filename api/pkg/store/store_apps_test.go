package store

import (
	"testing"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func (suite *PostgresStoreTestSuite) TestCreateApp() {
	ownerID := "test-" + system.GenerateUUID()

	app := &types.App{
		Owner:     ownerID,
		OwnerType: types.OwnerTypeUser,
		Config:    types.AppConfig{},
	}

	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)
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

func TestFilterOutEmptyTriggers(t *testing.T) {
	tests := []struct {
		name     string
		triggers []types.Trigger
		expected int
	}{
		{
			name:     "empty slice",
			triggers: []types.Trigger{},
			expected: 0,
		},
		{
			name:     "nil slice",
			triggers: nil,
			expected: 0,
		},
		{
			name: "empty trigger is filtered out",
			triggers: []types.Trigger{
				{}, // Empty trigger
			},
			expected: 0,
		},
		{
			name: "cron trigger preserved",
			triggers: []types.Trigger{
				{
					Cron: &types.CronTrigger{
						Schedule: "0 * * * *",
					},
				},
			},
			expected: 1,
		},
		{
			name: "slack trigger preserved",
			triggers: []types.Trigger{
				{
					Slack: &types.SlackTrigger{
						Enabled: true,
					},
				},
			},
			expected: 1,
		},
		{
			name: "discord trigger preserved",
			triggers: []types.Trigger{
				{
					Discord: &types.DiscordTrigger{
						ServerName: "test-server",
					},
				},
			},
			expected: 1,
		},
		{
			name: "teams trigger preserved",
			triggers: []types.Trigger{
				{
					Teams: &types.TeamsTrigger{
						Enabled: true,
					},
				},
			},
			expected: 1,
		},
		{
			name: "teams trigger preserved even when disabled",
			triggers: []types.Trigger{
				{
					Teams: &types.TeamsTrigger{
						Enabled:     false,
						AppID:       "test-app-id",
						AppPassword: "test-password",
					},
				},
			},
			expected: 1,
		},
		{
			name: "azure devops trigger preserved",
			triggers: []types.Trigger{
				{
					AzureDevOps: &types.AzureDevOpsTrigger{
						Enabled: true,
					},
				},
			},
			expected: 1,
		},
		{
			name: "crisp trigger preserved",
			triggers: []types.Trigger{
				{
					Crisp: &types.CrispTrigger{
						Enabled: true,
					},
				},
			},
			expected: 1,
		},
		{
			name: "multiple triggers preserved",
			triggers: []types.Trigger{
				{
					Slack: &types.SlackTrigger{
						Enabled: true,
					},
				},
				{
					Teams: &types.TeamsTrigger{
						Enabled: true,
					},
				},
				{
					Discord: &types.DiscordTrigger{
						ServerName: "test-server",
					},
				},
			},
			expected: 3,
		},
		{
			name: "empty triggers filtered, valid preserved",
			triggers: []types.Trigger{
				{}, // Empty
				{
					Teams: &types.TeamsTrigger{
						Enabled: true,
					},
				},
				{}, // Empty
				{
					Slack: &types.SlackTrigger{
						Enabled: true,
					},
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterOutEmptyTriggers(tt.triggers)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestFilterOutEmptyTriggersPreservesTeamsConfig(t *testing.T) {
	// Specific test to verify Teams trigger configuration is preserved
	triggers := []types.Trigger{
		{
			Teams: &types.TeamsTrigger{
				Enabled:     true,
				AppID:       "my-app-id",
				AppPassword: "my-secret",
				TenantID:    "my-tenant",
			},
		},
	}

	result := filterOutEmptyTriggers(triggers)
	assert.Equal(t, 1, len(result))
	assert.NotNil(t, result[0].Teams)
	assert.Equal(t, "my-app-id", result[0].Teams.AppID)
	assert.Equal(t, "my-secret", result[0].Teams.AppPassword)
	assert.Equal(t, "my-tenant", result[0].Teams.TenantID)
	assert.True(t, result[0].Teams.Enabled)
}
