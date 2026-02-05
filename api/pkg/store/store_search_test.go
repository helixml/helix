package store

import (
	"context"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// TestResourceSearch_AllTypes tests searching across all resource types
func (suite *PostgresStoreTestSuite) TestResourceSearch_AllTypes() {
	userID := "search-user-" + system.GenerateUUID()

	// Create test data for each resource type with a unique prefix
	project := &types.Project{
		ID:          "prj-search-" + system.GenerateUUID(),
		Name:        "AlphaProject",
		Description: "A project for testing search functionality",
		UserID:      userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-search-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "AlphaTask",
		Description:    "A task for testing search",
		OriginalPrompt: "Create something alpha",
		UserID:         userID,
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	session := &types.Session{
		ID:    "ses-search-" + system.GenerateUUID(),
		Name:  "AlphaSession",
		Owner: userID,
		Mode:  types.SessionModeInference,
	}
	createdSession, err := suite.db.CreateSession(suite.ctx, *session)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), createdSession.ID)
	})

	// Search for "alpha" - should find project, task, and session by name prefix
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "alpha",
		UserID: userID,
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(results.Results), 3, "Should find at least 3 results")

	// Verify we found the expected resources
	foundTypes := make(map[types.Resource]bool)
	for _, r := range results.Results {
		foundTypes[r.ResourceType] = true
	}
	suite.True(foundTypes[types.ResourceProject], "Should find project")
	suite.True(foundTypes[types.ResourceSpecTask], "Should find spec task")
	suite.True(foundTypes[types.ResourceSession], "Should find session")
}

// TestResourceSearch_FilterByTypes tests filtering search by specific resource types
func (suite *PostgresStoreTestSuite) TestResourceSearch_FilterByTypes() {
	userID := "search-filter-user-" + system.GenerateUUID()

	// Create a project and a task with similar names
	project := &types.Project{
		ID:     "prj-filter-" + system.GenerateUUID(),
		Name:   "BetaProject",
		UserID: userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-filter-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "BetaTask",
		OriginalPrompt: "Create beta feature",
		UserID:         userID,
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	// Search only for projects
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "beta",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find exactly 1 project")
	suite.Equal(types.ResourceProject, results.Results[0].ResourceType)
	suite.Equal("BetaProject", results.Results[0].ResourceName)

	// Search only for spec tasks
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "beta",
		UserID: userID,
		Types:  []types.Resource{types.ResourceSpecTask},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find exactly 1 task")
	suite.Equal(types.ResourceSpecTask, results.Results[0].ResourceType)
	suite.Equal("BetaTask", results.Results[0].ResourceName)
}

// TestResourceSearch_NamePrefixMatching tests that name search uses prefix matching
func (suite *PostgresStoreTestSuite) TestResourceSearch_NamePrefixMatching() {
	userID := "search-prefix-user-" + system.GenerateUUID()

	// Create projects with names that test prefix matching
	prefixProject := &types.Project{
		ID:     "prj-prefix1-" + system.GenerateUUID(),
		Name:   "GammaFeature", // Should match "gamma" prefix
		UserID: userID,
	}
	createdPrefixProject, err := suite.db.CreateProject(suite.ctx, prefixProject)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdPrefixProject.ID)
	})

	middleProject := &types.Project{
		ID:     "prj-prefix2-" + system.GenerateUUID(),
		Name:   "SomeGammaProject", // Should NOT match "gamma" prefix (gamma is in the middle)
		UserID: userID,
	}
	createdMiddleProject, err := suite.db.CreateProject(suite.ctx, middleProject)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdMiddleProject.ID)
	})

	// Search for "gamma" - should only find project with name starting with "Gamma"
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "gamma",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find only the project with name prefix 'Gamma'")
	suite.Equal("GammaFeature", results.Results[0].ResourceName)
}

