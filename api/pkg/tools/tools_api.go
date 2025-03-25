package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	openai "github.com/sashabaranov/go-openai"
)

func (c *ChainStrategy) prepareRequest(ctx context.Context, tool *types.Tool, action string, params map[string]string) (*http.Request, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	// Based on the operationId get the path and method
	var path, method string

	queryParams := make(map[string]bool)
	pathParams := make(map[string]bool)

	for p, pathItem := range schema.Paths.Map() {
		for m, operation := range pathItem.Operations() {
			if operation.OperationID == action {
				path = p
				method = m

				for _, param := range operation.Parameters {

					switch param.Value.In {
					case "query":
						queryParams[param.Value.Name] = true
					case "path":
						pathParams[param.Value.Name] = true
					}
				}

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

	q := req.URL.Query()

	// Add path params
	for k, v := range params {
		if pathParams[k] {
			req.URL.Path = strings.Replace(req.URL.Path, "{"+k+"}", v, -1)
		}

		if queryParams[k] {
			q.Add(k, v)
		}
	}

	req.URL.RawQuery = q.Encode()

	if tool.Config.API.Query != nil {
		q := req.URL.Query()
		for k, v := range tool.Config.API.Query {
			log.Debug().Str("key", k).Str("value", v).Msg("Adding query param")
			q.Add(k, v)
		}

		req.URL.RawQuery = q.Encode()
	}

	// Add standard headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Helix-Tool-Id", tool.ID)
	req.Header.Set("X-Helix-Action-Id", action)

	// Add custom headers from the tool configuration
	if tool.Config.API.Headers != nil {
		for k, v := range tool.Config.API.Headers {
			req.Header.Set(k, v)
		}
	}

	// Special logging for OAuth headers
	if tool.Config.API.OAuthProvider != "" {
		authHeader := req.Header.Get("Authorization")
		if authHeader != "" {
			prefix := authHeader
			if len(authHeader) > 15 {
				prefix = authHeader[:15] + "..."
			}

			log.Info().
				Str("tool", tool.Name).
				Str("oauth_provider", tool.Config.API.OAuthProvider).
				Str("action", action).
				Str("auth_header_prefix", prefix).
				Msg("OAuth Authorization header successfully added to request")
		} else {
			log.Warn().
				Str("tool", tool.Name).
				Str("oauth_provider", tool.Config.API.OAuthProvider).
				Str("action", action).
				Msg("OAuth provider configured but no Authorization header found")
		}
	}

	// Log request details for all API tools
	log.Info().
		Str("tool_name", tool.Name).
		Str("action", action).
		Str("method", method).
		Str("path", path).
		Str("url", req.URL.String()).
		Interface("params", params).
		Interface("query_params", queryParams).
		Interface("path_params", pathParams).
		Interface("headers", tool.Config.API.Headers).
		Msg("API request details")

	// Log authorization header if present (for debugging purposes only)
	if authHeader := req.Header.Get("Authorization"); authHeader != "" {
		// Log the JWT token for debugging (remove in production)
		log.Info().
			Str("auth_header_prefix", authHeader[:7]+"...").
			Msg("API request has Authorization header")
	} else {
		log.Warn().
			Interface("all_headers", req.Header).
			Interface("tool_headers", tool.Config.API.Headers).
			Msg("No Authorization header found for API request")
	}

	return req, nil
}

func (c *ChainStrategy) getAPIRequestParameters(ctx context.Context, client oai.Client, sessionID, interactionID string, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (map[string]string, error) {
	systemPrompt, err := c.getAPISystemPrompt(tool, action)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare system prompt: %w", err)
	}

	// Experiment: put main body of the prompt (and the OpenAPI schema) into the
	// system prompt and only add the user prompt as a message. Hypothesis is that
	// this will help the model to stop forgetting the user message.

	// userPrompt, err := c.getApiUserPrompt(tool, action)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to prepare user prompt: %w", err)
	// }

	messages := []openai.ChatCompletionMessage{
		systemPrompt,
	}

	for _, msg := range history {
		if msg.Role != openai.ChatMessageRoleSystem {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	// copy what works for the is_actionable prompt
	if len(messages) > 0 {
		messages[len(messages)-1].Content += "\nReturn the corresponding json for the last user input"
	}

	req := openai.ChatCompletionRequest{
		Stream:   false,
		Model:    c.cfg.Tools.Model,
		Messages: messages,
	}
	// override with tool model if specified, otherwise fallback to TOOLS_MODEL
	// env var
	if tool.Config.API.Model != "" {
		req.Model = tool.Config.API.Model
	}

	ctx = oai.SetContextValues(ctx, &oai.ContextValues{
		OwnerID:       oai.SystemID,
		SessionID:     sessionID,
		InteractionID: interactionID,
	})

	ctx = oai.SetStep(ctx, &oai.Step{
		Step: types.LLMCallStepPrepareAPIRequest,
	})

	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get response from inference API: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from inference API")
	}

	answer := resp.Choices[0].Message.Content

	// var params map[string]string
	params, err := unmarshalParams(answer)
	if err != nil {
		return nil, err
	}

	return params, nil
}

func unmarshalParams(data string) (map[string]string, error) {
	var initial map[string]interface{}
	err := unmarshalJSON(data, &initial)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from inference API: %w (%s)", err, data)
	}

	params := make(map[string]string)

	for k, v := range initial {
		// Convert any type of value to string
		if v == nil {
			params[k] = "" // Set empty string if value is nil
		} else {
			params[k] = fmt.Sprintf("%v", v)
		}
	}

	return params, nil
}

func (c *ChainStrategy) getAPISystemPrompt(tool *types.Tool, action string) (openai.ChatCompletionMessage, error) {
	// Render template
	apiUserPromptTemplate := apiUserPrompt

	if tool.Config.API.RequestPrepTemplate != "" {
		apiUserPromptTemplate = tool.Config.API.RequestPrepTemplate
	}

	tmpl, err := template.New("api_params").Parse(apiUserPromptTemplate)
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
		Schema string
	}{
		Schema: jsonSpec,
	})

	if err != nil {
		return openai.ChatCompletionMessage{}, err
	}

	return openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleSystem,
		Content: apiSystemPrompt + "\n\n" + sb.String(),
	}, nil
}

