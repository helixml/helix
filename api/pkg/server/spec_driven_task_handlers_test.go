package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateTaskFromPromptRejectsLegacyAgentFields(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	user := types.User{ID: "user1"}
	project := &types.Project{ID: "project1", UserID: user.ID}
	mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil)

	body := `{"project_id":"project1","prompt":"do work","app_id":"app-legacy"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/spec-tasks/from-prompt", bytes.NewBufferString(body))
	req = req.WithContext(setRequestUser(req.Context(), user))
	response := httptest.NewRecorder()

	server.createTaskFromPrompt(response, req)

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "code_agent_config")
}

func TestSpecTaskProviderPreflightHandlers(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		handler func(*HelixAPIServer, http.ResponseWriter, *http.Request)
	}{
		{
			name:    "start planning",
			path:    "/api/v1/spec-tasks/task1/start-planning",
			handler: (*HelixAPIServer).startPlanning,
		},
		{
			name:    "approve specs",
			path:    "/api/v1/spec-tasks/task1/approve-specs",
			body:    `{"approved":true}`,
			handler: (*HelixAPIServer).approveSpecs,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockProviderManager := manager.NewMockProviderManager(ctrl)
			server := &HelixAPIServer{
				Store:           mockStore,
				providerManager: mockProviderManager,
			}
			user := types.User{ID: "user1"}
			task := &types.SpecTask{
				ID:         "task1",
				ProjectID:  "project1",
				HelixAppID: "app1",
				Status:     types.TaskStatusBacklog,
			}
			project := &types.Project{ID: "project1", UserID: user.ID}
			app := &types.App{
				ID:             "app1",
				OrganizationID: "org1",
				Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
					AgentType:               types.AgentTypeZedExternal,
					CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
					CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
					Provider:                "anthropic",
					Model:                   "claude-opus-4-8",
					GenerationModelProvider: "pe_personal",
					GenerationModel:         "scope-e2e-model",
				}}}},
			}

			mockStore.EXPECT().GetSpecTask(gomock.Any(), task.ID).Return(task, nil)
			mockStore.EXPECT().GetProject(gomock.Any(), project.ID).Return(project, nil).Times(2)
			mockStore.EXPECT().GetApp(gomock.Any(), app.ID).Return(app, nil)
			mockProviderManager.EXPECT().ListProviderEndpointsForOwner(gomock.Any(), app.OrganizationID, types.OwnerTypeOrg).Return([]*types.ProviderEndpoint{{Name: "anthropic"}}, nil)

			req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewBufferString(tt.body))
			req = mux.SetURLVars(req, map[string]string{"taskId": task.ID})
			req = req.WithContext(setRequestUser(req.Context(), user))
			response := httptest.NewRecorder()

			tt.handler(server, response, req)

			require.Equal(t, http.StatusUnprocessableEntity, response.Code)
			var payload struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&payload))
			require.Equal(t, "agent_misconfigured", payload.Error)
			require.Equal(t, types.OrganizationProviderUnavailableMessage, payload.Message)
		})
	}
}

func TestDeriveAgentWorkState(t *testing.T) {
	cases := []struct {
		name   string
		task   *types.SpecTask
		latest *types.Interaction
		want   types.AgentWorkState
	}{
		{
			name:   "sandbox absent → empty (UI shows sandbox hint)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "absent"},
			latest: &types.Interaction{State: types.InteractionStateWaiting},
			want:   "",
		},
		{
			name:   "sandbox starting → empty",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "starting"},
			latest: nil,
			want:   "",
		},
		{
			name:   "running + waiting interaction → working",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateWaiting},
			want:   types.AgentWorkStateWorking,
		},
		{
			name:   "running + complete interaction → idle",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateComplete},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + error interaction → idle (agent isn't actively working)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateError},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + no interaction at all → idle",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: nil,
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + editing interaction → idle (only Waiting counts as working)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateEditing},
			want:   types.AgentWorkStateIdle,
		},
		{
			name:   "running + none-state interaction → idle (transient pre-Waiting state)",
			task:   &types.SpecTask{Status: types.TaskStatusImplementation, SandboxState: "running"},
			latest: &types.Interaction{State: types.InteractionStateNone},
			want:   types.AgentWorkStateIdle,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveAgentWorkState(tc.task, tc.latest)
			if got != tc.want {
				t.Errorf("deriveAgentWorkState(%+v, %+v) = %q; want %q", tc.task, tc.latest, got, tc.want)
			}
		})
	}
}
