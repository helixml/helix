package server

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

type systemTurnKey struct{}

// withSystemTurn marks ctx as an in-process Helix call that sends a turn
// through an HTTP handler. Only server code can set it; no request header or
// body reaches it.
func withSystemTurn(ctx context.Context, provenance types.TurnProvenance) context.Context {
	return context.WithValue(ctx, systemTurnKey{}, provenance)
}

// requestTurnProvenance is the provenance of a turn a handler is about to
// create: the in-process system caller's, or else the authenticated user's.
func requestTurnProvenance(ctx context.Context, user *types.User) (types.TurnProvenance, error) {
	if p, ok := ctx.Value(systemTurnKey{}).(types.TurnProvenance); ok {
		return p, p.Validate()
	}
	return types.RequestTurnProvenance(user)
}
