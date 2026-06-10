package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
		AgentSessionID: "session_keepalive",
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
		AgentSessionID: "session_keepalive",
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
		AgentSessionID: "session_keepalive",
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
