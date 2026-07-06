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
			result, err := s.server.getProviderModels(context.Background(), ep)
			s.NoError(err)
			s.NoError(result.Degraded)
			s.Require().Len(result.Models, 1)
			s.Equal("llama3", result.Models[0].ID)
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

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(result.Degraded)
	s.Require().Len(result.Models, 1)
	s.Equal("hermes-agent", result.Models[0].ID)
	s.Equal("chat", result.Models[0].Type)
	s.True(result.Models[0].Enabled)
	s.Equal("hermes", result.Models[0].OwnedBy)
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

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(result.Degraded)
	s.Require().Len(result.Models, 2)
	s.Equal("hermes-agent", result.Models[0].ID)
	s.Equal("hermes-mini", result.Models[1].ID)
	for _, m := range result.Models {
		s.Equal("chat", m.Type)
		s.True(m.Enabled)
	}
}

// When upstream errors AND no static Models list is set AND no cache entry
// exists, the call must error (hard failure — nothing to serve).
func (s *ProviderHandlersSuite) TestGetProviderModels_ErrorsWhenNoStaticListAndUpstreamFails() {
	endpoint := &types.ProviderEndpoint{
		Name:    "broken",
		Owner:   "user_id_3",
		BaseURL: "http://broken.local",
	}

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("connection refused"))

	_, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().Error(err)
}

// Core SWR behaviour: when a cached payload exists but its FetchedAt is older
// than ModelsCacheTTL, the next read triggers a refresh. If the refresh fails
// with an upstream-unreachable error, the handler serves the cached models
// and marks Degraded so the API response can show "error" status without
// dropping the picker. This is the user-reported scenario the PR fixes.
func (s *ProviderHandlersSuite) TestGetProviderModels_ServesCachedWhenRefreshFailsTransiently() {
	endpoint := &types.ProviderEndpoint{
		Name:    "intermittent",
		Owner:   "user_id_4",
		BaseURL: "http://intermittent.local",
	}

	// Seed an expired-but-present cache entry.
	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	expired, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "cached-model", Object: "model"}},
		FetchedAt: time.Now().Add(-10 * time.Minute), // older than ModelsCacheTTL (1m)
	})
	s.server.cache.SetWithTTL(key, string(expired), 1, time.Hour)
	s.server.cache.Wait()

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("connection refused"))

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err, "served from cache, no hard error")
	s.Require().Error(result.Degraded, "Degraded must surface the underlying upstream failure")
	s.True(errors.Is(result.Degraded, errUpstreamUnreachable),
		"Degraded must wrap errUpstreamUnreachable so the failure class is identifiable")
	s.Contains(result.Degraded.Error(), "connection refused")
	s.Require().Len(result.Models, 1)
	s.Equal("cached-model", result.Models[0].ID)
}

// A provider-client construction failure (misconfigured/deleted provider) must
// NOT be masked by serving cached models, even when a cached payload exists.
// Otherwise editing a provider into a broken state silently keeps the old
// models around for up to modelCacheTTL.
func (s *ProviderHandlersSuite) TestGetProviderModels_GetClientErrorIsHardFailure() {
	endpoint := &types.ProviderEndpoint{
		Name:    "broken-config",
		Owner:   "user_id_3b",
		BaseURL: "http://broken-config.local",
	}

	// Seed a stale cache entry that MUST NOT be served on a config error.
	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	stale, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "stale-model"}},
		FetchedAt: time.Now().Add(-10 * time.Minute),
	})
	s.server.cache.SetWithTTL(key, string(stale), 1, time.Hour)
	s.server.cache.Wait()

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("provider not found"))

	_, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().Error(err, "config errors must propagate, not be masked by cache")
	s.False(errors.Is(err, errUpstreamUnreachable),
		"config errors must not be tagged as upstream-unreachable")
}

// Static Models list takes precedence over both the upstream call AND a stale
// cache entry — if the operator explicitly configured a model list, that's the
// declared source of truth. Captures the priority for the cross-axis case the
// previous tests didn't cover.
func (s *ProviderHandlersSuite) TestGetProviderModels_StaticListBeatsCacheOnUpstreamError() {
	endpoint := &types.ProviderEndpoint{
		Name:    "static-wins",
		Owner:   "user_id_4b",
		BaseURL: "http://static-wins.local",
		Models:  []string{"explicit-model"},
	}

	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	stale, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "old-cached-model"}},
		FetchedAt: time.Now().Add(-10 * time.Minute),
	})
	s.server.cache.SetWithTTL(key, string(stale), 1, time.Hour)
	s.server.cache.Wait()

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return(nil, errors.New("404"))
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(result.Degraded, "static list is the source of truth — not a degraded fallback")
	s.Require().Len(result.Models, 1)
	s.Equal("explicit-model", result.Models[0].ID,
		"static list must win over stale cache when upstream errors")
}

