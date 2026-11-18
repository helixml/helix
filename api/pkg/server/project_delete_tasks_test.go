package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/connman"
	"github.com/helixml/helix/api/pkg/controller"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/sandbox"
	"github.com/helixml/helix/api/pkg/sessionresources"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/webservice"
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
	history  string
	// sessions returns ListSessions results by spec task id, or by project id
	// for the project-wide listing.
	sessions map[string][]*types.Session
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
	s.history = s.T().TempDir()
	s.sessions = map[string][]*types.Session{}
	s.sto.EXPECT().ListSessions(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, q store.ListSessionsQuery) ([]*types.Session, int64, error) {
			key := q.SpecTaskID
			if key == "" {
				key = q.ProjectID
			}
			return s.sessions[key], int64(len(s.sessions[key])), nil
		}).AnyTimes()
	s.server = &HelixAPIServer{
		sessionResources:      sessionresources.New(s.history, "test"),
		Cfg:                   &config.ServerConfig{},
		Store:                 s.sto,
		externalAgentExecutor: s.executor,
		webServiceController:  webservice.New(s.sto, sandbox.New(s.sto, nil, nil, "", "")),
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

// expectNoProjectOwnedResources expects the repository, access grant and
// trigger sweep for a personal project that owns none of them.
func (s *DeleteProjectTasksSuite) expectNoProjectOwnedResources() {
	s.sto.EXPECT().ListProjectRepositories(gomock.Any(), &types.ListProjectRepositoriesQuery{ProjectID: "prj_del"}).Return(nil, nil)
	s.sto.EXPECT().DeleteProjectRepositoriesByProject(gomock.Any(), "prj_del").Return(nil)
	s.sto.EXPECT().ListTriggerConfigurations(gomock.Any(), gomock.Any()).Return(nil, nil)
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
	s.sto.EXPECT().DeleteSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1"}, nil)
	s.expectTaskDeleted("t1")
	// t2 has no planning session: no stop, but it is still deleted.
	s.expectTaskDeleted("t2")

	s.sto.EXPECT().ListArtifacts(gomock.Any(), &store.ListArtifactsQuery{ProjectID: "prj_del"}).
		Return([]*types.Artifact{}, nil)
	s.sto.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_del").Return(nil, store.ErrNotFound)
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)
	s.expectNoProjectOwnedResources()

	// Resource history of the planning session, a later thread session of the
	// archived task, and a project chat — and of a session elsewhere.
	s.sessions["t2"] = []*types.Session{{ID: "ses_thread"}}
	s.sessions["prj_del"] = []*types.Session{{ID: "ses_projectchat"}, {ID: "ses_thread"}}
	owned := []string{"ses_1", "ses_thread", "ses_projectchat"}
	now := time.Now()
	for _, id := range append(owned, "ses_elsewhere") {
		s.server.sessionResources.Record([]types.SessionResourceSample{{SessionID: id, Timestamp: now.UnixMilli()}})
	}
	s.Require().NoError(s.server.sessionResources.Flush(now))

	resp, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)
	s.Require().NotNil(resp)
	s.Equal("project deleted successfully", resp["message"])

	day := filepath.Join(s.history, now.UTC().Format(time.DateOnly))
	for _, id := range owned {
		_, err := os.Stat(filepath.Join(day, id))
		s.True(os.IsNotExist(err), "history of %s left behind", id)
		// A desktop still reporting after the delete must not bring it back.
		s.server.sessionResources.Record([]types.SessionResourceSample{{SessionID: id, Timestamp: now.UnixMilli() + 1}})
	}
	s.Require().NoError(s.server.sessionResources.Flush(now))
	for _, id := range owned {
		got, err := s.server.sessionResources.Read(id, now.Add(-time.Hour), now.Add(time.Hour))
		s.Require().NoError(err)
		s.Empty(got, "history of %s recreated", id)
	}
	other, err := s.server.sessionResources.Read("ses_elsewhere", now.Add(-time.Hour), now.Add(time.Hour))
	s.Require().NoError(err)
	s.Len(other, 1, "another project's history must survive")
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
	s.sto.EXPECT().DeleteSession(gomock.Any(), "ses_task").Return(&types.Session{ID: "ses_task"}, nil)
	s.expectTaskDeleted("t1")
	s.sto.EXPECT().ListArtifacts(gomock.Any(), gomock.Any()).Return([]*types.Artifact{}, nil)
	s.sto.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_del").Return(nil, store.ErrNotFound)
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)
	s.expectNoProjectOwnedResources()

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(&types.Session{ID: "ses_explore"}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_explore").Return(fmt.Errorf("hydra API error (status 500)"))

	_, httpErr = s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Contains(httpErr.Error(), "stop exploratory session")
}

