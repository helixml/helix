package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/sashabaranov/go-openai"
)

type RunActionResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (c *ChainStrategy) RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage string) (*RunActionResponse, error) {
	switch tool.ToolType {
	case types.ToolTypeFunction:
		return nil, fmt.Errorf("function tool type is not supported yet")
	case types.ToolTypeAPI:
		return c.runApiAction(ctx, tool, history, currentMessage)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) runApiAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage string) (*RunActionResponse, error) {

	return nil, nil
}

func (c *ChainStrategy) getAPIRequestParameters(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage string) (map[string]string, error) {
	systemPrompt, err := c.getApiSystemPrompt(tool)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare system prompt: %w", err)
	}

	userPrompt, err := c.getApiUserPrompt(tool, history, currentMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare user prompt: %w", err)
	}

	messages := []openai.ChatCompletionMessage{
		systemPrompt,
		userPrompt,
	}

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

	var params map[string]string
	err = json.Unmarshal([]byte(resp.Choices[0].Message.Content), &params)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from inference API: %w", err)
	}

	return params, nil
}

func (c *ChainStrategy) getApiSystemPrompt(tool *types.Tool) (openai.ChatCompletionMessage, error) {
	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: apiSystemPrompt,
	}, nil
}

func (c *ChainStrategy) getApiUserPrompt(tool *types.Tool, history []*types.Interaction, currentMessage string) (openai.ChatCompletionMessage, error) {
	// Render template
	tmpl, err := template.New("api_params").Parse(apiUserPrompt)
	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	jsonSpec, err := stripOpenAPISchema(tool, "")
	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	// Render template
	var sb strings.Builder
	err = tmpl.Execute(&sb, struct {
		Schema  string
		Message string
	}{
		Schema:  jsonSpec,
		Message: currentMessage,
	})

	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: sb.String(),
	}, nil
}

const apiSystemPrompt = `You are an intelligent machine learning model that can produce REST API's params / query params in json format, given the json schema, user input, data from previous api calls, and current application state.`

const apiUserPrompt = `API JSON schema: {{.Schema}}

User's input: {{ .Message }}

Based on the information provided, construct a valid golang JSON map (map[string]string) object. In cases where user input does not contain information for a query, DO NOT add that specific query parameter to the output. If a user doesn't provide a required parameter, use sensible defaults for required params, and leave optional params.

Your output must be a valid json, without any commentary
`

func stripOpenAPISchema(tool *types.Tool, operationId string) (string, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return "", fmt.Errorf("failed to load openapi spec: %w", err)
	}

	schema.Paths.Map()

	jsonSpec, err := schema.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal openapi spec: %w", err)
	}

	return string(jsonSpec), nil
}

func getActionsFromSchema(tool *types.Tool) ([]*types.ToolApiAction, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var actions []*types.ToolApiAction

	for path, pathItem := range schema.Paths.Map() {

		for method, operation := range pathItem.Operations() {
			description := operation.Summary
			if description == "" {
				description = operation.Description
			}

			actions = append(actions, &types.ToolApiAction{
				Name:        operation.OperationID,
				Description: description,
				Path:        path,
				Method:      method,
			})
		}
	}

	return actions, nil
}
