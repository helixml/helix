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

const updateThreadSkillDescription = `Update a thread in a pull request, use it when you need to add a comment to a pull request.
DO NOT try to pass repository ID or project ID to this skill, it is set automatically to the correct values by the trigger.`

var updateThreadParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"content": {
			Type:        jsonschema.String,
			Description: "The content of the thread",
		},
	},
	Required: []string{"content"},
}

// NewUpdateThreadSkill - creates a skill that allows the agent to update a thread in a pull request. This is a dedicated skill
// that expects the agent to be told what comment to write
func NewUpdateThreadSkill(organizationURL string, personalAccessToken string) agent.Skill {
	client := newAzureDevOpsClient(organizationURL, personalAccessToken)
	return agent.Skill{
		Name:        "AzureDevOpsUpdateThread",
		Description: updateThreadSkillDescription,
		Parameters:  updateThreadParameters,
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
	client *azureDevOpsClient
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
				Name:        "UpdateThread",
				Description: updateThreadSkillDescription,
				Parameters:  updateThreadParameters,
			},
		},
	}
}

func (t *AzureDevOpsPullRequestUpdateThreadTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	threadID, ok := args["thread_id"].(int)
	if !ok {
		return "", fmt.Errorf("thread_id is required")
	}

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
		ThreadId:      &threadID,
	}

	createdThread, err := gitClient.UpdateThread(ctx, updateThreadArgs)
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	return fmt.Sprintf("Thread created: %d", createdThread.Id), nil
}
