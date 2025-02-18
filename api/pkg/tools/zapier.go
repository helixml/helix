package tools

import (
	"context"
	"fmt"

	oai "github.com/helixml/helix/api/pkg/openai"
	helix_langchain "github.com/helixml/helix/api/pkg/openai/langchain"
	"github.com/helixml/helix/api/pkg/openai/transport"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	openai "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/chains"
	"github.com/tmc/langchaingo/tools"
	"github.com/tmc/langchaingo/tools/zapier"
)

func (c *ChainStrategy) RunZapierAction(ctx context.Context, apiClient oai.Client, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*RunActionResponse, error) {

	llm, err := helix_langchain.New(apiClient, tool.Config.Zapier.Model)
	if err != nil {
		return nil, err
	}

	currentMessage := history[len(history)-1].Content

	log.Info().
		Str("action", action).
		Str("tool", tool.Name).
		Str("model", tool.Config.Zapier.Model).
		Str("api_key", tool.Config.Zapier.APIKey).
		Str("prompt", currentMessage).
		Msg("running Zapier action")

	// get all the available zapier NLA Tools
	tks, err := zapier.Toolkit(ctx, zapier.ToolkitOpts{
		APIKey: tool.Config.Zapier.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Zapier integration, error: %w", err)
	}

	for _, tk := range tks {
		tool, ok := tk.(*zapier.Tool)
		if !ok {
			return nil, fmt.Errorf("failed to convert zapier tool to *zapier.Tool")
		}
		tool.CallbacksHandler = callbacks.LogHandler{}
	}

	agentTools := []tools.Tool{
		// define tools here
	}

	// Add Zapier tools. These are all the tools that are enabled in your
	// Zapier NLA editor: https://actions.zapier.com/providers/
	agentTools = append(agentTools, tks...)

	iterations := 3
	if tool.Config.Zapier.MaxIterations > 0 {
		iterations = tool.Config.Zapier.MaxIterations
	}

	// Initialize the agent
	agent := agents.NewOneShotAgent(llm,
		agentTools,
		agents.WithMaxIterations(iterations),
	)

	executor := agents.NewExecutor(agent)

	// run a chain with the executor and defined input
	// input := "Get the last email from noreply@github.com"
	answer, err := chains.Run(ctx, executor, currentMessage)
	if err != nil {
		log.Err(err).
			Str("action", action).
			Str("tool", tool.Name).
			Str("model", tool.Config.Zapier.Model).
			Str("prompt", currentMessage).
			Msg("error running zapier action")

		return &RunActionResponse{
			Message:    answer,
			RawMessage: answer,
			Error:      err.Error(),
		}, nil
	}

	return &RunActionResponse{
		Message:    answer,
		RawMessage: answer,
		Error:      "",
	}, nil
}

func (c *ChainStrategy) RunZapierActionStream(ctx context.Context, apiClient oai.Client, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*openai.ChatCompletionStream, error) {
	downstream, downstreamWriter, err := transport.NewOpenAIStreamingAdapter(openai.ChatCompletionRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to create streaming adapter: %w", err)
	}

	go func() {
		defer downstreamWriter.Close()

		var result string

		resp, err := c.RunZapierAction(ctx, apiClient, tool, history, action)
		if err != nil {
			log.Err(err).
				Str("action", action).
				Str("tool", tool.Name).
				Str("model", tool.Config.Zapier.Model).
				Msg("error running zapier action")
			result = err.Error()
		} else {
			if resp.Error != "" {
				result = resp.Error
			} else {
				result = resp.Message
			}
		}

		if err := transport.WriteChatCompletionStream(downstreamWriter, &openai.ChatCompletionStreamResponse{
			Choices: []openai.ChatCompletionStreamChoice{
				{
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Content: result,
					},
				},
			},
		}); err != nil {
			log.Error().Msgf("failed streaming Zapier action: %v", err)
		}
	}()

	return downstream, nil
}
