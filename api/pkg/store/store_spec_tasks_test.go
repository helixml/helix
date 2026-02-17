package store

import (
	"context"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) createTestProject() *types.Project {
	project := &types.Project{
		ID:     "proj-" + system.GenerateUUID(),
		Name:   "Test Project",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	return createdProject
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSpecTask() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Test Task",
		Description:    "Test Description",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Implement a new feature",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := suite.db.CreateSpecTask(suite.ctx, task)
	suite.NoError(err)

	retrieved, err := suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.NoError(err)
	suite.Equal(task.ID, retrieved.ID)
	suite.Equal(task.ProjectID, retrieved.ProjectID)
	suite.Equal(task.Name, retrieved.Name)
	suite.Equal(task.Status, retrieved.Status)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSpecTask_ValidationErrors() {
	err := suite.db.CreateSpecTask(suite.ctx, &types.SpecTask{
		ProjectID: "some-project",
	})
	suite.Error(err)
	suite.Contains(err.Error(), "task ID is required")

	err = suite.db.CreateSpecTask(suite.ctx, &types.SpecTask{
		ID: "task-" + system.GenerateUUID(),
	})
	suite.Error(err)
	suite.Contains(err.Error(), "project ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSpecTask() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Test Task for Get",
		Type:           "bug",
		Priority:       types.SpecTaskPriorityHigh,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Fix a bug",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	retrieved, err := suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.NoError(err)
	suite.Equal(task.ID, retrieved.ID)
	suite.Equal(task.Name, retrieved.Name)
	suite.Equal(task.Type, retrieved.Type)
	suite.Equal(task.Priority, retrieved.Priority)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSpecTask_NotFound() {
	_, err := suite.db.GetSpecTask(suite.ctx, "non-existent-task")
	suite.Error(err)
	suite.Contains(err.Error(), "spec task not found")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSpecTask_EmptyID() {
	_, err := suite.db.GetSpecTask(suite.ctx, "")
	suite.Error(err)
	suite.Contains(err.Error(), "task ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSpecTask() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Original Name",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityLow,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Original prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err := suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	task.Name = "Updated Name"
	task.Status = types.TaskStatusSpecGeneration
	task.Priority = types.SpecTaskPriorityHigh
	task.PlanningSessionID = "session-123"

	err = suite.db.UpdateSpecTask(suite.ctx, task)
	suite.NoError(err)

	retrieved, err := suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.NoError(err)
	suite.Equal("Updated Name", retrieved.Name)
	suite.Equal(types.TaskStatusSpecGeneration, retrieved.Status)
	suite.Equal(types.SpecTaskPriorityHigh, retrieved.Priority)
	suite.Equal("session-123", retrieved.PlanningSessionID)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSpecTask_EmptyID() {
	err := suite.db.UpdateSpecTask(suite.ctx, &types.SpecTask{})
	suite.Error(err)
	suite.Contains(err.Error(), "task ID is required")
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	for i := 0; i < 5; i++ {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task " + string(rune('A'+i)),
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
	})
	suite.NoError(err)
	suite.Len(tasks, 5)

	for i := 0; i < len(tasks)-1; i++ {
		suite.True(tasks[i].CreatedAt.After(tasks[i+1].CreatedAt) || tasks[i].CreatedAt.Equal(tasks[i+1].CreatedAt))
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByStatus() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	statuses := []types.SpecTaskStatus{
		types.TaskStatusBacklog,
		types.TaskStatusSpecGeneration,
		types.TaskStatusImplementation,
	}

	for _, status := range statuses {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task with status " + status.String(),
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         status,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		Status:    types.TaskStatusBacklog,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(types.TaskStatusBacklog, tasks[0].Status)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByUserID() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	userID1 := "user-" + system.GenerateUUID()
	userID2 := "user-" + system.GenerateUUID()

	task1 := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task by User 1",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      userID1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, task1)
	suite.Require().NoError(err)

	task2 := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task by User 2",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      userID2,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task2)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		UserID:    userID1,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(userID1, tasks[0].CreatedBy)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByPriority() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	priorities := []types.SpecTaskPriority{
		types.SpecTaskPriorityLow,
		types.SpecTaskPriorityMedium,
		types.SpecTaskPriorityHigh,
	}

	for _, priority := range priorities {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task with priority " + string(priority),
			Type:           "feature",
			Priority:       priority,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		Priority:  string(types.SpecTaskPriorityHigh),
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(types.SpecTaskPriorityHigh, tasks[0].Priority)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByType() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	taskTypes := []string{"feature", "bug", "refactor"}

	for _, taskType := range taskTypes {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task of type " + taskType,
			Type:           taskType,
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		Type:      "bug",
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal("bug", tasks[0].Type)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_LimitAndOffset() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	for i := 0; i < 10; i++ {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task " + string(rune('A'+i)),
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		Limit:     3,
	})
	suite.NoError(err)
	suite.Len(tasks, 3)

	tasks, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		Limit:     3,
		Offset:    3,
	})
	suite.NoError(err)
	suite.Len(tasks, 3)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_ArchivedFilter() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	activeTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Active Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		Archived:       false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, activeTask)
	suite.Require().NoError(err)

	archivedTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Archived Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		Archived:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, archivedTask)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(activeTask.ID, tasks[0].ID)

	tasks, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:    project.ID,
		ArchivedOnly: true,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(archivedTask.ID, tasks[0].ID)

	tasks, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:       project.ID,
		IncludeArchived: true,
	})
	suite.NoError(err)
	suite.Len(tasks, 2)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByBranchName() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	branchName := "feature/test-branch-" + system.GenerateUUID()

	taskWithBranch := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task with branch",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		BranchName:     branchName,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, taskWithBranch)
	suite.Require().NoError(err)

	taskWithoutBranch := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task without branch",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, taskWithoutBranch)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:  project.ID,
		BranchName: branchName,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(branchName, tasks[0].BranchName)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByDesignDocPath() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	designDocPath := "2025-01-17_test-feature_1"

	taskWithDesignDoc := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task with design doc",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		DesignDocPath:  designDocPath,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, taskWithDesignDoc)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		DesignDocPath: designDocPath,
	})
	suite.NoError(err)
	suite.Len(tasks, 1)
	suite.Equal(designDocPath, tasks[0].DesignDocPath)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_WithDependsOn() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	dep1 := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Dependency 1",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Dep prompt 1",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, dep1)
	suite.Require().NoError(err)

	dep2 := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Dependency 2",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Dep prompt 2",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, dep2)
	suite.Require().NoError(err)

	mainTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Main Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityHigh,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Main prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	err = suite.db.gdb.WithContext(suite.ctx).Model(mainTask).Association("DependsOn").Append(dep1, dep2)
	suite.Require().NoError(err)

	tasksWithoutDependsOn, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		WithDependsOn: false,
	})
	suite.Require().NoError(err)

	var foundWithoutDependsOn *types.SpecTask
	for _, task := range tasksWithoutDependsOn {
		if task.ID == mainTask.ID {
			foundWithoutDependsOn = task
			break
		}
	}
	suite.Require().NotNil(foundWithoutDependsOn)
	suite.Empty(foundWithoutDependsOn.DependsOn)

	tasksWithDependsOn, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		WithDependsOn: true,
	})
	suite.Require().NoError(err)

	var foundWithDependsOn *types.SpecTask
	for _, task := range tasksWithDependsOn {
		if task.ID == mainTask.ID {
			foundWithDependsOn = task
			break
		}
	}
	suite.Require().NotNil(foundWithDependsOn)
	suite.Len(foundWithDependsOn.DependsOn, 2)

	dependsOnIDs := map[string]bool{}
	for _, dep := range foundWithDependsOn.DependsOn {
		dependsOnIDs[dep.ID] = true
	}
	suite.True(dependsOnIDs[dep1.ID])
	suite.True(dependsOnIDs[dep2.ID])
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_WithDependsOn_DeletedDependencyExcluded() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	dependency := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Dependency",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Dependency prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, dependency)
	suite.Require().NoError(err)

	mainTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Main Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityHigh,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Main prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	err = suite.db.gdb.WithContext(suite.ctx).Model(mainTask).Association("DependsOn").Append(dependency)
	suite.Require().NoError(err)

	err = suite.db.DeleteSpecTask(suite.ctx, dependency.ID)
	suite.Require().NoError(err)

	tasksWithDependsOn, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		WithDependsOn: true,
	})
	suite.Require().NoError(err)

	var foundMainTask *types.SpecTask
	for _, task := range tasksWithDependsOn {
		if task.ID == mainTask.ID {
			foundMainTask = task
			break
		}
	}
	suite.Require().NotNil(foundMainTask)
	suite.Empty(foundMainTask.DependsOn)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_SubscribeForTasks() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	var receivedTask *types.SpecTask
	var mu sync.Mutex
	taskReceived := make(chan struct{}, 1)

	sub, err := suite.db.SubscribeForTasks(suite.ctx, &SpecTaskSubscriptionFilter{
		ProjectID: project.ID,
	}, func(task *types.SpecTask) error {
		mu.Lock()
		receivedTask = task
		mu.Unlock()
		select {
		case taskReceived <- struct{}{}:
		default:
		}
		return nil
	})
	suite.Require().NoError(err)
	defer sub.Unsubscribe()

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Subscribed Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	select {
	case <-taskReceived:
		mu.Lock()
		suite.Equal(task.ID, receivedTask.ID)
		suite.Equal(task.Name, receivedTask.Name)
		mu.Unlock()
	case <-time.After(5 * time.Second):
		suite.Fail("Timeout waiting for task subscription notification")
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_SubscribeForTasks_FilterByStatus() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	var receivedTasks []*types.SpecTask
	var mu sync.Mutex
	taskReceived := make(chan struct{}, 10)

	sub, err := suite.db.SubscribeForTasks(suite.ctx, &SpecTaskSubscriptionFilter{
		ProjectID: project.ID,
		Statuses:  []types.SpecTaskStatus{types.TaskStatusSpecGeneration},
	}, func(task *types.SpecTask) error {
		mu.Lock()
		receivedTasks = append(receivedTasks, task)
		mu.Unlock()
		select {
		case taskReceived <- struct{}{}:
		default:
		}
		return nil
	})
	suite.Require().NoError(err)
	defer sub.Unsubscribe()

	taskBacklog := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Backlog Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, taskBacklog)
	suite.Require().NoError(err)

	taskSpecGen := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Spec Generation Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusSpecGeneration,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, taskSpecGen)
	suite.Require().NoError(err)

	select {
	case <-taskReceived:
		mu.Lock()
		suite.Len(receivedTasks, 1)
		suite.Equal(taskSpecGen.ID, receivedTasks[0].ID)
		suite.Equal(types.TaskStatusSpecGeneration, receivedTasks[0].Status)
		mu.Unlock()
	case <-time.After(5 * time.Second):
		suite.Fail("Timeout waiting for task subscription notification")
	}
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_SubscribeForTasks_UpdateNotification() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	notifications := make(chan *types.SpecTask, 10)

	sub, err := suite.db.SubscribeForTasks(suite.ctx, &SpecTaskSubscriptionFilter{
		ProjectID: project.ID,
	}, func(task *types.SpecTask) error {
		notifications <- task
		return nil
	})
	suite.Require().NoError(err)
	defer sub.Unsubscribe()

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task to Update",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	select {
	case received := <-notifications:
		suite.Equal(task.ID, received.ID)
		suite.Equal(types.TaskStatusBacklog, received.Status)
	case <-time.After(5 * time.Second):
		suite.Fail("Timeout waiting for create notification")
	}

	task.Status = types.TaskStatusSpecGeneration
	err = suite.db.UpdateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	select {
	case received := <-notifications:
		suite.Equal(task.ID, received.ID)
		suite.Equal(types.TaskStatusSpecGeneration, received.Status)
	case <-time.After(5 * time.Second):
		suite.Fail("Timeout waiting for update notification")
	}
}