// A successful fresh fetch must persist to the cache so future refreshes can
// serve from it on transient outages. Verifies the entry is present and
// parseable.
func (s *ProviderHandlersSuite) TestGetProviderModels_PersistsToCacheOnSuccess() {
	endpoint := &types.ProviderEndpoint{
		Name:    "healthy",
		Owner:   "user_id_5",
		BaseURL: "http://healthy.local",
	}

	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "good-model"}}, nil)
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	_, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)

	s.server.cache.Wait()

	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	raw, found := s.server.cache.Get(key)
	s.Require().True(found, "cache entry must be persisted on success")

	var payload cachedModels
	s.Require().NoError(json.Unmarshal([]byte(raw), &payload))
	s.Require().Len(payload.Models, 1)
	s.Equal("good-model", payload.Models[0].ID)
	s.WithinDuration(time.Now(), payload.FetchedAt, 5*time.Second,
		"FetchedAt should reflect the fetch time so freshness can be evaluated on read")
}

// A fresh cache hit (FetchedAt within ModelsCacheTTL) must skip the upstream
// entirely — including the mock provider client. If GetClient gets called when
// the cache is fresh, we'd be paying the network cost on every request.
func (s *ProviderHandlersSuite) TestGetProviderModels_FreshHitSkipsUpstream() {
	endpoint := &types.ProviderEndpoint{
		Name:    "fresh-hit",
		Owner:   "user_id_5b",
		BaseURL: "http://fresh-hit.local",
	}

	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	fresh, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "cached-fresh"}},
		FetchedAt: time.Now(), // within ModelsCacheTTL
	})
	s.server.cache.SetWithTTL(key, string(fresh), 1, time.Hour)
	s.server.cache.Wait()

	// No EXPECT on the provider manager — calling GetClient here would fail the test.

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(result.Degraded)
	s.Require().Len(result.Models, 1)
	s.Equal("cached-fresh", result.Models[0].ID)
}

// invalidateProviderModelCache must clear the cache entry so a subsequent read
// refetches. Without this, deleting/renaming a provider leaves a payload alive
// for up to modelCacheTTL and resurfaces the wrong models on the next call.
func (s *ProviderHandlersSuite) TestInvalidateProviderModelCache_ClearsEntry() {
	name, owner := "to-invalidate", "user_id_6"
	key := modelCacheKey(name, owner)

	payload, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "to-clear"}},
		FetchedAt: time.Now(),
	})
	s.server.cache.SetWithTTL(key, string(payload), 1, time.Hour)
	s.server.cache.Wait()

	s.server.invalidateProviderModelCache(name, owner)
	s.server.cache.Wait()

	_, found := s.server.cache.Get(key)
	s.False(found, "cache entry must be cleared so a deleted/renamed provider stops serving")
}

