package tools

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	"github.com/helixml/helix/api/pkg/types"
)

const echoGPT = `description: Returns back the input of the script
args: input: Any string
echo "${input}"`

func (suite *ActionTestSuite) TestAction_runGPTScriptAction_helloWorld() {

	echoGptScript := &types.Tool{
		Name:        "echo",
		Description: "echo tool, use it when you need to echo back the input",
		ToolType:    types.ToolTypeGPTScript,
		Config: types.ToolConfig{
			GPTScript: &types.ToolGPTScriptConfig{
				Script: echoGPT,
			},
		},
	}

	history := []*types.Interaction{}

	currentMessage := "echo back 'Hello World'"

	resp, err := suite.strategy.RunAction(suite.ctx, echoGptScript, history, currentMessage, "echo")
	suite.NoError(err)

	suite.Assert().Contains(resp.Message, "Hello World")

	spew.Dump(resp)

	fmt.Println("U:", currentMessage)
	fmt.Println("A:", resp.Message)
}

const truckGPTDescription = `is an intelligent remote system that should be used when getting asking for information about trucks`

const truckGPT = `name: jarvis
description: I'm jarvis, a truck guy.
args: question: The question to ask Jarvis about trucks.

When asked about trucks, respond with "Thanks for asking "${question}", I'm am looking into it and will send you an email once I am done!"`
