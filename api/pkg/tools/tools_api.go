package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

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

	jsonSpec, err := convertToOpenAPIV3(tool)
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

func convertToOpenAPIV3(tool *types.Tool) (string, error) {
	// spec := openapi3.T{}
	// spec.Info = &openapi3.Info{
	// 	Title:       tool.Name,
	// 	Description: tool.Description,
	// }

	// // Parse tool.Config.API.URL to get the path
	// u, err := url.Parse(tool.Config.API.URL)
	// if err != nil {
	// 	return "", fmt.Errorf("failed to parse url '%s': %w", tool.Config.API.URL, err)
	// }

	// op := openapi3.NewOperation()

	// for _, param := range tool.Config.API.Parameters {

	// }

	// spec.AddOperation(u.Path, tool.Config.API.Method, op)

	// // operation := openapi3.NewOperation()
	// // pathItem.Post = operation

	// requestBody := openapi3.NewRequestBody().
	// 	WithDescription("Your request body description").
	// 	WithJSONSchema(openapi3.NewSchema().
	// 		WithType("object").
	// 		WithProperties(map[string]*openapi3.SchemaRef{
	// 			"exampleField": openapi3.NewSchemaRef("",
	// 				openapi3.NewStringSchema()),
	// 		}),
	// 	)

	// operation.RequestBody = &openapi3.RequestBodyRef{
	// 	Value: requestBody,
	// }

	// jsonSpec, err := spec.MarshalJSON()
	// if err != nil {
	// 	return "", fmt.Errorf("failed to marshal openapi spec: %w", err)
	// }

	// return string(jsonSpec), nil
	return exampleSpec, nil
}

const apiSystemPrompt = `You are an intelligent machine learning model that can produce REST API's params / query params in json format, given the json schema, user input, data from previous api calls, and current application state.`

const apiUserPrompt = `API JSON schema: {{.Schema}}

User's input: {{ .Message }}

Based on the information provided, construct a valid golang JSON map (map[string]string) object. In cases where user input does not contain information for a query, DO NOT add that specific query parameter to the output. If a user doesn't provide a required parameter, use sensible defaults for required params, and leave optional params.

Your output must be a valid json, without any commentary
`

const exampleSpec = `openapi: "3.0.0"
info:
  version: 1.0.0
  title: Swagger Petstore
  license:
    name: MIT
servers:
  - url: http://petstore.swagger.io/v1
/pets/{petId}:
  get:
    summary: Info for a specific pet
    operationId: showPetById
    tags:
      - pets
    parameters:
      - name: petId
        in: path
        required: true
        description: The id of the pet to retrieve
        schema:
          type: string
    responses:
      '200':
        description: Expected response to a valid request
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/Pet"
      default:
        description: unexpected error
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/Error"
components:
  schemas:
    Pet:
      type: object
      required:
        - id
        - name
      properties:
        id:
          type: integer
          format: int64
        name:
          type: string
        tag:
          type: string
				description:
          type: string`
