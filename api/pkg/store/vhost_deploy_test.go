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
