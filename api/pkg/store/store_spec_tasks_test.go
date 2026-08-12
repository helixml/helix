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

func (suite *PostgresStoreTestSuite) TestPostgresStore_GetSpecTask_WithDependsOn() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	dependency := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Dependency Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		OriginalPrompt: "Dependency prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err := suite.db.CreateSpecTask(suite.ctx, dependency)
	suite.Require().NoError(err)

	task := &types.SpecTask{
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
		DependsOn: []types.SpecTask{
			{ID: dependency.ID},
		},
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	retrieved, err := suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.Require().NoError(err)
	suite.Len(retrieved.DependsOn, 1)
	suite.Equal(dependency.ID, retrieved.DependsOn[0].ID)
	suite.Equal(types.TaskStatusDone, retrieved.DependsOn[0].Status)
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
	task.AgentSessionID = "session-123"

	err = suite.db.UpdateSpecTask(suite.ctx, task)
	suite.NoError(err)

	retrieved, err := suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.NoError(err)
	suite.Equal("Updated Name", retrieved.Name)
	suite.Equal(types.TaskStatusSpecGeneration, retrieved.Status)
	suite.Equal(types.SpecTaskPriorityHigh, retrieved.Priority)
	suite.Equal("session-123", retrieved.AgentSessionID)
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

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_FilterByParticipants() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	aliceID := "user-" + system.GenerateUUID()
	bobID := "user-" + system.GenerateUUID()
	charlieID := "user-" + system.GenerateUUID()
	tasks := []*types.SpecTask{
		{
			ID: "task-" + system.GenerateUUID(), ProjectID: project.ID,
			Name: "Unassigned created by Alice", CreatedBy: aliceID,
			CreatedAt: time.Now().Add(-4 * time.Minute),
		},
		{
			ID: "task-" + system.GenerateUUID(), ProjectID: project.ID,
			Name: "Assigned to Bob", CreatedBy: charlieID, AssigneeID: bobID,
			CreatedAt: time.Now().Add(-3 * time.Minute),
		},
		{
			ID: "task-" + system.GenerateUUID(), ProjectID: project.ID,
			Name: "Alice assigned to Bob", CreatedBy: aliceID, AssigneeID: bobID,
			CreatedAt: time.Now().Add(-2 * time.Minute),
		},
		{
			ID: "task-" + system.GenerateUUID(), ProjectID: project.ID,
			Name: "Assigned to Alice", CreatedBy: charlieID, AssigneeID: aliceID,
			CreatedAt: time.Now().Add(-time.Minute),
		},
		{
			ID: "task-" + system.GenerateUUID(), ProjectID: project.ID,
			Name: "Only Charlie", CreatedBy: charlieID,
			CreatedAt: time.Now(),
		},
	}
	for _, task := range tasks {
		suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))
	}

	filtered, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:          project.ID,
		FilterParticipants: true,
		ParticipantIDs:     []string{aliceID, bobID},
		SortBy:             "created",
	})
	suite.Require().NoError(err)
	suite.Require().Len(filtered, 3)
	suite.Equal([]string{"Assigned to Alice", "Alice assigned to Bob", "Assigned to Bob"}, []string{
		filtered[0].Name,
		filtered[1].Name,
		filtered[2].Name,
	})

	// Assignment is the execution owner. A creator must not see a task that
	// another person is assigned to, and unassigned work does not match.
	filtered, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:          project.ID,
		FilterParticipants: true,
		ParticipantIDs:     []string{aliceID},
		SortBy:             "created",
	})
	suite.Require().NoError(err)
	suite.Require().Len(filtered, 1)
	suite.Equal("Assigned to Alice", filtered[0].Name)

	filtered, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:          project.ID,
		FilterParticipants: true,
	})
	suite.Require().NoError(err)
	suite.Empty(filtered)
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

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_SortByCreated() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	olderCreatedAt := time.Now().Add(-2 * time.Hour)
	newerCreatedAt := time.Now().Add(-time.Hour)
	recentlyUpdatedAt := time.Now()
	for _, task := range []*types.SpecTask{
		{
			ID:              "task-" + system.GenerateUUID(),
			ProjectID:       project.ID,
			Name:            "Older created, recently updated",
			Status:          types.TaskStatusBacklog,
			StatusUpdatedAt: &recentlyUpdatedAt,
			CreatedAt:       olderCreatedAt,
		},
		{
			ID:              "task-" + system.GenerateUUID(),
			ProjectID:       project.ID,
			Name:            "Newer created",
			Status:          types.TaskStatusBacklog,
			StatusUpdatedAt: &newerCreatedAt,
			CreatedAt:       newerCreatedAt,
		},
	} {
		suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		SortBy:    "created",
	})
	suite.Require().NoError(err)
	suite.Require().Len(tasks, 2)
	suite.Equal("Newer created", tasks[0].Name)

	tasks, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{ProjectID: project.ID})
	suite.Require().NoError(err)
	suite.Equal("Older created, recently updated", tasks[0].Name)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_ListSpecTasks_SortByLastMessageBeforeLimit() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	now := time.Now().UTC().Truncate(time.Microsecond)
	oldMessageAt := now.Add(-2 * time.Hour)
	newMessageAt := now.Add(-time.Hour)
	oldSessionID := system.GenerateSessionID()
	newSessionID := system.GenerateSessionID()

	for _, session := range []types.Session{
		{ID: oldSessionID, ProjectID: project.ID, Owner: project.UserID, Created: now.Add(-4 * time.Hour)},
		{ID: newSessionID, ProjectID: project.ID, Owner: project.UserID, Created: now.Add(-3 * time.Hour)},
	} {
		_, err := suite.db.CreateSession(suite.ctx, session)
		suite.Require().NoError(err)
	}

	recentStatusAt := now
	oldStatusAt := now.Add(-3 * time.Hour)
	for _, task := range []*types.SpecTask{
		{
			ID:                "task-" + system.GenerateUUID(),
			ProjectID:         project.ID,
			Name:              "Recent status, old message",
			Status:            types.TaskStatusDone,
			StatusUpdatedAt:   &recentStatusAt,
			AgentSessionID: oldSessionID,
			CreatedAt:         now.Add(-4 * time.Hour),
		},
		{
			ID:                "task-" + system.GenerateUUID(),
			ProjectID:         project.ID,
			Name:              "Old status, new message",
			Status:            types.TaskStatusImplementation,
			StatusUpdatedAt:   &oldStatusAt,
			AgentSessionID: newSessionID,
			CreatedAt:         now.Add(-3 * time.Hour),
		},
		{
			ID:        "task-" + system.GenerateUUID(),
			ProjectID: project.ID,
			Name:      "No session",
			Status:    types.TaskStatusBacklog,
			CreatedAt: now.Add(-90 * time.Minute),
		},
	} {
		suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))
	}

	for _, interaction := range []*types.Interaction{
		{ID: system.GenerateInteractionID(), SessionID: oldSessionID, UserID: project.UserID, Created: oldMessageAt},
		{ID: system.GenerateInteractionID(), SessionID: newSessionID, UserID: project.UserID, Created: newMessageAt},
	} {
		_, err := suite.db.CreateInteraction(suite.ctx, interaction)
		suite.Require().NoError(err)
	}

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		SortBy:    "last_message",
		Limit:     1,
	})
	suite.Require().NoError(err)
	suite.Require().Len(tasks, 1)
	suite.Equal("Old status, new message", tasks[0].Name)
	suite.Require().NotNil(tasks[0].LastMessageAt)
	suite.WithinDuration(newMessageAt, *tasks[0].LastMessageAt, time.Microsecond)

	tasks, err = suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID: project.ID,
		SortBy:    "last_message",
	})
	suite.Require().NoError(err)
	suite.Equal([]string{"Old status, new message", "No session", "Recent status, old message"}, []string{
		tasks[0].Name,
		tasks[1].Name,
		tasks[2].Name,
	})
	suite.Require().NotNil(tasks[1].LastMessageAt)
	suite.WithinDuration(now.Add(-90*time.Minute), *tasks[1].LastMessageAt, time.Microsecond)
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