const apiSystemPrompt = `You are an intelligent machine learning model that can produce REST API's params / query params in json format, given the json schema, user input, data from previous api calls, and current application state.`

const apiUserPrompt = `
Your output must be a valid json, without any commentary or additional formatting.

Examples:

**User Input:** Get project prj_1234 details
**OpenAPI schema path:** /projects/{projectId}
**Verdict:** response should be

` + "```" + `json
{
  "projectId": "prj_1234"
}
` + "```" + `

**User Input:** What job is Marcus applying for?
**OpenAPI schema path:** /jobvacancies/v1/list
**OpenAPI schema parameters:** [
	{
		"in": "query",
		"name": "candidate_name",
		"schema": {
			"type": "string"
		},
		"required": false,
		"description": "Filter vacancies by candidate name"
	}
]
**Verdict:** response should be:

` + "```" + `json
{
  "candidate_name": "Marcus"
}
` + "```" + `

**User Input:** List all users with status "active"
**OpenAPI schema path:** /users/findByStatus
**OpenAPI schema parameters:** [
	{
		"name": "status",
		"in": "query",
		"description": "Status values that need to be considered for filter",
		"required": true,
		"type": "array",
		"items": {
			"type": "string",
			"enum": ["active", "pending", "sold"],
			"default": "active"
		}
	}
]
**Verdict:** response should be:

` + "```" + `json
{
  "status": "active"
}
` + "```" + `

**Response Format:** Always respond with JSON without any commentary, wrapped in markdown json tags, for example:
` + "```" + `json
{
  "parameterName": "parameterValue",
  "parameterName2": "parameterValue2"
}
` + "```" + `

===END EXAMPLES===

OpenAPI schema:

{{.Schema}}

===END OPENAPI SCHEMA===

Based on conversation below, construct a valid JSON object. In cases where user input does not contain information for a query, DO NOT add that specific query parameter to the output. If a user doesn't provide a required parameter, use sensible defaults for required params, and leave optional params out. Do not pass parameters as null, instead just don't include them.
ONLY use search parameters from the user messages below - do NOT use search parameters provided in the examples.
`

func filterOpenAPISchema(tool *types.Tool, operationID string) (string, error) {
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
			if operation.OperationID == operationID {
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
	if err != nil {
		return "", fmt.Errorf("failed to marshal openapi spec: %w", err)
	}

	return string(jsonSpec), nil
}

func GetActionsFromSchema(spec string) ([]*types.ToolAPIAction, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(spec))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var actions []*types.ToolAPIAction

	for path, pathItem := range schema.Paths.Map() {

		for method, operation := range pathItem.Operations() {
			description := operation.Summary
			if description == "" {
				description = operation.Description
			}

			if operation.OperationID == "" {
				return nil, fmt.Errorf("operationId is missing for all %s %s", method, path)
			}

			actions = append(actions, &types.ToolAPIAction{
				Name:        operation.OperationID,
				Description: description,
				Path:        path,
				Method:      method,
			})
		}
	}

	return actions, nil
}

type Parameter struct {
	Name        string
	Required    bool
	Type        ParameterType
	Description string
}

type ParameterType string

const (
	ParameterTypeString  ParameterType = "string"
	ParameterTypeInteger ParameterType = "integer"
	ParameterTypeBoolean ParameterType = "boolean"
	ParameterTypeArray   ParameterType = "array"
	ParameterTypeObject  ParameterType = "object"
)

