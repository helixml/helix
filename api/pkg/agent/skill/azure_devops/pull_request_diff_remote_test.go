package azuredevops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

// The remote URL comes from an (unauthenticated) webhook payload and is cloned
// with the organization token, so it must not point anywhere else.
func TestGetPullRequestDiff_RejectsForeignRemoteURL(t *testing.T) {
	var hits atomic.Int32
	org := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer org.Close()

	for _, remote := range []string{
		"https://attacker.example/org/proj/_git/repo",
		"file:///filestore/git-repos/another-tenant",
		"/filestore/git-repos/another-tenant",
		"http://" + org.Listener.Addr().String() + "/org/_git/repo",
	} {
		_, err := GetPullRequestDiff(context.Background(), org.URL+"/org", "token", types.AzureDevopsRepositoryContext{
			RemoteURL:               remote,
			LastMergeTargetCommitID: "b0f6a1d75557bf3deb83b305c5ee6d79312545a0",
			LastMergeSourceCommitID: "30b66172254edd89648744f7421f703d78b7140a",
		}, netguard.Policy{AllowPrivate: true})
		require.Error(t, err, remote)
	}
	require.Zero(t, hits.Load())
}

func TestCheckRemoteURLMatchesOrg(t *testing.T) {
	require.NoError(t, checkRemoteURLMatchesOrg("https://helixml@dev.azure.com/helixml/p/_git/r", "https://dev.azure.com/helixml"))
	require.NoError(t, checkRemoteURLMatchesOrg("https://helixml.visualstudio.com/p/_git/r", "https://helixml.visualstudio.com/"))
	require.Error(t, checkRemoteURLMatchesOrg("https://dev.azure.com.attacker.example/helixml/p/_git/r", "https://dev.azure.com/helixml"))
	require.Error(t, checkRemoteURLMatchesOrg("http://dev.azure.com/helixml/p/_git/r", "https://dev.azure.com/helixml"))
	// Default ports are normalised; other ports are a different destination.
	require.NoError(t, checkRemoteURLMatchesOrg("https://dev.azure.com:443/helixml/p/_git/r", "https://dev.azure.com/helixml"))
	require.NoError(t, checkRemoteURLMatchesOrg("https://dev.azure.com/helixml/p/_git/r", "https://DEV.azure.com:443/helixml"))
	require.Error(t, checkRemoteURLMatchesOrg("https://dev.azure.com:8443/helixml/p/_git/r", "https://dev.azure.com/helixml"))
}
