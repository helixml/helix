package server

import (
	"context"
	"fmt"
	"net/http/httptest"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"go.uber.org/mock/gomock"
)

// recordingRepoDeleter records which repositories project deletion removed.
type recordingRepoDeleter struct {
	gitRepositoryServicer
	deleted []string
	err     error
}

func (r *recordingRepoDeleter) DeleteRepository(_ context.Context, repoID string) error {
	if r.err != nil {
		return r.err
	}
	r.deleted = append(r.deleted, repoID)
	return nil
}

func (s *DeleteProjectTasksSuite) expectEmptyProjectBeforeCleanup(project *types.Project) {
	s.sto.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), project.ID).Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.sto.EXPECT().ListArtifacts(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.sto.EXPECT().GetProjectWebServiceState(gomock.Any(), project.ID).Return(nil, store.ErrNotFound)
}

// Deleting a project leaves nothing of it reachable: repositories minted for
// it are deleted (unless another project still uses one), every repository
// link is removed, its access grants are deleted, and scheduled triggers that
// create work in it are disabled. A repository the user created separately
// and attached is only unlinked.
func (s *DeleteProjectTasksSuite) TestDeleteProjectReleasesProjectOwnedResources() {
	project := &types.Project{ID: "prj_del", UserID: s.userID, OrganizationID: "org_1"}
	repos := &recordingRepoDeleter{}
	s.server.gitRepositoryService = repos
	s.sto.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: s.userID, Role: types.OrganizationRoleOwner}, nil)
	s.expectEmptyProjectBeforeCleanup(project)

	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_del"}).Return([]*types.ProjectRepository{
		{ProjectID: "prj_del", RepositoryID: "prj_del-clone-demo-shape-star"}, // minted for this project
		{ProjectID: "prj_del", RepositoryID: "prj_del-shared"},                // minted here, also used elsewhere
		{ProjectID: "prj_del", RepositoryID: "repo_user_owned"},               // created separately, attached
	}, nil)
	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{RepositoryID: "prj_del-clone-demo-shape-star"}).
		Return([]*types.ProjectRepository{{ProjectID: "prj_del", RepositoryID: "prj_del-clone-demo-shape-star"}}, nil)
	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{RepositoryID: "prj_del-shared"}).
		Return([]*types.ProjectRepository{{ProjectID: "prj_del"}, {ProjectID: "prj_other"}}, nil)
	s.sto.EXPECT().DeleteProjectRepositoriesByProject(gomock.Any(), "prj_del").Return(nil)

	s.sto.EXPECT().ListAccessGrants(gomock.Any(), &store.ListAccessGrantsQuery{OrganizationID: "org_1", ResourceID: "prj_del"}).
		Return([]*types.AccessGrant{{ID: "ag_1"}, {ID: "ag_2"}}, nil)
	s.sto.EXPECT().DeleteAccessGrant(gomock.Any(), "ag_1").Return(nil)
	s.sto.EXPECT().DeleteAccessGrant(gomock.Any(), "ag_2").Return(nil)

	targeting := &types.TriggerConfiguration{ID: "trg_target", Enabled: true,
		Trigger: types.Trigger{Cron: &types.CronTrigger{Enabled: true, Action: "spec_task", ProjectID: "prj_del"}}}
	unrelated := &types.TriggerConfiguration{ID: "trg_other", Enabled: true,
		Trigger: types.Trigger{Cron: &types.CronTrigger{Enabled: true, Action: "spec_task", ProjectID: "prj_other"}}}
	s.sto.EXPECT().ListTriggerConfigurations(gomock.Any(), &store.ListTriggerConfigurationsQuery{
		OrganizationID: "org_1", TriggerType: types.TriggerTypeCron, Enabled: true,
	}).Return([]*types.TriggerConfiguration{targeting, unrelated}, nil)
	s.sto.EXPECT().UpdateTriggerConfiguration(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, tc *types.TriggerConfiguration) (*types.TriggerConfiguration, error) {
			s.Equal("trg_target", tc.ID, "only the trigger aimed at the deleted project changes")
			s.False(tc.Enabled)
			return tc, nil
		})

	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)
	s.Equal([]string{"prj_del-clone-demo-shape-star"}, repos.deleted,
		"only the repository minted for this project and used by no other project is deleted")
}

// A failure to release the project's resources aborts the delete, so a retry
// can finish the job instead of leaving the project gone and its rows behind.
func (s *DeleteProjectTasksSuite) TestDeleteProjectAbortsWhenRepositoryCleanupFails() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	s.server.gitRepositoryService = &recordingRepoDeleter{err: fmt.Errorf("disk on fire")}
	s.expectEmptyProjectBeforeCleanup(project)
	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_del"}).
		Return([]*types.ProjectRepository{{ProjectID: "prj_del", RepositoryID: "prj_del-code"}}, nil)
	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{RepositoryID: "prj_del-code"}).
		Return([]*types.ProjectRepository{{ProjectID: "prj_del"}}, nil)

	// No DeleteProject expectation: gomock fails the test if the project is
	// deleted after the cleanup failed.
	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Equal("failed to release project resources; retry the delete", httpErr.Message)
}
