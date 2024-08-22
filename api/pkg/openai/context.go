package openai

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

type contextValues struct {
	OwnerID       string
	SessionID     string
	InteractionID string
}

const contextValuesKey = "contextValues"

type ContextValues struct {
	OwnerID       string
	SessionID     string
	InteractionID string
	Step          types.LLMCallStep
}

func SetContextValues(ctx context.Context, vals *ContextValues) context.Context {
	return context.WithValue(ctx, contextValuesKey, vals)
}

func GetContextValues(ctx context.Context) (*ContextValues, bool) {
	if ctx == nil {
		return nil, false
	}

	values, ok := ctx.Value(contextValuesKey).(*ContextValues)
	if !ok {
		return nil, false
	}

	return values, true
}
