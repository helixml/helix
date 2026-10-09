package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// deleteProject must delete the project's spec tasks (including archived
// ones). Tasks that outlive their project stay in whatever status they had
// forever — the orchestrator re-drives the active ones every tick and nothing
// reconciles them. 102 such orphans had accumulated in prod by 2026-10.
type DeleteProjectTasksSuite struct {
	suite.Suite

	ctrl     *gomock.Controller
	sto      *store.MockStore
	executor *external_agent.MockExecutor
	fs       *filestore.MockFileStore
	server   *HelixAPIServer
	userID   string
}

func TestDeleteProjectTasksSuite(t *testing.T) {
	suite.Run(t, new(DeleteProjectTasksSuite))
}

func (s *DeleteProjectTasksSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.sto = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.fs = filestore.NewMockFileStore(s.ctrl)
	s.userID = "user-delete-test"
	s.server = &HelixAPIServer{
		Cfg:                   &config.ServerConfig{},
		Store:                 s.sto,
		externalAgentExecutor: s.executor,
		Controller: &controller.Controller{Options: controller.Options{
			Config:    &config.ServerConfig{},
			Filestore: s.fs,
		}},
	}
}

func (s *DeleteProjectTasksSuite) request() *http.Request {
	ctx := setRequestUser(context.Background(), types.User{
		ID:    s.userID,
		Email: "delete@example.com",
	})
	r := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/prj_del", nil)
	return mux.SetURLVars(r.WithContext(ctx), map[string]string{"id": "prj_del"})
}

func (s *DeleteProjectTasksSuite) expectTaskDeleted(id string) {
	s.sto.EXPECT().DeleteSpecTaskAttachmentsByTaskID(gomock.Any(), id).Return(nil)
	s.fs.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
	s.sto.EXPECT().DeleteSpecTask(gomock.Any(), id).Return(nil)
}

func (s *DeleteProjectTasksSuite) TestDeleteProjectDeletesAllTasksIncludingArchived() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	withSession := &types.SpecTask{ID: "t1", ProjectID: "prj_del", PlanningSessionID: "ses_1"}
	archived := &types.SpecTask{ID: "t2", ProjectID: "prj_del", Archived: true}

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), &types.SpecTaskFilters{
		ProjectID:       "prj_del",
		IncludeArchived: true,
	}).Return([]*types.SpecTask{withSession, archived}, nil)

	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_1").Return(nil)
	s.expectTaskDeleted("t1")
	// t2 has no planning session: no stop, but it is still deleted.
	s.expectTaskDeleted("t2")

	s.sto.EXPECT().ListArtifacts(gomock.Any(), &store.ListArtifactsQuery{ProjectID: "prj_del"}).
		Return([]*types.Artifact{}, nil)
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)

	resp, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)
	s.Require().NotNil(resp)
	s.Equal("project deleted successfully", resp["message"])
}

func (s *DeleteProjectTasksSuite) TestDeleteProjectAbortsWhenTaskDeleteFails() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	task := &types.SpecTask{ID: "t1", ProjectID: "prj_del"}

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).
		Return([]*types.SpecTask{task}, nil)
	s.sto.EXPECT().DeleteSpecTaskAttachmentsByTaskID(gomock.Any(), "t1").Return(nil)
	s.fs.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)
	s.sto.EXPECT().DeleteSpecTask(gomock.Any(), "t1").Return(fmt.Errorf("db down"))

	// No DeleteProject expectation: gomock fails the test if the project is
	// deleted after a task delete failed.
	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Contains(httpErr.Error(), "delete task t1")
}

func (s *DeleteProjectTasksSuite) TestDeleteProjectAbortsWhenTaskListFails() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("db down"))

	// Previously the list error was swallowed (err == nil guard) and the
	// project was deleted with all its tasks still alive.
	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Contains(httpErr.Error(), "list project tasks")
}

// A desktop whose sandbox host is gone (a replaced sandbox pod) must not block
// the project delete; once the project is deleted the orphan reaper owns it.
// A connected host that fails the stop still aborts the delete.
func (s *DeleteProjectTasksSuite) TestDeleteProjectSkipsDesktopsOnDisconnectedHosts() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	task := &types.SpecTask{ID: "t1", ProjectID: "prj_del", PlanningSessionID: "ses_task"}
	gone := fmt.Errorf("failed to dial Hydra via RevDial: %w", connman.ErrNoConnection)

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(&types.Session{ID: "ses_explore"}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_explore").Return(gone)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{task}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_task").Return(gone)
	s.expectTaskDeleted("t1")
	s.sto.EXPECT().ListArtifacts(gomock.Any(), gomock.Any()).Return([]*types.Artifact{}, nil)
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(&types.Session{ID: "ses_explore"}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_explore").Return(fmt.Errorf("hydra API error (status 500)"))

	_, httpErr = s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Contains(httpErr.Error(), "stop exploratory session")
}
