package server

import (
	"bytes"
	"context"

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

	// archiveFinished is closed by the last mock call the detached archive
	// goroutine makes. Waiting on it beats sleeping: a fixed delay either
	// flakes under CI load or wastes time when the goroutine is quick.
	archiveFinished chan struct{}
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
	s.archiveFinished = make(chan struct{})
}

func (s *SpecTaskArchiveStopSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpecTaskArchiveStopSuite) makeArchiveRequest() *http.Request {
	return s.makeArchiveRequestBody(`{"archived":true}`)
}

func (s *SpecTaskArchiveStopSuite) makeArchiveRequestBody(body string) *http.Request {
	req := httptest.NewRequest("PATCH", "/api/v1/spec-tasks/"+s.taskID+"/archive", bytes.NewReader([]byte(body)))
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

// expectLiveAgent arranges for the task's agent to look mid-turn.
func (s *SpecTaskArchiveStopSuite) expectLiveAgent() {
	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		Return(&types.SpecTaskExternalAgent{SpecTaskID: s.taskID, Status: "running"}, nil).
		AnyTimes()
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
}

// The regression: an agent mid-turn must survive an archive of its own task,
// even though the request does not mention stop_agent at all (the default).
func (s *SpecTaskArchiveStopSuite) TestArchive_WithLiveAgent_DoesNotStopDesktop() {
	task := s.makeTask(types.TaskStatusImplementation)
	s.expectArchiveBookkeeping(task)
	s.expectLiveAgent()
	// No executor expectation: gomock fails the test if StopDesktop is called.

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequest())

	s.Equal(http.StatusOK, rr.Code)
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
		DoAndReturn(func(context.Context, string, types.InteractionState, string) ([]*types.Interaction, error) {
			defer close(s.archiveFinished)
			return nil, nil
		}).
		Times(1)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequest())

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// stop_agent=false is the explicit "bookkeeping only" switch: even a finished
// agent keeps its sandbox.
func (s *SpecTaskArchiveStopSuite) TestArchive_StopAgentFalse_KeepsSandbox() {
	task := s.makeTask(types.TaskStatusDone)
	s.expectArchiveBookkeeping(task)
	// No store or executor expectations for the stop path at all.

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":false}`))

	s.Equal(http.StatusOK, rr.Code)
}

// An explicit stop_agent=true is the caller saying "stop it anyway": a live
// agent is torn down rather than skipped.
func (s *SpecTaskArchiveStopSuite) TestArchive_StopAgentTrue_StopsLiveAgent() {
	task := s.makeTask(types.TaskStatusImplementation)
	s.expectArchiveBookkeeping(task)

	// The agent row is "running" with an id, which is what makes it live and
	// what the stop path itself targets (externalAgent.ID, not the session).
	s.store.EXPECT().
		GetSpecTaskExternalAgent(gomock.Any(), s.taskID).
		Return(&types.SpecTaskExternalAgent{
			SpecTaskID: s.taskID,
			ID:         "agent_archive_test",
			Status:     "running",
		}, nil).
		AnyTimes()
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
	s.store.EXPECT().
		ReapWaitingInteractions(gomock.Any(), s.sessionID, types.InteractionStateInterrupted, "spec task archived").
		Return(nil, nil).
		Times(1)
	// An explicit stop tears down both the session desktop and the task's
	// external-agent container, so both targets are expected.
	s.executor.EXPECT().StopDesktop(gomock.Any(), s.sessionID).Return(nil).Times(1)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "agent_archive_test").Return(nil).Times(1)
	// The agent-status write is the goroutine's final step, so its completion
	// is the signal that all of the above happened.
	s.store.EXPECT().UpdateSpecTaskExternalAgent(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *types.SpecTaskExternalAgent) error {
			defer close(s.archiveFinished)
			return nil
		},
	).Times(1)

	rr := httptest.NewRecorder()
	s.server.archiveSpecTask(rr, s.makeArchiveRequestBody(`{"archived":true,"stop_agent":true}`))

	s.Equal(http.StatusOK, rr.Code)
	s.waitForArchiveSideEffects()
}

// The request decoding keeps the default and the explicit flag distinct.
func (s *SpecTaskArchiveStopSuite) TestArchiveRequest_StopAgentSemantics() {
	var omitted types.SpecTaskArchiveRequest
	s.Require().NoError(json.Unmarshal([]byte(`{"archived":true}`), &omitted))
	s.True(omitted.StopAgentEnabled(), "omitting stop_agent must keep the historical stop-on-archive default")
	s.False(omitted.StopAgentExplicit)

	var off types.SpecTaskArchiveRequest
	s.Require().NoError(json.Unmarshal([]byte(`{"archived":true,"stop_agent":false}`), &off))
	s.False(off.StopAgentEnabled())
	s.True(off.StopAgentExplicit)

	var on types.SpecTaskArchiveRequest
	s.Require().NoError(json.Unmarshal([]byte(`{"archived":true,"stop_agent":true}`), &on))
	s.True(on.StopAgentEnabled())
	s.True(on.StopAgentExplicit)
}

// The stop runs in a detached goroutine, so the test waits for the last
// expected mock call instead of sleeping for a guessed interval. Tests that
// expect no stop side effects do not call this at all.
func (s *SpecTaskArchiveStopSuite) waitForArchiveSideEffects() {
	select {
	case <-s.archiveFinished:
	case <-time.After(5 * time.Second):
		s.Fail("archive side effects did not complete")
	}
}
