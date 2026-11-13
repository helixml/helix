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

	"github.com/dgraph-io/ristretto/v2"
	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type interactiveEmbedModelHandlerFixture struct {
	server   *HelixAPIServer
	store    *store.MockStore
	manager  *manager.MockProviderManager
	task     *types.SpecTask
	session  *types.Session
	user     types.User
	endpoint *types.ProviderEndpoint
}

func newInteractiveEmbedModelHandlerFixture(t *testing.T) *interactiveEmbedModelHandlerFixture {
	t.Helper()
	ctrl := gomock.NewController(t)
	helixStore := store.NewMockStore(ctrl)
	providerManager := manager.NewMockProviderManager(ctrl)
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
	})
	require.NoError(t, err)
	cfg := &config.ServerConfig{}
	cfg.WebServer.ModelsCacheTTL = time.Minute
	endpoint := &types.ProviderEndpoint{
		ID: "pe_bunker", Name: "bunker", Owner: "owner", BaseURL: "https://bunker.example.test/v1",
		OwnerType: types.OwnerTypeUser,
	}
	cached, err := json.Marshal(cachedModels{
		Models: []types.OpenAIModel{
			{ID: "qwen3.8-flash-next", Type: "chat", Enabled: true},
			{ID: "glm-5.3-flash-uncensored", Type: "chat", Enabled: true},
		},
		FetchedAt: time.Now(),
	})
	require.NoError(t, err)
	cache.SetWithTTL(modelCacheKey(endpoint.Name, endpoint.Owner), string(cached), 1, time.Hour)
	cache.Wait()
	config := &types.CodeAgentExecutionConfig{
		Runtime:            types.CodeAgentRuntimeOpenCode,
		CredentialType:     types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:        endpoint.ID,
		Model:              "qwen3.8-flash-next",
		ReasoningEffort:    "xhigh",
		GooseRecipeRepoURL: "https://example.test/recipes",
		GooseRecipes:       []types.AssistantGooseRecipe{{Name: "review", Path: "recipes/review.yaml"}},
	}
	task := &types.SpecTask{
		ID: "spt_A", ProjectID: "prj_A", UserID: "owner", Status: types.TaskStatusImplementation,
		PlanningSessionID: "ses_A", CodeAgentConfig: config,
	}
	return &interactiveEmbedModelHandlerFixture{
		server: &HelixAPIServer{
			Cfg: cfg, Store: helixStore, providerManager: providerManager, cache: cache,
		},
		store: helixStore, manager: providerManager, task: task,
		session: &types.Session{ID: "ses_A", Owner: "owner", GenerationID: 1},
		user: types.User{
			ID: "owner", APIKeyType: types.APIkeytypeEmbed, EmbedInteractive: true,
			SpecTaskID: "spt_A", SessionID: "ses_A",
		},
		endpoint: endpoint,
	}
}

func (f *interactiveEmbedModelHandlerFixture) expectAuthorizedTaskLoad(providerReads int) {
	f.store.EXPECT().GetSpecTask(gomock.Any(), f.task.ID).Return(f.task, nil)
	f.store.EXPECT().GetProject(gomock.Any(), f.task.ProjectID).Return(&types.Project{
		ID: f.task.ProjectID, UserID: f.user.ID,
	}, nil)
	f.manager.EXPECT().ListProviderEndpoints(gomock.Any(), f.user.ID).
		Return([]*types.ProviderEndpoint{f.endpoint}, nil).
		Times(providerReads)
}

func (f *interactiveEmbedModelHandlerFixture) patch(model string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(types.InteractiveEmbedModelUpdateRequest{Model: model})
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/spec-tasks/"+f.task.ID+"/interactive-embed-model",
		bytes.NewReader(body),
	)
	request = mux.SetURLVars(request, map[string]string{"taskId": f.task.ID})
	request = request.WithContext(setRequestUser(request.Context(), f.user))
	response := httptest.NewRecorder()
	f.server.updateInteractiveEmbedModel(response, request)
	return response
}

