package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

// A self-hosted provider base URL comes from the repository record, so PR API
// clients built for it must apply the outbound policy: by default a base URL
// on loopback (or an internal service, or cloud metadata) is never contacted.
func TestProviderAPIClientsApplyOutboundPolicy(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	appKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))

	calls := map[string]func(s *GitRepositoryService) error{
		"github app": func(s *GitRepositoryService) error {
			client, err := s.getGitHubClient(context.Background(), &types.GitRepository{
				ExternalURL: server.URL + "/acme/app",
				GitHub:      &types.GitHub{BaseURL: server.URL, AppID: 1, InstallationID: 2, PrivateKey: appKey},
			}, "")
			require.NoError(t, err)
			_, err = client.GetRepository(context.Background(), "acme", "app")
			return err
		},
		"github": func(s *GitRepositoryService) error {
			client, err := s.getGitHubClient(context.Background(), &types.GitRepository{
				ExternalURL: server.URL + "/acme/app",
				GitHub:      &types.GitHub{BaseURL: server.URL, PersonalAccessToken: "pat"},
			}, "")
			require.NoError(t, err)
			_, err = client.GetRepository(context.Background(), "acme", "app")
			return err
		},
		"gitlab": func(s *GitRepositoryService) error {
			client, err := s.getGitLabClient(context.Background(), &types.GitRepository{
				ExternalURL: server.URL + "/acme/app",
				GitLab:      &types.GitLab{BaseURL: server.URL, PersonalAccessToken: "pat"},
			}, "")
			require.NoError(t, err)
			_, err = client.GetProjectByPath(context.Background(), "acme/app")
			return err
		},
		"bitbucket": func(s *GitRepositoryService) error {
			client, err := s.getBitbucketClient(context.Background(), &types.GitRepository{
				ExternalURL: server.URL + "/scm/acme/app.git",
				Bitbucket:   &types.Bitbucket{BaseURL: server.URL, Username: "u", AppPassword: "p"},
			})
			require.NoError(t, err)
			_, err = client.GetPullRequest(context.Background(), "acme", "app", 1)
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			hits.Store(0)
			strict := &GitRepositoryService{}
			strict.SetOutboundPolicy(netguard.Policy{})
			err := call(strict)
			require.Error(t, err)
			assert.ErrorIs(t, err, netguard.ErrBlockedDestination)
			assert.Zero(t, hits.Load(), "blocked request must not reach the server")

			permissive := &GitRepositoryService{}
			permissive.SetOutboundPolicy(netguard.Policy{AllowPrivate: true})
			_ = call(permissive)
			assert.NotZero(t, hits.Load(), "operator opt-in reaches a private self-hosted provider")
		})
	}
}

func TestProviderHTTPClientDefaultsForPublicHosts(t *testing.T) {
	assert.Nil(t, (&GitRepositoryService{}).providerHTTPClient(""))
	assert.NotNil(t, (&GitRepositoryService{}).providerHTTPClient("https://git.example.com"))
}
