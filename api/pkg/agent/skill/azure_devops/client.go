package azuredevops

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

// TODO: move to separate pkg/git/azure_devops package
type AzureDevOpsClient struct { //nolint:revive
	connection *azuredevops.Connection
}

func NewAzureDevOpsClientFromApp(app *types.App) (*AzureDevOpsClient, error) {
	// Find credentials in the app spec
	for _, tool := range app.Config.Helix.Assistants[0].Tools {
		if tool.Config.AzureDevOps != nil &&
			tool.Config.AzureDevOps.Enabled &&
			tool.Config.AzureDevOps.OrganizationURL != "" &&
			tool.Config.AzureDevOps.PersonalAccessToken != "" {
			return NewAzureDevOpsClient(tool.Config.AzureDevOps.OrganizationURL, tool.Config.AzureDevOps.PersonalAccessToken), nil
		}
	}

	return nil, fmt.Errorf("no Azure DevOps credentials found")
}

func NewAzureDevOpsClient(organizationURL string, personalAccessToken string) *AzureDevOpsClient {
	connection := azuredevops.NewPatConnection(organizationURL, personalAccessToken)

	return &AzureDevOpsClient{
		connection: connection,
	}
}

func (c *AzureDevOpsClient) GetComments(ctx context.Context, repositoryID string, pullRequestID int, threadID int) ([]git.Comment, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	comments, err := gitClient.GetComments(ctx, git.GetCommentsArgs{
		RepositoryId:  &repositoryID,
		PullRequestId: &pullRequestID,
		ThreadId:      &threadID,
	})
	if err != nil {
		return nil, err
	}

	return *comments, nil
}