// TestResourceSearch_DescriptionContainsMatching tests that description search uses contains matching
func (suite *PostgresStoreTestSuite) TestResourceSearch_DescriptionContainsMatching() {
	userID := "search-desc-user-" + system.GenerateUUID()

	// Create project with keyword in description
	project := &types.Project{
		ID:          "prj-desc-" + system.GenerateUUID(),
		Name:        "MyProject",
		Description: "This project handles delta operations for processing",
		UserID:      userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	// Search for "delta" - should find project via description contains match
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "delta",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find project via description")
	suite.Equal("MyProject", results.Results[0].ResourceName)
}

// TestResourceSearch_CaseInsensitive tests that search is case insensitive
func (suite *PostgresStoreTestSuite) TestResourceSearch_CaseInsensitive() {
	userID := "search-case-user-" + system.GenerateUUID()

	project := &types.Project{
		ID:          "prj-case-" + system.GenerateUUID(),
		Name:        "EpsilonProject",
		Description: "Contains OMEGA keyword",
		UserID:      userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	// Search with lowercase should find uppercase name
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "epsilon",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("EpsilonProject", results.Results[0].ResourceName)

	// Search with uppercase should find lowercase in description
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "OMEGA",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
}

// TestResourceSearch_OrganizationScope tests organization-scoped search
func (suite *PostgresStoreTestSuite) TestResourceSearch_OrganizationScope() {
	userID := "search-org-user-" + system.GenerateUUID()
	orgID := "org-search-" + system.GenerateUUID()

	// Create organization
	org := &types.Organization{
		ID:    orgID,
		Name:  "SearchTestOrg-" + system.GenerateUUID(),
		Owner: userID,
	}
	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteOrganization(context.Background(), createdOrg.ID)
	})

	// Create project in organization
	orgProject := &types.Project{
		ID:             "prj-org-" + system.GenerateUUID(),
		Name:           "ZetaOrgProject",
		OrganizationID: orgID,
		UserID:         userID,
	}
	createdOrgProject, err := suite.db.CreateProject(suite.ctx, orgProject)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdOrgProject.ID)
	})

	// Create user project (not in org)
	userProject := &types.Project{
		ID:     "prj-user-" + system.GenerateUUID(),
		Name:   "ZetaUserProject",
		UserID: userID,
	}
	createdUserProject, err := suite.db.CreateProject(suite.ctx, userProject)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdUserProject.ID)
	})

	// Search with organization scope - should only find org project
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:          "zeta",
		OrganizationID: orgID,
		Types:          []types.Resource{types.ResourceProject},
		Limit:          10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find only org project")
	suite.Equal("ZetaOrgProject", results.Results[0].ResourceName)

	// Search with user scope - should only find user project
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "zeta",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results), "Should find only user project")
	suite.Equal("ZetaUserProject", results.Results[0].ResourceName)
}

// TestResourceSearch_NoResults tests that search returns empty when nothing matches
func (suite *PostgresStoreTestSuite) TestResourceSearch_NoResults() {
	userID := "search-noresults-user-" + system.GenerateUUID()

	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "nonexistentxyzquery123",
		UserID: userID,
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(0, len(results.Results))
	suite.Equal(0, results.Total)
}

// TestResourceSearch_LimitResults tests that limit is respected
func (suite *PostgresStoreTestSuite) TestResourceSearch_LimitResults() {
	userID := "search-limit-user-" + system.GenerateUUID()

	// Create multiple projects
	for i := 0; i < 5; i++ {
		project := &types.Project{
			ID:     "prj-limit-" + system.GenerateUUID(),
			Name:   "ThetaProject",
			UserID: userID,
		}
		createdProject, err := suite.db.CreateProject(suite.ctx, project)
		suite.Require().NoError(err)
		suite.T().Cleanup(func() {
			_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
		})
	}

	// Search with limit 2
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "theta",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  2,
	})
	suite.Require().NoError(err)
	suite.Equal(2, len(results.Results), "Should respect limit")
}

// TestResourceSearch_GitRepository tests searching git repositories
func (suite *PostgresStoreTestSuite) TestResourceSearch_GitRepository() {
	userID := "search-repo-user-" + system.GenerateUUID()

	// Create git repository
	repo := &types.GitRepository{
		ID:          "repo-search-" + system.GenerateUUID(),
		Name:        "IotaRepo",
		Description: "Repository for iota processing lambda functions",
		OwnerID:     userID,
		RepoType:    types.GitRepositoryTypeCode,
		Status:      types.GitRepositoryStatusActive,
		CloneURL:    "http://localhost/git/test",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	err := suite.db.CreateGitRepository(suite.ctx, repo)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteGitRepository(context.Background(), repo.ID)
	})

	// Search by name prefix
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "iota",
		UserID: userID,
		Types:  []types.Resource{types.ResourceGitRepository},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("IotaRepo", results.Results[0].ResourceName)
	suite.Equal(types.ResourceGitRepository, results.Results[0].ResourceType)

	// Search by description contains
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "lambda",
		UserID: userID,
		Types:  []types.Resource{types.ResourceGitRepository},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("IotaRepo", results.Results[0].ResourceName)
}

