package controller

import (
	"context"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	goai "github.com/sashabaranov/go-openai"
)

// TestEvalAndAddOAuthTokens provides test access to the evalAndAddOAuthTokens method
func (c *Controller) TestEvalAndAddOAuthTokens(ctx context.Context, client openai.Client, opts *ChatCompletionOptions, user *types.User) error {
	return c.evalAndAddOAuthTokens(ctx, client, opts, user)
}

// TestSelectAndConfigureTool provides test access to the selectAndConfigureTool method
func (c *Controller) TestSelectAndConfigureTool(ctx context.Context, user *types.User, req goai.ChatCompletionRequest, opts *ChatCompletionOptions) (*types.Tool, *tools.IsActionableResponse, bool, error) {
	return c.selectAndConfigureTool(ctx, user, req, opts)
}
