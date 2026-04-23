package skill

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
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
var _ agent.ToolWithImages = &KnowledgeQueryTool{}

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

func (t *KnowledgeQueryTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	text, _, err := t.ExecuteWithImages(ctx, meta, args)
	return text, err
}

func (t *KnowledgeQueryTool) ExecuteWithImages(ctx context.Context, _ agent.Meta, args map[string]any) (string, []openai.ChatMessagePart, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", nil, fmt.Errorf("query is required")
	}

	log.Debug().
		Str("knowledge", t.knowledge.Name).
		Str("query", query).
		Int("doc_filter_count", len(t.documentIDs)).
		Msg("executing knowledge query")

	results, err := t.ragClient.Query(ctx, &types.SessionRAGQuery{
		Prompt:            query,
		DataEntityID:      t.knowledge.GetDataEntityID(),
		DistanceThreshold: t.knowledge.RAGSettings.Threshold,
		DistanceFunction:  t.knowledge.RAGSettings.DistanceFunction,
		MaxResults:        t.knowledge.RAGSettings.ResultsCount,
		DocumentIDList:    t.documentIDs,
	})
	if err != nil {
		return "", nil, fmt.Errorf("error querying RAG for knowledge %s: %w", t.knowledge.ID, err)
	}

	log.Debug().
		Str("knowledge", t.knowledge.Name).
		Int("result_count", len(results)).
		Msg("knowledge RAG query completed")

	// Render page images for visual search results.
	var images []openai.ChatMessagePart
	if renderer, ok := t.ragClient.(rag.PageImageRenderer); ok {
		images = renderPageImages(ctx, renderer, t.knowledge.GetDataEntityID(), results)
	}

	if t.resultsCollector != nil {
		t.resultsCollector(results)
	}

	return formatKnowledgeSearchResponse(results), images, nil
}

// renderPageImages renders document page images for RAG results that have
// page metadata and returns them as multimodal chat message parts.
func renderPageImages(ctx context.Context, renderer rag.PageImageRenderer, dataEntityID string, results []*types.SessionRAGResult) []openai.ChatMessagePart {
	var parts []openai.ChatMessagePart
	for _, result := range results {
		pageStr, has := result.Metadata["page_number"]
		if !has {
			continue
		}
		page, parseErr := strconv.Atoi(pageStr)
		if parseErr != nil || page <= 0 {
			log.Warn().
				Str("page_number", pageStr).
				Str("source", result.Source).
				Msg("invalid page_number metadata in RAG result, skipping image render")
			continue
		}
		imgBytes, renderErr := renderer.RenderPageImage(ctx, dataEntityID, result.Source, page)
		if renderErr != nil {
			log.Warn().Err(renderErr).
				Str("source", result.Source).
				Int("page", page).
				Msg("failed to render page image, skipping")
			continue
		}
		dataURI := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imgBytes)
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeText,
			Text: fmt.Sprintf("Page %d of %s (document_id: %s):", page, result.Source, result.DocumentID),
		})
		parts = append(parts, openai.ChatMessagePart{
			Type:     openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{URL: dataURI},
		})
	}
	if len(parts) > 0 {
		log.Info().Int("image_count", len(parts)/2).Msg("rendered page images for knowledge query")
	}
	return parts
}

// formatKnowledgeSearchResponse formats RAG results as chunks with document IDs,
// content boundary markers, and citation formatting instructions. The formatting
// instructions are here (not in the system prompt) because they need to be proximate
// to the data for the LLM to follow them reliably.
func formatKnowledgeSearchResponse(results []*types.SessionRAGResult) string {
	if len(results) == 0 {
		return "No results found"
	}
	var buf bytes.Buffer
	for _, result := range results {
		// Skip results with empty content (e.g. page_image enrichments
		// that only carry an embedding vector for visual search).
		if strings.TrimSpace(result.Content) == "" {
			continue
		}
		buf.WriteString(fmt.Sprintf("<chunk>\n  <document_id>%s</document_id>\n  <content>\n    ### START OF CONTENT FOR DOCUMENT %s ###\n    %s\n    ### END OF CONTENT FOR DOCUMENT %s ###\n  </content>\n</chunk>\n",
			result.DocumentID, result.DocumentID, result.Content, result.DocumentID))
	}

	buf.WriteString("\nProvide references in your answer in the format `[DOC_ID:DocumentID]`. " +
		"For example, \"According to [DOC_ID:f6962c8007], the answer is 42.\"\n\n" +
		"After your answer, write an excerpts block with an exact quote from each cited document:\n\n" +
		"<excerpts>\n  <excerpt>\n    <document_id>DocumentID</document_id>\n    <snippet>An exact quote from this document</snippet>\n  </excerpt>\n</excerpts>\n\n" +
		"Each document_id must appear at most once in the excerpts. No header before the excerpts block.")

	return buf.String()
}
