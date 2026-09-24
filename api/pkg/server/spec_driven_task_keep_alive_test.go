package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

type SpecTaskKeepAliveSuite struct {
	suite.Suite

	ctrl     *gomock.Controller
	store    *store.MockStore
	executor *external_agent.MockExecutor
	server   *HelixAPIServer

	userID string
	taskID string
}

func TestSpecTaskKeepAliveSuite(t *testing.T) {
	suite.Run(t, new(SpecTaskKeepAliveSuite))
}

func (s *SpecTaskKeepAliveSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.store = store.NewMockStore(s.ctrl)
	s.executor = external_agent.NewMockExecutor(s.ctrl)
	s.server = &HelixAPIServer{
		Cfg:                   &config.ServerConfig{},
		Store:                 s.store,
		externalAgentExecutor: s.executor,
	}
	s.userID = "user_keepalive_test"
	s.taskID = "task_keepalive_test"
}

func (s *SpecTaskKeepAliveSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SpecTaskKeepAliveSuite) makeUpdateRequest(body types.SpecTaskUpdateRequest) *http.Request {
	buf, err := json.Marshal(body)
	s.Require().NoError(err)
	req := httptest.NewRequest("PUT", "/api/v1/spec-tasks/"+s.taskID, bytes.NewReader(buf))
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: s.userID}))
	req = mux.SetURLVars(req, map[string]string{"taskId": s.taskID})
	return req
}

// Verifies that turning KeepAlive off on a Done task triggers StopDesktop —
// the user's explicit "release the desktop after merge" path.
func (s *SpecTaskKeepAliveSuite) TestKeepAliveOff_OnDoneTask_StopsDesktop() {
	existingTask := &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         "project_keepalive",
		Status:            types.TaskStatusDone,
		PlanningSessionID: "session_keepalive",
		KeepAlive:         true,
	}
	project := &types.Project{
		ID:     "project_keepalive",
		UserID: s.userID,
	}

	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, t *types.SpecTask) error {
		s.False(t.KeepAlive, "KeepAlive should be flipped to false")
		return nil
	})
	s.executor.EXPECT().StopDesktop(gomock.Any(), "session_keepalive").Return(nil)

	keepAliveOff := false
	rr := httptest.NewRecorder()
	s.server.updateSpecTask(rr, s.makeUpdateRequest(types.SpecTaskUpdateRequest{KeepAlive: &keepAliveOff}))

	s.Equal(http.StatusOK, rr.Code)
}

// Verifies that turning KeepAlive ON does NOT call StopDesktop, even on a Done task.
func (s *SpecTaskKeepAliveSuite) TestKeepAliveOn_OnDoneTask_DoesNotStopDesktop() {
	existingTask := &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         "project_keepalive",
		Status:            types.TaskStatusDone,
		PlanningSessionID: "session_keepalive",
		KeepAlive:         false,
	}
	project := &types.Project{
		ID:     "project_keepalive",
		UserID: s.userID,
	}

	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil)
	// No executor expectation — gomock will fail the test if StopDesktop is called.

	keepAliveOn := true
	rr := httptest.NewRecorder()
	s.server.updateSpecTask(rr, s.makeUpdateRequest(types.SpecTaskUpdateRequest{KeepAlive: &keepAliveOn}))

	s.Equal(http.StatusOK, rr.Code)
}

// Verifies that turning KeepAlive off on a non-Done task does NOT call StopDesktop —
// the existing idle-shutdown path will handle it normally.
func (s *SpecTaskKeepAliveSuite) TestKeepAliveOff_OnRunningTask_DoesNotStopDesktop() {
	existingTask := &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         "project_keepalive",
		Status:            types.TaskStatusImplementation,
		PlanningSessionID: "session_keepalive",
		KeepAlive:         true,
	}
	project := &types.Project{
		ID:     "project_keepalive",
		UserID: s.userID,
	}

	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).Return(nil)
	// No executor expectation — gomock will fail the test if StopDesktop is called.

	keepAliveOff := false
	rr := httptest.NewRecorder()
	s.server.updateSpecTask(rr, s.makeUpdateRequest(types.SpecTaskUpdateRequest{KeepAlive: &keepAliveOff}))

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskKeepAliveSuite) TestResetDoneExistingTaskToBacklogUsesNewBranchMode() {
	existingTask := &types.SpecTask{
		ID:         s.taskID,
		ProjectID:  "project_keepalive",
		Status:     types.TaskStatusDone,
		BranchMode: types.BranchModeExisting,
		BranchName: "merged-branch",
	}
	project := &types.Project{
		ID:     "project_keepalive",
		UserID: s.userID,
	}

	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, task *types.SpecTask) error {
		s.Empty(task.BranchName)
		s.Equal(types.BranchModeNew, task.BranchMode)
		return nil
	})

	rr := httptest.NewRecorder()
	s.server.updateSpecTask(rr, s.makeUpdateRequest(types.SpecTaskUpdateRequest{Status: types.TaskStatusBacklog}))

	s.Equal(http.StatusOK, rr.Code)
}

