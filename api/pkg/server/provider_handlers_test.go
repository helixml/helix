package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type ProviderHandlersSuite struct {
	suite.Suite

	store             *store.MockStore
	openAiClient      *openai.MockClient
	manager           *manager.MockProviderManager
	modelInfoProvider *model.MockModelInfoProvider

	authCtx context.Context

	server *HelixAPIServer
}

func TestProviderHandlersSuite(t *testing.T) {
	suite.Run(t, new(ProviderHandlersSuite))
}

func (s *ProviderHandlersSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())

	cfg := &config.ServerConfig{}
	cfg.RAG.EmbeddingsProvider = string(types.ProviderOpenAI)

	s.store = store.NewMockStore(ctrl)
	s.openAiClient = openai.NewMockClient(ctrl)
	s.manager = manager.NewMockProviderManager(ctrl)
	s.modelInfoProvider = model.NewMockModelInfoProvider(ctrl)

	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:       "user_id",
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1e4,
		MaxCost:     1 << 20,
		BufferItems: 64,
	})
	s.Require().NoError(err)

	cfg.WebServer.ModelsCacheTTL = 1 * time.Minute

	server := &HelixAPIServer{
		Cfg:               cfg,
		Store:             s.store,
		providerManager:   s.manager,
		modelInfoProvider: s.modelInfoProvider,
		cache:             cache,
	}

	s.server = server
}

func (s *ProviderHandlersSuite) TestListProviders() {
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{
			Name:           "openai",
			Models:         []string{"gpt-4o", "gpt-4o-mini"},
			BaseURL:        "https://openai.com",
			Owner:          "user_id",
			BillingEnabled: true,
		},
	}, nil)

	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "openai",
		Owner:    "user_id",
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{
		{
			ID: "gpt-4o",
		},
		{
			ID: "gpt-4o-mini",
		},
	}, nil)

	// We should get extra model info for them
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), &model.ModelInfoRequest{
		BaseURL:  "https://openai.com",
		Provider: "openai",
		Model:    "gpt-4o",
	}).Return(&types.ModelInfo{
		ContextLength: 5000,
		Pricing:       types.Pricing{Prompt: "0.0004"},
	}, nil)

	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), &model.ModelInfoRequest{
		BaseURL:  "https://openai.com",
		Provider: "openai",
		Model:    "gpt-4o-mini",
	}).Return(&types.ModelInfo{
		ContextLength: 4000,
		Pricing:       types.Pricing{Prompt: "0.0008"},
	}, nil)

	req, err := http.NewRequest("GET", "/v1/provider-endpoints", nil)
	s.Require().NoError(err)

	q := req.URL.Query()
	q.Add("with_models", "true")
	req.URL.RawQuery = q.Encode()

	rr := httptest.NewRecorder()

	req = req.WithContext(s.authCtx)

	s.server.listProviderEndpoints(rr, req)

	// Parse the response
	var resp []*types.ProviderEndpoint
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	s.Require().NoError(err)

	s.Require().Len(resp, 1)

	s.Require().Equal(true, resp[0].BillingEnabled)

	s.Require().Equal("gpt-4o", resp[0].AvailableModels[0].ID)
	s.Require().Equal("0.0004", resp[0].AvailableModels[0].ModelInfo.Pricing.Prompt)

	s.Require().Equal("gpt-4o-mini", resp[0].AvailableModels[1].ID)
	s.Require().Equal("0.0008", resp[0].AvailableModels[1].ModelInfo.Pricing.Prompt)
}

