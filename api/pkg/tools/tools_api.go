package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/sashabaranov/go-openai"
)

func (c *ChainStrategy) prepareRequest(ctx context.Context, tool *types.Tool, action string, pathParams map[string]string) (*http.Request, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	// Based on the operationId get the path and method
	var path, method string

	for p, pathItem := range schema.Paths.Map() {
		for m, operation := range pathItem.Operations() {
			if operation.OperationID == action {
				path = p
				method = m
				break
			}
		}
	}

	if path == "" || method == "" {
		return nil, fmt.Errorf("failed to find path and method for action %s", action)
	}

	// Prepare request
	req, err := http.NewRequestWithContext(ctx, method, tool.Config.API.URL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range tool.Config.API.Headers {
		req.Header.Set(k, v)
	}

	// Add path params
	for k, v := range pathParams {
		req.URL.Path = strings.Replace(req.URL.Path, "{"+k+"}", v, -1)
	}

	// TODO: Add query params
	// TODO: Add body

	return req, nil
}

func (c *ChainStrategy) getAPIRequestParameters(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (map[string]string, error) {
	systemPrompt, err := c.getApiSystemPrompt(tool)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare system prompt: %w", err)
	}

	userPrompt, err := c.getApiUserPrompt(tool, history, currentMessage, action)
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

func (c *ChainStrategy) getApiUserPrompt(tool *types.Tool, history []*types.Interaction, currentMessage, action string) (openai.ChatCompletionMessage, error) {
	// Render template
	tmpl, err := template.New("api_params").Parse(apiUserPrompt)
	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	jsonSpec, err := filterOpenAPISchema(tool, action)
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

func filterOpenAPISchema(tool *types.Tool, operationId string) (string, error) {
	loader := openapi3.NewLoader()

	if tool.Config.API == nil || tool.Config.API.Schema == "" {
		return "", fmt.Errorf("tool does not have an API schema")
	}

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return "", fmt.Errorf("failed to load openapi spec: %w", err)
	}

	filtered := &openapi3.T{}
	filtered.Info = schema.Info
	filtered.OpenAPI = schema.OpenAPI
	filtered.Paths = &openapi3.Paths{}
	filtered.Components = &openapi3.Components{}

	var usedRefs []string

	for path, pathItem := range schema.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.OperationID == operationId {
				// filtered.addOperation(path, method, operation)
				filtered.AddOperation(path, method, operation)

				for _, resp := range operation.Responses.Map() {
					jsonBody, ok := resp.Value.Content["application/json"]
					if !ok {
						continue
					}

					if jsonBody.Schema == nil {
						continue
					}

					if jsonBody.Schema.Ref != "" {
						parts := strings.Split(jsonBody.Schema.Ref, "/")
						if len(parts) > 0 {
							usedRefs = append(usedRefs, parts[len(parts)-1])
						}
					}
				}
			}
		}
	}

	if len(usedRefs) > 0 {
		filtered.Components.Schemas = make(map[string]*openapi3.SchemaRef)

		for _, ref := range usedRefs {
			filtered.Components.Schemas[ref] = schema.Components.Schemas[ref]
		}
	}

	jsonSpec, err := json.MarshalIndent(filtered, "", "  ")
	// jsonSpec, err := filtered.MarshalJSON()
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