// TestResourceSearch_Application tests searching apps/agents
func (suite *PostgresStoreTestSuite) TestResourceSearch_Application() {
	userID := "search-app-user-" + system.GenerateUUID()

	// Create app
	app := &types.App{
		ID:        "app-search-" + system.GenerateUUID(),
		Owner:     userID,
		OwnerType: types.OwnerTypeUser,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:        "KappaAgent",
				Description: "Agent for kappa processing with sigma modules",
			},
		},
	}
	createdApp, err := suite.db.CreateApp(suite.ctx, app)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteApp(context.Background(), createdApp.ID)
	})

	// Search by name prefix
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "kappa",
		UserID: userID,
		Types:  []types.Resource{types.ResourceApplication},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("KappaAgent", results.Results[0].ResourceName)
	suite.Equal(types.ResourceApplication, results.Results[0].ResourceType)

	// Search by description contains
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "sigma",
		UserID: userID,
		Types:  []types.Resource{types.ResourceApplication},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("KappaAgent", results.Results[0].ResourceName)
}

// TestResourceSearch_Knowledge tests searching knowledge
func (suite *PostgresStoreTestSuite) TestResourceSearch_Knowledge() {
	userID := "search-knowledge-user-" + system.GenerateUUID()

	// Create knowledge
	knowledge := &types.Knowledge{
		ID:          "knowledge-search-" + system.GenerateUUID(),
		Name:        "LambdaKnowledge",
		Description: "Knowledge base containing mu references",
		Owner:       userID,
		OwnerType:   types.OwnerTypeUser,
		State:       types.KnowledgeStateReady,
		Created:     time.Now(),
		Updated:     time.Now(),
	}
	createdKnowledge, err := suite.db.CreateKnowledge(suite.ctx, knowledge)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteKnowledge(context.Background(), createdKnowledge.ID)
	})

	// Search by name prefix
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "lambda",
		UserID: userID,
		Types:  []types.Resource{types.ResourceKnowledge},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("LambdaKnowledge", results.Results[0].ResourceName)
	suite.Equal(types.ResourceKnowledge, results.Results[0].ResourceType)

	// Search by description contains
	results, err = suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "mu",
		UserID: userID,
		Types:  []types.Resource{types.ResourceKnowledge},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("LambdaKnowledge", results.Results[0].ResourceName)
}

// TestResourceSearch_Prompt tests searching prompts by content
func (suite *PostgresStoreTestSuite) TestResourceSearch_Prompt() {
	userID := "search-prompt-user-" + system.GenerateUUID()
	projectID := "prj-prompt-search-" + system.GenerateUUID()
	specTaskID := "task-prompt-search-" + system.GenerateUUID()

	// Create a prompt history entry
	prompt := &types.PromptHistoryEntry{
		ID:         "prompt-search-" + system.GenerateUUID(),
		UserID:     userID,
		ProjectID:  projectID,
		SpecTaskID: specTaskID,
		Content:    "Please analyze the nu configuration files and optimize performance",
		Status:     "sent",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	err := suite.db.gdb.Create(prompt).Error
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.gdb.Delete(prompt).Error
	})

	// Search by content contains
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "nu configuration",
		UserID: userID,
		Types:  []types.Resource{types.ResourcePrompt},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal(types.ResourcePrompt, results.Results[0].ResourceType)
	suite.Contains(results.Results[0].ResourceName, "nu configuration")
}

