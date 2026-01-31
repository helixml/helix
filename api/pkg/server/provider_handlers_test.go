package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	cfg.ProvidersManagementEnabled = true // Enable provider management to avoid early-return path

	s.store = store.NewMockStore(ctrl)
	s.openAiClient = openai.NewMockClient(ctrl)
	s.manager = manager.NewMockProviderManager(ctrl)
	s.modelInfoProvider = model.NewMockModelInfoProvider(ctrl)

	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:       "user_id",
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	server := &HelixAPIServer{
		Cfg:               cfg,
		Store:             s.store,
		providerManager:   s.manager,
		modelInfoProvider: s.modelInfoProvider,
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
