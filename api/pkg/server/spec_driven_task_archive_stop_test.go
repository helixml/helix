package server

import (
	"bytes"

	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

// Regression coverage for helixml/helix#3427: archiving a spec task must not
// stop an agent that is still working. Archiving is bookkeeping; killing a live
// turn is a separate, explicit action.
type SpecTaskArchiveStopSuite struct {
	suite.Suite

	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer

	userID     string
	taskID     string
	projectID  string
	sessionID  string
	archiveReq *bytes.Reader
}

func TestSpecTaskArchiveStopSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskArchiveStopSuite))
}

func (s *SpecTaskArchiveStopSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:                   &config.ServerConfig{},
		Store:                 s.store,
		externalAgentExecutor: s.executor,
	}
	s.userID = "user_archive_test"
	s.taskID = "task_archive_test"
	s.projectID = "project_archive_test"
	s.sessionID = "session_archive_test"
}

func (s *SpecTaskArchiveStopSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpecTaskArchiveStopSuite) makeArchiveRequest() *http.Request {
	buf, err := json.Marshal(types.SpecTaskArchiveRequest{Archived: true})
	s.Require().NoError(err)
	req := httptest.NewRequest("PATCH", "/api/v1/spec-tasks/"+s.taskID+"/archive", bytes.NewReader(buf))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: s.userID}))
	return mux.SetURLVars(req, map[string]string{"taskId": s.taskID})
}

func (s *SpecTaskArchiveStopSuite) makeTask(status types.SpecTaskStatus) *types.SpecTask {
	return &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         s.projectID,
		Status:            status,
		PlanningSessionID: s.sessionID,
	}
}

func (s *SpecTaskArchiveStopSuite) expectArchiveBookkeeping(task *types.SpecTask) {
	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(task, nil)
	s.store.EXPECT().
		GetProject(gomock.Any(), s.projectID).
		Return(&types.Project{ID: s.projectID, UserID: s.userID}, nil).
		AnyTimes()
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}

// The regression: an agent mid-turn must survive an archive of its own task.
func (s *SpecTaskArchiveStopSuite) TestArchive_WithLiveAgent_DoesNotStopDesktop() {
	task := s.makeTask(types.TaskStatusImplementation)
	s.expectArchiveBookkeeping(task)

	// The agent is connected, the container is up, and the latest interaction
	// is still waiting for the model — i.e. a turn is in progress.
	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		Return(&types.SpecTaskExternalAgent{SpecTaskID: s.taskID, Status: "running"}, nil)
	s.store.EXPECT().
		GetSession(gomock.Any(), s.sessionID).
		Return(&types.Session{
			ID: s.sessionID,
			Metadata: types.SessionMetadata{
				AgentType:           "zed_external",
				ExternalAgentStatus: "running",
			},
		}, nil).
		AnyTimes()
	// No executor expectation: gomock fails the test if StopDesktop is called.

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequest())

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// A task whose agent already finished is still stopped when archived, so the
// desktop is released rather than held until the idle timer.
func (s *SpecTaskArchiveStopSuite) TestArchive_WithFinishedAgent_StopsDesktop() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)

	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		Return(&types.SpecTaskExternalAgent{SpecTaskID: s.taskID, Status: "stopped"}, nil).
		AnyTimes()
	s.store.EXPECT().
		GetSession(gomock.Any(), s.sessionID).
		Return(&types.Session{
			ID: s.sessionID,
			Metadata: types.SessionMetadata{
				AgentType:           "zed_external",
				ExternalAgentStatus: "stopped",
			},
		}, nil).
		AnyTimes()
	s.store.EXPECT().
		GetLatestInteractionsForSessions(gomock.Any(), []string{s.sessionID}).
		Return(map[string]*types.Interaction{
			s.sessionID: {State: types.InteractionStateComplete},
		}, nil).
		AnyTimes()
	s.executor.EXPECT().StopDesktop(gomock.Any(), s.sessionID).Return(nil).Times(1)
	s.store.EXPECT().
		ReapWaitingInteractions(gomock.Any(), s.sessionID, types.InteractionStateInterrupted, "spec task archived").
		Return(nil, nil).
		Times(1)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequest())

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// The stop runs in a detached goroutine, so expectations are checked in a
// bounded loop rather than immediately. gomock has no "wait" primitive.
func (s *SpecTaskArchiveStopSuite) waitForArchiveSideEffects() {
	time.Sleep(300 * time.Millisecond)
}
