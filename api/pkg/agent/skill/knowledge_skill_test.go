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

	skill := NewKnowledgeSkill(mockRAG, knowledge, []string{})
	assert.NotNil(t, skill)

	t.Run("BasicProperties", func(t *testing.T) {
		assert.Equal(t, "Knowledge_Test_Knowledge", skill.Name)
		assert.Equal(t, "Contains expert knowledge on topics: 'Test knowledge base for testing'", skill.Description)
		assert.Equal(t, knowledgeBaseMainPrompt, skill.SystemPrompt)
	})

	t.Run("ToolCount", func(t *testing.T) {
		assert.Equal(t, 1, len(skill.Tools))
	})

	t.Run("ToolProperties", func(t *testing.T) {
		tool := skill.Tools[0].(*KnowledgeQueryTool)
		assert.Equal(t, "KnowledgeQuery", tool.Name())
		assert.Equal(t, "Contains expert knowledge on topics: 'Test knowledge base for testing'", tool.Description())
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

	mockRAG := rag.NewMockRAG(ctrl)
	mockRAG.EXPECT().Query(context.Background(), &types.SessionRAGQuery{
		Prompt:            "test",
		DataEntityID:      knowledge.GetDataEntityID(),
		DistanceThreshold: knowledge.RAGSettings.Threshold,
		DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
		MaxResults:        knowledge.RAGSettings.ResultsCount,
		DocumentIDList:    []string{"doc1", "doc2"},
	}).Return([]*types.SessionRAGResult{
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
	}, nil)

	skill := NewKnowledgeSkill(mockRAG, knowledge, []string{"doc1", "doc2"})

	response, err := skill.Tools[0].(*KnowledgeQueryTool).Execute(context.Background(), agent.Meta{}, map[string]interface{}{"query": "test"})
	require.NoError(t, err)
	assert.Equal(t, "Source: test.txt\nContent: test content\n\n", response)

}
