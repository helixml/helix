package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/sashabaranov/go-openai"
)

type RunActionResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (c *ChainStrategy) RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	switch tool.ToolType {
	case types.ToolTypeFunction:
		return nil, fmt.Errorf("function tool type is not supported yet")
	case types.ToolTypeAPI:
		return c.runApiAction(ctx, tool, history, currentMessage, action)
	default:
		return nil, fmt.Errorf("unknown tool type: %s", tool.ToolType)
	}
}

func (c *ChainStrategy) runApiAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	// Validate whether action is valid
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}

	found := false

	for _, ac := range tool.Config.API.Actions {
		if ac.Name == action {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("action %s is not found in the tool %s", action, tool.Name)
	}

	// Get API request parameters
	params, err := c.getAPIRequestParameters(ctx, tool, history, currentMessage, action)
	if err != nil {
		return nil, fmt.Errorf("failed to get api request parameters: %w", err)
	}

	req, err := c.prepareRequest(ctx, tool, action, params)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare request: %w", err)
	}

	// Make API call
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make api call: %w", err)
	}
	defer resp.Body.Close()

	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &RunActionResponse{
		Message: string(bts),
		Error:   "", // TODO
	}, nil
}

func (c *ChainStrategy) prepareRequest(ctx context.Context, tool *types.Tool, action string, params map[string]string) (*http.Request, error) {
	jsonSpec, err := filterOpenAPISchema(tool, action)
	if err != nil {
		return nil, fmt.Errorf("failed to filter OpenAPI schema for action %s, error: %w. Did you forget to add operationId?", action, err)
	}

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
