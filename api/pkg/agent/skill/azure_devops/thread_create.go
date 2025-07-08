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

const createThreadSkillDescription = `Create a thread in a pull request, use it when you need to add a comment to a pull request.
DO NOT try to pass repository ID or project ID to this skill, it is set automatically to the correct values by the trigger.`

var createThreadParameters = jsonschema.Definition{
	Type: jsonschema.Object,
	Properties: map[string]jsonschema.Definition{
		"content": {
			Type:        jsonschema.String,
			Description: "The content of the thread",
		},
	},
	Required: []string{"content"},
}

// NewCreateThreadSkill - creates a skill that allows the agent to create a thread in a pull request. This is a dedicated skill
// that expects the agent to be told what comment to write
func NewCreateThreadSkill(organizationURL string, personalAccessToken string) agent.Skill {
	client := newAzureDevOpsClient(organizationURL, personalAccessToken)
	return agent.Skill{
		Name:        "AzureDevOpsCreateThread",
		Description: createThreadSkillDescription,
		Parameters:  createThreadParameters,
		Direct:      true,
		Tools: []agent.Tool{
			&AzureDevOpsPullRequestCreateThreadTool{
				client: client,
			},
		},
	}
}

// AzureDevOpsPullRequestCommentTool - allows the agent to comment on a pull request
type AzureDevOpsPullRequestCreateThreadTool struct { //nolint:revive
	client *azureDevOpsClient
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Name() string {
	return "CreateThread"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Description() string {
	return "Create a thread in a pull request, useful for adding comments to a pull request"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Icon() string {
	return ""
}

func (t *AzureDevOpsPullRequestCreateThreadTool) String() string {
	return "CreateThread"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) StatusMessage() string {
	return "Creating a thread in a pull request"
}

func (t *AzureDevOpsPullRequestCreateThreadTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "CreateThread",
				Description: createThreadSkillDescription,
				Parameters:  createThreadParameters,
			},
		},
	}
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Execute(ctx context.Context, _ agent.Meta, args map[string]interface{}) (string, error) {
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

	createThreadArgs := git.CreateThreadArgs{
		CommentThread: &git.GitPullRequestCommentThread{
			Comments: &comment,
		},
		RepositoryId:  &azureCtx.RepositoryID,
		PullRequestId: &azureCtx.PullRequestID,
		Project:       &azureCtx.ProjectID,
	}

	createdThread, err := gitClient.CreateThread(ctx, createThreadArgs)
	if err != nil {
		return "", fmt.Errorf("failed to create thread: %w", err)
	}

	return fmt.Sprintf("Thread created: %d", createdThread.Id), nil
}
