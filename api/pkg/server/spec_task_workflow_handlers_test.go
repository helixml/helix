package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestShouldOpenPullRequest(t *testing.T) {
	s := &HelixAPIServer{}

	cases := []struct {
		name string
		repo *types.GitRepository
		want bool
	}{
		{
			name: "GitHub with OAuth connection",
			repo: &types.GitRepository{
				ExternalType:      types.ExternalRepositoryTypeGitHub,
				OAuthConnectionID: "conn_123",
			},
			want: true,
		},
		{
			name: "GitHub with PAT only",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitHub,
				GitHub:       &types.GitHub{PersonalAccessToken: "ghp_xxx"},
			},
			want: true,
		},
		{
			name: "Azure DevOps configured",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeADO,
				AzureDevOps:  &types.AzureDevOps{},
			},
			want: true,
		},
		{
			name: "GitLab with OAuth connection",
			repo: &types.GitRepository{
				ExternalType:      types.ExternalRepositoryTypeGitLab,
				OAuthConnectionID: "conn_456",
			},
			want: true,
		},
		{
			name: "GitLab with personal access token",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitLab,
				GitLab:       &types.GitLab{PersonalAccessToken: "glpat_xxx"},
			},
			want: true,
		},
		{
			name: "GitLab with bare type (auth resolved at client layer)",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeGitLab,
			},
			want: true,
		},
		{
			name: "Bitbucket — not yet wired into shouldOpenPullRequest",
			repo: &types.GitRepository{
				ExternalType: types.ExternalRepositoryTypeBitbucket,
			},
			want: false,
		},
		{
			name: "Internal repo (no external type)",
			repo: &types.GitRepository{},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, s.shouldOpenPullRequest(tc.repo))
		})
	}
}
