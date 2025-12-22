package rag

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"

	oai "github.com/sashabaranov/go-openai"
	gomock "go.uber.org/mock/gomock"
)

type QdrantTestSuite struct {
	suite.Suite
	ctx  context.Context
	ctrl *gomock.Controller

	mockProvider *manager.MockProviderManager
	mockClient   *openai.MockClient
	qd           *Qdrant
}

func TestQdrantTestSuite(t *testing.T) {
	suite.Run(t, new(QdrantTestSuite))
}

func (suite *QdrantTestSuite) SetupTest() {
	suite.ctx = context.Background()

	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl

	suite.mockProvider = manager.NewMockProviderManager(ctrl)
	suite.mockClient = openai.NewMockClient(ctrl)

	collectionName := "test-collection-" + system.GenerateID()

	cfg := &config.ServerConfig{}
	cfg.RAG.Qdrant.Host = "localhost"
	cfg.RAG.Qdrant.Port = 6334
	cfg.RAG.Qdrant.APIKey = ""
	cfg.RAG.Qdrant.UseTLS = false
	cfg.RAG.Qdrant.Collection = collectionName
	cfg.RAG.Qdrant.Provider = "openai"
	cfg.RAG.Qdrant.EmbeddingsModel = "text-embedding-3-small"
	cfg.RAG.Qdrant.EmbeddingsConcurrency = 1
	cfg.RAG.Qdrant.Dimensions = 5

	if os.Getenv("QDRANT_HOST") != "" {
		cfg.RAG.Qdrant.Host = os.Getenv("QDRANT_HOST")
	}
	if port := os.Getenv("QDRANT_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			cfg.RAG.Qdrant.Port = p
		}
	}
	if os.Getenv("QDRANT_API_KEY") != "" {
		cfg.RAG.Qdrant.APIKey = os.Getenv("QDRANT_API_KEY")
	}
	if os.Getenv("QDRANT_USE_TLS") == "true" {
		cfg.RAG.Qdrant.UseTLS = true
	}

	qd, err := NewQdrant(cfg, suite.mockProvider)
	suite.Require().NoError(err)

	suite.NotNil(qd)

	suite.T().Logf("collectionName: %s", collectionName)

	suite.qd = qd
}

func (suite *QdrantTestSuite) Test_ensureReady() {
	err := suite.qd.ensureReady(suite.ctx)
	suite.Require().NoError(err)

	err = suite.qd.ensureReady(suite.ctx)
	suite.Require().NoError(err)
}

func (suite *QdrantTestSuite) TestIndexAndQuery() {
	suite.mockProvider.EXPECT().GetClient(suite.ctx, &manager.GetClientRequest{
		Provider: "openai",
	}).Return(suite.mockClient, nil).AnyTimes()

	suite.mockClient.EXPECT().CreateEmbeddings(suite.ctx, gomock.Any()).Return(oai.EmbeddingResponse{
		Data: []oai.Embedding{
			{
				Embedding: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
			},
		},
	}, nil).AnyTimes()

	sampleDocs := []types.SessionRAGIndexChunk{
		{
			DataEntityID:    "doc1",
			DocumentGroupID: "1",
			DocumentID:      "1",
			Source:          "test",
			Content:         "This is a sample document about AI.",
			ContentOffset:   0,
		},
		{
			DataEntityID:    "doc1",
			DocumentGroupID: "1",
			DocumentID:      "2",
			Source:          "test",
			Content:         "Machine learning is a subset of AI.",
			ContentOffset:   50,
		},
		{
			DataEntityID:    "doc2",
			DocumentGroupID: "2",
			DocumentID:      "3",
			Source:          "test",
			Content:         "Natural language processing is an important field in AI.",
			ContentOffset:   0,
		},
	}

	for _, doc := range sampleDocs {
		err := suite.qd.Index(suite.ctx, &doc)
		suite.Require().NoError(err)
	}

	testCases := []struct {
		name        string
		query       types.SessionRAGQuery
		expectedIDs []string
	}{
		{
			name: "Query for AI",
			query: types.SessionRAGQuery{
				DataEntityID: "doc1",
				Prompt:       "AI",
			},
			expectedIDs: []string{"1", "2"},
		},
		{
			name: "Query for NLP",
			query: types.SessionRAGQuery{
				DataEntityID: "doc2",
				Prompt:       "natural language processing",
			},
			expectedIDs: []string{"3"},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			results, err := suite.qd.Query(suite.ctx, &tc.query)
			suite.Require().NoError(err)
			suite.Require().Len(results, len(tc.expectedIDs))

			resultIDs := make([]string, len(results))
			for i, result := range results {
				resultIDs[i] = result.DocumentID
			}
			suite.ElementsMatch(tc.expectedIDs, resultIDs)
		})
	}
}

func (suite *QdrantTestSuite) TestIndexQueryAndDelete() {
	suite.mockProvider.EXPECT().GetClient(suite.ctx, &manager.GetClientRequest{
		Provider: "openai",
	}).Return(suite.mockClient, nil).AnyTimes()

	suite.mockClient.EXPECT().CreateEmbeddings(suite.ctx, gomock.Any()).Return(oai.EmbeddingResponse{
		Data: []oai.Embedding{
			{
				Embedding: []float32{0.1, 0.2, 0.3, 0.4, 0.5},
			},
		},
	}, nil).AnyTimes()

	sampleDocs := []types.SessionRAGIndexChunk{
		{
			DataEntityID:    "doc1",
			DocumentGroupID: "1",
			DocumentID:      "1",
			Source:          "test",
			Content:         "This is a sample document about AI.",
			ContentOffset:   0,
		},
		{
			DataEntityID:    "doc1",
			DocumentGroupID: "1",
			DocumentID:      "2",
			Source:          "test",
			Content:         "Machine learning is a subset of AI.",
			ContentOffset:   50,
		},
		{
			DataEntityID:    "doc2",
			DocumentGroupID: "2",
			DocumentID:      "3",
			Source:          "test",
			Content:         "Natural language processing is an important field in AI.",
			ContentOffset:   0,
		},
	}

	for _, doc := range sampleDocs {
		err := suite.qd.Index(suite.ctx, &doc)
		suite.Require().NoError(err)
	}

	query := types.SessionRAGQuery{
		DataEntityID: "doc1",
		Prompt:       "AI",
	}
	results, err := suite.qd.Query(suite.ctx, &query)
	suite.Require().NoError(err)
	suite.Require().Len(results, 2)

	results, err = suite.qd.Query(suite.ctx, &types.SessionRAGQuery{
		DataEntityID: "doc1",
		Prompt:       "AI",
		MaxResults:   1,
	})
	suite.Require().NoError(err)
	suite.Require().Len(results, 1, "Expected max results to be 1 as we specified it")

	deleteReq := &types.DeleteIndexRequest{
		DataEntityID: "doc1",
	}
	err = suite.qd.Delete(suite.ctx, deleteReq)
	suite.Require().NoError(err)

	results, err = suite.qd.Query(suite.ctx, &query)
	suite.Require().NoError(err)
	suite.Require().Len(results, 0, "Expected no results after deletion")

	query.DataEntityID = "doc2"
	query.Prompt = "natural language processing"
	results, err = suite.qd.Query(suite.ctx, &query)
	suite.Require().NoError(err)
	suite.Require().Len(results, 1, "Expected doc2 to still exist")
	suite.Equal("3", results[0].DocumentID)
}
