package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
)

func TestHaystackRAGSuite(t *testing.T) {
	suite.Run(t, new(HaystackRAGSuite))
}

type HaystackRAGSuite struct {
	suite.Suite

	haystackRAG *HaystackRAG
	server      *httptest.Server
}

func (suite *HaystackRAGSuite) SetupTest() {
	// Create a test server that will handle our mock responses
	suite.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// We'll implement the handler in our tests
	}))
	suite.haystackRAG = NewHaystackRAG(suite.server.URL)
}

func (suite *HaystackRAGSuite) TearDownTest() {
	suite.server.Close()
}

func (suite *HaystackRAGSuite) TestQuery() {
	testCases := []struct {
		name           string
		query          *types.SessionRAGQuery
		expectedQuery  QueryRequest
		mockResponse   QueryResponse
		expectedError  bool
		expectedResult []*types.SessionRAGResult
	}{
		{
			name: "single document ID filter",
			query: &types.SessionRAGQuery{
				Prompt:         "test query",
				DataEntityID:   "entity1",
				DocumentIDList: []string{"doc1"},
				MaxResults:     5,
			},
			expectedQuery: QueryRequest{
				Query: "test query",
				Filters: QueryFilter{
					Operator: "AND",
					Conditions: []Condition{
						{
							Field:    "meta.data_entity_id",
							Operator: "==",
							Value:    "entity1",
						},
						{
							Operator: "OR",
							Conditions: []Condition{
								{
									Field:    "meta.document_id",
									Operator: "==",
									Value:    "doc1",
								},
							},
						},
					},
				},
				TopK: 5,
			},
			mockResponse: QueryResponse{
				Results: []QueryResult{
					{
						Content: "test content",
						Metadata: ResultMetadata{
							DocumentID:      "doc1",
							DocumentGroupID: "group1",
							Source:          "test.txt",
							ContentOffset:   0,
						},
						Score: 0.95,
					},
				},
			},
			expectedResult: []*types.SessionRAGResult{
				{
					Content:         "test content",
					Distance:        0.95,
					DocumentID:      "doc1",
					DocumentGroupID: "group1",
					Source:          "test.txt",
					ContentOffset:   0,
				},
			},
		},
		{
			name: "multiple document ID filter",
			query: &types.SessionRAGQuery{
				Prompt:         "test query",
				DataEntityID:   "entity1",
				DocumentIDList: []string{"doc1", "doc2"},
				MaxResults:     5,
			},
			expectedQuery: QueryRequest{
				Query: "test query",
				Filters: QueryFilter{
					Operator: "AND",
					Conditions: []Condition{
						{
							Field:    "meta.data_entity_id",
							Operator: "==",
							Value:    "entity1",
						},
						{
							Operator: "OR",
							Conditions: []Condition{
								{
									Field:    "meta.document_id",
									Operator: "==",
									Value:    "doc1",
								},
								{
									Field:    "meta.document_id",
									Operator: "==",
									Value:    "doc2",
								},
							},
						},
					},
				},
				TopK: 5,
			},
			mockResponse: QueryResponse{
				Results: []QueryResult{
					{
						Content: "test content 1",
						Metadata: ResultMetadata{
							DocumentID:      "doc1",
							DocumentGroupID: "group1",
							Source:          "test1.txt",
							ContentOffset:   0,
						},
						Score: 0.95,
					},
					{
						Content: "test content 2",
						Metadata: ResultMetadata{
							DocumentID:      "doc2",
							DocumentGroupID: "group1",
							Source:          "test2.txt",
							ContentOffset:   0,
						},
						Score: 0.85,
					},
				},
			},
			expectedResult: []*types.SessionRAGResult{
				{
					Content:         "test content 1",
					Distance:        0.95,
					DocumentID:      "doc1",
					DocumentGroupID: "group1",
					Source:          "test1.txt",
					ContentOffset:   0,
				},
				{
					Content:         "test content 2",
					Distance:        0.85,
					DocumentID:      "doc2",
					DocumentGroupID: "group1",
					Source:          "test2.txt",
					ContentOffset:   0,
				},
			},
		},
		{
			name: "no document ID filter",
			query: &types.SessionRAGQuery{
				Prompt:         "test query",
				DataEntityID:   "entity1",
				DocumentIDList: []string{},
				MaxResults:     5,
			},
			expectedQuery: QueryRequest{
				Query: "test query",
				Filters: QueryFilter{
					Operator: "AND",
					Conditions: []Condition{
						{
							Field:    "meta.data_entity_id",
							Operator: "==",
							Value:    "entity1",
						},
					},
				},
				TopK: 5,
			},
			mockResponse: QueryResponse{
				Results: []QueryResult{},
			},
			expectedResult: []*types.SessionRAGResult{},
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Update the test server handler for this test case
			suite.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				suite.Equal("/query", r.URL.Path)
				suite.Equal("application/json", r.Header.Get("Content-Type"))

				// Read and verify the request body
				var requestBody QueryRequest
				err := json.NewDecoder(r.Body).Decode(&requestBody)
				suite.NoError(err)
				suite.Equal(tc.expectedQuery, requestBody)

				// Send mock response
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(tc.mockResponse)
			})

			// Execute the query
			results, err := suite.haystackRAG.Query(context.Background(), tc.query)

			if tc.expectedError {
				suite.Error(err)
			} else {
				suite.NoError(err)
				suite.Equal(tc.expectedResult, results)
			}
		})
	}
}