func (s *SpecTaskKeepAliveSuite) TestReopenDoneTask_ClearsTerminalFields() {
	now := time.Now()
	existingTask := &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         "project_keepalive",
		Status:            types.TaskStatusDone,
		BranchName:        "feature/reopen",
		PlanningSessionID: "session_keepalive",
		CompletedAt:       &now,
		MergedToMain:      true,
		MergedAt:          &now,
		MergeCommitHash:   "merge-sha",
	}
	project := &types.Project{ID: "project_keepalive", UserID: s.userID}

	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(func(_ interface{}, task *types.SpecTask) error {
		s.Equal(types.TaskStatusImplementation, task.Status)
		s.Nil(task.CompletedAt)
		s.False(task.MergedToMain)
		s.Nil(task.MergedAt)
		s.Empty(task.MergeCommitHash)
		s.Equal("feature/reopen", task.BranchName)
		s.Equal("session_keepalive", task.PlanningSessionID)
		return nil
	})

	rr := httptest.NewRecorder()
	s.server.updateSpecTask(rr, s.makeUpdateRequest(types.SpecTaskUpdateRequest{Status: types.TaskStatusImplementation}))

	s.Equal(http.StatusOK, rr.Code)
}

// Archiving must not block on desktop teardown: the HTTP response returns
// while StopDesktop still runs in the background. Stopping a desktop is slow
// (screenshot, container teardown, key revocation) and previously stalled the
// archive request for its full duration.
func (s *SpecTaskKeepAliveSuite) TestArchiveTask_ReturnsBeforeDesktopStopCompletes() {
	existingTask := &types.SpecTask{
		ID:                s.taskID,
		ProjectID:         "project_keepalive",
		Status:            types.TaskStatusDone,
		PlanningSessionID: "session_keepalive",
	}
	project := &types.Project{ID: "project_keepalive", UserID: s.userID}
	session := &types.Session{
		ID:       "session_keepalive",
		Metadata: types.SessionMetadata{AgentType: "zed_external"},
	}

	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	stopFinished := make(chan struct{})
	s.store.EXPECT().GetSpecTask(gomock.Any(), s.taskID).Return(existingTask, nil)
	s.store.EXPECT().GetProject(gomock.Any(), "project_keepalive").Return(project, nil)
	s.store.EXPECT().GetSession(gomock.Any(), "session_keepalive").Return(session, nil)
	s.executor.EXPECT().StopDesktop(gomock.Any(), "session_keepalive").DoAndReturn(
		func(_ context.Context, _ string) error {
			close(stopStarted)
			<-releaseStop // block until the response has been verified
			return nil
		},
	)
	// The turn in flight when the task is archived is ended, so auto-wake
	// does not read it as a stuck cold start and boot the desktop again.
	s.store.EXPECT().ReapWaitingInteractions(gomock.Any(), "session_keepalive", types.InteractionStateInterrupted, "spec task archived").
		Return(nil, nil)
	s.store.EXPECT().GetSpecTaskExternalAgent(gomock.Any(), s.taskID).DoAndReturn(
		func(_ context.Context, _ string) (*types.SpecTaskExternalAgent, error) {
			defer close(stopFinished)
			return nil, errors.New("no external agent")
		},
	)
	s.store.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ interface{}, t *types.SpecTask) error {
			s.True(t.Archived)
			return nil
		},
	)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/spec-tasks/"+s.taskID+"/archive",
		strings.NewReader(`{"archived":true}`),
	)
	req = req.WithContext(setRequestUser(req.Context(), types.User{ID: s.userID}))
	req = mux.SetURLVars(req, map[string]string{"taskId": s.taskID})

	rr := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.server.archiveSpecTask(rr, req)
	}()

	select {
	case <-stopStarted:
	case <-time.After(5 * time.Second):
		s.Fail("background StopDesktop was never started")
	}

	select {
	case <-handlerDone:
		s.Equal(http.StatusOK, rr.Code)
	case <-time.After(5 * time.Second):
		s.Fail("archive response blocked on StopDesktop")
	}

	close(releaseStop)
	select {
	case <-stopFinished:
	case <-time.After(5 * time.Second):
		s.Fail("background stop goroutine did not finish")
	}
}
