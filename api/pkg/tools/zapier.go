package tools

import (
	"context"
	"fmt"
	"net/http"

	helix_langchain "github.com/helixml/helix/api/pkg/openai/langchain"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"

	openai "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	oai "github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
	"github.com/tmc/langchaingo/tools/zapier"
)

func (c *ChainStrategy) RunZapierAction(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*RunActionResponse, error) {
	llm, err := helix_langchain.New(c.apiClient, tool.Config.Zapier.Model)
	if err != nil {
		return nil, err
	}

	currentMessage := history[len(history)-1].Content

	// get all the available zapier NLA Tools
	tks, err := zapier.Toolkit(ctx, zapier.ToolkitOpts{
		APIKey: tool.Config.Zapier.APIKey,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Zapier integration, error: %w", err)
	}

	agentTools := []tools.Tool{
		// define tools here
	}
	// add the zapier tools to the existing agentTools
	agentTools = append(agentTools, tks...)

	iterations := 3
	if tool.Config.Zapier.MaxIterations != 0 {
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

func (c *ChainStrategy) RunZapierActionStream(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*openai.ChatCompletionStream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *ChainStrategy) newClient(tool *types.Tool) (*oai.LLM, error) {
	return oai.New(
		oai.WithModel(tool.Config.Zapier.Model),
		oai.WithHTTPClient(http.DefaultClient),
	)
}
