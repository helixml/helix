package types

import (
	"context"
)

// GitHubRepositoryContext allows passing certain metadata between triggers and skills, useful to prevent the LLM from making mistakes
// around IDs, links, etc.
type GitHubRepositoryContext struct {
	RemoteURL      string // For example "https://github.com/owner/repo"
	Owner          string
	RepositoryName string
	PullRequestID  int

	HeadSHA    string // PR head commit
	BaseSHA    string // Base branch commit
	HeadBranch string // For example "feature/my-feature"
	BaseBranch string // For example "main"
}

type GitHubRepositoryContextKeyType string

const GitHubRepositoryContextKey GitHubRepositoryContextKeyType = "github_repository_context"

func SetGitHubRepositoryContext(ctx context.Context, vals GitHubRepositoryContext) context.Context {
	ctx = context.WithValue(ctx, GitHubRepositoryContextKey, vals)
	return ctx
}

func GetGitHubRepositoryContext(ctx context.Context) (GitHubRepositoryContext, bool) {
	vals, ok := ctx.Value(GitHubRepositoryContextKey).(GitHubRepositoryContext)
	if !ok {
		return GitHubRepositoryContext{}, false
	}
	return vals, true
}
