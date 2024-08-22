package openai

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	contextValuesKey = "contextValues"
	stepKey          = "step"
)

type Step struct {
	Step types.LLMCallStep
}

type ContextValues struct {
	OwnerID         string
	SessionID       string
	InteractionID   string
	OriginalRequest []byte
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

func SetStep(ctx context.Context, step *Step) context.Context {
	return context.WithValue(ctx, stepKey, step)
}

func GetStep(ctx context.Context) (*Step, bool) {
	if ctx == nil {
		return nil, false
	}

	step, ok := ctx.Value(stepKey).(*Step)
	if !ok {
		return nil, false
	}

	return step, true
}
