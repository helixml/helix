package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

func NewDirectMCPClientSkills(clientGetter ClientGetter, oauthManager *oauth.Manager, tool *types.Tool) []agent.Skill {
	var skills []agent.Skill

	for _, mcpTool := range tool.Config.MCP.Tools {
		tool, err := newMCPTool(clientGetter, oauthManager, tool.Config.MCP, mcpTool)
		if err != nil {
			log.Error().Err(err).Msg("failed to create MCP tool")
			continue
		}

		skills = append(skills, agent.Skill{
			Name:        agent.SanitizeToolName(mcpTool.Name),
			Description: mcpTool.Description,
			Parameters:  buildParameters(mcpTool.InputSchema),
			Direct:      true,
			Tools:       []agent.Tool{tool},
		})
	}

	return skills
}

func newMCPTool(clientGetter ClientGetter, oauthManager *oauth.Manager, cfg *types.ToolMCPClientConfig, mcpTool mcp.Tool) (*MCPClientTool, error) {
	return &MCPClientTool{
		clientGetter: clientGetter,
		oauthManager: oauthManager,
		cfg:          cfg,
		mcpTool:      mcpTool,
	}, nil
}

type MCPClientTool struct { //nolint:revive
	clientGetter ClientGetter
	oauthManager *oauth.Manager
	cfg          *types.ToolMCPClientConfig
	mcpTool      mcp.Tool // Parsed configuration from the MCP server
}

func (t *MCPClientTool) Name() string {
	return fmt.Sprintf("mcp_%s", agent.SanitizeToolName(t.mcpTool.Name))
}

func (t *MCPClientTool) Description() string {
	return t.mcpTool.Description
}

func (t *MCPClientTool) Execute(ctx context.Context, meta agent.Meta, args map[string]any) (string, error) {
	client, err := t.clientGetter.NewClient(ctx, meta, t.oauthManager, &types.AssistantMCP{
		URL:           t.cfg.URL,
		Headers:       t.cfg.Headers,
		OAuthProvider: t.cfg.OAuthProvider,
		OAuthScopes:   t.cfg.OAuthScopes,
	})
	if err != nil {
		return "", err
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = t.mcpTool.Name
	req.Params.Arguments = args

	res, err := client.CallTool(ctx, mcp.CallToolRequest{
		Params: req.Params,
	})

	if err != nil {
		log.Error().Err(err).Msg("failed to call MCP tool")
		return "", err
	}

	var results []string
	for _, content := range res.Content {
		switch content := content.(type) {
		case mcp.TextContent:
			results = append(results, content.Text)
		case mcp.ImageContent:
			results = append(results, content.Data)
		case mcp.AudioContent:
			results = append(results, content.Data)
		default:
			// Reflect to
			results = append(results, fmt.Sprintf("Unknown content type: %T", content))
		}
	}

	if len(results) == 0 {
		return "No response from MCP tool", nil
	}

	final := strings.Join(results, "\n")

	return final, nil

}

func (t *MCPClientTool) Icon() string {
	return "HubIcon"
}

func (t *MCPClientTool) String() string {
	return t.mcpTool.Name
}

func (t *MCPClientTool) StatusMessage() string {
	return "Calling MCP tool"
}

func (t *MCPClientTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "mcp_" + t.mcpTool.Name,
				Description: t.mcpTool.Description,
				Parameters:  buildParameters(t.mcpTool.InputSchema),
			},
		},
	}
}

func buildParameters(inputSchema mcp.ToolInputSchema) jsonschema.Definition {
	// Log the input schema for debugging
	log.Debug().
		Str("schema_type", inputSchema.Type).
		Int("properties_count", len(inputSchema.Properties)).
		Int("required_count", len(inputSchema.Required)).
		Msg("Processing MCP tool InputSchema")

	properties := make(map[string]jsonschema.Definition)
	required := inputSchema.Required

	// Validate and sanitize the input schema
	if inputSchema.Properties == nil {
		// If no properties are defined, create an empty properties map
		// This ensures we always return a valid object schema
		properties = make(map[string]jsonschema.Definition)
		log.Debug().Msg("MCP tool has no properties, creating empty schema")
	} else {
		// Convert MCP properties to jsonschema.Definition
		for key, prop := range inputSchema.Properties {
			// Convert the property to jsonschema.Definition
			// Since MCP properties are map[string]any, we need to handle type conversion
			if propMap, ok := prop.(map[string]any); ok {
				jsonschemaProp := convertMapToDefinition(propMap)
				properties[key] = jsonschemaProp
			} else {
				// If it's not a map, create a basic string definition as fallback
				properties[key] = jsonschema.Definition{
					Type: jsonschema.String,
				}
				log.Debug().Str("property_key", key).Msg("Property is not a map, using string fallback")
			}
		}
	}

	// Always ensure we return an object type schema with properties
	// This is required by the OpenAI function calling API
	// Even if the MCP tool doesn't specify a type, we must return "object"
	result := jsonschema.Definition{
		Properties: properties,
		Required:   required,
	}

	if len(result.Properties) > 0 {
		result.Type = jsonschema.Object
	}

	log.Debug().
		Str("result_type", string(result.Type)).
		Int("result_properties_count", len(result.Properties)).
		Msg("Generated JSON schema for MCP tool")

	return result
}

// convertMapToDefinition recursively converts a map[string]any to jsonschema.Definition
func convertMapToDefinition(data map[string]any) jsonschema.Definition {
	def := jsonschema.Definition{}

	// Handle type - ensure we always have a valid type
	if typeVal, ok := data["type"].(string); ok && typeVal != "" {
		switch typeVal {
		case "string":
			def.Type = jsonschema.String
		case "integer":
			def.Type = jsonschema.Integer
		case "number":
			def.Type = jsonschema.Number
		case "boolean":
			def.Type = jsonschema.Boolean
		case "array":
			def.Type = jsonschema.Array
		case "object":
			def.Type = jsonschema.Object
		default:
			def.Type = jsonschema.String // fallback for unknown types
		}
	} else {
		// If no type is specified or it's empty, default to string
		def.Type = jsonschema.String
	}

	// Handle title (convert to description if no description exists)
	if title, ok := data["title"].(string); ok && title != "" {
		if desc, ok := data["description"].(string); ok && desc != "" {
			def.Description = desc
		} else {
			def.Description = title
		}
	} else if desc, ok := data["description"].(string); ok && desc != "" {
		def.Description = desc
	}

	// Handle enum
	if enumVal, ok := data["enum"].([]any); ok {
		enum := make([]string, 0, len(enumVal))
		for _, item := range enumVal {
			if str, ok := item.(string); ok {
				enum = append(enum, str)
			}
		}
		if len(enum) > 0 {
			def.Enum = enum
		}
	}

	// Handle properties (recursive)
	if props, ok := data["properties"].(map[string]any); ok {
		properties := make(map[string]jsonschema.Definition)
		for key, prop := range props {
			if propMap, ok := prop.(map[string]any); ok {
				properties[key] = convertMapToDefinition(propMap)
			}
		}
		if len(properties) > 0 {
			def.Properties = properties
			def.Type = jsonschema.Object
		}
	}

	// Handle items (for arrays)
	if items, ok := data["items"].(map[string]any); ok {
		itemsDef := convertMapToDefinition(items)
		def.Items = &itemsDef
	}

	return def
}
