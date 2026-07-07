package store

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestOrganizationsTestSuite(t *testing.T) {
	suite.Run(t, new(OrganizationsTestSuite))
}

type OrganizationsTestSuite struct {
	suite.Suite
	ctx context.Context
	db  *PostgresStore
}

func (suite *OrganizationsTestSuite) SetupTest() {
	suite.ctx = context.Background()
	suite.db = GetTestDB()
}

func (suite *OrganizationsTestSuite) TearDownTestSuite() {
	// No need to close the database connection here as it's managed by TestMain
}

func (suite *OrganizationsTestSuite) TestCreateOrganization() {
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)
	suite.Equal(org.ID, createdOrg.ID)
	suite.Equal(org.Name, createdOrg.Name)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestGetOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Test getting by ID
	fetchedOrg, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.NoError(err)
	suite.NotNil(fetchedOrg)
	suite.Equal(createdOrg.ID, fetchedOrg.ID)
	suite.Equal(createdOrg.Name, fetchedOrg.Name)

	// Test getting by Name
	fetchedByName, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{Name: createdOrg.Name})
	suite.NoError(err)
	suite.NotNil(fetchedByName)
	suite.Equal(createdOrg.ID, fetchedByName.ID)

	// Test getting non-existent organization
	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: "non-existent-id"})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestUpdateOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Update the organization
	updatedName := "Updated Organization " + id
	createdOrg.Name = updatedName
	updatedOrg, err := suite.db.UpdateOrganization(suite.ctx, createdOrg)
	suite.NoError(err)
	suite.NotNil(updatedOrg)
	suite.Equal(updatedName, updatedOrg.Name)

	// Verify the update
	fetchedOrg, err := suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.NoError(err)
	suite.Equal(updatedName, fetchedOrg.Name)

	suite.T().Cleanup(func() {
		err := suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
		suite.NoError(err)
	})
}

func (suite *OrganizationsTestSuite) TestDeleteOrganization() {
	// Create a test organization first
	id := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    id,
		Name:  "Test Organization " + id,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.NotNil(createdOrg)

	// Delete the organization
	err = suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
	suite.NoError(err)

	// Verify the deletion
	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	// Test deleting non-existent organization
	err = suite.db.DeleteOrganization(suite.ctx, "non-existent-id")
	suite.NoError(err) // Should not return error as delete is idempotent
}

func (suite *OrganizationsTestSuite) TestDeleteOrganization_RemovesProjectsAndRepositories() {
	orgID := system.GenerateOrganizationID()
	org := &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	}

	createdOrg, err := suite.db.CreateOrganization(suite.ctx, org)
	suite.Require().NoError(err)
	suite.Require().NotNil(createdOrg)

	project := &types.Project{
		ID:             "project-" + system.GenerateUUID(),
		Name:           "Project for org delete test",
		UserID:         "test-user",
		OrganizationID: createdOrg.ID,
	}

	createdProject, err := suite.db.CreateProject(suite.ctx, project)
	suite.Require().NoError(err)
	suite.Require().NotNil(createdProject)

	repo := &types.GitRepository{
		ID:             "repo-" + system.GenerateUUID(),
		Name:           "Repo for org delete test",
		Description:    "Repo under org that should be deleted with organization",
		OwnerID:        "test-user",
		OrganizationID: createdOrg.ID,
		RepoType:       types.GitRepositoryTypeCode,
		Status:         types.GitRepositoryStatusActive,
		CloneURL:       "http://localhost/git/test",
	}

	err = suite.db.CreateGitRepository(suite.ctx, repo)
	suite.Require().NoError(err)

	err = suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
	suite.Require().NoError(err)

	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.Error(err)
	suite.Equal(ErrNotFound, err)

	projects, err := suite.db.ListProjects(suite.ctx, &ListProjectsQuery{
		OrganizationID: createdOrg.ID,
	})
	suite.Require().NoError(err)
	suite.Empty(projects)

	repositories, err := suite.db.ListGitRepositories(suite.ctx, &types.ListGitRepositoriesRequest{
		OrganizationID: createdOrg.ID,
	})
	suite.Require().NoError(err)
	suite.Empty(repositories)
}

