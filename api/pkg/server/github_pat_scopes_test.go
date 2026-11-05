package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGitHubPATScopeError(t *testing.T) {
	require.NoError(t, githubPATScopeError([]string{"repo", "workflow"}, true))
	require.NoError(t, githubPATScopeError([]string{"read:org", "workflow", "repo"}, true))

	err := githubPATScopeError([]string{"repo"}, true)
	require.ErrorContains(t, err, "missing required scopes: workflow")

	err = githubPATScopeError([]string{"read:org"}, true)
	require.ErrorContains(t, err, "missing required scopes: repo, workflow")

	require.ErrorContains(t, githubPATScopeError(nil, true), "no scopes")
	require.ErrorContains(t, githubPATScopeError(nil, false), "Fine-grained")
}