func (suite *PostgresStoreTestSuite) TestPostgresStore_CreateSpecTask_WithDependsOn() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	createTask := func(name string) *types.SpecTask {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           name,
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
		return task
	}

	dependencyOne := createTask("Dependency one")
	dependencyTwo := createTask("Dependency two")

	mainTask := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Main task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		DependsOn: []types.SpecTask{
			{ID: dependencyOne.ID},
			{ID: dependencyTwo.ID},
		},
	}

	err := suite.db.CreateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		WithDependsOn: true,
	})
	suite.Require().NoError(err)

	var foundMainTask *types.SpecTask
	for _, task := range tasks {
		if task.ID == mainTask.ID {
			foundMainTask = task
			break
		}
	}
	suite.Require().NotNil(foundMainTask)
	suite.Len(foundMainTask.DependsOn, 2)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSpecTask_WithDependsOn() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	createTask := func(name string) *types.SpecTask {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           name,
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
		return task
	}

	mainTask := createTask("Main task")
	dependencyOne := createTask("Dependency one")
	dependencyTwo := createTask("Dependency two")
	dependencyThree := createTask("Dependency three")

	mainTask.DependsOn = []types.SpecTask{{ID: dependencyOne.ID}, {ID: dependencyTwo.ID}}
	err := suite.db.UpdateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	mainTask.DependsOn = []types.SpecTask{{ID: dependencyThree.ID}}
	err = suite.db.UpdateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	mainTask.DependsOn = []types.SpecTask{}
	err = suite.db.UpdateSpecTask(suite.ctx, mainTask)
	suite.Require().NoError(err)

	tasks, err := suite.db.ListSpecTasks(suite.ctx, &types.SpecTaskFilters{
		ProjectID:     project.ID,
		WithDependsOn: true,
	})
	suite.Require().NoError(err)

	var foundMainTask *types.SpecTask
	for _, task := range tasks {
		if task.ID == mainTask.ID {
			foundMainTask = task
			break
		}
	}
	suite.Require().NotNil(foundMainTask)
	suite.Empty(foundMainTask.DependsOn)
}

