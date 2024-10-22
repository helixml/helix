package tools

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
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

func (c *ChainStrategy) RunGPTScriptAction(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*RunActionResponse, error) {
	// TODO: maybe we should pass the whole history to the script executor, like we do for api tools
	currentMessage := history[len(history)-1].Content
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

func (c *ChainStrategy) RunGPTScriptActionStream(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*openai.ChatCompletionStream, error) {
	currentMessage := history[len(history)-1].Content
	script := getScriptFromTool(tool, currentMessage)
	result, err := c.gptScriptExecutor.ExecuteScript(ctx, script)
	if err != nil {
		return nil, fmt.Errorf("failed to run gptscript: %w", err)
	}

	downstream, downstreamWriter, err := transport.NewOpenAIStreamingAdapter(openai.ChatCompletionRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming adapter: %w", err)
	}

	go func() {
		defer downstreamWriter.Close()

		transport.WriteChatCompletionStream(downstreamWriter, &openai.ChatCompletionStreamResponse{
			Choices: []openai.ChatCompletionStreamChoice{
				{
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Content: result.Output,
					},
				},
			},
		})
	}()

	return downstream, nil
}