func TestUpdateInteractiveEmbedModelPersistsOnlySelectedModel(t *testing.T) {
	fixture := newInteractiveEmbedModelHandlerFixture(t)
	fixture.expectAuthorizedTaskLoad(2)
	fixture.store.EXPECT().GetSession(gomock.Any(), fixture.session.ID).Return(fixture.session, nil)
	fixture.store.EXPECT().ListInteractions(gomock.Any(), gomock.Any()).Return(nil, int64(0), nil).Times(2)
	expected := *fixture.task.CodeAgentConfig
	expected.Model = "glm-5.3-flash-uncensored"
	fixture.store.EXPECT().UpdateSpecTask(gomock.Any(), fixture.task).DoAndReturn(
		func(_ context.Context, task *types.SpecTask) error {
			require.Equal(t, &expected, task.CodeAgentConfig)
			return nil
		},
	)
	fixture.store.EXPECT().UpdateSession(gomock.Any(), gomock.Any()).Return(fixture.session, nil)
	fixture.store.EXPECT().CreateInteraction(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, interaction *types.Interaction) (*types.Interaction, error) {
			interaction.ID = "int_seed"
			return interaction, nil
		},
	)

	response := fixture.patch(expected.Model)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, &expected, fixture.task.CodeAgentConfig)
	var payload types.InteractiveEmbedModelResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Equal(t, expected.Model, payload.Model)
}

func TestUpdateInteractiveEmbedModelRejectsModelOutsideEnabledCatalogueBeforePersistence(t *testing.T) {
	fixture := newInteractiveEmbedModelHandlerFixture(t)
	fixture.expectAuthorizedTaskLoad(1)

	response := fixture.patch("operator-disabled-model")

	require.Equal(t, http.StatusBadRequest, response.Code)
	require.Contains(t, response.Body.String(), "model is not enabled for this provider")
	require.Equal(t, "qwen3.8-flash-next", fixture.task.CodeAgentConfig.Model)
}

func TestInteractiveEmbedModelExistsOnlyAcceptsExactEnabledCatalogueID(t *testing.T) {
	models := []types.OpenAIModel{{ID: "qwen3.8-flash-next"}, {ID: "glm-5.3-flash-uncensored"}}
	if !interactiveEmbedModelExists(models, "glm-5.3-flash-uncensored") {
		t.Fatal("approved model was rejected")
	}
	for _, model := range []string{"glm-5.3-flash", "bunker/glm-5.3-flash-uncensored", ""} {
		if interactiveEmbedModelExists(models, model) {
			t.Fatalf("unapproved model %q was accepted", model)
		}
	}
}

func TestUpdateInteractiveEmbedModelRejectsInjectedConfigFields(t *testing.T) {
	server := &HelixAPIServer{}
	request := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/spec-tasks/spt_A/interactive-embed-model",
		strings.NewReader(`{"model":"glm-5.3-flash-uncensored","provider_ref":"pe_other","runtime":"claude-code"}`),
	)
	response := httptest.NewRecorder()

	server.updateInteractiveEmbedModel(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLoadInteractiveEmbedModelTaskRejectsReadOnlyCrossTaskAndStaleSessionKeys(t *testing.T) {
	for _, test := range []struct {
		name string
		user *types.User
		want int
	}{
		{
			name: "read only",
			user: &types.User{ID: "owner", APIKeyType: types.APIkeytypeEmbed, SpecTaskID: "spt_A", SessionID: "ses_A"},
			want: http.StatusForbidden,
		},
		{
			name: "cross task",
			user: &types.User{ID: "owner", APIKeyType: types.APIkeytypeEmbed, EmbedInteractive: true, SpecTaskID: "spt_B", SessionID: "ses_A"},
			want: http.StatusForbidden,
		},
		{
			name: "stale session",
			user: &types.User{ID: "owner", APIKeyType: types.APIkeytypeEmbed, EmbedInteractive: true, SpecTaskID: "spt_A", SessionID: "ses_old"},
			want: http.StatusConflict,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
			if test.want == http.StatusConflict {
				mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_A").Return(&types.SpecTask{
					ID: "spt_A", ProjectID: "prj_A", UserID: "owner", PlanningSessionID: "ses_A",
					CodeAgentConfig: &types.CodeAgentExecutionConfig{
						Runtime: types.CodeAgentRuntimeQwenCode, CredentialType: types.CodeAgentCredentialTypeAPIKey,
						ProviderRef: "pe_bunker", Model: "qwen3.8-flash-next",
					},
				}, nil)
				mockStore.EXPECT().GetProject(gomock.Any(), "prj_A").Return(&types.Project{ID: "prj_A", UserID: "owner"}, nil)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/spec-tasks/spt_A/interactive-embed-model", nil)
			request = mux.SetURLVars(request, map[string]string{"taskId": "spt_A"})
			request = request.WithContext(setRequestUser(request.Context(), *test.user))
			response := httptest.NewRecorder()

			_, _, _, ok := server.loadInteractiveEmbedModelTask(response, request, "spt_A", types.ActionGet)
			if ok || response.Code != test.want {
				t.Fatalf("ok = %v, status = %d, want false/%d", ok, response.Code, test.want)
			}
		})
	}
}

