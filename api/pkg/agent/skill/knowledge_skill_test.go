package skill

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewKnowledgeSkill(t *testing.T) {
	knowledge := &types.Knowledge{
		Name:        "Test Knowledge",
		Description: "Test knowledge base for testing",
		RAGSettings: types.RAGSettings{
			Threshold:        0.8,
			DistanceFunction: "cosine",
			ResultsCount:     5,
		},
	}

	// Create a mock RAG client
	var mockRAG rag.RAG

	skill := NewKnowledgeSkill(mockRAG, knowledge, []string{}, nil)
	assert.NotNil(t, skill)

	t.Run("BasicProperties", func(t *testing.T) {
		assert.Equal(t, "Knowledge_Test_Knowledge", skill.Name)
		assert.Equal(t, "Test knowledge base for testing", skill.Description)
		assert.True(t, skill.Direct)
		assert.Equal(t, knowledgeSkillParameters, skill.Parameters)
	})

	t.Run("ToolCount", func(t *testing.T) {
		assert.Equal(t, 1, len(skill.Tools))
	})

	t.Run("ToolProperties", func(t *testing.T) {
		tool := skill.Tools[0].(*KnowledgeQueryTool)
		assert.Equal(t, "KnowledgeQuery", tool.Name())
		assert.Equal(t, "Test knowledge base for testing", tool.Description())
		assert.Equal(t, "SchoolIcon", tool.Icon())
		assert.Equal(t, "Searching the knowledge base", tool.StatusMessage())
	})

	t.Run("OpenAISchema", func(t *testing.T) {
		tool := skill.Tools[0].(*KnowledgeQueryTool)
		openAiSpec := tool.OpenAI()

		parameters := openAiSpec[0].Function.Parameters.(jsonschema.Definition)

		queryProperty, ok := parameters.Properties["query"]
		require.True(t, ok)

		// Check query property description and type
		assert.Equal(t, "For query use concise, main keywords as the engine is performing both semantic and full text search", queryProperty.Description)
		assert.Equal(t, jsonschema.String, queryProperty.Type)

		// Check that query is required
		assert.Equal(t, []string{"query"}, parameters.Required)
	})
}

func TestNewKnowledgeSkill_Execute_WithFiltering(t *testing.T) {
	knowledge := &types.Knowledge{
		Name:        "Test Knowledge",
		Description: "Test knowledge base for testing",
		RAGSettings: types.RAGSettings{
			Threshold:        0.8,
			DistanceFunction: "cosine",
			ResultsCount:     5,
		},
	}

	ctrl := gomock.NewController(t)

	ragResults := []*types.SessionRAGResult{
		{
			Content:         "test content",
			Source:          "test.txt",
			DocumentID:      "doc1",
			DocumentGroupID: "group1",
			ContentOffset:   0,
			Metadata: map[string]string{
				"document_id":       "doc1",
				"document_group_id": "group1",
				"source":            "test.txt",
				"content_offset":    "0",
			},
		},
	}

	mockRAG := rag.NewMockRAG(ctrl)
	mockRAG.EXPECT().Query(context.Background(), &types.SessionRAGQuery{
		Prompt:            "test",
		DataEntityID:      knowledge.GetDataEntityID(),
		DistanceThreshold: knowledge.RAGSettings.Threshold,
		DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
		MaxResults:        knowledge.RAGSettings.ResultsCount,
		DocumentIDList:    []string{"doc1", "doc2"},
	}).Return(ragResults, nil)

	var collectedResults []*types.SessionRAGResult
	collector := func(results []*types.SessionRAGResult) {
		collectedResults = append(collectedResults, results...)
	}

	skill := NewKnowledgeSkill(mockRAG, knowledge, []string{"doc1", "doc2"}, collector)

	response, err := skill.Tools[0].(*KnowledgeQueryTool).Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "test"})
	require.NoError(t, err)
	assert.Contains(t, response, "<chunk>\n  <document_id>doc1</document_id>")
	assert.Contains(t, response, "test content")

	// Verify the collector received the RAG results
	require.Len(t, collectedResults, 1)
	assert.Equal(t, "doc1", collectedResults[0].DocumentID)
	assert.Equal(t, "test.txt", collectedResults[0].Source)
	assert.Equal(t, "test content", collectedResults[0].Content)
}

func TestNewKnowledgeSkill_Execute_NilCollector(t *testing.T) {
	knowledge := &types.Knowledge{
		Name:        "Test Knowledge",
		Description: "Test knowledge base for testing",
		RAGSettings: types.RAGSettings{
			Threshold:        0.8,
			DistanceFunction: "cosine",
			ResultsCount:     5,
		},
	}

	ctrl := gomock.NewController(t)

	mockRAG := rag.NewMockRAG(ctrl)
	mockRAG.EXPECT().Query(context.Background(), &types.SessionRAGQuery{
		Prompt:            "test",
		DataEntityID:      knowledge.GetDataEntityID(),
		DistanceThreshold: knowledge.RAGSettings.Threshold,
		DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
		MaxResults:        knowledge.RAGSettings.ResultsCount,
		DocumentIDList:    []string{},
	}).Return([]*types.SessionRAGResult{
		{
			Content:    "result content",
			Source:     "file.txt",
			DocumentID: "abc123",
		},
	}, nil)

	// nil collector should not panic
	skill := NewKnowledgeSkill(mockRAG, knowledge, []string{}, nil)

	response, err := skill.Tools[0].(*KnowledgeQueryTool).Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "test"})
	require.NoError(t, err)
	assert.Contains(t, response, "<chunk>\n  <document_id>abc123</document_id>")
	assert.Contains(t, response, "result content")
}
