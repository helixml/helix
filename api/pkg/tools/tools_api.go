package tools

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

type RunActionResponse struct {
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (c *ChainStrategy) RunAction(ctx context.Context, tool *types.Tool, history []*types.Interaction, currentMessage string) (*RunActionResponse, error) {

}
