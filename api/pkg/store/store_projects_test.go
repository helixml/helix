package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_EmptyProject() {
	project := &types.Project{
		ID:     "proj-stats-empty-" + system.GenerateUUID(),
		Name:   "Empty Project",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(0, projects[0].Stats.TotalTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
	suite.Equal(0, projects[0].Stats.InProgressTasks)
	suite.Equal(0, projects[0].Stats.BacklogTasks)
	suite.Equal(0, projects[0].Stats.PendingReviewTasks)
	suite.Equal(float64(0), projects[0].Stats.AverageTaskTime)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_BasicCounts() {
	project := &types.Project{
		ID:     "proj-stats-basic-" + system.GenerateUUID(),
		Name:   "Project with Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	statuses := []types.SpecTaskStatus{
		types.TaskStatusBacklog,
		types.TaskStatusBacklog,
		types.TaskStatusSpecGeneration,
		types.TaskStatusImplementation,
		types.TaskStatusSpecReview,
		types.TaskStatusDone,
		types.TaskStatusDone,
	}

	for i, status := range statuses {
		task := &types.SpecTask{
			ID:             "task-stats-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Task " + string(rune('A'+i)),
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(7, projects[0].Stats.TotalTasks)
	suite.Equal(2, projects[0].Stats.CompletedTasks)
	suite.Equal(2, projects[0].Stats.InProgressTasks)
	suite.Equal(2, projects[0].Stats.BacklogTasks)
	suite.Equal(1, projects[0].Stats.PendingReviewTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_AllInProgressStatuses() {
	project := &types.Project{
		ID:     "proj-stats-inprog-" + system.GenerateUUID(),
		Name:   "Project with In Progress Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	inProgressStatuses := []types.SpecTaskStatus{
		types.TaskStatusSpecGeneration,
		types.TaskStatusImplementation,
		types.TaskStatusSpecRevision,
		types.TaskStatusQueuedImplementation,
		types.TaskStatusQueuedSpecGeneration,
	}

	for i, status := range inProgressStatuses {
		task := &types.SpecTask{
			ID:             "task-inprog-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "In Progress Task " + string(rune('A'+i)),
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(5, projects[0].Stats.TotalTasks)
	suite.Equal(5, projects[0].Stats.InProgressTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
	suite.Equal(0, projects[0].Stats.BacklogTasks)
	suite.Equal(0, projects[0].Stats.PendingReviewTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_AllReviewStatuses() {
	project := &types.Project{
		ID:     "proj-stats-review-" + system.GenerateUUID(),
		Name:   "Project with Review Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	reviewStatuses := []types.SpecTaskStatus{
		types.TaskStatusSpecReview,
		types.TaskStatusImplementationReview,
		types.TaskStatusPullRequest,
	}

	for i, status := range reviewStatuses {
		task := &types.SpecTask{
			ID:             "task-review-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Review Task " + string(rune('A'+i)),
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(3, projects[0].Stats.TotalTasks)
	suite.Equal(3, projects[0].Stats.PendingReviewTasks)
	suite.Equal(0, projects[0].Stats.InProgressTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
	suite.Equal(0, projects[0].Stats.BacklogTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_AverageTaskTime() {
	project := &types.Project{
		ID:     "proj-stats-avgtime-" + system.GenerateUUID(),
		Name:   "Project with Completed Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	now := time.Now()
	completedAt1 := now
	createdAt1 := now.Add(-2 * time.Hour)

	completedAt2 := now
	createdAt2 := now.Add(-4 * time.Hour)

	task1 := &types.SpecTask{
		ID:             "task-avgtime-1-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Completed Task 1",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      createdAt1,
		UpdatedAt:      now,
		CompletedAt:    &completedAt1,
	}
	err = suite.db.CreateSpecTask(suite.ctx, task1)
	suite.Require().NoError(err)

	task2 := &types.SpecTask{
		ID:             "task-avgtime-2-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Completed Task 2",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      createdAt2,
		UpdatedAt:      now,
		CompletedAt:    &completedAt2,
	}
	err = suite.db.CreateSpecTask(suite.ctx, task2)
	suite.Require().NoError(err)

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(2, projects[0].Stats.TotalTasks)
	suite.Equal(2, projects[0].Stats.CompletedTasks)
	suite.InDelta(3.0, projects[0].Stats.AverageTaskTime, 0.1)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_CompletedWithoutCompletedAt() {
	project := &types.Project{
		ID:     "proj-stats-notime-" + system.GenerateUUID(),
		Name:   "Project with Done Tasks without CompletedAt",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-notime-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Done Task without CompletedAt",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		CompletedAt:    nil,
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(1, projects[0].Stats.TotalTasks)
	suite.Equal(1, projects[0].Stats.CompletedTasks)
	suite.Equal(float64(0), projects[0].Stats.AverageTaskTime)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_ExcludesArchivedTasks() {
	project := &types.Project{
		ID:     "proj-stats-archived-" + system.GenerateUUID(),
		Name:   "Project with Archived Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	activeTask := &types.SpecTask{
		ID:             "task-active-" + system.GenerateUUID(),
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
	err = suite.db.CreateSpecTask(suite.ctx, activeTask)
	suite.Require().NoError(err)

	archivedTask := &types.SpecTask{
		ID:             "task-archived-" + system.GenerateUUID(),
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(1, projects[0].Stats.TotalTasks)
	suite.Equal(1, projects[0].Stats.BacklogTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_MultipleProjects() {
	userID := "test-user-multi-" + system.GenerateUUID()

	project1 := &types.Project{
		ID:     "proj-stats-multi1-" + system.GenerateUUID(),
		Name:   "Project 1",
		UserID: userID,
	}
	createdProject1, err := suite.db.CreateProject(suite.ctx, project1)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject1.ID)
	})

	project2 := &types.Project{
		ID:     "proj-stats-multi2-" + system.GenerateUUID(),
		Name:   "Project 2",
		UserID: userID,
	}
	createdProject2, err := suite.db.CreateProject(suite.ctx, project2)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject2.ID)
	})

	for i := 0; i < 3; i++ {
		task := &types.SpecTask{
			ID:             "task-p1-" + system.GenerateUUID(),
			ProjectID:      project1.ID,
			Name:           "Project 1 Task " + string(rune('A'+i)),
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
	}

	for i := 0; i < 5; i++ {
		task := &types.SpecTask{
			ID:             "task-p2-" + system.GenerateUUID(),
			ProjectID:      project2.ID,
			Name:           "Project 2 Task " + string(rune('A'+i)),
			Type:           "feature",
			Priority:       types.SpecTaskPriorityMedium,
			Status:         types.TaskStatusDone,
			OriginalPrompt: "Prompt",
			CreatedBy:      "test-user",
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		err := suite.db.CreateSpecTask(suite.ctx, task)
		suite.Require().NoError(err)
	}

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       userID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 2)

	statsMap := make(map[string]types.ProjectStats)
	for _, p := range projects {
		statsMap[p.ID] = p.Stats
	}

	suite.Equal(3, statsMap[project1.ID].TotalTasks)
	suite.Equal(3, statsMap[project1.ID].BacklogTasks)
	suite.Equal(0, statsMap[project1.ID].CompletedTasks)

	suite.Equal(5, statsMap[project2.ID].TotalTasks)
	suite.Equal(5, statsMap[project2.ID].CompletedTasks)
	suite.Equal(0, statsMap[project2.ID].BacklogTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithoutStats() {
	project := &types.Project{
		ID:     "proj-nostats-" + system.GenerateUUID(),
		Name:   "Project without Stats",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-nostats-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task",
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: false,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(0, projects[0].Stats.TotalTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_ErrorStatuses() {
	project := &types.Project{
		ID:     "proj-stats-errors-" + system.GenerateUUID(),
		Name:   "Project with Error Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	errorStatuses := []types.SpecTaskStatus{
		types.TaskStatusSpecFailed,
		types.TaskStatusImplementationFailed,
	}

	for i, status := range errorStatuses {
		task := &types.SpecTask{
			ID:             "task-error-" + system.GenerateUUID(),
			ProjectID:      project.ID,
			Name:           "Error Task " + string(rune('A'+i)),
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

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(2, projects[0].Stats.TotalTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
	suite.Equal(0, projects[0].Stats.InProgressTasks)
	suite.Equal(0, projects[0].Stats.BacklogTasks)
	suite.Equal(0, projects[0].Stats.PendingReviewTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_SpecApprovedStatus() {
	project := &types.Project{
		ID:     "proj-stats-approved-" + system.GenerateUUID(),
		Name:   "Project with Approved Tasks",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-approved-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Approved Task",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusSpecApproved,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(1, projects[0].Stats.TotalTasks)
	suite.Equal(0, projects[0].Stats.InProgressTasks)
	suite.Equal(0, projects[0].Stats.PendingReviewTasks)
	suite.Equal(0, projects[0].Stats.BacklogTasks)
	suite.Equal(0, projects[0].Stats.CompletedTasks)
}

func (suite *PostgresStoreTestSuite) TestListProjects_WithStats_ActiveAgentSessionsNotSet() {
	project := &types.Project{
		ID:     "proj-stats-sessions-" + system.GenerateUUID(),
		Name:   "Project for Session Test",
		UserID: "test-user-" + system.GenerateUUID(),
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-session-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "Task with Session",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusImplementation,
		OriginalPrompt: "Prompt",
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		UserID:       project.UserID,
		IncludeStats: true,
	})
	suite.Require().NoError(err)
	suite.Require().Len(projects, 1)

	suite.Equal(0, projects[0].Stats.ActiveAgentSessions)
}
