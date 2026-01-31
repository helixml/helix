package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

// NewAPICallingSkillWithReasoning converts an API tool into a list of API calling tools for the
// agent to use. It converts into a list of tools because the API tool can have multiple
// actions (each API path is an action).
func NewAPICallingSkillWithReasoning(planner tools.Planner, oauthManager *oauth.Manager, tool *types.Tool) agent.Skill {
	var skillTools []agent.Tool
	for _, action := range tool.Config.API.Actions {
		parameters, err := tools.GetParametersFromSchema(tool.Config.API.Schema, action.Name)
		if err != nil {
			log.Error().Err(err).Msg("failed to get parameters from schema")
			continue
		}

		// Build parameter name mapping
		parameterNameMap := make(map[string]string)
		for _, param := range parameters {
			sanitizedName := agent.SanitizeParameterName(param.Name)
			parameterNameMap[sanitizedName] = param.Name
		}

		skillTools = append(skillTools, &APICallingTool{
			toolID:           tool.ID,
			toolName:         agent.SanitizeToolName(action.Name), // Summary field of the API path
			description:      action.Description,                  // OpenAPI API path description
			tool:             tool,
			action:           action,
			parameters:       parameters,
			planner:          planner,
			oauthManager:     oauthManager,
			parameterNameMap: parameterNameMap,
		})
	}

	return agent.Skill{
		Name:         agent.SanitizeToolName(tool.Name),
		Description:  tool.Description,
		SystemPrompt: tool.SystemPrompt,
		Tools:        skillTools,
	}
}

// NewDirectAPICallingSkills converts an API tool into multiple direct skills, one per API action.
// This allows the main agent to orchestrate API calls alongside other tools (like Calculator, Currency_Exchange_Rates)
// instead of having an inner skill context runner try to coordinate everything.
func NewDirectAPICallingSkills(planner tools.Planner, oauthManager *oauth.Manager, tool *types.Tool) []agent.Skill {
	var skills []agent.Skill

	for _, action := range tool.Config.API.Actions {
		parameters, err := tools.GetParametersFromSchema(tool.Config.API.Schema, action.Name)
		if err != nil {
			log.Error().Err(err).Msg("failed to get parameters from schema")
			continue
		}

		// Build parameter name mapping
		parameterNameMap := make(map[string]string)
		for _, param := range parameters {
			sanitizedName := agent.SanitizeParameterName(param.Name)
			parameterNameMap[sanitizedName] = param.Name
		}

		apiTool := &APICallingTool{
			toolID:           tool.ID,
			toolName:         agent.SanitizeToolName(action.Name), // Summary field of the API path
			description:      action.Description,                  // OpenAPI API path description
			tool:             tool,
			action:           action,
			parameters:       parameters,
			planner:          planner,
			oauthManager:     oauthManager,
			parameterNameMap: parameterNameMap,
		}

		// Build parameters schema for this action
		parametersSchema := buildParametersSchema(parameters)

		// Create a direct skill for this specific API action
		skills = append(skills, agent.Skill{
			Name:        agent.SanitizeToolName(action.Name), // Use action name as skill name
			Description: action.Description,                  // Use action description as skill description
			Parameters:  parametersSchema,                    // Expose parameters to outer agent loop
			Direct:      true,                                // Make it direct so it bypasses skill context runner
			Tools:       []agent.Tool{apiTool},               // Single tool per skill
		})
	}

	return skills
}

// buildParametersSchema builds a JSON schema for the API tool parameters
// This is used both for OpenAI tool definitions and skill parameter definitions
func buildParametersSchema(parameters []*tools.Parameter) jsonschema.Definition {
	properties := map[string]jsonschema.Definition{}
	required := []string{}

	for _, param := range parameters {
		// Create base property definition
		property := jsonschema.Definition{
			Description: param.Description,
		}

		// Handle different parameter types
		switch param.Type {
		case tools.ParameterTypeString:
			property.Type = jsonschema.String
		case tools.ParameterTypeInteger:
			property.Type = jsonschema.Integer
		case tools.ParameterTypeBoolean:
			property.Type = jsonschema.Boolean
		case tools.ParameterTypeArray:
			property.Type = jsonschema.Array
			if param.Schema != nil && param.Schema.Value != nil {
				// Handle array items type
				if param.Schema.Value.Items != nil {
					itemType := param.Schema.Value.Items.Value.Type.Slice()[0]
					switch itemType {
					case "string":
						property.Items = &jsonschema.Definition{Type: jsonschema.String}
					case "integer":
						property.Items = &jsonschema.Definition{Type: jsonschema.Integer}
					case "boolean":
						property.Items = &jsonschema.Definition{Type: jsonschema.Boolean}
					case "object":
						property.Items = &jsonschema.Definition{Type: jsonschema.Object}
					}
				} else {
					// Default to string array if items type is not specified
					property.Items = &jsonschema.Definition{Type: jsonschema.String}
				}
			} else {
				property.Items = &jsonschema.Definition{Type: jsonschema.String}
			}
		case tools.ParameterTypeObject:
			property.Type = jsonschema.Object
			if param.Schema != nil && param.Schema.Value != nil {
				// Handle object properties
				objectProperties := make(map[string]jsonschema.Definition)
				for propName, propSchema := range param.Schema.Value.Properties {
					propDef := jsonschema.Definition{
						Description: propSchema.Value.Description,
					}

					// Set property type
					if len(propSchema.Value.Type.Slice()) > 0 {
						switch propSchema.Value.Type.Slice()[0] {
						case "string":
							propDef.Type = jsonschema.String
						case "integer":
							propDef.Type = jsonschema.Integer
						case "boolean":
							propDef.Type = jsonschema.Boolean
						case "array":
							propDef.Type = jsonschema.Array
							if propSchema.Value.Items != nil && propSchema.Value.Items.Value != nil {
								itemType := propSchema.Value.Items.Value.Type.Slice()[0]
								switch itemType {
								case "string":
									propDef.Items = &jsonschema.Definition{Type: jsonschema.String}
								case "integer":
									propDef.Items = &jsonschema.Definition{Type: jsonschema.Integer}
								case "boolean":
									propDef.Items = &jsonschema.Definition{Type: jsonschema.Boolean}
								case "object":
									propDef.Items = &jsonschema.Definition{Type: jsonschema.Object}
								}
							}
						case "object":
							propDef.Type = jsonschema.Object
						}
					}

					objectProperties[propName] = propDef
				}
				property.Properties = objectProperties

				// Add required fields if specified
				if len(param.Schema.Value.Required) > 0 {
					property.Required = param.Schema.Value.Required
				}
			}
		}

		// Use sanitized parameter name for function definition
		sanitizedName := agent.SanitizeParameterName(param.Name)
		properties[sanitizedName] = property
		if param.Required {
			required = append(required, sanitizedName)
		}
	}

	// Always return a valid object schema, even if empty
	return jsonschema.Definition{
		Type:       jsonschema.Object,
		Properties: properties,
		Required:   required,
	}
}

