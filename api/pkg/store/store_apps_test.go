package store

import (
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
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