func (s *DeleteProjectTasksSuite) TestDeleteProjectRetriesAfterSessionWasDeleted() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	task := &types.SpecTask{ID: "t1", ProjectID: "prj_del", PlanningSessionID: "ses_task"}

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{task}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_task").Return(nil)
	s.sto.EXPECT().DeleteSession(gomock.Any(), "ses_task").Return(nil, store.ErrNotFound)
	s.expectTaskDeleted("t1")
	s.sto.EXPECT().ListArtifacts(gomock.Any(), gomock.Any()).Return([]*types.Artifact{}, nil)
	s.sto.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_del").Return(nil, store.ErrNotFound)
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)
	s.expectNoProjectOwnedResources()

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().Nil(httpErr)
}

func (s *DeleteProjectTasksSuite) TestDeleteProjectAbortsWhenSessionDeleteFails() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	task := &types.SpecTask{ID: "t1", ProjectID: "prj_del", PlanningSessionID: "ses_task"}

	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return([]*types.SpecTask{task}, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "ses_task").Return(nil)
	s.sto.EXPECT().DeleteSession(gomock.Any(), "ses_task").Return(nil, fmt.Errorf("db down"))

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Require().NotNil(httpErr)
	s.Contains(httpErr.Error(), "delete session ses_task")
}

// Deleting a project removes its web service: the public route, the sandbox
// running it, and the enabled state the health monitor watches.
func (s *DeleteProjectTasksSuite) TestDeleteProjectTearsDownWebService() {
	project := &types.Project{ID: "prj_del", UserID: s.userID}
	s.sto.EXPECT().GetProject(gomock.Any(), "prj_del").Return(project, nil)
	s.sto.EXPECT().GetProjectExploratorySession(gomock.Any(), "prj_del").Return(nil, nil)
	s.sto.EXPECT().ListSpecTasks(gomock.Any(), gomock.Any()).Return(nil, nil)
	s.sto.EXPECT().ListArtifacts(gomock.Any(), &store.ListArtifactsQuery{ProjectID: "prj_del"}).Return(nil, nil)

	state := &types.ProjectWebServiceState{ProjectID: "prj_del", Enabled: true, ContainerPort: 8080, ActiveSandboxID: "sbx_ws"}
	s.sto.EXPECT().GetProjectWebServiceState(gomock.Any(), "prj_del").Return(state, nil)
	s.sto.EXPECT().DeleteVHostRoutesByTarget(gomock.Any(), types.VHostTargetProjectWebService, "prj_del").Return(nil)
	s.sto.EXPECT().GetSandbox(gomock.Any(), "sbx_ws").Return(&types.Sandbox{ID: "sbx_ws", Status: types.SandboxStatusStopped}, nil)
	s.sto.EXPECT().SetSandboxStatus(gomock.Any(), "sbx_ws", types.SandboxStatusStopping, "").Return(nil)
	s.sto.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound)
	s.sto.EXPECT().DeleteSandbox(gomock.Any(), "sbx_ws").Return(nil)
	s.sto.EXPECT().UpsertProjectWebServiceState(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, st *types.ProjectWebServiceState) error {
			s.False(st.Enabled, "the web service must be disabled")
			s.Empty(st.ActiveSandboxID)
			return nil
		})
	s.sto.EXPECT().DeleteProject(gomock.Any(), "prj_del").Return(nil)
	s.expectNoProjectOwnedResources()

	_, httpErr := s.server.deleteProject(httptest.NewRecorder(), s.request())
	s.Nil(httpErr)
}

// Deleting one archived task removes the sandbox resource history of every
// session it ran, and only theirs. Archiving alone keeps it.
func (s *DeleteProjectTasksSuite) TestDeleteSpecTaskRemovesItsResourceHistory() {
	task := &types.SpecTask{ID: "t1", ProjectID: "prj_del", PlanningSessionID: "ses_plan", Archived: true}
	s.sessions["t1"] = []*types.Session{{ID: "ses_plan"}, {ID: "ses_retry"}}
	now := time.Now()
	for _, id := range []string{"ses_plan", "ses_retry", "ses_sibling"} {
		s.server.sessionResources.Record([]types.SessionResourceSample{{SessionID: id, Timestamp: now.UnixMilli()}})
	}
	s.Require().NoError(s.server.sessionResources.Flush(now))
	s.expectTaskDeleted("t1")

	s.Require().NoError(s.server.deleteSpecTaskCascade(context.Background(), task))

	read := func(id string) int {
		got, err := s.server.sessionResources.Read(id, now.Add(-time.Hour), now.Add(time.Hour))
		s.Require().NoError(err)
		return len(got)
	}
	s.Zero(read("ses_plan"))
	s.Zero(read("ses_retry"))
	s.Equal(1, read("ses_sibling"), "another task's history must survive")
}
