package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/lukemarsden/go-openai2"
)

type IsActionableResponse struct {
	NeedsApi      string `json:"needs_api"`
	Api           string `json:"api"`
	Justification string `json:"justification"`
}

func (c *ChainStrategy) IsActionable(ctx context.Context, tools []*types.Tool, history []*types.Interaction, currentMessage string) (*IsActionableResponse, error) {
	if len(tools) == 0 {
		return &IsActionableResponse{
			NeedsApi:      "no",
			Justification: "No tools available to check if the user input is actionable or not",
		}, nil
	}

	systemPrompt, err := c.getActionableSystemPrompt(tools)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare system prompt: %w", err)
	}

	var messages []openai.ChatCompletionMessage

	messages = append(messages, systemPrompt)

	for _, interaction := range history {
		switch interaction.Creator {
		case types.CreatorTypeUser:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleUser,
				Content: interaction.Message,
			})
		case types.CreatorTypeSystem:
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleAssistant,
				Content: interaction.Message,
			})
		}
	}

	// Adding current message
	messages = append(messages,
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: currentMessage,
		},
		openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleUser,
			Content: "Return the corresponding json for the last user input",
		},
	)

	resp, err := c.apiClient.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Stream:    false,
			MaxTokens: 100,
			Model:     c.cfg.ToolsModel,
			Messages:  messages,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	var actionableResponse IsActionableResponse
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &actionableResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response from inference API: %w", err)
	}

	log.Info().
		Str("user_input", currentMessage).
		Str("justification", actionableResponse.Justification).
		Str("needs_api", actionableResponse.NeedsApi).
		Msg("is_actionable")

	return &actionableResponse, nil
}

func (c *ChainStrategy) getActionableSystemPrompt(tools []*types.Tool) (openai.ChatCompletionMessage, error) {
	// Render template
	tmpl, err := template.New("system_prompt").Parse(isInformativeOrActionablePrompt)
	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	var modelTools []*modelTool

	for _, tool := range tools {
		switch tool.ToolType {
		case types.ToolTypeAPI:
			// For APIs we need to add all the actions that have been parsed
			// from the OpenAPI spec
			for _, action := range tool.Config.API.Actions {
				modelTools = append(modelTools, &modelTool{
					Name:        action.Name,
					Description: action.Description,
				})
			}
		case types.ToolTypeFunction:
			modelTools = append(modelTools, &modelTool{
				Name:        tool.Name,
				Description: tool.Description,
			})
		}

	}

	// Render template
	var sb strings.Builder
	err = tmpl.Execute(&sb, struct {
		Tools []*modelTool
	}{
		Tools: modelTools,
	})

	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: sb.String(),
	}, nil
}

// modelTool is used to render the template. It can be an API endpoint, a function, etc.
type modelTool struct {
	Name        string
	Description string
}

const isInformativeOrActionablePrompt = `You are an AI tool that classifies whether user input requires an API call or not. You should recommend using an API if the user request matches one of the APIs descriptions below. The user requests that can be fulfilled by calling an external API to either execute something or fetch more data to help in answering the question. Also, if the user question is asking you to perform actions (e.g. list, create, update, delete) then you will need to use an API.

Examples:  

**User Input:** Create a B-1 visa application

**Available APIs:**  
- API(createVisaApplication): This API creates a B-1 visa application. 
- API(getVisaStatus): This API queries B-1 visa status.   

**Verdict:** Needs API call so the response should be {"needs_api": "yes", "justification": "The reason behind your verdict", "api": "createVisaApplication"}

**Justification:** The user is asking to create a visa application and the (createVisaApplication) API can be used to satisfy the user requirement.  

**Another Example:**

**User Input:** How to renew a B-1 visa  

**Available APIs:**   
- API(createVisaApplication): This API creates a B-1 visa application.  
- API(renewVisa): This API renews an existing B-1 visa.

**Verdict:** Does not need API call so the response should be {"needs_api": "no", "justification": "The reason behind your verdict", "api": ""}  

**Justification:** The user is asking how to renew a B-1 visa, which is an informational question that does not require an API call.

**One More Example:**

**User Input:** Get status of my B-1 visa application  

**Available APIs:**    
- API(getVisaStatus): This API queries status of a B-1 visa application.

**Verdict:** Needs API call so the response should be {"needs_api": "yes", "justification": "The user is asking to get visa status", "api": "getVisaStatus"}

**Response Format:** Always respond with JSON without any commentary, for example: {"needs_api": "no", "justification": "The reason behind your verdict", "api": "apiName"}  

===END EXAMPLES===
The available tools:

{{ range $index, $tool := .Tools }}
{{ $index }}. {{ $tool.Name }} ({{ $tool.Description }})
{{ end }}

Based on the above, here is the user input/questions:
`
