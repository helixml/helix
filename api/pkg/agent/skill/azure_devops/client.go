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
	// Check for assistants before accessing
	if len(app.Config.Helix.Assistants) == 0 {
		return nil, fmt.Errorf("app %s has no assistants configured", app.ID)
	}

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

// ListRepositories lists all repositories accessible in the Azure DevOps organization
func (c *AzureDevOpsClient) ListRepositories(ctx context.Context, project string) ([]git.GitRepository, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps git client: %w", err)
	}

	// If project is empty, list all repos in the organization
	var projectPtr *string
	if project != "" {
		projectPtr = &project
	}

	repos, err := gitClient.GetRepositories(ctx, git.GetRepositoriesArgs{
		Project: projectPtr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	if repos == nil {
		return []git.GitRepository{}, nil
	}

	return *repos, nil
}

// ListProjects lists all projects in the Azure DevOps organization by extracting from repositories
func (c *AzureDevOpsClient) ListProjects(ctx context.Context) ([]string, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps git client: %w", err)
	}

	// Get all repos and extract unique project names
	repos, err := gitClient.GetRepositories(ctx, git.GetRepositoriesArgs{})
	if err != nil {
		return nil, fmt.Errorf("failed to list repositories: %w", err)
	}

	projectSet := make(map[string]bool)
	for _, repo := range *repos {
		if repo.Project != nil && repo.Project.Name != nil {
			projectSet[*repo.Project.Name] = true
		}
	}

	projects := make([]string, 0, len(projectSet))
	for name := range projectSet {
		projects = append(projects, name)
	}

	return projects, nil
}
