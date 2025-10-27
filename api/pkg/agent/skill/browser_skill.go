package skill

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/controller/knowledge/readability"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/go-rod/rod/lib/proto"
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
- Fetches the URL content, converts HTML to markdown
- Processes the content with the prompt using a small, fast model
- Returns the model's response about the content
- Use this tool when you need to retrieve and analyze web content

Usage notes:
- The URL must be a fully-formed valid URL
- The prompt should describe what information you want to extract from the page
- This tool is read-only and does not modify any files
- Results may be summarized if the content is very large
- Includes a self-cleaning 15-minute cache for faster responses when repeatedly accessing the same URL
- When a URL redirects to a different host, the tool will inform you and provide the redirect URL in a special format. You should then make a new WebFetch request with the redirect URL to fetch the content.`

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

// NewBrowserSkill creates a new browser skill, this skill provides a tool to open URLs in a browser (Chrome runner)
func NewBrowserSkill(config *types.ToolBrowserConfig, browser *browser.Browser, llm *agent.LLM) agent.Skill {
	return agent.Skill{
		Name:          "Browser",
		Description:   browserSkillDescription,
		SystemPrompt:  browserMainPrompt,
		Parameters:    browserSkillParameters,
		Direct:        true,
		ProcessOutput: config.ProcessOutput,
		Tools: []agent.Tool{
			&browserTool{
				browser:    browser,
				config:     config,
				llm:        llm,
				parser:     readability.NewParser(), // TODO: add config for this
				converter:  md.NewConverter("", true, nil),
				httpClient: http.DefaultClient,
			},
		},
	}
}

type browserTool struct {
	browser    *browser.Browser
	config     *types.ToolBrowserConfig
	parser     readability.Parser
	converter  *md.Converter
	llm        *agent.LLM
	httpClient *http.Client
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
	url, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("url is required")
	}

	prompt, ok := args["prompt"].(string)
	if !ok {
		return "", fmt.Errorf("prompt is required")
	}

	log.Info().Str("url", url).
		Str("user_id", meta.UserID).
		Str("session_id", meta.SessionID).
		Str("interaction_id", meta.InteractionID).
		Str("app_id", meta.AppID).
		Msg("Executing browser tool")

	var (
		respCh = make(chan string, 1)
		errCh  = make(chan error, 1)
	)

	go func() {
		var html string

		if t.config.NoBrowser {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				errCh <- fmt.Errorf("error creating request: %w", err)
				return
			}
			resp, err := t.httpClient.Do(req)
			if err != nil {
				errCh <- fmt.Errorf("error doing request: %w", err)
				return
			}
			defer resp.Body.Close()

			bts, err := io.ReadAll(resp.Body)
			if err != nil {
				errCh <- fmt.Errorf("error reading response body: %w", err)
				return
			}

			if resp.StatusCode >= 400 {
				errCh <- fmt.Errorf("error getting response body: %w", err)
				return
			}

			html = string(bts)

		} else {
			b, err := t.browser.GetBrowser()
			if err != nil {
				errCh <- fmt.Errorf("error getting browser: %w", err)
				return
			}
			defer func() {
				if err := t.browser.PutBrowser(b); err != nil {
					log.Warn().Err(err).Msg("error putting browser")
				}
			}()

			page, err := t.browser.GetPage(b, proto.TargetCreateTarget{URL: url})
			if err != nil {
				errCh <- fmt.Errorf("error getting page for %s: %w", url, err)
				return
			}
			defer t.browser.PutPage(page)

			err = page.WaitLoad()
			if err != nil {
				errCh <- fmt.Errorf("error waiting for page to load for %s: %w", url, err)
				return
			}

			// Wait until stable
			err = page.WaitStable(5 * time.Second)
			if err != nil {
				log.Warn().Err(err).Str("url", url).Msg("error waiting for page to be stable")
			}

			html, err = page.HTML()
			if err != nil {
				errCh <- fmt.Errorf("error getting HTML for %s: %w", url, err)
				return
			}
		}

		if t.config.MarkdownPostProcessing {
			markdown, err := t.converter.ConvertString(html)
			if err != nil {
				log.Warn().Err(err).Msg("error converting HTML to markdown")
				// Return the HTML as is, we can't do anything about it
				respCh <- html
				return
			}

			respCh <- markdown
			return
		}

		respCh <- html
	}()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case <-ctx.Done():
		log.Warn().Str("url", url).
			Str("user_id", meta.UserID).
			Str("session_id", meta.SessionID).
			Str("interaction_id", meta.InteractionID).
			Str("app_id", meta.AppID).
			Msg("timeout while browsing the web")
		return "", fmt.Errorf("timeout while browsing the web")
	case err := <-errCh:
		return "", err
	case resp := <-respCh:
		if t.config.ProcessOutput {
			processedOutput, err := t.processOutput(ctx, prompt, resp)
			if err != nil {
				log.Error().
					Err(err).
					Str("url", url).
					Str("user_id", meta.UserID).
					Str("session_id", meta.SessionID).
					Str("interaction_id", meta.InteractionID).
					Str("app_id", meta.AppID).
					Msg("error processing output")
				return resp, nil
			}
			return processedOutput, nil
		}
		return resp, nil
	}
}

func (t *browserTool) processOutput(ctx context.Context, prompt, output string) (string, error) {
	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStep("skill_context_runner (browser | process_output)"),
	})

	prompt = fmt.Sprintf(browserProcessOutputUserMessage, prompt, output)

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
