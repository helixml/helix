package tools

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/golang/mock/gomock"
	"github.com/kelseyhightower/envconfig"
	openai_ext "github.com/lukemarsden/go-openai2"
	"github.com/stretchr/testify/suite"
)

func TestActionTestSuite(t *testing.T) {
	suite.Run(t, new(ActionTestSuite))
}

type ActionTestSuite struct {
	suite.Suite
	ctrl     *gomock.Controller
	executor *gptscript.MockExecutor
	ctx      context.Context
	strategy *ChainStrategy
}

func (suite *ActionTestSuite) SetupTest() {
	suite.ctx = context.Background()

	suite.ctrl = gomock.NewController(suite.T())

	suite.executor = gptscript.NewMockExecutor(suite.ctrl)

	var cfg config.ServerConfig
	err := envconfig.Process("", &cfg)
	suite.NoError(err)

	strategy, err := NewChainStrategy(&cfg, nil, suite.executor, nil)
	strategy.Local = true
	suite.NoError(err)

	suite.strategy = strategy
}

func (suite *ActionTestSuite) TestIsActionable_Yes() {
	tools := []*types.Tool{
		{
			Name:     "weatherAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getWeather",
							Description: "Weather API that can return the current weather for a given location",
						},
					},
				},
			},
		},
		{
			Name:     "productsAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "What is the weather like in San Francisco?"

	resp, err := suite.strategy.IsActionable(suite.ctx, tools, history, currentMessage)
	suite.Require().NoError(err)

	suite.Equal("yes", resp.NeedsTool)
	suite.Equal("getWeather", resp.Api)
}

func (suite *ActionTestSuite) TestIsActionable_Retryable() {
	defer suite.ctrl.Finish()

	apiClient := openai.NewMockClient(suite.ctrl)
	suite.strategy.apiClient = apiClient

	apiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(openai_ext.ChatCompletionResponse{
		Choices: []openai_ext.ChatCompletionChoice{
			{
				Message: openai_ext.ChatCompletionMessage{
					Content: `incorrect json maybe? {"justification": "yes", "needs_tool": "yes", "api": "getWeather"}`,
				},
			},
		},
	}, nil)

	apiClient.EXPECT().CreateChatCompletion(gomock.Any(), gomock.Any()).Return(openai_ext.ChatCompletionResponse{
		Choices: []openai_ext.ChatCompletionChoice{
			{
				Message: openai_ext.ChatCompletionMessage{
					Content: `{"justification": "yes", "needs_tool": "yes", "api": "getWeather"}`,
				},
			},
		},
	}, nil)

	tools := []*types.Tool{
		{
			Name:     "weatherAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getWeather",
							Description: "Weather API that can return the current weather for a given location",
						},
					},
				},
			},
		},
		{
			Name:     "productsAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "What is the weather like in San Francisco?"

	resp, err := suite.strategy.IsActionable(suite.ctx, tools, history, currentMessage)
	suite.Require().NoError(err)

	suite.Equal("yes", resp.NeedsTool)
	suite.Equal("getWeather", resp.Api)
}

func (suite *ActionTestSuite) TestIsActionable_NotActionable() {
	tools := []*types.Tool{
		{
			Name:     "weatherAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getWeather",
							Description: "Weather API that can return the current weather for a given location",
						},
					},
				},
			},
		},
		{
			Name:     "productsAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolApiConfig{
					Actions: []*types.ToolApiAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "What's the reason why oceans have less fish??"

	resp, err := suite.strategy.IsActionable(suite.ctx, tools, history, currentMessage)
	suite.NoError(err)

	suite.Equal("no", resp.NeedsTool)
	suite.Equal("", resp.Api)
}
