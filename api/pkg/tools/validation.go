package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *ChainStrategy) ValidateAndDefault(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
	switch tool.ToolType {
	case types.ToolTypeAPI:
		return c.validateAndDefaultAPI(ctx, tool)
	default:
		return tool, nil
	}
}

func (c *ChainStrategy) validateAndDefaultAPI(ctx context.Context, tool *types.Tool) (*types.Tool, error) {
	loader := openapi3.NewLoader()

	schema, err := loader.LoadFromData([]byte(tool.Config.API.Schema))
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI spec: %w", err)
	}

	if tool.Config.API.URL == "" {
		if len(schema.Servers) > 0 {
			tool.Config.API.URL = schema.Servers[0].URL
		} else {
			return nil, fmt.Errorf("API URL is required for API tools")
		}
	}

	// Validate that all paths have operation IDs set
	err = c.validateOperationIDs(ctx, tool, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure operation IDs: %w", err)
	}

	// TODO: create a better summary for the tool if possible

	return tool, nil
}

func (c *ChainStrategy) validateOperationIDs(_ context.Context, _ *types.Tool, schema *openapi3.T) error {

	for path, pathItem := range schema.Paths.Map() {

		for method, operation := range pathItem.Operations() {
			if operation.Summary == "" {
				operation.Summary = operation.Description
			}

			if operation.OperationID == "" {
				operation.OperationID = strings.ToLower(fmt.Sprintf("%s%s", method, strings.ReplaceAll(path, "/", "_")))
			}
		}
	}

	return nil
}

func ValidateTool(tool *types.Tool, planner Planner, strict bool) error {
	switch tool.ToolType {
	case types.ToolTypeGPTScript:

		if tool.Config.GPTScript.Script == "" && tool.Config.GPTScript.ScriptURL == "" {
			return system.NewHTTPError400("script or script URL is required for GPTScript tools")
		}

		if tool.Config.GPTScript.Script != "" && tool.Config.GPTScript.ScriptURL != "" {
			return system.NewHTTPError400("only one of script or script URL is allowed for GPTScript tools")
		}

		// OK
		if tool.Description == "" && strict {
			return system.NewHTTPError400("description is required for GPTScript tools, make as descriptive as possible")
		}

	case types.ToolTypeAPI:
		// Validate the API
		if tool.Config.API == nil {
			return system.NewHTTPError400("API config is required for API tools")
		}

		if tool.Config.API.URL == "" {
			return system.NewHTTPError400("API URL is required for API tools")
		}

		if tool.Config.API.Schema == "" {
			return system.NewHTTPError400("API schema is required for API tools")
		}

		// If schema is base64 encoded, decode it
		decoded, err := base64.StdEncoding.DecodeString(tool.Config.API.Schema)
		if err == nil {
			tool.Config.API.Schema = string(decoded)
		}

		actions, err := GetActionsFromSchema(tool.Config.API.Schema)
		if err != nil {
			return system.NewHTTPError400("failed to get actions from schema, error: %s", err)
		}

		if len(actions) == 0 {
			return system.NewHTTPError400("no actions found in the schema, please check the documentation for required fields (operationId, summary or description)")
		}

		tool.Config.API.Actions = actions

		_, err = planner.ValidateAndDefault(context.Background(), tool)
		if err != nil {
			return system.NewHTTPError400("failed to validate and default tool, error: %s", err)
		}

	default:
		return system.NewHTTPError400("invalid tool type %s, only API tools are supported at the moment", tool.ToolType)
	}

	return nil
}
