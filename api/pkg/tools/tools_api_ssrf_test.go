package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

// API tool URLs are user-defined and called by the API server, which returns
// the response to the model and the user: they must not reach internal or
// metadata addresses unless the operator allows private networks.
func TestAPIToolsRejectInternalDestinations(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"id": 1}`))
	}))
	defer internal.Close()

	strategy, err := NewChainStrategy(&config.ServerConfig{}, nil, nil)
	require.NoError(t, err)

	for _, baseURL := range []string{internal.URL, "http://169.254.169.254"} {
		tool := &types.Tool{
			Name:     "internal",
			ToolType: types.ToolTypeAPI,
			Config: types.ToolConfig{API: &types.ToolAPIConfig{
				URL:    baseURL,
				Schema: petStoreAPISpec,
				Actions: []*types.ToolAPIAction{
					{Name: "listPets", Description: "List all pets", Method: "GET", Path: "/pets"},
				},
			}},
		}
		resp, err := strategy.RunAPIActionWithParameters(context.Background(), &types.RunAPIActionRequest{Tool: tool, Action: "listPets"})
		if err == nil {
			require.NotEmpty(t, resp.Error, baseURL)
		}
	}
	require.Zero(t, hits.Load(), "internal listener was contacted")
}
