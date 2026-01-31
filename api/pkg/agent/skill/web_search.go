package skill

import (
	"bytes"
	"context"
	"fmt"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/readability"
	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func NewSearchSkill(config *types.ToolWebSearchConfig, provider searxng.SearchProvider, browser *browser.Browser) agent.Skill {
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
			&browserTool{
				browser: browser,
				config: &types.ToolBrowserConfig{
					Enabled:                true,
					MarkdownPostProcessing: true,
					NoBrowser:              false,
					Cache:                  false,
				},
				parser:    readability.NewParser(), // TODO: add config for this
				converter: md.NewConverter("", true, nil),
			},
		},
	}
}

type searchTool struct {
	config   *types.ToolWebSearchConfig
	provider searxng.SearchProvider
}

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
							Description: "Optional category for the search (general, news, social_media)",
							Enum:        []string{"general", "news", "social_media"},
						},
						"language": {
							Type:        jsonschema.String,
							Description: "Optional language code for the search (e.g., 'en', 'es', 'fr')",
						},
					},
					Required: []string{"query"},
				},
			},
		},
	}
}

func (t *searchTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	query, ok := args["query"].(string)
	if !ok {
		return "", fmt.Errorf("query parameter is required and must be a string")
	}

	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	category := searxng.GeneralCategory
	if cat, ok := args["category"].(string); ok && cat != "" {
		category = cat
	}

	language := ""
	if lang, ok := args["language"].(string); ok {
		language = lang
	}

	log.Debug().
		Str("query", query).
		Str("category", category).
		Str("language", language).
		Msg("Executing web search")

	searchReq := &searxng.SearchRequest{
		MaxResults: t.config.MaxResults,
		Queries: []searxng.SearchQuery{
			{
				Query:    query,
				Category: category,
				Language: language,
			},
		},
	}

	results, err := t.provider.Search(ctx, searchReq)
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("Error searching with searxng")
		return "", fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return "No search results found for the query.", nil
	}

	// Format results as JSON for better structure
	output := searxng.Output{
		Query:    query,
		Category: category,
		Results:  results,
	}

	return formatSearchResponse(&output), nil
}

func formatSearchResponse(output *searxng.Output) string {
	if len(output.Results) == 0 {
		return "No results found"
	}
	var buf bytes.Buffer

	buf.WriteString("## Results: \n")

	for _, result := range output.Results {
		buf.WriteString(fmt.Sprintf("### Source: %s\n- URL: %s\n- Snippet: %s\n\n", result.Title, result.URL, result.Content))
	}

	return buf.String()
}