// TestDeleteOrganization_RemovesSpecTaskChildren guards the FK-violation
// regression: a spec task with a work session (and zed thread) blocked org
// deletion with fk_spec_tasks_work_sessions because those children carried
// NO ACTION FKs and DeleteOrganization did not clear them first.
func (suite *OrganizationsTestSuite) TestDeleteOrganization_RemovesSpecTaskChildren() {
	orgID := system.GenerateOrganizationID()
	createdOrg, err := suite.db.CreateOrganization(suite.ctx, &types.Organization{
		ID:    orgID,
		Name:  "Test Organization " + orgID,
		Owner: "test-user",
	})
	suite.Require().NoError(err)

	project, err := suite.db.CreateProject(suite.ctx, &types.Project{
		ID:             "project-" + system.GenerateUUID(),
		Name:           "Project for spec-task child delete test",
		UserID:         "test-user",
		OrganizationID: createdOrg.ID,
	})
	suite.Require().NoError(err)

	session, err := suite.db.CreateSession(suite.ctx, types.Session{
		ID:      system.GenerateSessionID(),
		Owner:   "test-user",
		Created: time.Now(),
		Updated: time.Now(),
	})
	suite.Require().NoError(err)

	task := &types.SpecTask{
		ID:             "task-" + system.GenerateUUID(),
		ProjectID:      project.ID,
		OrganizationID: createdOrg.ID,
		Name:           "Task with children",
		Type:           "feature",
		Priority:       types.SpecTaskPriorityMedium,
		Status:         types.TaskStatusBacklog,
		CreatedBy:      "test-user",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	suite.Require().NoError(suite.db.CreateSpecTask(suite.ctx, task))

	workSession := &types.SpecTaskWorkSession{
		SpecTaskID:     task.ID,
		HelixSessionID: session.ID,
		Phase:          types.SpecTaskPhaseImplementation,
		Status:         types.SpecTaskWorkSessionStatusActive,
	}
	suite.Require().NoError(suite.db.CreateSpecTaskWorkSession(suite.ctx, workSession))

	suite.Require().NoError(suite.db.CreateSpecTaskZedThread(suite.ctx, &types.SpecTaskZedThread{
		WorkSessionID: workSession.ID,
		SpecTaskID:    task.ID,
		ZedThreadID:   "zed-thread-" + system.GenerateUUID(),
	}))

	// This previously failed with the fk_spec_tasks_work_sessions violation.
	err = suite.db.DeleteOrganization(suite.ctx, createdOrg.ID)
	suite.Require().NoError(err)

	_, err = suite.db.GetOrganization(suite.ctx, &GetOrganizationQuery{ID: createdOrg.ID})
	suite.Equal(ErrNotFound, err)

	_, err = suite.db.GetSpecTask(suite.ctx, task.ID)
	suite.Error(err)

	sessions, err := suite.db.ListSpecTaskWorkSessions(suite.ctx, task.ID)
	suite.Require().NoError(err)
	suite.Empty(sessions)
}

func (suite *OrganizationsTestSuite) TestListOrganizations() {
	// Create multiple test organizations
	owner1 := "test-user-1"
	owner2 := "test-user-2"
	orgsToCreate := []*types.Organization{
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 1",
			Owner: owner1,
		},
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 2",
			Owner: owner1,
		},
		{
			ID:    system.GenerateOrganizationID(),
			Name:  "Test Org 3",
			Owner: owner2,
		},
	}

	for _, org := range orgsToCreate {
		_, err := suite.db.CreateOrganization(suite.ctx, org)
		suite.Require().NoError(err)
	}

	// Test listing all organizations
	allOrgs, err := suite.db.ListOrganizations(suite.ctx, nil)
	suite.NoError(err)
	suite.GreaterOrEqual(len(allOrgs), len(orgsToCreate))

	// Test listing by owner
	owner1Orgs, err := suite.db.ListOrganizations(suite.ctx, &ListOrganizationsQuery{Owner: owner1})
	suite.NoError(err)
	suite.Equal(2, len(owner1Orgs))
	for _, org := range owner1Orgs {
		suite.Equal(owner1, org.Owner)
	}

	// Cleanup
	suite.T().Cleanup(func() {
		for _, org := range orgsToCreate {
			err := suite.db.DeleteOrganization(suite.ctx, org.ID)
			suite.NoError(err)
		}
	})
}
