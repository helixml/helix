package types

import "context"

type HelixProjectContext struct {
	ProjectID string // Helix internal project ID
}

type HelixProjectContextKeyType string

const HelixProjectContextKey HelixProjectContextKeyType = "helix_project_context"

func SetHelixProjectContext(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, HelixProjectContextKey, projectID)
}

func GetHelixProjectContext(ctx context.Context) (HelixProjectContext, bool) {
	vals, ok := ctx.Value(HelixProjectContextKey).(HelixProjectContext)
	if !ok {
		return HelixProjectContext{}, false
	}
	return vals, true
}