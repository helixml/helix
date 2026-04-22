package github

import (
	"context"
	"fmt"
	"strconv"

	"github.com/google/go-github/v57/github"
	"github.com/sashabaranov/go-openai"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"
)

const createReviewCommentSkillDescription = `Create a review comment on a GitHub pull request. Use it when you need to add a comment to a pull request.
If you provide file_path and line_number, the comment will be an inline review comment on that specific line.
If you omit file_path and line_number, the comment will be a general comment on the pull request.
DO NOT try to pass owner or repository name to this skill, they are set automatically to the correct values by the trigger.`

var createReviewCommentParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"content": {
			Type:        jsonschema.String,
			Description: "The content of the comment",
		},
		"file_path": {
			Type:        jsonschema.String,
			Description: "The relative file path to comment on (e.g. src/main.go). Required for inline comments.",
		},
		"line_number": {
			Type:        jsonschema.Integer,
			Description: "The line number in the file to comment on. Starts at 1. Required for inline comments.",
		},
	},
	Required: []string{"content"},
}

func NewCreateReviewCommentSkill(token, baseURL string) agent.Skill {
	return agent.Skill{
		Name:        "GitHubCreateReviewComment",
		Description: createReviewCommentSkillDescription,
		Parameters:  createReviewCommentParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&GitHubCreateReviewCommentTool{
				token:   token,
				baseURL: baseURL,
			},
		},
	}
}

type GitHubCreateReviewCommentTool struct {
	token   string
	baseURL string
}

func (t *GitHubCreateReviewCommentTool) Name() string {
	return "GitHubCreateReviewComment"
}

func (t *GitHubCreateReviewCommentTool) Description() string {
	return "Create a review comment on a GitHub pull request"
}

func (t *GitHubCreateReviewCommentTool) Icon() string {
	return ""
}

func (t *GitHubCreateReviewCommentTool) String() string {
	return "GitHubCreateReviewComment"
}

func (t *GitHubCreateReviewCommentTool) StatusMessage() string {
	return "Creating a review comment on a GitHub pull request"
}

func (t *GitHubCreateReviewCommentTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "GitHubCreateReviewComment",
				Description: createReviewCommentSkillDescription,
				Parameters:  createReviewCommentParameters,
			},
		},
	}
}

func (t *GitHubCreateReviewCommentTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	ghCtx, ok := types.GetGitHubRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("github repository context not found")
	}

	client := NewClientWithPATAndBaseURL(t.token, t.baseURL)

	content = fmt.Sprintf("[Helix] %s", content)

	filePath, hasFilePath := args["file_path"].(string)
	lineNumberRaw, hasLineNumber := args["line_number"]

	if hasFilePath && hasLineNumber {
		lineNumber, err := parseInt(lineNumberRaw)
		if err != nil {
			return "", fmt.Errorf("failed to parse line_number: %w", err)
		}
		if lineNumber < 1 {
			lineNumber = 1
		}

		comment := &github.PullRequestComment{
			Body:     &content,
			CommitID: &ghCtx.HeadSHA,
			Path:     &filePath,
			Line:     &lineNumber,
		}

		created, _, err := client.client.PullRequests.CreateComment(ctx, ghCtx.Owner, ghCtx.RepositoryName, ghCtx.PullRequestID, comment)
		if err != nil {
			return "", fmt.Errorf("failed to create inline review comment: %w", err)
		}

		return fmt.Sprintf("Inline review comment created on %s line %d (ID: %d)", filePath, lineNumber, created.GetID()), nil
	}

	comment := &github.IssueComment{
		Body: &content,
	}

	created, _, err := client.client.Issues.CreateComment(ctx, ghCtx.Owner, ghCtx.RepositoryName, ghCtx.PullRequestID, comment)
	if err != nil {
		return "", fmt.Errorf("failed to create PR comment: %w", err)
	}

	return fmt.Sprintf("PR comment created (ID: %d)", created.GetID()), nil
}

func parseInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	}
	return 0, fmt.Errorf("invalid integer value")
}
