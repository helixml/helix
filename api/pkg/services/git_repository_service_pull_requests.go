package services

import (
	"context"
	"fmt"
	"strconv"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/types"
	// "github.com/microsoft/azure-devops-go-api/azuredevops/v7"
)

// CreatePullRequest opens a pull request in the external repository. Should be called after the changes are committed to the local repository and
// it has been pushed to the external repository.
func (s *GitRepositoryService) CreatePullRequest(ctx context.Context, repoID string, title string, description string, branch string) (string, error) {
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.ExternalURL == "" {
		return "", fmt.Errorf("repository is not external, cannot create pull request")
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeADO:
		return s.createAzureDevOpsPullRequest(ctx, repo, title, description, branch)

	default:
		return "", fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (s *GitRepositoryService) createAzureDevOpsPullRequest(ctx context.Context, repo *types.GitRepository, title string, description string, branch string) (string, error) {

	if repo.AzureDevOps == nil {
		return "", fmt.Errorf("azure devops repository not found")
	}

	if repo.AzureDevOps.OrganizationURL == "" {
		return "", fmt.Errorf("azure devops organization URL not found")
	}

	if repo.AzureDevOps.PersonalAccessToken == "" {
		return "", fmt.Errorf("azure devops personal access token not found, get yours from https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows")
	}

	client := azuredevops.NewAzureDevOpsClient(repo.AzureDevOps.OrganizationURL, repo.AzureDevOps.PersonalAccessToken)

	pr, err := client.CreatePullRequest(ctx, repo.ID, title, description, branch, "main", repo.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	if pr.PullRequestId == nil {
		return "", fmt.Errorf("pull request ID is nil")
	}

	id := strconv.Itoa(*pr.PullRequestId)

	return id, nil
}