func (s *ProviderHandlersSuite) TestListProviders_NoModelInfo() {
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{
			Name:           "openai",
			Models:         []string{"gpt-4o", "gpt-4o-mini"},
			BaseURL:        "https://openai.com",
			Owner:          "user_id",
			BillingEnabled: true,
		},
	}, nil)

	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "openai",
		Owner:    "user_id",
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{
		{
			ID: "gpt-4o",
		},
		{
			ID: "gpt-4o-mini",
		},
	}, nil)

	// We should get extra model info for them
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), &model.ModelInfoRequest{
		BaseURL:  "https://openai.com",
		Provider: "openai",
		Model:    "gpt-4o",
	}).Return(&types.ModelInfo{
		ContextLength: 5000,
		Pricing:       types.Pricing{Prompt: "0.0004"},
	}, nil)

	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), &model.ModelInfoRequest{
		BaseURL:  "https://openai.com",
		Provider: "openai",
		Model:    "gpt-4o-mini",
	}).Return(nil, errors.New(("not found")))

	req, err := http.NewRequest("GET", "/v1/provider-endpoints", nil)
	s.Require().NoError(err)

	q := req.URL.Query()
	q.Add("with_models", "true")
	req.URL.RawQuery = q.Encode()

	rr := httptest.NewRecorder()

	req = req.WithContext(s.authCtx)

	s.server.listProviderEndpoints(rr, req)

	// Parse the response
	var resp []*types.ProviderEndpoint
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	s.Require().NoError(err)

	s.Require().Len(resp, 1)

	s.Require().Equal(true, resp[0].BillingEnabled)
	// Expecting 2 available models
	s.Require().Len(resp[0].AvailableModels, 2)

	s.Require().Equal("gpt-4o", resp[0].AvailableModels[0].ID)
	s.Require().Equal("0.0004", resp[0].AvailableModels[0].ModelInfo.Pricing.Prompt)

	s.Require().Equal("gpt-4o-mini", resp[0].AvailableModels[1].ID)
	s.Require().Equal(false, resp[0].AvailableModels[1].Enabled)
	s.Require().Nil(resp[0].AvailableModels[1].ModelInfo)
}

func (s *ProviderHandlersSuite) TestGetProviderModels_Singleflight() {
	// When multiple goroutines call getProviderModels concurrently for endpoints
	// pointing at the same BaseURL, the provider should only be hit once
	// (singleflight deduplicates by URL, not by endpoint name).
	endpoints := []*types.ProviderEndpoint{
		{Name: "my-ollama", Owner: "user_a", BaseURL: "http://localhost:11434"},
		{Name: "local-llm", Owner: "user_b", BaseURL: "http://localhost:11434"},
		{Name: "ollama-server", Owner: "user_c", BaseURL: "http://localhost:11434"},
	}

	var listModelsCount atomic.Int32

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil).AnyTimes()

	s.openAiClient.EXPECT().ListModels(gomock.Any()).DoAndReturn(func(_ context.Context) ([]types.OpenAIModel, error) {
		listModelsCount.Add(1)
		// Simulate slow provider response so concurrent calls overlap.
		time.Sleep(50 * time.Millisecond)
		return []types.OpenAIModel{{ID: "llama3"}}, nil
	}).AnyTimes()

	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).AnyTimes()

	var wg sync.WaitGroup
	wg.Add(len(endpoints))
	for _, ep := range endpoints {
		go func() {
			defer wg.Done()
			models, staleErr, err := s.server.getProviderModels(context.Background(), ep)
			s.NoError(err)
			s.NoError(staleErr)
			s.Require().Len(models, 1)
			s.Equal("llama3", models[0].ID)
		}()
	}
	wg.Wait()

	// Singleflight should collapse all 3 concurrent calls (same BaseURL) into 1 provider fetch.
	s.Equal(int32(1), listModelsCount.Load(), "singleflight should deduplicate concurrent fetches to the same URL")
}

func (s *ProviderHandlersSuite) TestCreateProviderEndpoint_WarmsCacheAndMasksAPIKey() {
	// The create handler should:
	// 1. Mask the API key in the HTTP response ("*****")
	// 2. Warm the cache asynchronously with the REAL API key (no race)

	// Set up auth middleware so isAdmin doesn't panic.
	s.server.authMiddleware = &authMiddleware{
		store: s.store,
		cfg: authMiddlewareConfig{
			adminUserIDs: []string{"user_id"},
		},
	}
	s.server.Cfg.Providers.EnableCustomUserProviders = true

	endpoint := types.ProviderEndpoint{
		Name:         "my-ollama",
		BaseURL:      "http://localhost:11434",
		APIKey:       "real-secret-key",
		EndpointType: types.ProviderEndpointTypeUser,
	}

	body, err := json.Marshal(endpoint)
	s.Require().NoError(err)

	// Mock store: return endpoint with API key intact
	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)

	s.manager.EXPECT().ListProviders(gomock.Any(), "user_id").Return([]types.Provider{}, nil)

	createdEndpoint := &types.ProviderEndpoint{
		ID:           "ep_123",
		Name:         "my-ollama",
		BaseURL:      "http://localhost:11434",
		APIKey:       "real-secret-key",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}
	s.store.EXPECT().CreateProviderEndpoint(gomock.Any(), gomock.Any()).Return(createdEndpoint, nil)

	// Track what API key the warm goroutine sees via GetClient
	var warmAPIKeySeen atomic.Value
	warmDone := make(chan struct{})

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "my-ollama",
		Owner:    "user_id",
	}).DoAndReturn(func(_ context.Context, req *manager.GetClientRequest) (openai.Client, error) {
		defer close(warmDone)
		// The warm goroutine shouldn't see "*****" — it should use the copy with the real key.
		// We can't directly observe the API key from GetClientRequest, but we can verify
		// the goroutine completed without error.
		warmAPIKeySeen.Store("called")
		return s.openAiClient, nil
	})

	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "llama3"}}, nil)
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	req, err := http.NewRequest("POST", "/v1/provider-endpoints", bytes.NewReader(body))
	s.Require().NoError(err)
	req = req.WithContext(s.authCtx)

	rr := httptest.NewRecorder()
	s.server.createProviderEndpoint(rr, req)

	s.Equal(http.StatusOK, rr.Code)

	// Verify API key is masked in response
	var resp types.ProviderEndpoint
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	s.Require().NoError(err)
	s.Equal("*****", resp.APIKey, "API key must be masked in response")

	// Wait for warm goroutine to complete
	select {
	case <-warmDone:
		s.Equal("called", warmAPIKeySeen.Load(), "cache warm goroutine should have run")
	case <-time.After(5 * time.Second):
		s.Fail("cache warm goroutine did not complete in time")
	}
}

