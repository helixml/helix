package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	agentpod "github.com/helixml/helix/api/pkg/agent"
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
			toolName:    action.Name,        // Summary field of the API path
			description: action.Description, // OpenAPI API path description
			tool:        tool,
			action:      action,
			parameters:  parameters,
			planner:     planner,
		})
	}

	return agent.Skill{
		Name:         tool.Name,
		Description:  tool.Description,
		SystemPrompt: tool.SystemPrompt,
		Tools:        skillTools,
	}
}

type APICallingTool struct {
	toolName    string
	description string
	tool        *types.Tool
	action      *types.ToolAPIAction
	parameters  []*tools.Parameter
	planner     tools.Planner
}

var _ agentpod.Tool = &APICallingTool{}

func (t *APICallingTool) Name() string {
	return t.toolName
}

func (t *APICallingTool) Description() string {
	return t.description
}

func (t *APICallingTool) String() string {
	return t.toolName
}

func (t *APICallingTool) StatusMessage() string {
	return "Calling the API"
}

func (t *APICallingTool) OpenAI() []openai.Tool {
	properties := map[string]jsonschema.Definition{}
	required := []string{}

	for _, param := range t.parameters {
		properties[param.Name] = jsonschema.Definition{
			Type:        jsonschema.String,
			Description: param.Description,
			// TODO: implement all types
		}
		if param.Required {
			required = append(required, param.Name)
		}
	}

	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.toolName,
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

func (t *APICallingTool) Execute(ctx context.Context, meta agentpod.Meta, args map[string]interface{}) (string, error) {

	params := make(map[string]string)

	// Convert the args to the correct types
	for _, param := range t.parameters {
		params[param.Name] = fmt.Sprintf("%v", args[param.Name])
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
