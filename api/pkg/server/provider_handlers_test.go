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
	cfg.RAG.PGVector.Provider = string(types.ProviderOpenAI)

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
			models, err := s.server.getProviderModels(context.Background(), ep)
			s.NoError(err)
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