// When a custom endpoint has a static Models list and upstream /v1/models
// errors (e.g. the upstream doesn't implement that route), getProviderModels
// must synthesize entries from Models so the picker is non-empty and chat
// completions routing can find the model.
func (s *ProviderHandlersSuite) TestGetProviderModels_SynthesizesFromStaticListOnError() {
	endpoint := &types.ProviderEndpoint{
		Name:    "hermes",
		Owner:   "user_id",
		BaseURL: "http://hermes.local",
		Models:  []string{"hermes-agent"},
	}

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "hermes",
		Owner:    "user_id",
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("404 not found"))
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	models, staleErr, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(staleErr)
	s.Require().Len(models, 1)
	s.Equal("hermes-agent", models[0].ID)
	s.Equal("chat", models[0].Type)
	s.True(models[0].Enabled)
	s.Equal("hermes", models[0].OwnedBy)
}

// Upstream returning an empty list with a static Models list configured should
// also fall back to the synthesized list (some endpoints answer /v1/models with
// `{"data":[]}`).
func (s *ProviderHandlersSuite) TestGetProviderModels_SynthesizesFromStaticListOnEmptyUpstream() {
	endpoint := &types.ProviderEndpoint{
		Name:    "hermes",
		Owner:   "user_id_2",
		BaseURL: "http://hermes-empty.local",
		Models:  []string{"hermes-agent", "hermes-mini"},
	}

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{}, nil)
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found")).Times(2)

	models, staleErr, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(staleErr)
	s.Require().Len(models, 2)
	s.Equal("hermes-agent", models[0].ID)
	s.Equal("hermes-mini", models[1].ID)
	for _, m := range models {
		s.Equal("chat", m.Type)
		s.True(m.Enabled)
	}
}

// When upstream errors AND no static Models list is set AND no stale cache
// exists, the call still errors (hard failure path).
func (s *ProviderHandlersSuite) TestGetProviderModels_ErrorsWhenNoStaticListAndUpstreamFails() {
	endpoint := &types.ProviderEndpoint{
		Name:    "broken",
		Owner:   "user_id_3",
		BaseURL: "http://broken.local",
	}

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("connection refused"))

	_, _, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().Error(err)
}

// When upstream errors but a previously-cached "stale" snapshot exists, the
// handler must serve the stale models with a non-nil staleErr so the caller
// can mark the provider as degraded without dropping the model list. This is
// the core stale-while-revalidate behaviour that keeps the UI responsive when
// an intermittent provider goes briefly unreachable.
func (s *ProviderHandlersSuite) TestGetProviderModels_FallsBackToStaleOnUpstreamError() {
	endpoint := &types.ProviderEndpoint{
		Name:    "intermittent",
		Owner:   "user_id_4",
		BaseURL: "http://intermittent.local",
	}

	// Seed the long-TTL stale cache as if a previous successful fetch had populated it.
	staleKey := staleModelCacheKey(endpoint.Name, endpoint.Owner)
	s.server.cache.SetWithTTL(staleKey, `[{"id":"cached-model","object":"model"}]`, 1, time.Hour)
	s.server.cache.Wait()

	// Fresh entry is absent (TTL expired in the user's scenario).
	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("connection refused"))

	models, staleErr, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err, "served from stale cache, no hard error")
	s.Require().Error(staleErr, "staleErr must surface the underlying upstream failure")
	s.Contains(staleErr.Error(), "connection refused")
	s.Require().Len(models, 1, "stale cache models must be served")
	s.Equal("cached-model", models[0].ID)
}

