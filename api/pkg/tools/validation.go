package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
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
