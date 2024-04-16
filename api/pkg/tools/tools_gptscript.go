package tools

import (
	"context"
	"fmt"

	gptscript_runner "github.com/helixml/helix/api/pkg/gptscript"
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

func (c *ChainStrategy) RunRemoteGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	script := getScriptFromTool(tool, currentMessage)
	result, err := gptscript_runner.RunGPTScriptTestfaster(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to run gptscript: %w", err)
	}
	return &RunActionResponse{
		Message:    result.Output,
		RawMessage: result.Output,
	}, nil

}

// this is run locally
func (c *ChainStrategy) RunGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage, action string) (*RunActionResponse, error) {
	script := getScriptFromTool(tool, currentMessage)
	result, err := gptscript_runner.RunGPTScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to run gptscript: %w", err)
	}
	return &RunActionResponse{
		Message:    result,
		RawMessage: result,
	}, nil
}
