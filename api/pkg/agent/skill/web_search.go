package skill

import (
	"bytes"
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/helixml/helix/api/pkg/websearch"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func NewSearchSkill(config *types.ToolWebSearchConfig, provider websearch.Provider) agent.Skill {
	return agent.Skill{
		Name:        "WebSearch",
		Description: "Search the web for current information and recent data, can open URLs, look for information in the page and return the results.",
		SystemPrompt: `You are a web search expert that can search the internet for current information. 
		Use the search tool to find recent news, facts, or any up-to-date information that the user requests.
		Do not try to answer the question yourself, just use the search tool to find the information and present it
		to the user in a structured way. Use the browser tool to deep dive into the information. If you think multiple URLs should be visited, then
		return a list of individual tool calls to the browser tool.`,
		Tools: []agent.Tool{
			&searchTool{
				config:   config,
				provider: provider,
			},
			&browserTool{provider: provider},
		},
	}
}

type searchTool struct {
	config   *types.ToolWebSearchConfig
	provider websearch.Provider
}

// searchSnippetChars bounds the page text shown per result: enough for the
// model to judge relevance before opening the page with the browser tool.
const searchSnippetChars = 1000

func (t *searchTool) String() string {
	return "Web Search Tool"
}

func (t *searchTool) Name() string {
	return "web_search"
}

func (t *searchTool) Icon() string {
	return "🔍"
}

func (t *searchTool) StatusMessage() string {
	return "Searching the web..."
}

func (t *searchTool) Description() string {
	return "Search the web for current information. Use this when you need to find recent or up-to-date information about topics, news, events, or any current data."
}

func (t *searchTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "web_search",
				Description: "Search the web for current information. Use this when you need to find recent or up-to-date information about topics, news, events, or any current data.",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"query": {
							Type:        jsonschema.String,
							Description: "The search query to look up on the web",
						},
						"category": {
							Type:        jsonschema.String,
							Description: "Optional category for the search (general, news)",
							Enum:        []string{searchCategoryGeneral, websearch.CategoryNews},
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}
}

const searchCategoryGeneral = "general"

func (t *searchTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query parameter is required and must be a string")
	}
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	req := &websearch.SearchRequest{
		Query:      query,
		NumResults: t.config.MaxResults,
		Contents: &websearch.ContentsOptions{
			Text: &websearch.TextOption{Enabled: true, TextOptions: websearch.TextOptions{MaxCharacters: searchSnippetChars}},
		},
	}
	switch category, _ := args["category"].(string); category {
	case "", searchCategoryGeneral:
	case websearch.CategoryNews:
		req.Category = websearch.CategoryNews
	default:
		return "", fmt.Errorf("unsupported category %q: use general or news", category)
	}

	log.Debug().Str("query", query).Str("category", req.Category).Msg("Executing web search")

	resp, err := t.provider.Search(withWebSearchCaller(ctx, meta), req)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	return formatSearchResponse(resp.Results), nil
}

// withWebSearchCaller attributes a skill's web search request to the agent
// turn's user and organization: the same wallet the turn's LLM calls are
// billed to (the organization's when the session has one, else the user's).
// The skill is the request's only owner, so it assigns the request id.
func withWebSearchCaller(ctx context.Context, meta agent.Meta) context.Context {
	orgID, _ := oai.GetContextOrganizationID(ctx)
	return websearch.WithCaller(ctx, websearch.Caller{
		UserID:    meta.UserID,
		OrgID:     orgID,
		RequestID: system.GenerateRequestID(),
		Source:    types.WebSearchSourceAgent,
	})
}

func formatSearchResponse(results []websearch.Result) string {
	if len(results) == 0 {
		return "No search results found for the query."
	}
	var buf bytes.Buffer
	buf.WriteString("## Results: \n")
	for _, result := range results {
		snippet := ""
		if result.Text != nil {
			snippet = *result.Text
		}
		fmt.Fprintf(&buf, "### Source: %s\n- URL: %s\n- Snippet: %s\n\n", result.Title, result.URL, snippet)
	}
	return buf.String()
}