func GetParametersFromSchema(spec string, action string) ([]*Parameter, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(spec))
	if err != nil {
		return nil, fmt.Errorf("failed to load openapi spec: %w", err)
	}

	var parameters []*Parameter

	for _, pathItem := range schema.Paths.Map() {
		for _, operation := range pathItem.Operations() {
			if operation.OperationID == action {
				for _, param := range operation.Parameters {
					parameters = append(parameters, &Parameter{
						Name:        param.Value.Name,
						Required:    param.Value.Required,
						Type:        getParameterType(param.Value.Schema),
						Description: param.Value.Description,
					})
				}

				// if operation.RequestBody != nil {
				// 	// TODO: Add body parameter. Need to lookup the schema in components
				// 	// and then add it to the parameters list all items.
				// }
			}
		}
	}

	return parameters, nil
}

func getParameterType(schema *openapi3.SchemaRef) ParameterType {
	if len(schema.Value.Type.Slice()) > 0 {
		return ParameterType(schema.Value.Type.Slice()[0])
	}

	return ParameterTypeString
}

// processOAuthTokens processes OAuth tokens for a tool
func processOAuthTokens(tool *types.Tool, oauthTokens map[string]string) {
	if oauthTokens == nil || len(oauthTokens) == 0 {
		log.Debug().
			Str("tool_name", tool.Name).
			Msg("No OAuth tokens available for tool")
		return
	}

	log.Debug().
		Str("tool_name", tool.Name).
		Int("token_count", len(oauthTokens)).
		Msg("Processing OAuth tokens for tool")

	// Only process API tools with an OAuth provider configured
	if tool.ToolType == types.ToolTypeAPI && tool.Config.API != nil && tool.Config.API.OAuthProvider != "" {
		toolProviderName := tool.Config.API.OAuthProvider

		log.Debug().
			Str("tool_name", tool.Name).
			Str("oauth_provider", toolProviderName).
			Bool("has_headers", tool.Config.API.Headers != nil).
			Int("headers_count", len(tool.Config.API.Headers)).
			Msg("Checking OAuth token for tool with provider")

		// Check if we have a matching OAuth token for this exact provider
		if token, exists := oauthTokens[toolProviderName]; exists {
			log.Debug().
				Str("tool_name", tool.Name).
				Str("provider", toolProviderName).
				Bool("token_exists", token != "").
				Str("token_prefix", token[:5]+"...").
				Msg("Found matching OAuth token for tool provider")

			// Initialize headers map if it doesn't exist
			if tool.Config.API.Headers == nil {
				log.Debug().
					Str("tool_name", tool.Name).
					Str("provider", toolProviderName).
					Msg("Creating new headers map for tool")
				tool.Config.API.Headers = make(map[string]string)
			}

			// Add the token as a Bearer token in the Authorization header
			authHeader := fmt.Sprintf("Bearer %s", token)
			tool.Config.API.Headers["Authorization"] = authHeader

			log.Debug().
				Str("tool_name", tool.Name).
				Str("provider", toolProviderName).
				Int("headers_count_after", len(tool.Config.API.Headers)).
				Bool("auth_header_exists", tool.Config.API.Headers["Authorization"] != "").
				Str("auth_header_prefix", authHeader[:12]+"...").
				Msg("Added OAuth token to tool headers")

			return
		} else {
			// Try a case-insensitive match as a fallback
			for tokenKey, tokenValue := range oauthTokens {
				if strings.EqualFold(tokenKey, toolProviderName) {
					log.Debug().
						Str("tool_name", tool.Name).
						Str("tool_provider", toolProviderName).
						Str("token_key", tokenKey).
						Bool("case_sensitive_match", tokenKey == toolProviderName).
						Bool("case_insensitive_match", strings.EqualFold(tokenKey, toolProviderName)).
						Msg("Found OAuth token with case-insensitive match")

					// Initialize headers map if it doesn't exist
					if tool.Config.API.Headers == nil {
						tool.Config.API.Headers = make(map[string]string)
					}

					// Add the token as a Bearer token in the Authorization header
					authHeader := fmt.Sprintf("Bearer %s", tokenValue)
					tool.Config.API.Headers["Authorization"] = authHeader

					log.Debug().
						Str("tool_name", tool.Name).
						Str("provider", tokenKey).
						Int("headers_count", len(tool.Config.API.Headers)).
						Str("auth_header_prefix", authHeader[:12]+"...").
						Msg("Added case-insensitive matched OAuth token to tool headers")

					return
				}
			}
		}

		// If we get here, no matching token was found
		tokenKeys := make([]string, 0, len(oauthTokens))
		for key := range oauthTokens {
			tokenKeys = append(tokenKeys, key)
		}

		log.Warn().
			Str("tool_name", tool.Name).
			Str("tool_provider", toolProviderName).
			Strs("available_tokens", tokenKeys).
			Msg("No matching OAuth token found for tool provider")
	}
}
