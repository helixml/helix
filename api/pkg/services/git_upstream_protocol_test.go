package services

import (
	"context"
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
	upstreamGitAllowedProtocols = "http:https"
	t.Cleanup(func() { upstreamGitAllowedProtocols = "file:http:https" })
	// A permissive value inherited from the server environment must not
	// widen what a user-supplied URL may use.
	t.Setenv("GIT_ALLOW_PROTOCOL", "file:ext:http:https")

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
