package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
	"github.com/helixml/helix/api/pkg/websearch"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

const browserMainPrompt = `You are an expert at browsing the web to find and synthesize information. Your role is to help users by visiting relevant websites, summarizing their contents, and preserving URLs for further exploration.

Key responsibilities:
1. Content Summarization:
   - Extract and summarize the most relevant information from the webpage
   - Focus on content that directly addresses the user's query
   - Ignore irrelevant content, ads, and navigation elements
   - Identify key facts, data, and insights
   - Provide a concise but comprehensive summary

2. URL Preservation:
   - Always include and preserve the URLs of visited pages in your response
   - Identify and extract any relevant links that might contain additional useful information
   - Suggest specific URLs that could be visited next if more information is needed
   - Make it clear which URLs have been visited and which are suggested for future visits

3. Information Presentation:
   - Present information in a clear, structured format
   - Cite the source URL of information
   - Highlight the most important points
   - Provide context when necessary   

Best Practices:
- Always verify information from reliable sources
- Be transparent about the source of information
- Acknowledge when information is not available or unclear
- Maintain objectivity and avoid making assumptions beyond the provided information
- Preserve all relevant URLs for potential follow-up visits
- If multiple visits are needed, explain your reasoning and provide the specific URLs to visit

When using the browser tool:
- Use the tool proactively when users need current or factual information
- Choose URLs that are most likely to contain the relevant information
- If initial results are insufficient, suggest specific additional URLs to visit
- Present information in a clear, organized manner, citing sources appropriately
- Always include the visited URL and any relevant links found

Remember: Your goal is to provide accurate, well-sourced information while maintaining clarity and relevance to the user's needs. Always preserve URLs so the agent can visit more pages if needed for comprehensive information gathering.

The first user message will be the browser's response, summarize it.`

const browserProcessOutputUserMessage = `
## User Prompt
%s

## WEBSITE Content
%s
`

const browserSkillDescription = `- Fetches content from a specified URL and processes it using an AI model
- Takes a URL and a prompt as input
- Fetches the URL content and converts it to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
- The URL must be a fully-formed valid URL
- The prompt should describe what information you want to extract from the page
- This tool is read-only and does not modify any files
- Results may be summarized if the content is very large
- Recently fetched pages may be served from a cache
- Private and internal network addresses cannot be fetched`

var browserSkillParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"url": {
			Type:        jsonschema.String,
			Description: "The URL to visit",
		},
		"prompt": {
			Type:        jsonschema.String,
			Description: "The prompt to run on the fetched content",
		},
	},
	Required: []string{"url", "prompt"},
}

// NewBrowserSkill creates the Browser skill, which reads web pages through the
// web search provider's Contents API.
func NewBrowserSkill(config *types.ToolBrowserConfig, provider websearch.Provider, llm *agent.LLM) agent.Skill {
	return agent.Skill{
		Name:          "Browser",
		Description:   browserSkillDescription,
		SystemPrompt:  browserMainPrompt,
		Parameters:    browserSkillParameters,
		Direct:        true,
		ProcessOutput: config.ProcessOutput,
		Tools: []agent.Tool{
			&browserTool{
				provider:      provider,
				llm:           llm,
				processOutput: config.ProcessOutput,
			},
		},
	}
}

type browserTool struct {
	provider websearch.Provider
	llm      *agent.LLM
	// processOutput answers the prompt from the page with the small
	// generation model instead of returning the page itself.
	processOutput bool
}

func (t *browserTool) Name() string {
	return "Browser"
}

func (t *browserTool) Description() string {
	return "Use the browser to open website URLs"
}

func (t *browserTool) String() string {
	return "Browser"
}

func (t *browserTool) StatusMessage() string {
	return "Browsing the web, open URLs"
}

func (t *browserTool) Icon() string {
	return "LanguageIcon"
}

func (t *browserTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "Browser",
				Description: browserSkillDescription,
				Parameters:  browserSkillParameters,
			},
		},
	}
}

func (t *browserTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	pageURL, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("url is required")
	}

	prompt, ok := args["prompt"].(string)
	if !ok {
		return "", fmt.Errorf("prompt is required")
	}

	log.Info().Str("url", pageURL).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Str("interaction_id", meta.InteractionID).
		Str("app_id", meta.AppID).
		Msg("Executing browser tool")

	ctx = websearch.WithCaller(ctx, websearch.Caller{UserID: meta.UserID})
	page, err := t.fetch(ctx, pageURL)
	if err != nil {
		return "", err
	}
	if !t.processOutput {
		return page, nil
	}
	return t.answer(ctx, prompt, page)
}

// fetch returns the page text as markdown.
func (t *browserTool) fetch(ctx context.Context, pageURL string) (string, error) {
	resp, err := t.provider.Contents(ctx, &websearch.ContentsRequest{
		URLs:            []string{pageURL},
		ContentsOptions: websearch.ContentsOptions{Text: &websearch.TextOption{Enabled: true}},
	})
	if err != nil {
		return "", fmt.Errorf("error fetching %s: %w", pageURL, err)
	}
	for _, status := range resp.Statuses {
		if status.Status == websearch.ContentStatusError {
			return "", contentError(pageURL, status.Error)
		}
	}
	if len(resp.Results) != 1 || resp.Results[0].Text == nil {
		return "", fmt.Errorf("error fetching %s: no content returned", pageURL)
	}
	return *resp.Results[0].Text, nil
}

func contentError(pageURL string, e *websearch.ContentError) error {
	if e == nil {
		return fmt.Errorf("error fetching %s", pageURL)
	}
	if e.HTTPStatusCode != 0 {
		return fmt.Errorf("error fetching %s: %s (HTTP %d)", pageURL, e.Tag, e.HTTPStatusCode)
	}
	return fmt.Errorf("error fetching %s: %s", pageURL, e.Tag)
}

func (t *browserTool) answer(ctx context.Context, prompt, page string) (string, error) {
	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("skill_context_runner (browser | process_output)"),
	})

	prompt = fmt.Sprintf(browserProcessOutputUserMessage, prompt, page)

	completion, err := t.llm.New(ctx, t.llm.SmallGenerationModel, openai.ChatCompletionRequest{
		Model: t.llm.SmallGenerationModel.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: browserMainPrompt},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("error processing output: %w", err)
	}

	return completion.Choices[0].Message.Content, nil
}
