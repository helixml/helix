package skill

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func NewSearchSkill(config *types.ToolWebSearchConfig, provider searxng.SearchProvider) agent.Skill {
	return agent.Skill{
		Name:         "Search",
		Description:  "Search the web for current information and recent data",
		SystemPrompt: "You are a web search assistant that can search the internet for current information. Use the search tool to find recent news, facts, or any up-to-date information that the user requests.",
		Tools: []agent.Tool{
			&searchTool{
				config:   config,
				provider: provider,
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
	return "search"
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
				Name:        "search",
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

func (t *searchTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
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

	jsonOutput, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Error marshaling search results to JSON")
		return "", fmt.Errorf("failed to format search results: %w", err)
	}

	return string(jsonOutput), nil
}
