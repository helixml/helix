package skill

import (
	"bytes"
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/jsonschema"
)

const knowledgeBaseMainPrompt = `TODO`

func NewKnowledgeSkill(ragClient rag.RAG, knowledge *types.Knowledge) agent.Skill {
	return agent.Skill{
		Name:         "knowledge base",
		Description:  fmt.Sprintf("Performs a search using the Helix knowledge base, ideal for finding information on a specific topic. This tool contains information on: %s", knowledge.Description),
		SystemPrompt: knowledgeBaseMainPrompt,
		Tools: []agent.Tool{
			&KnowledgeQueryTool{
				name:        "knowledge_query",
				description: "Performs a search using the Helix knowledge base, ideal for finding information on a specific topic. This tool contains information on: " + knowledge.Description,
				ragClient:   ragClient,
				knowledge:   knowledge,
			}},
	}
}

type KnowledgeQueryTool struct {
	name        string
	description string
	ragClient   rag.RAG
	knowledge   *types.Knowledge
}

var _ agent.Tool = &KnowledgeQueryTool{}

func (t *KnowledgeQueryTool) Name() string {
	return t.name
}

func (t *KnowledgeQueryTool) Description() string {
	return t.description
}

func (t *KnowledgeQueryTool) String() string {
	return t.name
}

func (t *KnowledgeQueryTool) StatusMessage() string {
	return "Searching the knowledge base"
}

func (t *KnowledgeQueryTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.name,
				Description: t.description,
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"query": {
							Type:        jsonschema.String,
							Description: "For query use concise, main keywords as the engine is performing both semantic and full text search",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}
}

func (t *KnowledgeQueryTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query is required")
	}

	results, err := t.ragClient.Query(ctx, &types.SessionRAGQuery{
		Prompt:            query,
		DataEntityID:      t.knowledge.GetDataEntityID(),
		DistanceThreshold: t.knowledge.RAGSettings.Threshold,
		DistanceFunction:  t.knowledge.RAGSettings.DistanceFunction,
		MaxResults:        t.knowledge.RAGSettings.ResultsCount,
		DocumentIDList:    []string{},
	})
	if err != nil {
		return "", fmt.Errorf("error querying RAG for knowledge %s: %w", t.knowledge.ID, err)
	}

	return formatKnowledgeSearchResponse(results), nil
}

// formatKnowledgeSearchResponse formats the results into a text with just Source and Content fields. Each section is separated by an empty line
//
// Source: <URL>
// Content: <Content>
// ...
// Source: <URL>
// Content: <Content>

func formatKnowledgeSearchResponse(results []*types.SessionRAGResult) string {
	if len(results) == 0 {
		return "No results found"
	}
	var buf bytes.Buffer
	for _, result := range results {
		buf.WriteString(fmt.Sprintf("Source: %s\nContent: %s\n\n", result.Source, result.Content))
	}

	return buf.String()
}
