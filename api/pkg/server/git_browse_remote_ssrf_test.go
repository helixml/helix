package server

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// TST-01-272: POST /api/v1/git/browse-remote must not fetch caller-supplied
// base_url / organization_url destinations on internal networks. A loopback
// listener stands in for the assessor's OAST host / an internal service.
func TestBrowseRemoteRejectsInternalDestinations_TST01272(t *testing.T) {
	var hits atomic.Int32
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer listener.Close()

	cases := []struct {
		name string
		req  types.BrowseRemoteRepositoriesRequest
	}{
		{"gitlab PoC http", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: listener.URL, Token: "dummy", Username: "probe"}},
		{"gitlab localhost name", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: "http://localhost:" + listenerPort(t, listener), Token: "dummy"}},
		{"gitlab metadata", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: "http://169.254.169.254/", Token: "dummy"}},
		{"gitlab bad scheme", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: "file:///etc/passwd", Token: "dummy"}},
		{"github enterprise", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeGitHub, BaseURL: listener.URL + "/api/v3/", Token: "dummy"}},
		{"ado", types.BrowseRemoteRepositoriesRequest{ProviderType: types.ExternalRepositoryTypeADO, OrganizationURL: listener.URL + "/org", Token: "dummy"}},
	}

	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := postBrowseRemote(t, srv, tc.req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
			// The response must not echo the URL or the resolution result.
			if bytes.Contains(rec.Body.Bytes(), []byte("127.0.0.1")) || bytes.Contains(rec.Body.Bytes(), []byte("resolve")) {
				t.Fatalf("response leaks destination details: %s", rec.Body.String())
			}
		})
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("internal listener received %d requests from the platform", n)
	}
}

// Operators can opt private networks back in (on-prem GitLab); the
// same request then reaches the listener. This also proves the test above
// would detect a regression.
func TestBrowseRemoteAllowsPrivateNetworksWhenEnabled_TST01272(t *testing.T) {
	var hits atomic.Int32
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer listener.Close()

	cfg := &config.ServerConfig{}
	cfg.GitProviders.AllowPrivateNetworks = true
	srv := &HelixAPIServer{Cfg: cfg}

	rec := postBrowseRemote(t, srv, types.BrowseRemoteRepositoriesRequest{
		ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: listener.URL, Token: "dummy",
	})
	if hits.Load() == 0 {
		t.Fatalf("expected allowed destination to be contacted; got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected upstream 401 to map to 400 invalid token, got %d: %s", rec.Code, rec.Body.String())
	}
}

func postBrowseRemote(t *testing.T, srv *HelixAPIServer, body types.BrowseRemoteRepositoriesRequest) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git/browse-remote", bytes.NewReader(b))
	rec := httptest.NewRecorder()
	srv.browseRemoteRepositories(rec, req)
	return rec
}

func listenerPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

// Integer, hex, octal and shortened IPv4 spellings of an internal address are
// refused like their dotted-quad form, for every provider type.
func TestBrowseRemoteRejectsNonCanonicalIPv4Literals(t *testing.T) {
	var hits atomic.Int32
	listener := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer listener.Close()
	port := listenerPort(t, listener)

	srv := &HelixAPIServer{Cfg: &config.ServerConfig{}}
	for _, host := range []string{"2130706433", "0x7f000001", "0177.0.0.1", "127.1", "2852039166"} {
		base := "http://" + host + ":" + port
		for _, req := range []types.BrowseRemoteRepositoriesRequest{
			{ProviderType: types.ExternalRepositoryTypeGitHub, BaseURL: base + "/api/v3/", Token: "dummy"},
			{ProviderType: types.ExternalRepositoryTypeGitLab, BaseURL: base, Token: "dummy"},
			{ProviderType: types.ExternalRepositoryTypeADO, OrganizationURL: "https://" + host + ":" + port + "/org", Token: "dummy"},
		} {
			rec := postBrowseRemote(t, srv, req)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s %s: expected 400, got %d: %s", req.ProviderType, host, rec.Code, rec.Body.String())
			}
		}
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("internal listener received %d requests", n)
	}
}
