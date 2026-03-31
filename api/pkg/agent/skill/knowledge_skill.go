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

var knowledgeSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"query": {
			Type:        jsonschema.String,
			Description: "A search query string to find relevant information in the knowledge base.",
		},
	},
	Required: []string{"query"},
}

// RAGResultsCollector is a callback that receives RAG results from knowledge queries.
// It is called each time a knowledge query executes, allowing the controller to
// accumulate results for session metadata (citations).
type RAGResultsCollector func(results []*types.SessionRAGResult)

func NewKnowledgeSkill(ragClient rag.RAG, knowledge *types.Knowledge, documentIDs []string, resultsCollector RAGResultsCollector) agent.Skill {
	description := knowledge.Description
	if description == "" {
		description = "Search the '" + knowledge.Name + "' knowledge base."
	}
	return agent.Skill{
		Name:        "Knowledge_" + agent.SanitizeToolName(knowledge.Name),
		Description: description,
		Parameters:  knowledgeSkillParameters,
		Direct:      true,
		// SystemPrompt: "", // NOTE: This is not used by the agent. It does not work.
		Tools: []agent.Tool{
			&KnowledgeQueryTool{
				toolName:         "KnowledgeQuery",
				description:      description,
				ragClient:        ragClient,
				knowledge:        knowledge,
				documentIDs:      documentIDs,
				resultsCollector: resultsCollector,
			}},
	}
}

type KnowledgeQueryTool struct {
	toolName         string
	description      string
	ragClient        rag.RAG
	knowledge        *types.Knowledge
	documentIDs      []string // Filter by document IDs
	resultsCollector RAGResultsCollector
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

	if t.resultsCollector != nil {
		t.resultsCollector(results)
	}

	return formatKnowledgeSearchResponse(results), nil
}

// formatKnowledgeSearchResponse formats RAG results as chunks with document IDs and
// content boundary markers. Citation and excerpts formatting instructions are in the
// system prompt (inference_agent.go), not here.
func formatKnowledgeSearchResponse(results []*types.SessionRAGResult) string {
	if len(results) == 0 {
		return "No results found"
	}
	var buf bytes.Buffer
	for _, result := range results {
		buf.WriteString(fmt.Sprintf("<chunk>\n  <document_id>%s</document_id>\n  <content>\n    ### START OF CONTENT FOR DOCUMENT %s ###\n    %s\n    ### END OF CONTENT FOR DOCUMENT %s ###\n  </content>\n</chunk>\n",
			result.DocumentID, result.DocumentID, result.Content, result.DocumentID))
	}
	return buf.String()
}