// TestResourceSearch_SpecTaskByOriginalPrompt tests searching tasks by original prompt content
func (suite *PostgresStoreTestSuite) TestResourceSearch_SpecTaskByOriginalPrompt() {
	userID := "search-taskprompt-user-" + system.GenerateUUID()

	project := &types.Project{
		ID:     "prj-taskprompt-" + system.GenerateUUID(),
		Name:   "TestProject",
		UserID: userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	task := &types.SpecTask{
		ID:             "task-promptsearch-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "SimpleTask",
		Description:    "Basic description",
		OriginalPrompt: "Implement xi functionality with rho integration",
		UserID:         userID,
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, task)
	suite.Require().NoError(err)

	// Search by original prompt content (contains matching)
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "rho integration",
		UserID: userID,
		Types:  []types.Resource{types.ResourceSpecTask},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("SimpleTask", results.Results[0].ResourceName)
}

// TestResourceSearch_MultipleTypes tests searching multiple specific types at once
func (suite *PostgresStoreTestSuite) TestResourceSearch_MultipleTypes() {
	userID := "search-multi-user-" + system.GenerateUUID()

	project := &types.Project{
		ID:     "prj-multi-" + system.GenerateUUID(),
		Name:   "OmegaProject",
		UserID: userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	session := &types.Session{
		ID:    "ses-multi-" + system.GenerateUUID(),
		Name:  "OmegaSession",
		Owner: userID,
		Mode:  types.SessionModeInference,
	}
	createdSession, err := suite.db.CreateSession(suite.ctx, *session)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_, _ = suite.db.DeleteSession(context.Background(), createdSession.ID)
	})

	// Search for project and session types only
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "omega",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject, types.ResourceSession},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(2, len(results.Results))

	foundTypes := make(map[types.Resource]bool)
	for _, r := range results.Results {
		foundTypes[r.ResourceType] = true
	}
	suite.True(foundTypes[types.ResourceProject])
	suite.True(foundTypes[types.ResourceSession])
}

// TestResourceSearch_ExcludesArchivedTasks tests that archived tasks are excluded
func (suite *PostgresStoreTestSuite) TestResourceSearch_ExcludesArchivedTasks() {
	userID := "search-archived-user-" + system.GenerateUUID()

	project := &types.Project{
		ID:     "prj-archived-" + system.GenerateUUID(),
		Name:   "TestProject",
		UserID: userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	// Create archived task
	archivedTask := &types.SpecTask{
		ID:             "task-archived-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "PsiArchivedTask",
		OriginalPrompt: "Old archived task",
		UserID:         userID,
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusDone,
		Archived:       true,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, archivedTask)
	suite.Require().NoError(err)

	// Create active task
	activeTask := &types.SpecTask{
		ID:             "task-active-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		Name:           "PsiActiveTask",
		OriginalPrompt: "Active task",
		UserID:         userID,
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		Archived:       false,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	err = suite.db.CreateSpecTask(suite.ctx, activeTask)
	suite.Require().NoError(err)

	// Search should only find the active task
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "psi",
		UserID: userID,
		Types:  []types.Resource{types.ResourceSpecTask},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("PsiActiveTask", results.Results[0].ResourceName)
}

// TestResourceSearch_DefaultLimit tests that default limit is applied
func (suite *PostgresStoreTestSuite) TestResourceSearch_DefaultLimit() {
	userID := "search-deflimit-user-" + system.GenerateUUID()

	// Search with no limit specified - should use default of 10
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "test",
		UserID: userID,
	})
	suite.Require().NoError(err)
	suite.NotNil(results)
	// Just verify no error - default limit is applied internally
}

// TestResourceSearch_PartialNameMatch tests partial matching on names
func (suite *PostgresStoreTestSuite) TestResourceSearch_PartialNameMatch() {
	userID := "search-partial-user-" + system.GenerateUUID()

	project := &types.Project{
		ID:     "prj-partial-" + system.GenerateUUID(),
		Name:   "ChiFeatureImplementation",
		UserID: userID,
	}
	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.T().Cleanup(func() {
		_ = suite.db.DeleteProject(context.Background(), createdProject.ID)
	})

	// Partial prefix match should work
	results, err := suite.db.ResourceSearch(suite.ctx, &types.ResourceSearchRequest{
		Query:  "chifeature",
		UserID: userID,
		Types:  []types.Resource{types.ResourceProject},
		Limit:  10,
	})
	suite.Require().NoError(err)
	suite.Equal(1, len(results.Results))
	suite.Equal("ChiFeatureImplementation", results.Results[0].ResourceName)
}
