package azuredevops

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/microsoft/azure-devops-go-api/azuredevops/v7/git"
)

func (c *AzureDevOpsClient) CreatePullRequest(ctx context.Context, repositoryID string, title string, description string, sourceBranch string, targetBranch string, project string) (*git.GitPullRequest, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	sourceRefName := fmt.Sprintf("refs/heads/%s", sourceBranch)
	targetRefName := fmt.Sprintf("refs/heads/%s", targetBranch)

	gitPullRequestToCreate := &git.GitPullRequest{
		Title:         &title,
		Description:   &description,
		SourceRefName: &sourceRefName,
		TargetRefName: &targetRefName,
	}

	supportsIterations := true

	pr, err := gitClient.CreatePullRequest(ctx, git.CreatePullRequestArgs{
		GitPullRequestToCreate: gitPullRequestToCreate,
		RepositoryId:           &repositoryID,
		Project:                &project,
		SupportsIterations:     &supportsIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return pr, nil
}

// ListPullRequests lists pull requests for a given repository name and project
// repositoryName is the name of the repository
// project is the name of the project in Azure DevOps
func (c *AzureDevOpsClient) ListPullRequests(ctx context.Context, repositoryName string, project string) ([]git.GitPullRequest, error) {
	gitClient, err := git.NewClient(ctx, c.connection)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure DevOps client: %w", err)
	}

	// repoID, err := uuid.Parse(repositoryID)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to parse repository ID: %w", err)
	// }

	prs, err := gitClient.GetPullRequests(ctx, git.GetPullRequestsArgs{
		SearchCriteria: &git.GitPullRequestSearchCriteria{
			Status: ptr.To(git.PullRequestStatusValues.Active),
		},
		RepositoryId: &repositoryName,
		Project:      &project,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	return *prs, nil
}
