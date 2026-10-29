package store

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) queuedImplementationTask(projectID string) *types.SpecTask {
	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      projectID,
		Name:           "Capacity Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusQueuedImplementation,
		OriginalPrompt: "do it",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))
	return task
}

// Two API replicas racing for the last free sandbox slot: the advisory lock
// serializes them inside the claim, so exactly one promotes and the other is
// refused as capacity-full instead of overshooting the cap.
func (suite *PostgresStoreTestSuite) TestTransitionSpecTaskStatusWithinCapacity_LastSlotRace() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() { _ = suite.db.DeleteProject(context.Background(), project.ID) })

	from := []types.SpecTaskStatus{types.TaskStatusQueuedImplementation}
	// The admission count is global, so open the window now: only claims made
	// by this test fall inside it.
	since := time.Now()
	tasks := []*types.SpecTask{
		suite.queuedImplementationTask(project.ID),
		suite.queuedImplementationTask(project.ID),
		suite.queuedImplementationTask(project.ID),
		suite.queuedImplementationTask(project.ID),
	}

	type result struct{ claimed, full bool }
	results := make(chan result, len(tasks))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, task := range tasks {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			claimed, full, err := suite.db.TransitionSpecTaskStatusWithinCapacity(
				context.Background(), id, from, types.TaskStatusImplementation, since, 1,
			)
			suite.NoError(err)
			results <- result{claimed, full}
		}(task.ID)
	}
	close(start)
	wg.Wait()
	close(results)

	claimed, full := 0, 0
	for r := range results {
		if r.claimed {
			claimed++
		}
		if r.full {
			full++
		}
	}
	suite.Equal(1, claimed, "exactly one replica may take the last slot")
	suite.Equal(len(tasks)-1, full, "every other replica must be refused as capacity-full")
}

// Claims made before settleSince have already shown up in Runner heartbeats
// and are not counted again, and archived tasks never count.
func (suite *PostgresStoreTestSuite) TestTransitionSpecTaskStatusWithinCapacity_CountsOnlyRecentLiveClaims() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() { _ = suite.db.DeleteProject(context.Background(), project.ID) })
	from := []types.SpecTaskStatus{types.TaskStatusQueuedImplementation}
	// Open the window first so the archived claim falls inside it and only the
	// archived filter keeps it out of the count.
	since := time.Now()

	archived := suite.queuedImplementationTask(project.ID)
	ok, err := suite.db.TransitionSpecTaskStatus(suite.ctx, archived.ID, from, types.TaskStatusImplementation, map[string]any{"archived": true})
	suite.Require().NoError(err)
	suite.Require().True(ok)

	first := suite.queuedImplementationTask(project.ID)
	claimed, full, err := suite.db.TransitionSpecTaskStatusWithinCapacity(suite.ctx, first.ID, from, types.TaskStatusImplementation, since, 2)
	suite.Require().NoError(err)
	suite.True(claimed, "archived claim must not consume budget")
	suite.False(full)

	second := suite.queuedImplementationTask(project.ID)
	claimed, full, err = suite.db.TransitionSpecTaskStatusWithinCapacity(suite.ctx, second.ID, from, types.TaskStatusImplementation, since, 2)
	suite.Require().NoError(err)
	suite.True(claimed)
	suite.False(full)

	third := suite.queuedImplementationTask(project.ID)
	claimed, full, err = suite.db.TransitionSpecTaskStatusWithinCapacity(suite.ctx, third.ID, from, types.TaskStatusImplementation, since, 2)
	suite.Require().NoError(err)
	suite.False(claimed)
	suite.True(full, "two recent live claims exhaust a budget of 2")

	got, err := suite.db.GetSpecTask(suite.ctx, third.ID)
	suite.Require().NoError(err)
	suite.Equal(types.TaskStatusQueuedImplementation, got.Status, "a refused task stays queued")

	// A window that excludes those claims (they are now visible in
	// heartbeats) frees the budget again.
	claimed, full, err = suite.db.TransitionSpecTaskStatusWithinCapacity(suite.ctx, third.ID, from, types.TaskStatusImplementation, time.Now().Add(time.Second), 2)
	suite.Require().NoError(err)
	suite.True(claimed)
	suite.False(full)
}
