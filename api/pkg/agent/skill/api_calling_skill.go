package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/jsonschema"
)

// NewAPICallingSkill converts an API tool into a list of API calling tools for the
// agent to use. It converts into a list of tools because the API tool can have multiple
// actions (each API path is an action).
func NewAPICallingSkill(planner tools.Planner, tool *types.Tool) agent.Skill {
	var skillTools []agent.Tool
	for _, action := range tool.Config.API.Actions {
		parameters, err := tools.GetParametersFromSchema(tool.Config.API.Schema, action.Name)
		if err != nil {
			log.Error().Err(err).Msg("failed to get parameters from schema")
			continue
		}

		skillTools = append(skillTools, &APICallingTool{
			toolID:      tool.ID,
			toolName:    agent.SanitizeToolName(action.Name), // Summary field of the API path
			description: action.Description,                  // OpenAPI API path description
			tool:        tool,
			action:      action,
			parameters:  parameters,
			planner:     planner,
		})
	}

	return agent.Skill{
		Name:         agent.SanitizeToolName(tool.Name),
		Description:  tool.Description,
		SystemPrompt: tool.SystemPrompt,
		Tools:        skillTools,
	}
}

type APICallingTool struct {
	toolID      string
	toolName    string
	description string
	tool        *types.Tool
	action      *types.ToolAPIAction
	parameters  []*tools.Parameter
	planner     tools.Planner
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
	properties := map[string]jsonschema.Definition{}
	required := []string{}

	for _, param := range t.parameters {
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

		properties[param.Name] = property
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        agent.SanitizeToolName(t.toolName),
				Description: t.description,
				Parameters: jsonschema.Definition{
					Type:       jsonschema.Object,
					Properties: properties,
					Required:   required,
				},
			},
		},
	}
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

	// Convert the args to the correct types
	for _, param := range t.parameters {
		// For cases like "params={"candidate_name":"marcus","job_title":null}", skip the null values
		if args[param.Name] == nil {
			continue
		}

		params[param.Name] = args[param.Name]
	}

	req := &types.RunAPIActionRequest{
		Action:     t.toolName,
		Tool:       t.tool,
		Parameters: params,
	}

	// TODO: OAuth tokens should be added here

	resp, err := t.planner.RunAPIActionWithParameters(ctx, req)
	if err != nil {
		return "", err
	}

	if resp.Error != "" {
		return fmt.Sprintf("Error occurred while executing API action: %s", resp.Error), nil
	}

	return resp.Response, nil
}
