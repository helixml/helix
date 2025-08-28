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

	RemoteURL string // For example "https://dev.azure.com/helixml/helix-agents/_git/helix-agents"

	LastMergeSourceCommitID string // PR commit
	LastMergeTargetCommitID string // Master branch (for example 'main')

	SourceRefName string // For example "refs/heads/feature/pr-2"
	TargetRefName string // For example "refs/heads/main"

	ThreadID  int // Thread that triggered this event
	CommentID int // Comment that triggered this event
}

type AzureDevopsRepositoryContextKeyType string

const AzureDevopsRepositoryContextKey AzureDevopsRepositoryContextKeyType = "azure_devops_repository_context"

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