// A successful fresh fetch must populate BOTH the short-TTL fresh entry and
// the long-TTL stale entry, otherwise the next outage has nothing to serve.
func (s *ProviderHandlersSuite) TestGetProviderModels_PopulatesBothFreshAndStaleOnSuccess() {
	endpoint := &types.ProviderEndpoint{
		Name:    "healthy",
		Owner:   "user_id_5",
		BaseURL: "http://healthy.local",
	}

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "good-model"}}, nil)
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	_, _, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)

	s.server.cache.Wait()

	freshKey := freshModelCacheKey(endpoint.Name, endpoint.Owner)
	staleKey := staleModelCacheKey(endpoint.Name, endpoint.Owner)
	_, foundFresh := s.server.cache.Get(freshKey)
	s.True(foundFresh, "fresh cache entry must be populated on success")
	_, foundStale := s.server.cache.Get(staleKey)
	s.True(foundStale, "stale cache entry must be populated on success so future outages have a fallback")
}

// invalidateProviderModelCache must clear BOTH cache keys — otherwise editing
// or deleting a provider could leave a stale entry alive for the long TTL,
// re-surfacing the wrong models on the next upstream blip.
func (s *ProviderHandlersSuite) TestInvalidateProviderModelCache_ClearsBothFreshAndStale() {
	name, owner := "to-invalidate", "user_id_6"
	freshKey := freshModelCacheKey(name, owner)
	staleKey := staleModelCacheKey(name, owner)

	s.server.cache.SetWithTTL(freshKey, `[{"id":"fresh"}]`, 1, time.Minute)
	s.server.cache.SetWithTTL(staleKey, `[{"id":"stale"}]`, 1, time.Hour)
	s.server.cache.Wait()

	s.server.invalidateProviderModelCache(name, owner)
	s.server.cache.Wait()

	_, foundFresh := s.server.cache.Get(freshKey)
	s.False(foundFresh, "fresh cache entry must be cleared")
	_, foundStale := s.server.cache.Get(staleKey)
	s.False(foundStale, "stale cache entry must also be cleared (otherwise a deleted provider keeps serving for hours)")
}

func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_SwitchUserToGlobal() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_123"

	adminCtx := setRequestUser(context.Background(), types.User{
		ID:    "admin_id",
		Admin: true,
	})

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "my-endpoint",
		BaseURL:      "http://localhost:11434",
		Owner:        "admin_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)
	s.store.EXPECT().UpdateProviderEndpoint(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
			s.Equal(types.ProviderEndpointTypeGlobal, ep.EndpointType)
			s.Equal(string(types.OwnerTypeSystem), ep.Owner)
			s.Equal(types.OwnerTypeSystem, ep.OwnerType)
			return ep, nil
		})

	update := types.UpdateProviderEndpoint{
		Name:         "my-endpoint",
		EndpointType: types.ProviderEndpointTypeGlobal,
		BaseURL:      "http://localhost:11434",
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/"+endpointID, bytes.NewReader(body))
	req = req.WithContext(adminCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Equal(http.StatusOK, rr.Code)
}

func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_NonAdminCannotSwitchToGlobal() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_123"

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "my-endpoint",
		BaseURL:      "http://localhost:11434",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)

	update := types.UpdateProviderEndpoint{
		Name:         "my-endpoint",
		EndpointType: types.ProviderEndpointTypeGlobal,
		BaseURL:      "http://localhost:11434",
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/"+endpointID, bytes.NewReader(body))
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Equal(http.StatusForbidden, rr.Code)
	s.Contains(rr.Body.String(), "Only admins can update global endpoints")
}

func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_SwitchToOrgRejected() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_123"

	adminCtx := setRequestUser(context.Background(), types.User{
		ID:    "admin_id",
		Admin: true,
	})

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "my-endpoint",
		BaseURL:      "http://localhost:11434",
		Owner:        "admin_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)

	update := types.UpdateProviderEndpoint{
		Name:         "my-endpoint",
		EndpointType: types.ProviderEndpointTypeOrg,
		BaseURL:      "http://localhost:11434",
	}
	body, _ := json.Marshal(update)

	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/"+endpointID, bytes.NewReader(body))
	req = req.WithContext(adminCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Equal(http.StatusBadRequest, rr.Code)
	s.Contains(rr.Body.String(), "Unsupported endpoint type switch")
}

