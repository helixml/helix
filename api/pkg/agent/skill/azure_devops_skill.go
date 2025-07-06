package skill

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

type AzureDevOpsPullRequestSkill struct {
}

func NewAzureDevOpsPullRequestSkill() *AzureDevOpsPullRequestSkill {
	return &AzureDevOpsPullRequestSkill{}
}

// AzureDevOpsPullRequestCommentTool - allows the agent to comment on a pull request
type AzureDevOpsPullRequestCreateThreadTool struct {
	organizationURL     string
	personalAccessToken string
}

func (t *AzureDevOpsPullRequestCreateThreadTool) Execute(ctx context.Context, meta agent.Meta, args map[string]interface{}) (string, error) {
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("content is required")
	}

	azureCtx, ok := types.GetAzureDevopsRepositoryContext(ctx)
	if !ok {
		return "", fmt.Errorf("azure devops repository context not found")
	}

	connection := azuredevops.NewPatConnection(t.organizationURL, t.personalAccessToken)

	gitClient, err := git.NewClient(ctx, connection)
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