// A cache entry containing malformed JSON must be dropped so the next read
// repopulates cleanly, rather than tripping the same unmarshal error every
// call. Verifies the corrupted-entry recovery path in loadCachedModels.
func (s *ProviderHandlersSuite) TestGetProviderModels_DropsCorruptCacheEntry() {
	endpoint := &types.ProviderEndpoint{
		Name:    "corrupt",
		Owner:   "user_id_6b",
		BaseURL: "http://corrupt.local",
	}

	key := modelCacheKey(endpoint.Name, endpoint.Owner)
	s.server.cache.SetWithTTL(key, `{not valid json`, 1, time.Hour)
	s.server.cache.Wait()

	// Corrupt entry should be dropped and the refresh path entered.
	s.manager.EXPECT().GetClient(gomock.Any(), gomock.Any()).Return(s.openAiClient, nil)
	s.openAiClient.EXPECT().ListModels(gomock.Any()).Return([]types.OpenAIModel{{ID: "refetched"}}, nil)
	s.modelInfoProvider.EXPECT().GetModelInfo(gomock.Any(), gomock.Any()).Return(nil, errors.New("not found"))

	result, err := s.server.getProviderModels(context.Background(), endpoint)
	s.Require().NoError(err)
	s.NoError(result.Degraded)
	s.Require().Len(result.Models, 1)
	s.Equal("refetched", result.Models[0].ID)
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
			// Owner must stay the admin creator, NOT be reassigned to "system".
			// Reassigning to "system" made the row look like a synthetic env-var
			// endpoint and stranded it as read-only in the UI.
			s.Equal("admin_id", ep.Owner)
			s.Equal(types.OwnerTypeUser, ep.OwnerType)
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

// A non-admin who owns a global endpoint (e.g. one an admin switched to global,
// which now retains the original owner) must not be able to edit it, even via a
// request that omits endpoint_type. The gate keys on the stored row's type.
func (s *ProviderHandlersSuite) TestUpdateProviderEndpoint_NonAdminCannotEditGlobal() {
	s.server.Cfg.Providers.EnableCustomUserProviders = true
	endpointID := "ep_123"

	existing := &types.ProviderEndpoint{
		ID:           endpointID,
		Name:         "my-endpoint",
		BaseURL:      "http://localhost:11434",
		Owner:        "user_id",
		OwnerType:    types.OwnerTypeUser,
		EndpointType: types.ProviderEndpointTypeGlobal,
	}

	s.store.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		ProvidersManagementEnabled: true,
	}, nil)
	s.store.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
		ID: endpointID,
	}).Return(existing, nil)

	// Payload deliberately omits endpoint_type — the attack that skipped the old
	// payload-only gate.
	update := types.UpdateProviderEndpoint{
		Name:    "my-endpoint",
		BaseURL: "http://evil.example.com",
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
	cacheKey := modelCacheKey(existing.Name, existing.Owner)
	payload, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "qwen3-coder"}},
		FetchedAt: time.Now(),
	})
	s.server.cache.SetWithTTL(cacheKey, string(payload), 1, time.Hour)
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

	oldKey := modelCacheKey("old-name", "user_id")
	newKey := modelCacheKey("new-name", "user_id")
	oldPayload, _ := json.Marshal(cachedModels{Models: []types.OpenAIModel{{ID: "m1"}}, FetchedAt: time.Now()})
	newPayload, _ := json.Marshal(cachedModels{Models: []types.OpenAIModel{{ID: "m2-stale"}}, FetchedAt: time.Now()})
	s.server.cache.SetWithTTL(oldKey, string(oldPayload), 1, time.Hour)
	s.server.cache.SetWithTTL(newKey, string(newPayload), 1, time.Hour)
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

	key := modelCacheKey("stable-name", "user_id")
	payload, _ := json.Marshal(cachedModels{Models: []types.OpenAIModel{{ID: "old-host-model"}}, FetchedAt: time.Now()})
	s.server.cache.SetWithTTL(key, string(payload), 1, time.Hour)
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

// TestGetCachedModels_DecodesWrappedPayload guards the cache-format unification.
// The cache stores the wrapped cachedModels{Models,FetchedAt} payload (the only
// writer is refreshProviderModels). getCachedModels previously unmarshalled the
// raw entry into a bare []OpenAIModel, which always failed — leaving the
// aggregate /v1/models empty for env-baked global providers. It must now decode
// the wrapped shape.
func (s *ProviderHandlersSuite) TestGetCachedModels_DecodesWrappedPayload() {
	key := modelCacheKey("openai", string(types.OwnerTypeSystem))
	payload, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "gpt-4o"}, {ID: "gpt-4o-mini"}},
		FetchedAt: time.Now(),
	})
	s.server.cache.SetWithTTL(key, string(payload), 1, time.Hour)
	s.server.cache.Wait()

	models := s.server.getCachedModels(key)
	s.Require().Len(models, 2, "wrapped cache payload must decode into models")
	s.Equal("gpt-4o", models[0].ID)
	s.Equal("gpt-4o-mini", models[1].ID)
}

// TestFindProviderWithModel_ResolvesFromWrappedCache guards that chat-completion
// routing can resolve a bare model id against a dynamically-fetched provider's
// cached model list. Before the fix findProviderWithModel read the cache as a
// bare []OpenAIModel and silently missed the wrapped payload, so only providers
// with a static Models list ever resolved.
func (s *ProviderHandlersSuite) TestFindProviderWithModel_ResolvesFromWrappedCache() {
	// No env-baked globals in this test.
	s.manager.EXPECT().ListProviders(gomock.Any(), "").Return([]types.Provider{}, nil)

	// One DB provider with NO static Models list — only the dynamic cache can match.
	s.store.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return([]*types.ProviderEndpoint{
		{Name: "dynprov", Owner: "user_id", BaseURL: "http://dynprov.local"},
	}, nil)

	key := modelCacheKey("dynprov", "user_id")
	payload, _ := json.Marshal(cachedModels{
		Models:    []types.OpenAIModel{{ID: "some-dynamic-model"}},
		FetchedAt: time.Now(),
	})
	s.server.cache.SetWithTTL(key, string(payload), 1, time.Hour)
	s.server.cache.Wait()

	provider, bareModel := s.server.findProviderWithModel(context.Background(), "some-dynamic-model", "user_id", "")
	s.Equal("dynprov", provider, "must resolve provider from wrapped cache payload")
	s.Equal("some-dynamic-model", bareModel)
}
