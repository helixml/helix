package tools

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
)

func TestActionTestSuite(t *testing.T) {
	suite.Run(t, new(ActionTestSuite))
}

type ActionTestSuite struct {
	suite.Suite
	ctx      context.Context
	strategy *ChainStrategy
}

func (suite *ActionTestSuite) SetupTest() {
	suite.ctx = context.Background()

	var cfg Config
	err := envconfig.Process("", &cfg)
	suite.NoError(err)

	spew.Dump(cfg)

	strategy, err := NewChainStrategy(&cfg)
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
	suite.NoError(err)

	suite.Equal("yes", resp.NeedsApi)
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

	suite.Equal("no", resp.NeedsApi)
	suite.Equal("", resp.Api)
}
