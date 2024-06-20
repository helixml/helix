package controller

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

// TODO: add streaming options to the call
// TODO: types.InternalSessionRequest strip to the minimal version of it

func (c *Controller) RunChatCompletion(ctx context.Context, user *types.User, req types.InternalSessionRequest) (*types.Session, error) {
	return nil, nil
}
