package tools

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

func getScriptFromTool(tool *types.Tool, prompt string) *types.GptScript {
	script := &types.GptScript{
		Input: prompt,
	}
	if tool.Config.GPTScript.Script != "" {
		script.Source = tool.Config.GPTScript.Script
	} else if tool.Config.GPTScript.ScriptURL != "" {
		script.URL = tool.Config.GPTScript.ScriptURL
	}
	return script
}

func (c *ChainStrategy) RunGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, currentMessage, action string) (*RunActionResponse, error) {
	script := getScriptFromTool(tool, currentMessage)
	result, err := c.gptScriptExecutor.ExecuteScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to run gptscript: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("failed to run gptscript: %s", result.Error)
	}
	return &RunActionResponse{
		Message:    result.Output,
		RawMessage: result.Output,
	}, nil
}
