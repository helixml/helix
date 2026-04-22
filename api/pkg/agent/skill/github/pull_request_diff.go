package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v57/github"
	"github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
)

const pullRequestDiffSkillDescription = `Get the diff of a GitHub pull request, use it when you need to see the changes in a pull request.
DO NOT try to pass owner or repository name to this skill, they are set automatically to the correct values by the trigger.`

var pullRequestDiffParameters = jsonschema.Definition{
	Type:       jsonschema.Object,
	Properties: map[string]jsonschema.Definition{},
	Required:   []string{},
}

func NewPullRequestDiffSkill(token, baseURL string) agent.Skill {
	return agent.Skill{
		Name:        "GitHubPullRequestDiff",
		Description: pullRequestDiffSkillDescription,
		Parameters:  pullRequestDiffParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&GitHubPullRequestDiffTool{
				token:   token,
				baseURL: baseURL,
			},
		},
	}
}

type GitHubPullRequestDiffTool struct {
	token   string
	baseURL string
}

func (t *GitHubPullRequestDiffTool) Name() string {
	return "GitHubPullRequestDiff"
}

func (t *GitHubPullRequestDiffTool) Description() string {
	return "Get the diff of a GitHub pull request"
}

func (t *GitHubPullRequestDiffTool) Icon() string {
	return ""
}

func (t *GitHubPullRequestDiffTool) String() string {
	return "GitHubPullRequestDiff"
}

func (t *GitHubPullRequestDiffTool) StatusMessage() string {
	return "Getting GitHub pull request diff"
}

func (t *GitHubPullRequestDiffTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "GitHubPullRequestDiff",
				Description: pullRequestDiffSkillDescription,
				Parameters:  pullRequestDiffParameters,
			},
		},
	}
}

func (t *GitHubPullRequestDiffTool) Execute(ctx context.Context, _ agent.Meta, _ map[string]interface{}) (string, error) {
	ghCtx, ok := types.GetGitHubRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("github repository context not found")
	}

	client := NewClientWithPATAndBaseURL(t.token, t.baseURL)

	pr, err := client.GetPullRequest(ctx, ghCtx.Owner, ghCtx.RepositoryName, ghCtx.PullRequestID)
	if err != nil {
		return "", fmt.Errorf("failed to get pull request: %w", err)
	}

	opts := &github.ListOptions{PerPage: 100}
	files, _, err := client.client.PullRequests.ListFiles(ctx, ghCtx.Owner, ghCtx.RepositoryName, ghCtx.PullRequestID, opts)
	if err != nil {
		return "", fmt.Errorf("failed to list pull request files: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("Pull Request Details:\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", pr.GetTitle()))
	sb.WriteString(fmt.Sprintf("Description: %s\n", pr.GetBody()))
	sb.WriteString(fmt.Sprintf("Author: %s\n", pr.GetUser().GetLogin()))
	sb.WriteString(fmt.Sprintf("Head: %s (%s)\n", ghCtx.HeadBranch, ghCtx.HeadSHA))
	sb.WriteString(fmt.Sprintf("Base: %s (%s)\n\n", ghCtx.BaseBranch, ghCtx.BaseSHA))

	sb.WriteString(fmt.Sprintf("Changed files (%d):\n\n", len(files)))

	for _, f := range files {
		sb.WriteString(fmt.Sprintf("--- %s (%s, +%d -%d)\n", f.GetFilename(), f.GetStatus(), f.GetAdditions(), f.GetDeletions()))
		if patch := f.GetPatch(); patch != "" {
			sb.WriteString(patch)
			sb.WriteString("\n\n")
		}
	}

	return sb.String(), nil
}