func (suite *PostgresStoreTestSuite) TestSpecTaskSubscriptionFilter_Matches() {
	task := &types.SpecTask{
		ID:        "task-1",
		ProjectID: "project-1",
		Status:    types.TaskStatusBacklog,
	}

	filter := &SpecTaskSubscriptionFilter{}
	suite.True(filter.Matches(task))

	filter = &SpecTaskSubscriptionFilter{
		Statuses: []types.SpecTaskStatus{types.TaskStatusBacklog},
	}
	suite.True(filter.Matches(task))

	filter = &SpecTaskSubscriptionFilter{
		Statuses: []types.SpecTaskStatus{types.TaskStatusBacklog, types.TaskStatusSpecGeneration},
	}
	suite.True(filter.Matches(task))

	filter = &SpecTaskSubscriptionFilter{
		Statuses: []types.SpecTaskStatus{types.TaskStatusSpecGeneration},
	}
	suite.False(filter.Matches(task))

	var nilFilter *SpecTaskSubscriptionFilter
	suite.True(nilFilter.Matches(task))
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSpecTaskZedThreadByZedThreadID() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	// Create a SpecTask
	specTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Test Task for ZedThread lookup",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusImplementation,
		OriginalPrompt: "Implement something",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, specTask)
	suite.Require().NoError(err)

	// Create a Helix session for the work session
	helixSession := types.Session{
		ID:      system.GenerateSessionID(),
		Name:    "Test Session",
		Owner:   "test-user",
		Type:    types.SessionTypeText,
		Mode:    types.SessionModeInference,
		Created: time.Now(),
		Updated: time.Now(),
	}
	_, err = suite.db.CreateSession(suite.ctx, helixSession)
	suite.Require().NoError(err)

	// Create a work session
	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     specTask.ID,
		HelixSessionID: helixSession.ID,
		Name:           "Work session for thread test",
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	err = suite.db.CreateSpecTaskWorkSession(suite.ctx, workSession)
	suite.Require().NoError(err)

	// Create a ZedThread with a known ZedThreadID
	zedThreadID := "acp-thread-" + system.GenerateUUID()
	now := time.Now()
	zedThread := &types.SpecTaskZedThread{
		WorkSessionID:  workSession.ID,
		SpecTaskID:     specTask.ID,
		ZedThreadID:    zedThreadID,
		Status:         types.SpecTaskZedStatusActive,
		LastActivityAt: &now,
	}
	err = suite.db.CreateSpecTaskZedThread(suite.ctx, zedThread)
	suite.Require().NoError(err)
	suite.NotEmpty(zedThread.ID)

	// Test: Look up by ZedThreadID (the ACP thread ID, not the DB primary key)
	found, err := suite.db.GetSpecTaskZedThreadByZedThreadID(suite.ctx, zedThreadID)
	suite.NoError(err)
	suite.Equal(zedThread.ID, found.ID)
	suite.Equal(zedThreadID, found.ZedThreadID)
	suite.Equal(workSession.ID, found.WorkSessionID)
	suite.Equal(specTask.ID, found.SpecTaskID)
	suite.Equal(types.SpecTaskZedStatusActive, found.Status)

	// Test: WorkSession preload works
	suite.NotNil(found.WorkSession)
	suite.Equal(workSession.ID, found.WorkSession.ID)

	// Test: Not found returns error
	_, err = suite.db.GetSpecTaskZedThreadByZedThreadID(suite.ctx, "non-existent-thread-id")
	suite.Error(err)
	suite.Contains(err.Error(), "spec task zed thread not found for zed thread ID")
}