// TestDeleteProviderEndpoint_InvalidatesModelCache verifies that deleting a
// provider clears its cached model list so subsequent reads stop serving the
// deleted provider's models. Without invalidation, stale entries linger for up
// to ModelsCacheTTL and a deleted provider can still resolve in /v1/chat/completions
// routing during that window. See deviqon/P1-3.
func (s *ProviderHandlersSuite) TestDeleteProviderEndpoint_InvalidatesModelCache() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_to_delete"

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "user-ollama",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	// Pre-populate the cache as if a prior /providers/models call had filled it.
	cacheKey := freshModelCacheKey(existing.Name, existing.Owner)
	s.server.cache.SetWithTTL(cacheKey, `[{"id":"qwen3-coder"}]`, 1, time.Minute)
	s.server.cache.Wait()
	if _, found := s.server.cache.Get(cacheKey); !found {
		s.Fail("precondition: cache should be populated before delete")
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)
	s.store.EXPECT().DeleteProviderEndpoint(gomock.Any(), endpointID).Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/v1/provider-endpoints/"+endpointID, nil)
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.deleteProviderEndpoint(rr, req)

	s.Equal(http.StatusOK, rr.Code)

	s.server.cache.Wait()
	_, found := s.server.cache.Get(cacheKey)
	s.False(found, "cache entry for deleted provider must be cleared")
}

// TestUpdateProviderEndpoint_RenameInvalidatesOldKey covers the rename path:
// the cache key includes the provider name, so a rename leaves a stranded
// entry under the old name that nothing will ever clean up. The update handler
// must drop both the old and new keys.
func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_RenameInvalidatesOldKey() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_rename"

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "old-name",
		BaseURL:      "http://localhost:11434",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	oldKey := freshModelCacheKey("old-name", "user_id")
	newKey := freshModelCacheKey("new-name", "user_id")
	s.server.cache.SetWithTTL(oldKey, `[{"id":"m1"}]`, 1, time.Minute)
	s.server.cache.SetWithTTL(newKey, `[{"id":"m2-stale"}]`, 1, time.Minute)
	s.server.cache.Wait()

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)
	s.manager.EXPECT().ListProviders(gomock.Any(), "user_id").Return([]types.Provider{}, nil)
	s.store.EXPECT().UpdateProviderEndpoint(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
			s.Equal("new-name", ep.Name)
			return ep, nil
		})

	update := types.UpdateProviderEndpoint{
		Name:    "new-name",
		BaseURL: "http://localhost:11434",
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/"+endpointID, bytes.NewReader(body))
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Equal(http.StatusOK, rr.Code)

	s.server.cache.Wait()
	_, foundOld := s.server.cache.Get(oldKey)
	s.False(foundOld, "old-name cache entry must be cleared after rename")
	_, foundNew := s.server.cache.Get(newKey)
	s.False(foundNew, "new-name cache entry must be cleared so next read refetches with current upstream state")
}

// TestUpdateProviderEndpoint_NoRenameStillInvalidates covers the case where
// only BaseURL/Models/APIKey changed — the cached models are now potentially
// wrong because the upstream is different. Cache must be cleared even when
// the name is unchanged.
func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_NoRenameStillInvalidates() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_url_change"

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "stable-name",
		BaseURL:      "http://old-host:11434",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeUser,
	}

	key := freshModelCacheKey("stable-name", "user_id")
	s.server.cache.SetWithTTL(key, `[{"id":"old-host-model"}]`, 1, time.Minute)
	s.server.cache.Wait()

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)
	s.store.EXPECT().UpdateProviderEndpoint(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, ep *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
			s.Equal("http://new-host:11434", ep.BaseURL)
			return ep, nil
		})

	update := types.UpdateProviderEndpoint{
		Name:    "stable-name",
		BaseURL: "http://new-host:11434",
	}
	body, _ := json.Marshal(update)
	req := httptest.NewRequest(http.MethodPut, "/v1/provider-endpoints/"+endpointID, bytes.NewReader(body))
	req = req.WithContext(s.authCtx)
	req = mux.SetURLVars(req, map[string]string{"id": endpointID})

	rr := httptest.NewRecorder()
	s.server.updateProviderEndpoint(rr, req)

	s.Equal(http.StatusOK, rr.Code)

	s.server.cache.Wait()
	_, found := s.server.cache.Get(key)
	s.False(found, "cache must be cleared after BaseURL change so next read sees the new upstream")
}
