package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type OpenAIEmbeddingsSuite struct {
	suite.Suite

	store        *store.MockStore
	openAiClient *openai.MockClient
	manager      *manager.MockProviderManager

	authCtx context.Context

	server *HelixAPIServer
}

func TestOpenAIEmbeddingsSuite(t *testing.T) {
	suite.Run(t, new(OpenAIEmbeddingsSuite))
}

func (s *OpenAIEmbeddingsSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())

	cfg := &config.ServerConfig{}
	cfg.RAG.PGVector.Provider = string(types.ProviderOpenAI)

	s.store = store.NewMockStore(ctrl)
	s.openAiClient = openai.NewMockClient(ctrl)
	s.manager = manager.NewMockProviderManager(ctrl)

	s.authCtx = setRequestUser(context.Background(), types.User{
		ID:       "user_id",
		Email:    "foo@email.com",
		FullName: "Foo Bar",
	})

	server := &HelixAPIServer{
		Cfg:             cfg,
		Store:           s.store,
		providerManager: s.manager,
	}

	s.server = server
}

func (s *OpenAIEmbeddingsSuite) TestCreateEmbeddingsStandardFormat() {
	req, err := http.NewRequest("POST", "/v1/embeddings", bytes.NewBufferString(`{
    "input": "The food was delicious and the waiter...",
    "model": "text-embedding-ada-002",
    "encoding_format": "float"
  }`))
	s.NoError(err)

	req = req.WithContext(s.authCtx)

	rec := httptest.NewRecorder()

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: string(types.ProviderOpenAI),
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().CreateFlexibleEmbeddings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req types.FlexibleEmbeddingRequest) (types.FlexibleEmbeddingResponse, error) {
			s.Equal("text-embedding-ada-002", req.Model)
			s.Equal("float", req.EncodingFormat)
			s.Equal("The food was delicious and the waiter...", req.Input)

			return types.FlexibleEmbeddingResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Index     int       `json:"index"`
					Embedding []float32 `json:"embedding"`
				}{
					{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
				},
				Model: "text-embedding-ada-002",
				Usage: struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				}{
					PromptTokens: 10,
					TotalTokens:  10,
				},
			}, nil
		})

	s.server.createEmbeddings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *OpenAIEmbeddingsSuite) TestCreateEmbeddingsWithChatFormat() {
	req, err := http.NewRequest("POST", "/v1/embeddings", bytes.NewBufferString(`{
    "model": "text-embedding-ada-002",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"},
      {"role": "assistant", "content": "The capital of France is Paris."}
    ],
    "encoding_format": "float"
  }`))
	s.NoError(err)

	req = req.WithContext(s.authCtx)

	rec := httptest.NewRecorder()

	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: string(types.ProviderOpenAI),
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().CreateFlexibleEmbeddings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req types.FlexibleEmbeddingRequest) (types.FlexibleEmbeddingResponse, error) {
			s.Equal("text-embedding-ada-002", req.Model)
			s.Equal("float", req.EncodingFormat)
			s.Len(req.Messages, 2)
			s.Equal("user", req.Messages[0].Role)
			s.Equal("What is the capital of France?", req.Messages[0].Content)
			s.Equal("assistant", req.Messages[1].Role)
			s.Equal("The capital of France is Paris.", req.Messages[1].Content)

			return types.FlexibleEmbeddingResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Index     int       `json:"index"`
					Embedding []float32 `json:"embedding"`
				}{
					{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
					{Object: "embedding", Index: 1, Embedding: []float32{0.4, 0.5, 0.6}},
				},
				Model: "text-embedding-ada-002",
				Usage: struct {
					PromptTokens int `json:"prompt_tokens"`
					TotalTokens  int `json:"total_tokens"`
				}{
					PromptTokens: 20,
					TotalTokens:  20,
				},
			}, nil
		})

	s.server.createEmbeddings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *OpenAIEmbeddingsSuite) TestRAGEmbeddingPlaceholderSubstitution() {
	req, err := http.NewRequest("POST", "/v1/embeddings", bytes.NewBufferString(`{
    "input": "test query",
    "model": "rag-embedding"
  }`))
	s.NoError(err)

	req = req.WithContext(s.authCtx)

	rec := httptest.NewRecorder()

	// Mock GetEffectiveSystemSettings returning configured RAG embedding model
	s.store.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{
		RAGEmbeddingsProvider: "openai",
		RAGEmbeddingsModel:    "text-embedding-3-small",
	}, nil)

	// Expect the client to be fetched with the configured provider (not the default)
	s.manager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "openai",
	}).Return(s.openAiClient, nil)

	s.openAiClient.EXPECT().CreateFlexibleEmbeddings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req types.FlexibleEmbeddingRequest) (types.FlexibleEmbeddingResponse, error) {
			// Model should be substituted to provider/model format
			s.Equal("openai/text-embedding-3-small", req.Model)

			return types.FlexibleEmbeddingResponse{
				Object: "list",
				Data: []struct {
					Object    string    `json:"object"`
					Index     int       `json:"index"`
					Embedding []float32 `json:"embedding"`
				}{
					{Object: "embedding", Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
				},
				Model: "text-embedding-3-small",
			}, nil
		})

	s.server.createEmbeddings(rec, req)

	s.Equal(http.StatusOK, rec.Code)
}

func (s *OpenAIEmbeddingsSuite) TestRAGEmbeddingPlaceholderNotConfigured() {
	req, err := http.NewRequest("POST", "/v1/embeddings", bytes.NewBufferString(`{
    "input": "test query",
    "model": "rag-embedding"
  }`))
	s.NoError(err)

	req = req.WithContext(s.authCtx)

	rec := httptest.NewRecorder()

	// Mock GetEffectiveSystemSettings returning empty settings (not configured)
	s.store.EXPECT().GetEffectiveSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)

	s.server.createEmbeddings(rec, req)

	s.Equal(http.StatusBadRequest, rec.Code)
	s.Contains(rec.Body.String(), "RAG embedding model not configured")
}
