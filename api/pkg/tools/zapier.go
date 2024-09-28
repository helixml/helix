package tools

import (
	"context"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"

	openai "github.com/sashabaranov/go-openai"
	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	oai "github.com/tmc/langchaingo/llms/openai"
	"github.com/tmc/langchaingo/tools"
	"github.com/tmc/langchaingo/tools/zapier"
)

func (c *ChainStrategy) RunZapierAction(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*RunActionResponse, error) {
	llm, err := oai.New()
	if err != nil {
		panic(err)
	}
	// set env variable ZAPIER_NLA_API_KEY to your Zapier API key

	// get all the available zapier NLA Tools
	tks, err := zapier.Toolkit(ctx, zapier.ToolkitOpts{
		APIKey: "sk-ak-ytQKPzjpgoaRZmSPx1hPHJ3mUc",
		// APIKey: "SOME_KEY_HERE", Or pass in a key here
		// AccessToken: "ACCESS_TOKEN", this is if your using OAuth
	})
	if err != nil {
		panic(err)
	}

	agentTools := []tools.Tool{
		// define tools here
	}
	// add the zapier tools to the existing agentTools
	agentTools = append(agentTools, tks...)

	// Initialize the agent
	agent := agents.NewOneShotAgent(llm,
		agentTools,
		agents.WithMaxIterations(3))
	executor := agents.NewExecutor(agent)

	// run a chain with the executor and defined input
	input := "Get the last email from noreply@github.com"
	answer, err := chains.Run(context.Background(), executor, input)
	if err != nil {
		panic(err)
	}
	fmt.Println(answer)
	return &RunActionResponse{
		Message:    answer,
		RawMessage: answer,
		Error:      "",
	}, nil
}

func (c *ChainStrategy) RunZapierActionStream(ctx context.Context, tool *types.Tool, history []*types.ToolHistoryMessage, action string) (*openai.ChatCompletionStream, error) {
	return nil, nil
}

func (c *ChainStrategy) newClient(tool *types.Tool) (*oai.LLM, error) {
	return oai.New(
		oai.WithModel(tool.Config.Zapier.Model),
		oai.WithHTTPClient(http.DefaultClient),
	)
}
