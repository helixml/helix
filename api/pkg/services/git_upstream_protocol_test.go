package services

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Upstream URLs are user input: local paths, file:// and ext:: must not be
// usable to clone repositories from the API server's own filesystem or to
// run commands.
func TestCloneUpstreamMirror_RefusesNonNetworkTransports(t *testing.T) {
	// TestMain loosens the restriction for the rest of the package.
	t.Setenv("GIT_ALLOW_PROTOCOL", "")
	require.NoError(t, os.Unsetenv("GIT_ALLOW_PROTOCOL"))

	dir := t.TempDir()
	local := filepath.Join(dir, "server-side.git")
	require.NoError(t, exec.Command("git", "init", "-q", "--bare", local).Run())
	marker := filepath.Join(dir, "ext-ran")

	for _, rawURL := range []string{
		local,
		"file://" + local,
		"ext::sh -c touch% " + marker,
	} {
		dest := filepath.Join(t.TempDir(), "mirror.git")
		err := cloneUpstreamMirror(context.Background(), newUpstreamRemote(rawURL, "", ""), dest)
		require.Error(t, err, rawURL)
		require.Contains(t, err.Error(), "not allowed", rawURL)
		require.NoDirExists(t, dest, rawURL)
	}
	require.NoFileExists(t, marker)
}
