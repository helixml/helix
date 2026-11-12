package anthropic

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// TST-01-270: the Anthropic proxy must not forward to an internal address
// configured on a tenant-controlled endpoint, while admin-managed global
// endpoints keep working against self-hosted upstreams.
func TestEndpointGuardTransport(t *testing.T) {
	hits := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
	}))
	defer upstream.Close()

	transport := newEndpointGuardTransport(http.DefaultTransport.(*http.Transport).Clone(), false)
	do := func(endpoint *types.ProviderEndpoint) error {
		req, err := http.NewRequest(http.MethodGet, upstream.URL+"/v1/models", nil)
		require.NoError(t, err)
		if endpoint != nil {
			req = SetRequestProviderEndpoint(req, endpoint)
		}
		resp, err := transport.RoundTrip(req)
		if err == nil {
			resp.Body.Close()
		}
		return err
	}

	for _, ep := range []*types.ProviderEndpoint{
		{EndpointType: types.ProviderEndpointTypeUser},
		{EndpointType: types.ProviderEndpointTypeOrg},
		nil, // no endpoint in context: fail closed
	} {
		err := do(ep)
		require.True(t, errors.Is(err, netguard.ErrBlockedDestination), "endpoint %+v: err = %v", ep, err)
	}
	require.Zero(t, hits)

	require.NoError(t, do(&types.ProviderEndpoint{EndpointType: types.ProviderEndpointTypeGlobal}))
	require.Equal(t, 1, hits)
}
