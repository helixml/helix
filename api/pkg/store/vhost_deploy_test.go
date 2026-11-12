package store

import (
	"sync"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestCreateWebServiceDeploySupersedesInFlightDeploys() {
	projectID := system.GenerateProjectID()
	suite.Require().NoError(suite.db.gdb.Create(&types.ProjectWebServiceState{
		ProjectID:     projectID,
		Enabled:       true,
		ContainerPort: 8080,
	}).Error)
	defer suite.db.gdb.Where("project_id = ?", projectID).Delete(&types.WebServiceDeploy{})
	defer suite.db.gdb.Delete(&types.ProjectWebServiceState{}, "project_id = ?", projectID)

	live := &types.WebServiceDeploy{ProjectID: projectID, Status: types.WebServiceDeployStatusLive}
	suite.Require().NoError(suite.db.CreateWebServiceDeploy(suite.ctx, live))

	deployA := &types.WebServiceDeploy{ProjectID: projectID, Status: types.WebServiceDeployStatusPending}
	deployB := &types.WebServiceDeploy{ProjectID: projectID, Status: types.WebServiceDeployStatusPending}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, deploy := range []*types.WebServiceDeploy{deployA, deployB} {
		wg.Add(1)
		go func(deploy *types.WebServiceDeploy) {
			defer wg.Done()
			<-start
			errs <- suite.db.CreateWebServiceDeploy(suite.ctx, deploy)
		}(deploy)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		suite.Require().NoError(err)
	}

	deploys, err := suite.db.ListWebServiceDeploys(suite.ctx, projectID, 3)
	suite.Require().NoError(err)
	suite.Require().Len(deploys, 3)

	statusCounts := map[types.WebServiceDeployStatus]int{}
	var pending *types.WebServiceDeploy
	for _, deploy := range deploys {
		statusCounts[deploy.Status]++
		if deploy.Status == types.WebServiceDeployStatusPending {
			pending = deploy
		}
	}
	suite.Equal(1, statusCounts[types.WebServiceDeployStatusPending])
	suite.Equal(1, statusCounts[types.WebServiceDeployStatusSuperseded])
	suite.Equal(1, statusCounts[types.WebServiceDeployStatusLive])
	suite.Require().NotNil(pending)
	suite.Equal(pending.ID, deploys[0].ID)
	for _, deploy := range deploys {
		if deploy.Status == types.WebServiceDeployStatusSuperseded {
			suite.NotNil(deploy.FinishedAt)
		}
	}
}

func (suite *PostgresStoreTestSuite) TestListActiveWebServicesSkipsDeletedProjects() {
	live := &types.Project{ID: system.GenerateProjectID(), Name: "live", UserID: "test-user-" + system.GenerateUUID()}
	deleted := &types.Project{ID: system.GenerateProjectID(), Name: "deleted", UserID: "test-user-" + system.GenerateUUID()}
	for _, p := range []*types.Project{live, deleted} {
		_, err := suite.db.CreateProject(suite.ctx, p)
		suite.Require().NoError(err)
		suite.Require().NoError(suite.db.gdb.Create(&types.ProjectWebServiceState{
			ProjectID:       p.ID,
			Enabled:         true,
			ContainerPort:   8080,
			ActiveSandboxID: "sbx-" + p.ID,
		}).Error)
		defer suite.db.gdb.Unscoped().Delete(&types.Project{}, "id = ?", p.ID)
		defer suite.db.gdb.Delete(&types.ProjectWebServiceState{}, "project_id = ?", p.ID)
	}
	// A pre-teardown delete: the project is soft-deleted but its web-service
	// state row is still enabled with an active sandbox.
	suite.Require().NoError(suite.db.DeleteProject(suite.ctx, deleted.ID))

	states, err := suite.db.ListActiveWebServices(suite.ctx)
	suite.Require().NoError(err)
	seen := map[string]bool{}
	for _, st := range states {
		seen[st.ProjectID] = true
	}
	suite.True(seen[live.ID], "live project's web service must be listed")
	suite.False(seen[deleted.ID], "deleted project's web service must not be listed")
}
