package skill

import (
	"bytes"
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/sashabaranov/go-openai"
)

const knowledgeBaseMainPrompt = `You are an expert at retrieving and synthesizing information from knowledge bases. Your role is to help users find relevant information by crafting effective search queries and presenting the results in a clear, organized manner.

Key responsibilities:
1. Query Formulation:
   - Create concise, focused search queries using key terms and concepts
   - Avoid overly broad or vague queries that might return irrelevant results
   - Use specific terminology when available to improve search accuracy

2. Result Analysis:
   - Review and synthesize information from multiple sources
   - Identify the most relevant and reliable information
   - Present information in a clear, structured format

3. Best Practices:
   - Always verify information from multiple sources when available
   - Clearly indicate the source of information
   - Acknowledge when information is not available or incomplete
   - Maintain objectivity and avoid making assumptions beyond the provided information

When using the knowledge query tool:
- Use the tool proactively when users need factual information
- Craft queries that are specific enough to find relevant information but broad enough to capture related content
- If initial results are insufficient, refine the query based on the context
- Present information in a clear, organized manner, citing sources appropriately

Remember: Your goal is to provide accurate, well-sourced information while maintaining clarity and relevance to the user's needs.`

func NewKnowledgeSkill(ragClient rag.RAG, knowledge *types.Knowledge, documentIDs []string) agent.Skill {
	return agent.Skill{
		Name:         "Knowledge_" + agent.SanitizeToolName(knowledge.Name),
		Description:  fmt.Sprintf("Contains expert knowledge on topics: '%s'", knowledge.Description),
		SystemPrompt: knowledgeBaseMainPrompt,
		Tools: []agent.Tool{
			&KnowledgeQueryTool{
				toolName:    "KnowledgeQuery",
				description: "Contains expert knowledge on topics: '" + knowledge.Description + "'",
				ragClient:   ragClient,
				knowledge:   knowledge,
				documentIDs: documentIDs,
			}},
	}
}

type KnowledgeQueryTool struct {
	toolName    string
	description string
	ragClient   rag.RAG
	knowledge   *types.Knowledge
	documentIDs []string // Filter by document IDs
}

var _ agent.Tool = &KnowledgeQueryTool{}

func (t *KnowledgeQueryTool) Name() string {
	return agent.SanitizeToolName(t.toolName)
}

func (t *KnowledgeQueryTool) Description() string {
	return t.description
}

func (t *KnowledgeQueryTool) String() string {
	return agent.SanitizeToolName(t.toolName)
}

func (t *KnowledgeQueryTool) StatusMessage() string {
	return "Searching the knowledge base"
}

func (t *KnowledgeQueryTool) Icon() string {
	return "SchoolIcon"
}

func (t *KnowledgeQueryTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        agent.SanitizeToolName(t.toolName),
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

func (t *KnowledgeQueryTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
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
		DocumentIDList:    t.documentIDs,
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
