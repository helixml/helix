package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestRefreshSpecTaskPullRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	task := &types.SpecTask{
		ID:        "task-1",
		ProjectID: "project-1",
		Status:    types.TaskStatusPullRequest,
	}
	user := types.User{ID: "user-1"}
	project := &types.Project{ID: task.ProjectID, UserID: user.ID}
	mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil).Times(2)
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)

	server := &HelixAPIServer{
		Store:                mockStore,
		specTaskOrchestrator: services.NewSpecTaskOrchestrator(mockStore, nil, nil, nil),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/task-1/refresh-pull-request", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()

	server.refreshSpecTaskPullRequest(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
}
