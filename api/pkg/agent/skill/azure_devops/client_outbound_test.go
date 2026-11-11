package azuredevops

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/netguard"
)

// The organization URL is user-supplied and the Azure DevOps SDK makes its
// own HTTP clients, so the outbound policy must still apply to its requests.
func TestAzureDevOpsClient_OutboundPolicy(t *testing.T) {
	var hits atomic.Int32
	org := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer org.Close()

	strict := NewAzureDevOpsClient(org.URL+"/org", "pat")
	_, err := strict.ListProjects(context.Background())
	require.Error(t, err)
	_, err = strict.GetUserProfile(context.Background())
	require.Error(t, err)
	require.Zero(t, hits.Load(), "request reached a loopback organization URL")

	// Even where private destinations are allowed, plain http is refused:
	// a connection refused after dialling could be left in the shared pool.
	_, err = NewAzureDevOpsClient(org.URL+"/org", "pat").WithOutboundPolicy(netguard.Policy{AllowPrivate: true}).
		GetUserProfile(context.Background())
	require.Error(t, err)
	require.Zero(t, hits.Load(), "request sent to a plain-http organization URL")

	tlsOrg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer tlsOrg.Close()
	// The https organization URL is dialled (the test certificate is not
	// trusted, so the request fails in the TLS handshake, not on policy).
	_, err = NewAzureDevOpsClient(tlsOrg.URL+"/org", "pat").WithOutboundPolicy(netguard.Policy{AllowPrivate: true}).
		GetUserProfile(context.Background())
	require.ErrorContains(t, err, "certificate")
}

func TestCheckOrganizationURL(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, CheckOrganizationURL(ctx, netguard.Policy{}, "https://dev.azure.com/org"))
	require.ErrorIs(t, CheckOrganizationURL(ctx, netguard.Policy{}, "https://127.0.0.1:8080/tfs"), netguard.ErrBlockedDestination)
	require.ErrorIs(t, CheckOrganizationURL(ctx, netguard.Policy{AllowPrivate: true}, "https://169.254.169.254/"), netguard.ErrBlockedDestination)
	// Plain http is refused even where private destinations are allowed.
	require.ErrorIs(t, CheckOrganizationURL(ctx, netguard.Policy{AllowPrivate: true}, "http://tfs.internal:8080/tfs"), netguard.ErrBlockedDestination)
	require.Error(t, CheckOrganizationURL(ctx, netguard.Policy{}, "file:///etc"))
}
