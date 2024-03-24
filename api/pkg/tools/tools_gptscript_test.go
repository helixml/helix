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
