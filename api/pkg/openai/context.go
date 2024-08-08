package openai

import "context"

type contextValues struct {
	OwnerID       string
	SessionID     string
	InteractionID string
}

const contextValuesKey = "contextValues"

func SetContextValues(ctx context.Context, ownerID, sessionID, interactionID string) context.Context {
	return context.WithValue(ctx, contextValuesKey, contextValues{
		OwnerID:       ownerID,
		SessionID:     sessionID,
		InteractionID: interactionID,
	})
}

func GetContextValues(ctx context.Context) (ownerID, sessionID, interactionID string) {
	if ctx == nil {
		return "", "", ""
	}

	values, ok := ctx.Value(contextValuesKey).(contextValues)
	if !ok {
		return "", "", ""
	}

	return values.OwnerID, values.SessionID, values.InteractionID
}
