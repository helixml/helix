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

	oai "github.com/sashabaranov/go-openai"
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

func (s *OpenAIEmbeddingsSuite) TestCreateEmbeddings() {
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

	s.openAiClient.EXPECT().CreateEmbeddings(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req oai.EmbeddingRequest) (oai.EmbeddingResponse, error) {
			s.Equal("text-embedding-ada-002", string(req.Model))
			s.Equal("float", string(req.EncodingFormat))
			s.Equal("The food was delicious and the waiter...", req.Input)

			return oai.EmbeddingResponse{
				Data: []oai.Embedding{
					{Index: 0, Embedding: []float32{0.1, 0.2, 0.3}},
				},
			}, nil
		})

	s.server.createEmbeddings(rec, req)

	s.Equal(http.StatusOK, rec.Code)

}