func (suite *PostgresStoreTestSuite) TestPostgresStore_UpdateSpecTask_WithDependsOn_CircularDependencyRejected() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	createTask := func(name string) *types.SpecTask {
		task := &types.SpecTask{
			ID:             "task-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           name,
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusBacklog,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
		return task
	}

	taskA := createTask("Task A")
	taskB := createTask("Task B")

	taskB.DependsOn = []types.SpecTask{{ID: taskA.ID}}
	err := suite.db.UpdateSpecTask(suite.ctx, taskB)
	suite.Require().NoError(err)

	taskA.DependsOn = []types.SpecTask{{ID: taskB.ID}}
	err = suite.db.UpdateSpecTask(suite.ctx, taskA)
	suite.Error(err)
	suite.Contains(err.Error(), "circular dependency detected")
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

func (suite *PostgresStoreTestSuite) TestPostgresStore_DeleteSpecTaskRemovesThreadTracking() {
	project := suite.createTestProject()
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), project.ID)
	})

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task with an ACP handoff",
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: "Test task deletion after a model switch",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))

	helixSession := types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "test-user",
		Created: time.Now(),
		Updated: time.Now(),
	}
	_, err := suite.db.CreateSession(suite.ctx, helixSession)
	suite.Require().NoError(err)

	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     task.ID,
		HelixSessionID: helixSession.ID,
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	suite.Require().NoError(suite.db.CreateSpecTaskWorkSession(suite.ctx, workSession))
	suite.Require().NoError(suite.db.CreateSpecTaskZedThread(suite.ctx, &types.SpecTaskZedThread{
		WorkSessionID: workSession.ID,
		SpecTaskID:    task.ID,
		ZedThreadID:   "zed-thread-" + system.GenerateUUID(),
	}))

	suite.Require().NoError(suite.db.DeleteSpecTask(suite.ctx, task.ID))
	_, err = suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.Error(err)
	workSessions, err := suite.db.ListSpecTaskWorkSessions(suite.ctx, task.ID)
	suite.Require().NoError(err)
	suite.Empty(workSessions)
}
