package azuredevops

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/util/jsonschema"

	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
	"github.com/sashabaranov/go-openai"
)

const replyToCommentSkillDescription = `Reply to a comment in a pull request, use it when you need to add a comment to a pull request.
DO NOT try to pass repository ID or project ID to this skill, it is set automatically to the correct values by the trigger.`

var replyToCommentParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"content": {
			Type:        jsonschema.String,
			Description: "Contents of the reply that will be sent to the thread",
		},
	},
	Required: []string{"content"},
}

// NewUpdateThreadSkill - creates a skill that allows the agent to update a thread in a pull request. This is a dedicated skill
// that expects the agent to be told what comment to write
func NewReplyToCommentSkill(organizationURL string, personalAccessToken string) agent.Skill {
	client := NewAzureDevOpsClient(organizationURL, personalAccessToken)
	return agent.Skill{
		Name:        "AzureDevOpsReplyToComment",
		Description: replyToCommentSkillDescription,
		Parameters:  replyToCommentParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&AzureDevOpsPullRequestUpdateThreadTool{
				client: client,
			},
		},
	}
}

// AzureDevOpsPullRequestCommentTool - allows the agent to comment on a pull request
type AzureDevOpsPullRequestUpdateThreadTool struct { //nolint:revive
	client *AzureDevOpsClient
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) Name() string {
	return "UpdateThread"
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) Description() string {
	return "Update a thread in a pull request, useful for adding comments to a pull request"
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) Icon() string {
	return ""
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) String() string {
	return "UpdateThread"
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) StatusMessage() string {
	return "Updating a thread in a pull request"
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "ReplyToComment",
				Description: replyToCommentSkillDescription,
				Parameters:  replyToCommentParameters,
			},
		},
	}
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("azure devops repository context not found")
	}

	gitClient, err := git.NewClient(ctx, t.client.connection)
	if err != nil {
		return "", fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	content = fmt.Sprintf("[Helix] %s", content)

	comment := []git.Comment{
		{
			Content: &content,
		},
	}

	updateThreadArgs := git.UpdateThreadArgs{
		CommentThread: &git.GitPullRequestCommentThread{
			Comments: &comment,
		},
		RepositoryId:  &azureCtx.RepositoryID,
		PullRequestId: &azureCtx.PullRequestID,
		Project:       &azureCtx.ProjectID,
		ThreadId:      &azureCtx.ThreadID,
	}

	if azureCtx.CommentID != 0 {
		updateThreadArgs.CommentThread.Id = &azureCtx.CommentID
	}

	createdThread, err := gitClient.UpdateThread(ctx, updateThreadArgs)
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	return fmt.Sprintf("Thread created: %d", createdThread.Id), nil
}
