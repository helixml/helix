package git

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
)

type recordingServer struct {
	*httptest.Server
	hits atomic.Int32
	auth atomic.Value
}

func newRecordingServer(t *testing.T, h http.HandlerFunc) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.hits.Add(1)
		rs.auth.Store(r.Header.Get("Authorization"))
		if h != nil {
			h(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(rs.Close)
	return rs
}

// A redirect from the (vetted) remote must not take the clone, or the token
// it carries, to another host.
func TestCloneRepository_DoesNotFollowRedirects(t *testing.T) {
	internal := newRecordingServer(t, nil)
	remote := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+r.URL.RequestURI(), http.StatusFound)
	})

	_, err := NewGitManager(remote.URL+"/org/_git/repo", "pat").CloneRepository(context.Background(), filepath.Join(t.TempDir(), "clone"))
	require.Error(t, err)
	require.NotZero(t, remote.hits.Load())
	require.Zero(t, internal.hits.Load(), "clone followed the redirect")
}

func TestCloneRepository_SendsToken(t *testing.T) {
	remote := newRecordingServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		http.NotFound(w, r)
	})
	_, _ = NewGitManager(remote.URL+"/org/_git/repo", "pat").CloneRepository(context.Background(), filepath.Join(t.TempDir(), "clone"))
	require.Equal(t, basicAuthHeader("helixml", "pat")[len("Authorization: "):], remote.auth.Load())
}

func TestCloneRepository_OutboundPolicy(t *testing.T) {
	remote := newRecordingServer(t, nil)
	_, err := NewGitManager(remote.URL+"/repo.git", "").WithOutboundPolicy(netguard.Policy{}).
		CloneRepository(context.Background(), filepath.Join(t.TempDir(), "clone"))
	require.ErrorIs(t, err, netguard.ErrBlockedDestination)
	require.Zero(t, remote.hits.Load())

	// An allowed host is pinned: git connects to the vetted address.
	u, err := url.Parse(remote.URL)
	require.NoError(t, err)
	pin, err := ResolvePin(context.Background(), netguard.Policy{AllowPrivate: true}, "http://localhost:"+u.Port()+"/repo.git")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(pin, "localhost:"+u.Port()+":"), pin)
	_, _ = NewGitManager("http://localhost:"+u.Port()+"/repo.git", "").WithOutboundPolicy(netguard.Policy{AllowPrivate: true}).
		CloneRepository(context.Background(), filepath.Join(t.TempDir(), "clone"))
	require.NotZero(t, remote.hits.Load())
}

// Option-shaped commit IDs must be refused before anything is cloned or
// checked out.
func TestGetPullRequestChange_ValidatesCommitIDsFirst(t *testing.T) {
	remote := newRecordingServer(t, nil)
	_, err := GetPullRequestChange(context.Background(), "", GetPullRequestChangeOptions{
		RemoteURL:      remote.URL + "/repo.git",
		TargetCommitID: "--orphan=x",
		SourceCommitID: "abcd",
	})
	require.ErrorContains(t, err, "invalid commit ID")
	require.Zero(t, remote.hits.Load(), "repository was cloned before the commit IDs were checked")

	repo := t.TempDir()
	require.NoError(t, exec.Command("git", "init", "-q", repo).Run())
	require.Error(t, NewGitManager("", "").CheckoutCommit(context.Background(), repo, "--orphan=x"))
}

// A name that does not resolve for the server would be resolved by git on
// its own (a resolver can answer the two lookups differently), so without a
// proxy it is refused rather than left unpinned.
func TestResolvePin_UnresolvableHostFailsClosedWithoutProxy(t *testing.T) {
	for _, k := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(k, "")
	}
	_, err := ResolvePin(context.Background(), netguard.Policy{}, "https://git.example.invalid/repo.git")
	require.ErrorIs(t, err, netguard.ErrBlockedDestination)

	// Behind an operator proxy, the proxy resolves and controls egress.
	t.Setenv("HTTPS_PROXY", "http://proxy.internal:3128")
	pin, err := ResolvePin(context.Background(), netguard.Policy{}, "https://git.example.invalid/repo.git")
	require.NoError(t, err)
	require.Empty(t, pin)
}
