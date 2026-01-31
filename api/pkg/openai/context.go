package openai

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

type (
	contextValuesKeyType         int
	contextAppIDKeyType          int
	contextOrganizationIDKeyType int
	contextSessionIDKeyType      int
	stepKeyType                  int
)

var (
	contextValuesKey         contextValuesKeyType
	contextAppIDKey          contextAppIDKeyType
	contextSessionIDKey      contextSessionIDKeyType
	contextOrganizationIDKey contextOrganizationIDKeyType
	stepKey                  stepKeyType
)

const (
	// TODO: this needs to be removed and replaced
	// with actual RunnerID that needs to be passed
	// to the request handlers.
	RunnerID = "runner"
	SystemID = "system"
)

type Step struct {
	Step types.LLMCallStep
}

type ContextValues struct {
	OwnerID         string
	SessionID       string
	InteractionID   string
	ProjectID       string
	SpecTaskID      string
	OriginalRequest []byte
}

func SetContextAppID(ctx context.Context, appID string) context.Context {
	return context.WithValue(ctx, contextAppIDKey, appID)
}

func GetContextAppID(ctx context.Context) (string, bool) {
	appID, ok := ctx.Value(contextAppIDKey).(string)
	return appID, ok
}

func SetContextOrganizationID(ctx context.Context, organizationID string) context.Context {
	return context.WithValue(ctx, contextOrganizationIDKey, organizationID)
}

func GetContextOrganizationID(ctx context.Context) (string, bool) {
	organizationID, ok := ctx.Value(contextOrganizationIDKey).(string)
	return organizationID, ok
}

func SetContextSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, contextSessionIDKey, sessionID)
}

func GetContextSessionID(ctx context.Context) (string, bool) {
	sessionID, ok := ctx.Value(contextSessionIDKey).(string)
	return sessionID, ok
}

func SetContextValues(ctx context.Context, vals *ContextValues) context.Context {
	// Check if the context already has values, if it does,
	// preserve the OriginalRequest
	existingValues, ok := GetContextValues(ctx)
	if ok {
		vals.OriginalRequest = existingValues.OriginalRequest
	}

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