func TestResolveInteractiveEmbedCodeAgentConfigMaterializesLegacyTaskWithoutWriting(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	task := &types.SpecTask{
		ID:         "spt_legacy",
		HelixAppID: "app_legacy",
		CodeAgentOverrides: &types.CodeAgentOverrides{
			Model: "glm-5.3-flash-uncensored",
		},
	}
	mockStore.EXPECT().GetApp(gomock.Any(), "app_legacy").Return(&types.App{
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeQwenCode,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider:                "pe_bunker",
			Model:                   "qwen3.8-flash-next",
		}}}},
	}, nil)

	config, err := server.resolveInteractiveEmbedCodeAgentConfig(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if config.Runtime != types.CodeAgentRuntimeQwenCode || config.ProviderRef != "pe_bunker" ||
		config.Model != "glm-5.3-flash-uncensored" {
		t.Fatalf("unexpected legacy config: %#v", config)
	}
	if task.CodeAgentConfig != nil || task.HelixAppID != "app_legacy" {
		t.Fatalf("GET-side resolution mutated legacy task: %#v", task)
	}
}

func TestInteractiveEmbedModelsUsesCurrentProvidersEnabledCatalogue(t *testing.T) {
	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	client := openai.NewMockClient(ctrl)
	modelInfoProvider := model.NewMockModelInfoProvider(ctrl)
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.ServerConfig{}
	cfg.WebServer.ModelsCacheTTL = time.Minute
	server := &HelixAPIServer{
		Cfg:               cfg,
		providerManager:   providerManager,
		modelInfoProvider: modelInfoProvider,
		cache:             cache,
	}
	endpoint := &types.ProviderEndpoint{
		ID:        "pe_bunker",
		Name:      "bunker",
		Owner:     "owner",
		BaseURL:   "https://bunker.example.test/v1",
		Models:    []string{"qwen3.8-flash-next", "glm-5.3-flash-uncensored"},
		OwnerType: types.OwnerTypeUser,
	}
	providerManager.EXPECT().ListProviderEndpoints(gomock.Any(), "viewer").Return([]*types.ProviderEndpoint{endpoint}, nil)
	providerManager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(client, nil)
	client.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{
		{ID: "qwen3.8-flash-next", Type: "chat", Enabled: true},
		{ID: "glm-5.3-flash-uncensored", Type: "chat", Enabled: true},
		{ID: "operator-disabled-model", Type: "chat", Enabled: true},
	}, nil)
	modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).Times(2)
	config := &types.CodeAgentExecutionConfig{
		Runtime: types.CodeAgentRuntimeQwenCode, CredentialType: types.CodeAgentCredentialTypeAPIKey,
		ProviderRef: "pe_bunker", Model: "qwen3.8-flash-next",
	}

	models, err := server.interactiveEmbedModels(
		context.Background(),
		&types.User{ID: "viewer"},
		&types.SpecTask{UserID: "owner"},
		config,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "qwen3.8-flash-next" || models[1].ID != "glm-5.3-flash-uncensored" {
		t.Fatalf("models = %#v", models)
	}
}
