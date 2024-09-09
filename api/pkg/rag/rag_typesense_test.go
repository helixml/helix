package rag

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

type TypesenseTestSuite struct {
	suite.Suite
	ctx context.Context

	ts *Typesense
}

func TestTypesenseTestSuite(t *testing.T) {
	suite.Run(t, new(TypesenseTestSuite))
}

func (suite *TypesenseTestSuite) SetupTest() {
	suite.ctx = context.Background()
	collectionName := "test-collection-" + system.GenerateID()

	cfg := &types.RAGSettings{}
	cfg.Typesense.URL = "http://localhost:8108"
	cfg.Typesense.APIKey = "typesense"
	cfg.Typesense.Collection = collectionName

	if os.Getenv("TYPESENSE_URL") != "" {
		cfg.Typesense.URL = os.Getenv("TYPESENSE_URL")
	}
	if os.Getenv("TYPESENSE_API_KEY") != "" {
		cfg.Typesense.APIKey = os.Getenv("TYPESENSE_API_KEY")
	}

	ts, err := NewTypesense(cfg)
	suite.Require().NoError(err)

	suite.NotNil(ts)

	suite.T().Logf("collectionName: %s", collectionName)

	suite.ts = ts
}

func (suite *TypesenseTestSuite) TestEnsureCollection() {
	err := suite.ts.ensureCollection(suite.ctx)
	suite.Require().NoError(err)
}

func (suite *TypesenseTestSuite) TestIndexAndQuery() {
	// Index sample data
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
		err := suite.ts.Index(suite.ctx, &doc)
		suite.Require().NoError(err)
	}

	// Wait for indexing to complete
	time.Sleep(2 * time.Second)

	// Query for documents
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
			results, err := suite.ts.Query(suite.ctx, &tc.query)
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
