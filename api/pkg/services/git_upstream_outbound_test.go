package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
)

func countingServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if h != nil {
			h(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// The upstream URL is vetted before git runs; a redirect from it must not
// take git (and the credentials it sends) to another host.
func TestUpstreamGit_DoesNotFollowRedirects(t *testing.T) {
	internal, internalHits := countingServer(t, nil)
	upstream, _ := countingServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL+r.URL.Path+"?"+r.URL.RawQuery, http.StatusFound)
	})

	dest := filepath.Join(t.TempDir(), "mirror.git")
	err := cloneUpstreamMirror(context.Background(), newUpstreamRemote(upstream.URL+"/repo.git", "u", "secret"), dest)
	require.Error(t, err)
	require.Zero(t, internalHits.Load(), "git followed the redirect")
}

// With an outbound policy set, an upstream on a disallowed address is refused
// before git runs.
func TestUpstreamGit_RefusesDisallowedAddress(t *testing.T) {
	srv, hits := countingServer(t, nil)
	s := &GitRepositoryService{}
	s.SetOutboundPolicy(netguard.Policy{})

	remote := s.pinUpstream(context.Background(), newUpstreamRemote(srv.URL+"/repo.git", "", ""))
	err := cloneUpstreamMirror(context.Background(), remote, filepath.Join(t.TempDir(), "mirror.git"))
	require.ErrorIs(t, err, netguard.ErrBlockedDestination)
	require.Zero(t, hits.Load())
}

// The approved addresses are handed to git, so it connects to them rather
// than resolving the host again.
func TestUpstreamGit_PinsVettedAddresses(t *testing.T) {
	srv, hits := countingServer(t, nil)
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)

	s := &GitRepositoryService{}
	s.SetOutboundPolicy(netguard.Policy{AllowPrivate: true})
	remote := s.pinUpstream(context.Background(), newUpstreamRemote("http://localhost:"+u.Port()+"/repo.git", "", ""))
	require.NoError(t, remote.blocked)
	require.Contains(t, remote.resolve, "localhost:"+u.Port()+":")

	// A name that does not resolve at all is reachable only through the pin.
	remote = upstreamRemote{URL: "http://git.example.invalid:" + u.Port() + "/repo.git", resolve: "git.example.invalid:" + u.Port() + ":127.0.0.1"}
	_ = cloneUpstreamMirror(context.Background(), remote, filepath.Join(t.TempDir(), "mirror.git"))
	require.NotZero(t, hits.Load(), "git did not use the pinned address")
}
