package rag

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/pgvector/pgvector-go"

	oai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	gomock "go.uber.org/mock/gomock"
)

type PGVectorTestSuite struct {
	suite.Suite
	ctx  context.Context
	ctrl *gomock.Controller

	mockEmbeddingsStore *store.MockEmbeddingsStore
	mockProvider        *manager.MockProviderManager
	mockClient          *openai.MockClient
	pg                  *PGVector
}

func TestPGVectorTestSuite(t *testing.T) {
	suite.Run(t, new(PGVectorTestSuite))
}

func (suite *PGVectorTestSuite) SetupTest() {
	suite.ctx = context.Background()

	ctrl := gomock.NewController(suite.T())

	suite.ctrl = ctrl

	suite.mockProvider = manager.NewMockProviderManager(ctrl)
	suite.mockEmbeddingsStore = store.NewMockEmbeddingsStore(ctrl)
	suite.mockClient = openai.NewMockClient(ctrl)
	cfg := &config.ServerConfig{}
	cfg.RAG.PGVector.EmbeddingsModel = "thenlper/gte-small"
	cfg.Embeddings.Provider = "vllm"

	suite.pg = NewPGVector(cfg, suite.mockProvider, suite.mockEmbeddingsStore)
}

func (suite *PGVectorTestSuite) TestIndex_384_gte_small() {
	suite.mockProvider.EXPECT().GetClient(suite.ctx, &manager.GetClientRequest{
		Provider: "vllm",
	}).Return(suite.mockClient, nil)

	suite.mockClient.EXPECT().CreateEmbeddings(suite.ctx, oai.EmbeddingRequest{
		Model: "thenlper/gte-small",
		Input: "test-content",
	}).Return(oai.EmbeddingResponse{
		Data: []oai.Embedding{
			{
				Embedding: []float32{0.1, 0.2, 0.3},
			},
		},
	}, nil)

	vector := pgvector.NewVector([]float32{0.1, 0.2, 0.3})

	suite.mockEmbeddingsStore.EXPECT().CreateKnowledgeEmbedding(suite.ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, embeddings ...*types.KnowledgeEmbeddingItem) error {
		suite.Equal(embeddings[0].Embedding384, &vector)
		suite.Nil(embeddings[0].Embedding512)
		suite.Nil(embeddings[0].Embedding1024)
		suite.Nil(embeddings[0].Embedding1536)
		suite.Nil(embeddings[0].Embedding3584)

		suite.Equal("test-data-entity-id", embeddings[0].KnowledgeID)
		suite.Equal("test-document-id", embeddings[0].DocumentID)
		suite.Equal("test-document-group-id", embeddings[0].DocumentGroupID)
		suite.Equal("test-content", embeddings[0].Content)
		suite.Equal("test-source", embeddings[0].Source)

		return nil
	})

	err := suite.pg.Index(suite.ctx, &types.SessionRAGIndexChunk{
		DataEntityID:    "test-data-entity-id",
		DocumentID:      "test-document-id",
		DocumentGroupID: "test-document-group-id",
		Source:          "test-source",
		Content:         "test-content",
	})
	suite.Require().NoError(err)
}
