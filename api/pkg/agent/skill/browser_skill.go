package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/readability"
	"github.com/helixml/helix/api/pkg/types"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/jsonschema"
)

const browserMainPrompt = `You are an expert at browsing the web to find and synthesize information. Your role is to help users by visiting relevant websites and presenting the information in a clear, organized manner.

Key responsibilities:
1. URL Selection:
   - Carefully analyze the user's request to determine the most relevant URL to visit
   - Consider multiple sources if needed to get comprehensive information
   - Prioritize official, reliable sources when available
   - Think step by step about which URL would best answer the user's question

2. Result Analysis:
   - Extract the most relevant information from the webpage
   - Focus on content that directly addresses the user's query
   - Ignore irrelevant content, ads, and navigation elements
   - Identify key facts, data, and insights

3. Information Presentation:
   - Present information in a clear, structured format
   - Cite the source of information
   - Highlight the most important points
   - Provide context when necessary
   - If the information is not found or unclear, explain why and suggest alternatives

Best Practices:
- Always verify information from reliable sources
- Be transparent about the source of information
- Acknowledge when information is not available or unclear
- Maintain objectivity and avoid making assumptions beyond the provided information
- If multiple visits are needed, explain your reasoning and approach

When using the browser tool:
- Use the tool proactively when users need current or factual information
- Choose URLs that are most likely to contain the relevant information
- If initial results are insufficient, consider visiting additional URLs
- Present information in a clear, organized manner, citing sources appropriately

Remember: Your goal is to provide accurate, well-sourced information while maintaining clarity and relevance to the user's needs.`

// NewBrowserSkill creates a new browser skill, this skill provides a tool to open URLs in a browser (Chrome runner)
func NewBrowserSkill(config *types.ToolBrowserConfig, browser *browser.Browser) agent.Skill {
	return agent.Skill{
		Name:         "Browser",
		Description:  "Use the browser to search the web",
		SystemPrompt: browserMainPrompt,
		Tools: []agent.Tool{
			&BrowserTool{
				browser:   browser,
				config:    config,
				parser:    readability.NewParser(), // TODO: add config for this
				converter: md.NewConverter("", true, nil),
			},
		},
	}
}

type BrowserTool struct {
	browser   *browser.Browser
	config    *types.ToolBrowserConfig
	parser    readability.Parser
	converter *md.Converter
}

func (t *BrowserTool) Name() string {
	return "Browser"
}

func (t *BrowserTool) Description() string {
	return "Use the browser to open website URLs"
}

func (t *BrowserTool) String() string {
	return "Browser"
}

func (t *BrowserTool) StatusMessage() string {
	return "Browsing the web, open URLs"
}

func (t *BrowserTool) Icon() string {
	return "LanguageIcon"
}

func (t *BrowserTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "Browser",
				Description: "Use the browser to search the web",
				Parameters: jsonschema.Definition{
					Type: jsonschema.Object,
					Properties: map[string]jsonschema.Definition{
						"url": {
							Type:        jsonschema.String,
							Description: "The URL to visit",
						},
					},
					Required: []string{"url"},
				},
			},
		},
	}
}

func (t *BrowserTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	url, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("url is required")
	}

	b, err := t.browser.GetBrowser()
	if err != nil {
		return "", fmt.Errorf("error getting browser: %w", err)
	}
	defer func() {
		if err := t.browser.PutBrowser(b); err != nil {
			log.Warn().Err(err).Msg("error putting browser")
		}
	}()

	page, err := t.browser.GetPage(b, proto.TargetCreateTarget{URL: url})
	if err != nil {
		return "", fmt.Errorf("error getting page for %s: %w", url, err)
	}
	defer t.browser.PutPage(page)

	err = page.WaitLoad()
	if err != nil {
		return "", fmt.Errorf("error waiting for page to load for %s: %w", url, err)
	}

	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("error getting HTML for %s: %w", url, err)
	}

	if t.config.MarkdownPostProcessing {
		markdown, err := t.converter.ConvertString(html)
		if err != nil {
			log.Warn().Err(err).Msg("error converting HTML to markdown")
			// Return the HTML as is, we can't do anything about it
			return html, nil
		}

		return markdown, nil
	}

	return html, nil
}
