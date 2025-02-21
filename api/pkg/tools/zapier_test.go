package tools

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	openai "github.com/sashabaranov/go-openai"
)

func (suite *ActionTestSuite) TestAction_GetLastEmail() {
	if suite.zapierAPIKey == "" {
		suite.T().Skip("Skipping test because Zapier API key is not set")
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: "What's the last email from Tamao?",
		},
	}

	resp, err := suite.strategy.RunZapierAction(suite.ctx, suite.strategy.apiClient, &types.Tool{
		Config: types.ToolConfig{
			Zapier: &types.ToolZapierConfig{
				APIKey:        suite.zapierAPIKey,
				Model:         "mistralai/Mixtral-8x7B-Instruct-v0.1",
				MaxIterations: 3,
			},
		},
	}, history, "todo")
	suite.NoError(err)

	fmt.Println(resp.Message)
}
