package types

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDesktopAgent_SetRepoContext_EmptyReposIsNoOp(t *testing.T) {
	a := &DesktopAgent{
		RepositoryIDs:       []string{"preserved"},
		PrimaryRepositoryID: "preserved",
	}
	a.SetRepoContext(nil, "")
	require.Equal(t, []string{"preserved"}, a.RepositoryIDs)
	require.Equal(t, "preserved", a.PrimaryRepositoryID)
}

func TestDesktopAgent_SetRepoContext_DefaultRepoIDWins(t *testing.T) {
	repos := []*GitRepository{
		{ID: "repo_a"},
		{ID: "repo_b"},
	}
	a := &DesktopAgent{}
	a.SetRepoContext(repos, "repo_b")
	require.Equal(t, []string{"repo_a", "repo_b"}, a.RepositoryIDs)
	require.Equal(t, "repo_b", a.PrimaryRepositoryID)
}

func TestDesktopAgent_SetRepoContext_NoDefaultUsesFirstRepo(t *testing.T) {
	repos := []*GitRepository{
		{ID: "repo_a"},
		{ID: "repo_b"},
	}
	a := &DesktopAgent{}
	a.SetRepoContext(repos, "")
	require.Equal(t, []string{"repo_a", "repo_b"}, a.RepositoryIDs)
	require.Equal(t, "repo_a", a.PrimaryRepositoryID)
}

func TestDesktopAgent_SetRepoContext_FiltersEmptyIDsAndNils(t *testing.T) {
	repos := []*GitRepository{
		nil,
		{ID: ""},
		{ID: "repo_real"},
	}
	a := &DesktopAgent{}
	a.SetRepoContext(repos, "")
	require.Equal(t, []string{"repo_real"}, a.RepositoryIDs)
	require.Equal(t, "repo_real", a.PrimaryRepositoryID)
}

func TestDesktopAgent_SetRepoContext_AllFilteredIsNoOp(t *testing.T) {
	a := &DesktopAgent{
		RepositoryIDs:       []string{"preserved"},
		PrimaryRepositoryID: "preserved",
	}
	a.SetRepoContext([]*GitRepository{nil, {ID: ""}}, "")
	require.Equal(t, []string{"preserved"}, a.RepositoryIDs)
	require.Equal(t, "preserved", a.PrimaryRepositoryID)
}
