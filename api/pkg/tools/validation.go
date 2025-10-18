package tools

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/agent/skill/mcp"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
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

func ValidateTool(userID string, assistant *types.AssistantConfig, tool *types.Tool, oauthManager *oauth.Manager, planner Planner, mcpClientGetter mcp.ClientGetter, strict bool) error {
	switch tool.ToolType {
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

		// Trim space
		tool.Config.API.Schema = strings.TrimSpace(tool.Config.API.Schema)
		tool.Config.API.URL = strings.TrimSpace(tool.Config.API.URL)

		if assistant.IsAgentMode() && tool.Config.API.SystemPrompt == "" {
			return system.NewHTTPError400("system prompt is required for API tools when using the agent mode")
		}

		// If schema is base64 encoded, decode it
		decoded, err := base64.StdEncoding.DecodeString(tool.Config.API.Schema)
		if err == nil {
			tool.Config.API.Schema = string(decoded)
		}

		actions, err := GetActionsFromSchema(tool.Config.API.Schema)
		if err != nil {
			return system.NewHTTPError400(fmt.Sprintf("failed to get actions from schema, error: %s", err))
		}

		if len(actions) == 0 {
			return system.NewHTTPError400("no actions found in the schema, please check the documentation for required fields (operationId, summary or description)")
		}

		tool.Config.API.Actions = actions

		_, err = planner.ValidateAndDefault(context.Background(), tool)
		if err != nil {
			return system.NewHTTPError400(fmt.Sprintf("failed to validate and default tool, error: %s", err))
		}

	case types.ToolTypeZapier:
		if tool.Config.Zapier == nil {
			return system.NewHTTPError400("Zapier config is required for Zapier tools")
		}

		if tool.Config.Zapier.APIKey == "" {
			return system.NewHTTPError400("API key is required for Zapier tools")
		}
	case types.ToolTypeAzureDevOps:
		if tool.Config.AzureDevOps == nil {
			return system.NewHTTPError400("Azure DevOps config is required for Azure DevOps tools")
		}

		// TODO: re-enable this when we have a way to validate the credentials
		// if tool.Config.AzureDevOps.OrganizationURL == "" {
		// 	return system.NewHTTPError400("Organization URL is required for Azure DevOps tools")
		// }

		// if tool.Config.AzureDevOps.PersonalAccessToken == "" {
		// 	return system.NewHTTPError400("Personal access token is required for Azure DevOps tools")
		// }
	case types.ToolTypeMCP:
		if tool.Config.MCP == nil {
			return system.NewHTTPError400("MCP config is required for MCP tools")
		}

		if tool.Config.MCP.URL == "" {
			return system.NewHTTPError400("URL is required for MCP tools")
		}

		// Get MCP config from assistant
		mcpConfig := &types.AssistantMCP{
			Name:          tool.Config.MCP.Name,
			URL:           tool.Config.MCP.URL,
			Headers:       tool.Config.MCP.Headers,
			OAuthProvider: tool.Config.MCP.OAuthProvider,
			OAuthScopes:   tool.Config.MCP.OAuthScopes,
		}

		// Attempt to initialize the MCP client
		resp, err := mcp.InitializeMCPClientSkill(context.Background(), mcpClientGetter, agent.Meta{UserID: userID}, oauthManager, mcpConfig)
		if err != nil {
			log.Warn().
				Err(err).
				Str("url", mcpConfig.URL).
				Str("name", mcpConfig.Name).
				Str("user_id", userID).
				Msg("failed to initialize MCP client, might not work during runtime")
		} else {
			// Find assistant mcp tool with the same name and update the tools
			for idx, mcp := range assistant.MCPs {
				if mcp.Name == mcpConfig.Name {
					assistant.MCPs[idx].Tools = resp.Tools
					break
				}
			}
			tool.Config.MCP.Tools = resp.Tools
		}

	case types.ToolTypeBrowser, types.ToolTypeCalculator, types.ToolTypeEmail, types.ToolTypeWebSearch:
		// No validation needed
	default:
		return system.NewHTTPError400(fmt.Sprintf("invalid tool type %s", tool.ToolType))
	}

	return nil
}
