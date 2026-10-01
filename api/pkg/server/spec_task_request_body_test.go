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

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func archiveSpecTaskRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *types.SpecTask) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	user := types.User{ID: "user-1"}
	task := &types.SpecTask{ID: "spt_archive", ProjectID: "project-1", Status: types.TaskStatusBacklog}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil).AnyTimes()
	mockStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{ID: task.ProjectID, UserID: user.ID}, nil).AnyTimes()
	mockStore.EXPECT().UpdateSpecTask(gomock.Any(), task).Return(nil).AnyTimes()
	// Archiving stops agents in the background; wait for it so no mock call
	// lands after the test ends.
	stopped := make(chan struct{})
	mockStore.EXPECT().GetSpecTaskExternalAgent(gomock.Any(), task.ID).DoAndReturn(
		func(context.Context, string) (*types.SpecTaskExternalAgent, error) {
			close(stopped)
			return nil, errors.New("not found")
		},
	).MaxTimes(1)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/spec-tasks/"+task.ID+"/archive", strings.NewReader(body))
	req = req.WithContext(setRequestUser(req.Context(), user))
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	response := httptest.NewRecorder()
	(&HelixAPIServer{Store: mockStore}).archiveSpecTask(response, req)
	if task.Archived {
		<-stopped
	}
	return response, task
}

func TestArchiveSpecTaskWithEmptyBodyArchives(t *testing.T) {
	response, task := archiveSpecTaskRequest(t, "")
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, task.Archived)
}

func TestArchiveSpecTaskExplicitBodyStillUnarchives(t *testing.T) {
	response, task := archiveSpecTaskRequest(t, `{}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.False(t, task.Archived)
}

func TestArchiveSpecTaskMalformedBodyExplainsExpectedShape(t *testing.T) {
	response, task := archiveSpecTaskRequest(t, "archive please")
	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), `expected {"archived": true} or {"archived": false}`)
	require.False(t, task.Archived)
}

func TestCreateTaskFromPromptRejectsOverlongName(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	user := types.User{ID: "user-1"}
	mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{ID: "project-1", UserID: user.ID}, nil).AnyTimes()
	// No CreateSpecTask expectation: the request must be rejected before any write.
	body, err := json.Marshal(types.CreateTaskRequest{
		ProjectID: "project-1",
		Prompt:    "do the thing",
		Name:      strings.Repeat("n", types.SpecTaskNameMaxRunes+1),
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewReader(body))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	(&HelixAPIServer{Store: mockStore}).createTaskFromPrompt(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "name must be at most 200 characters")
}
