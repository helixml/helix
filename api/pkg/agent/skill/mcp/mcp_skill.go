package mcp

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/rs/zerolog/log"
	"github.com/sashabaranov/go-openai"
)

func NewDirectMCPClientSkills(tool *types.Tool) []agent.Skill {
	var skills []agent.Skill

	for _, mcpTool := range tool.Config.MCP.Tools {
		tool, err := newMCPTool(tool.Config.MCP, mcpTool)
		if err != nil {
			log.Error().Err(err).Msg("failed to create MCP tool")
			continue
		}

		skills = append(skills, agent.Skill{
			Name:        agent.SanitizeToolName(mcpTool.Name),
			Description: mcpTool.Description,
			Parameters:  mcpTool.InputSchema.Properties,
			Direct:      true,
			Tools:       []agent.Tool{tool},
		})
	}

	return skills
}

func newMCPTool(cfg *types.ToolMCPClientConfig, mcpTool mcp.Tool) (*MCPClientTool, error) {
	return &MCPClientTool{
		cfg:     cfg,
		mcpTool: mcpTool,
	}, nil
}

type MCPClientTool struct { //nolint:revive
	cfg     *types.ToolMCPClientConfig
	mcpTool mcp.Tool // Parsed configuration from the MCP server
}

func (t *MCPClientTool) Name() string {
	return fmt.Sprintf("mcp_%s", agent.SanitizeToolName(t.mcpTool.Name))
}

func (t *MCPClientTool) Description() string {
	return t.mcpTool.Description
}

func (t *MCPClientTool) Execute(ctx context.Context, meta agent.Meta, args map[string]any) (string, error) {
	client, err := newMcpClient(ctx, &types.AssistantMCP{
		URL:     t.cfg.URL,
		Headers: t.cfg.Headers,
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
		return "", err
	}

	return res.Result.String(), nil

}

func (t *MCPClientTool) Icon() string {
	return ""
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
				Parameters:  t.mcpTool.InputSchema.Properties,
			},
		},
	}
}
