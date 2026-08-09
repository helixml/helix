package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestUpdateSpecTaskExecutionConfigStoresSandboxPreset(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	executor := external_agent.NewMockExecutor(ctrl)
	server := &HelixAPIServer{Store: helixStore, externalAgentExecutor: executor}
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1", PlanningSessionID: "ses_1"}
	owner := types.User{ID: "user_1"}
	resources := &types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 8192}

	helixStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	helixStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{
		ID: task.ProjectID, UserID: owner.ID,
	}, nil)
	executor.EXPECT().HasRunningContainer(gomock.Any(), task.PlanningSessionID).Return(true)
	executor.EXPECT().UpdateDesktopResources(gomock.Any(), task.PlanningSessionID, resources).Return(nil)
	helixStore.EXPECT().UpdateSpecTask(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, updated *types.SpecTask) error {
			require.Equal(t, resources, updated.SandboxResourceOverrides)
			return nil
		},
	)

	body, err := json.Marshal(types.SpecTaskExecutionConfigUpdateRequest{
		SandboxResourceOverrides: resources,
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/spec-tasks/spt_1/execution-config", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	req = req.WithContext(setRequestUser(req.Context(), owner))
	rr := httptest.NewRecorder()

	server.updateSpecTaskExecutionConfig(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var response types.SpecTaskExecutionConfigUpdateResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.True(t, response.SandboxResourcesApplied)
	require.Equal(t, resources, response.Task.SandboxResourceOverrides)
}

func TestUpdateSpecTaskExecutionConfigRejectsInvalidSandboxPreset(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: helixStore}
	task := &types.SpecTask{ID: "spt_1", ProjectID: "prj_1"}
	owner := types.User{ID: "user_1"}

	helixStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	helixStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{
		ID: task.ProjectID, UserID: owner.ID,
	}, nil)
	body, err := json.Marshal(types.SpecTaskExecutionConfigUpdateRequest{
		SandboxResourceOverrides: &types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 4096},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/spec-tasks/spt_1/execution-config", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	req = req.WithContext(setRequestUser(req.Context(), owner))
	rr := httptest.NewRecorder()

	server.updateSpecTaskExecutionConfig(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "sandbox size must be")
}
