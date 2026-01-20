package services

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestGetPullRequestURL(t *testing.T) {
	tests := []struct {
		name          string
		repo          *types.GitRepository
		pullRequestID string
		expected      string
	}{
		{
			name: "GitHub repo without .git suffix",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/chocobar/demo-recipes",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			pullRequestID: "1",
			expected:      "https://github.com/chocobar/demo-recipes/pull/1",
		},
		{
			name: "GitHub repo with .git suffix",
			repo: &types.GitRepository{
				ExternalURL:  "https://github.com/chocobar/demo-recipes.git",
				ExternalType: types.ExternalRepositoryTypeGitHub,
			},
			pullRequestID: "1",
			expected:      "https://github.com/chocobar/demo-recipes/pull/1",
		},
		{
			name: "Azure DevOps repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://dev.azure.com/org/project/_git/repo",
				ExternalType: types.ExternalRepositoryTypeADO,
			},
			pullRequestID: "42",
			expected:      "https://dev.azure.com/org/project/_git/repo/pullrequest/42",
		},
		{
			name: "GitLab repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://gitlab.com/org/repo",
				ExternalType: types.ExternalRepositoryTypeGitLab,
			},
			pullRequestID: "123",
			expected:      "https://gitlab.com/org/repo/merge_requests/123",
		},
		{
			name: "Bitbucket repo",
			repo: &types.GitRepository{
				ExternalURL:  "https://bitbucket.org/org/repo",
				ExternalType: types.ExternalRepositoryTypeBitbucket,
			},
			pullRequestID: "99",
			expected:      "https://bitbucket.org/org/repo/pull-requests/99",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetPullRequestURL(tt.repo, tt.pullRequestID)
			if result != tt.expected {
				t.Errorf("GetPullRequestURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}
