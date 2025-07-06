package types

import (
	"context"
)

// AzureDevopsRepositoryContext allows passing certain metadata between triggers and skills, useful to prevent the LLM from making mistakes
// around IDs, links, etc.
type AzureDevopsRepositoryContext struct {
	RepositoryID  string
	PullRequestID int
	ProjectID     string
}

var AzureDevopsRepositoryContextKey = "azure_devops_repository_context"

func SetAzureDevopsRepositoryContext(ctx context.Context, vals AzureDevopsRepositoryContext) context.Context {
	ctx = context.WithValue(ctx, AzureDevopsRepositoryContextKey, vals)
	return ctx
}

func GetAzureDevopsRepositoryContext(ctx context.Context) (AzureDevopsRepositoryContext, bool) {
	vals, ok := ctx.Value(AzureDevopsRepositoryContextKey).(AzureDevopsRepositoryContext)
	if !ok {
		return AzureDevopsRepositoryContext{}, false
	}
	return vals, true
}
