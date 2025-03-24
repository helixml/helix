package tools

import (
	"context"
	"os"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/gptscript"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/kelseyhightower/envconfig"
	openai_ext "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

func TestActionTestSuite(t *testing.T) {
	suite.Run(t, new(ActionTestSuite))
}

type ActionTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	executor     *gptscript.MockExecutor
	store        *store.MockStore
	ctx          context.Context
	strategy     *ChainStrategy
	zapierAPIKey string
}

func (suite *ActionTestSuite) SetupTest() {
	suite.ctx = context.Background()

	suite.ctrl = gomock.NewController(suite.T())

	suite.executor = gptscript.NewMockExecutor(suite.ctrl)
	suite.store = store.NewMockStore(suite.ctrl)

	suite.zapierAPIKey = os.Getenv("ZAPIER_API_KEY")

	var cfg config.ServerConfig
	err := envconfig.Process("", &cfg)
	suite.NoError(err)

	var apiClient openai.Client

	if cfg.Providers.TogetherAI.APIKey != "" {
		apiClient = openai.New(
			cfg.Providers.TogetherAI.APIKey,
			cfg.Providers.TogetherAI.BaseURL)
		cfg.Tools.Model = "meta-llama/Llama-3-8b-chat-hf"
	} else {
		apiClient = openai.NewMockClient(suite.ctrl)
	}

	strategy, err := NewChainStrategy(&cfg, suite.store, suite.executor, apiClient)
	suite.NoError(err)

	suite.strategy = strategy
}

func (suite *ActionTestSuite) TestIsActionable_Yes() {
	tools := []*types.Tool{
		{
			Name:     "weatherAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
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
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai_ext.ChatMessageRoleUser,
			Content: "What is the weather like in San Francisco?",
		},
	}

	resp, err := suite.strategy.IsActionable(suite.ctx, "session-123", "i-123", tools, history)
	suite.Require().NoError(err)

	suite.strategy.wg.Wait()

	suite.Equal("yes", resp.NeedsTool)
	suite.Equal("getWeather", resp.API)
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
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
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
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai_ext.ChatMessageRoleUser,
			Content: "What is the weather like in San Francisco?",
		},
	}

	resp, err := suite.strategy.IsActionable(suite.ctx, "session-123", "i-123", tools, history)
	suite.Require().NoError(err)

	suite.strategy.wg.Wait()

	suite.Equal("yes", resp.NeedsTool)
	suite.Equal("getWeather", resp.API)
}

func (suite *ActionTestSuite) TestIsActionable_NotActionable() {
	tools := []*types.Tool{
		{
			Name:     "weatherAPI",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
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
				API: &types.ToolAPIConfig{
					Actions: []*types.ToolAPIAction{
						{
							Name:        "getProductDetails",
							Description: "database API that can be used to query product information in the database",
						},
					},
				},
			},
		},
	}

	history := []*types.ToolHistoryMessage{
		{
			Role:    openai_ext.ChatMessageRoleUser,
			Content: "What's the reason why oceans have less fish??",
		},
	}

	resp, err := suite.strategy.IsActionable(suite.ctx, "session-123", "i-123", tools, history)
	suite.NoError(err)

	suite.strategy.wg.Wait()

	suite.Equal("no", resp.NeedsTool)
	suite.Equal("", resp.API)
}