type APICallingTool struct {
	toolID           string
	toolName         string
	description      string
	tool             *types.Tool
	action           *types.ToolAPIAction
	parameters       []*tools.Parameter
	planner          tools.Planner
	oauthManager     *oauth.Manager
	parameterNameMap map[string]string
}

var _ agent.Tool = &APICallingTool{}

func (t *APICallingTool) Name() string {
	return agent.SanitizeToolName(t.toolName)
}

func (t *APICallingTool) Description() string {
	return t.description
}

func (t *APICallingTool) String() string {
	return agent.SanitizeToolName(t.toolName)
}

func (t *APICallingTool) StatusMessage() string {
	return "Calling the API"
}

func (t *APICallingTool) Icon() string {
	return ""
}

func (t *APICallingTool) OpenAI() []openai.Tool {
	tool := []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        agent.SanitizeToolName(t.toolName),
				Description: t.description,
			},
		},
	}

	// Build parameters schema using the helper function
	parametersSchema := buildParametersSchema(t.parameters)

	// Only set function parameters if we have any
	if parametersSchema.Type != "" {
		tool[0].Function.Parameters = parametersSchema
	}

	return tool
}

func (t *APICallingTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {

	log.Info().
		Str("tool_name", t.toolName).
		Str("tool_id", t.toolID).
		Str("interaction_id", meta.InteractionID).
		Str("session_id", meta.SessionID).
		Str("user_id", meta.UserID).
		Any("args", args).Msg("Executing API calling tool")

	params := make(map[string]interface{})

	// Convert sanitized parameter names back to original parameter names
	for sanitizedName, value := range args {
		if value == nil {
			continue
		}

		// Look up the original parameter name
		originalName, exists := t.parameterNameMap[sanitizedName]
		if !exists {
			// If not found in map, it might be an unchanged parameter name
			originalName = sanitizedName
		}

		params[originalName] = value
	}

	req := &types.RunAPIActionRequest{
		Action:     t.toolName,
		Tool:       t.tool,
		Parameters: params,
	}

	// Add OAuth tokens from context
	if t.tool.Config.API != nil && t.tool.Config.API.OAuthProvider != "" {
		req.OAuthTokens = make(map[string]string)

		// Try to get OAuth token for the provider with required scopes
		token, err := t.oauthManager.GetTokenForTool(ctx, meta.UserID, t.tool.Config.API.OAuthProvider, t.tool.Config.API.OAuthScopes)
		if err != nil {
			log.Warn().
				Err(err).
				Str("tool_name", t.toolName).
				Str("user_id", meta.UserID).
				Str("oauth_provider", t.tool.Config.API.OAuthProvider).
				Strs("required_scopes", t.tool.Config.API.OAuthScopes).
				Msg("Failed to get OAuth token for API tool")
		} else if token != "" {
			req.OAuthTokens[t.tool.Config.API.OAuthProvider] = token
			log.Info().
				Str("tool_name", t.toolName).
				Str("user_id", meta.UserID).
				Str("oauth_provider", t.tool.Config.API.OAuthProvider).
				Str("token_prefix", token[:min(len(token), 10)]+"...").
				Msg("Successfully retrieved OAuth token for API tool")
		}
	}

	resp, err := t.planner.RunAPIActionWithParameters(ctx, req)
	if err != nil {
		return "", err
	}

	if resp.Error != "" {
		return fmt.Sprintf("Error occurred while executing API action: %s", resp.Error), nil
	}

	return resp.Response, nil
}
