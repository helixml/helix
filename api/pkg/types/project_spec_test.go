package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateRepositories(t *testing.T) {
	tests := []struct {
		name    string
		spec    ProjectSpec
		wantErr string
	}{
		{
			name: "no repos — valid",
			spec: ProjectSpec{},
		},
		{
			name: "singular repository shorthand — valid",
			spec: ProjectSpec{
				Repository: &ProjectRepositorySpec{URL: "https://github.com/org/a"},
			},
		},
		{
			name: "single item in repositories list — valid (primary implied)",
			spec: ProjectSpec{
				Repositories: []ProjectRepositorySpec{
					{URL: "https://github.com/org/a"},
				},
			},
		},
		{
			name: "multi-repo with exactly one primary — valid",
			spec: ProjectSpec{
				Repositories: []ProjectRepositorySpec{
					{URL: "https://github.com/org/a", Primary: true},
					{URL: "https://github.com/org/b"},
				},
			},
		},
		{
			name: "both repository and repositories set — error",
			spec: ProjectSpec{
				Repository:   &ProjectRepositorySpec{URL: "https://github.com/org/a"},
				Repositories: []ProjectRepositorySpec{{URL: "https://github.com/org/b"}},
			},
			wantErr: "cannot specify both 'repository' and 'repositories'",
		},
		{
			name: "multi-repo with no primary — error",
			spec: ProjectSpec{
				Repositories: []ProjectRepositorySpec{
					{URL: "https://github.com/org/a"},
					{URL: "https://github.com/org/b"},
				},
			},
			wantErr: "exactly one repository must be designated primary",
		},
		{
			name: "multi-repo with two primaries — error",
			spec: ProjectSpec{
				Repositories: []ProjectRepositorySpec{
					{URL: "https://github.com/org/a", Primary: true},
					{URL: "https://github.com/org/b", Primary: true},
				},
			},
			wantErr: "only one repository may be designated primary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.ValidateRepositories()
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestResolvedRepositories(t *testing.T) {
	t.Run("empty spec returns empty slice", func(t *testing.T) {
		spec := ProjectSpec{}
		assert.Empty(t, spec.ResolvedRepositories())
	})

	t.Run("singular shorthand is always primary", func(t *testing.T) {
		spec := ProjectSpec{
			Repository: &ProjectRepositorySpec{URL: "https://github.com/org/a", DefaultBranch: "main"},
		}
		repos := spec.ResolvedRepositories()
		require.Len(t, repos, 1)
		assert.Equal(t, "https://github.com/org/a", repos[0].URL)
		assert.Equal(t, "main", repos[0].DefaultBranch)
		assert.True(t, repos[0].Primary)
	})

	t.Run("singular shorthand overrides Primary=false", func(t *testing.T) {
		spec := ProjectSpec{
			Repository: &ProjectRepositorySpec{URL: "https://github.com/org/a", Primary: false},
		}
		repos := spec.ResolvedRepositories()
		require.Len(t, repos, 1)
		assert.True(t, repos[0].Primary, "singular repository is always primary regardless of field value")
	})

	t.Run("single item in list implies primary", func(t *testing.T) {
		spec := ProjectSpec{
			Repositories: []ProjectRepositorySpec{
				{URL: "https://github.com/org/a"},
			},
		}
		repos := spec.ResolvedRepositories()
		require.Len(t, repos, 1)
		assert.True(t, repos[0].Primary)
	})

	t.Run("multi-repo preserves explicit primary flags", func(t *testing.T) {
		spec := ProjectSpec{
			Repositories: []ProjectRepositorySpec{
				{URL: "https://github.com/org/frontend", Primary: true},
				{URL: "https://github.com/org/backend"},
			},
		}
		repos := spec.ResolvedRepositories()
		require.Len(t, repos, 2)
		assert.True(t, repos[0].Primary)
		assert.False(t, repos[1].Primary)
	})
}
