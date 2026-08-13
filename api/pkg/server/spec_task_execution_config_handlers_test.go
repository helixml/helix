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

func TestGetSpecTaskExecutionConfigReturnsLegacyPersonalAgentSnapshot(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: helixStore}
	owner := types.User{ID: "user_1"}
	task := &types.SpecTask{
		ID: "spt_legacy", ProjectID: "prj_1", OrganizationID: "org_1",
		HelixAppID: "app_personal",
	}
	app := &types.App{
		ID: task.HelixAppID, Owner: owner.ID, AgentKind: types.AgentKindCoding,
		Config: types.AgentConfig{
			Secrets: map[string]string{"ANTHROPIC_API_KEY": "must-not-leak"},
			Helix: types.AgentHelixConfig{
				Name: "Opus 4.5 in Zed Agent (1)",
				Assistants: []types.AssistantConfig{{
					AgentType:               types.AgentTypeZedExternal,
					CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
					GenerationModelProvider: "anthropic",
					GenerationModel:         "claude-opus-4-5-20251101",
				}},
			},
		},
	}

	helixStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	helixStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{
		ID: task.ProjectID, UserID: owner.ID,
	}, nil)
	helixStore.EXPECT().GetApp(gomock.Any(), task.HelixAppID).Return(app, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/spec-tasks/"+task.ID+"/execution-config", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	req = req.WithContext(setRequestUser(req.Context(), owner))
	rr := httptest.NewRecorder()

	server.getSpecTaskExecutionConfig(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var response types.AgentExecutionConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.True(t, response.AgentAvailable)
	require.Equal(t, "Opus 4.5 in Zed Agent (1)", response.AgentName)
	require.Equal(t, types.CodeAgentRuntimeZedAgent, response.Runtime)
	require.Equal(t, "anthropic", response.ProviderRef)
	require.Equal(t, "claude-opus-4-5-20251101", response.Model)
	require.NotContains(t, rr.Body.String(), "must-not-leak")
}

func TestGetSpecTaskExecutionConfigFallsBackToSessionForDeletedAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: helixStore}
	owner := types.User{ID: "user_1"}
	task := &types.SpecTask{
		ID: "spt_deleted", ProjectID: "prj_1", OrganizationID: "org_1",
		HelixAppID: "deleted-app", PlanningSessionID: "ses_1",
	}

	helixStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
	helixStore.EXPECT().GetProject(gomock.Any(), task.ProjectID).Return(&types.Project{
		ID: task.ProjectID, UserID: owner.ID,
	}, nil)
	helixStore.EXPECT().GetApp(gomock.Any(), task.HelixAppID).Return(nil, store.ErrNotFound)
	helixStore.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil)
	helixStore.EXPECT().GetSession(gomock.Any(), task.PlanningSessionID).Return(&types.Session{
		ID: task.PlanningSessionID,
		Metadata: types.SessionMetadata{
			CodeAgentRuntime: types.CodeAgentRuntimeCodexCLI,
			ZedAgentName:     "codex",
		},
	}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/spec-tasks/"+task.ID+"/execution-config", http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
	req = req.WithContext(setRequestUser(req.Context(), owner))
	rr := httptest.NewRecorder()

	server.getSpecTaskExecutionConfig(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var response types.AgentExecutionConfig
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &response))
	require.False(t, response.AgentAvailable)
	require.Equal(t, "deleted-app", response.AgentID)
	require.Equal(t, "codex", response.AgentName)
	require.Equal(t, types.CodeAgentRuntimeCodexCLI, response.Runtime)
	require.Empty(t, response.Model)
}
