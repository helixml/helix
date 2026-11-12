package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/types"
)

// MCP server URLs are user-supplied and connected to from the API server
// (skill validation, app create/update, runtime): they must not reach
// internal or metadata addresses, except the Helix API's own address.
func TestDefaultClientGetter_RejectsInternalDestinations(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer internal.Close()

	getter := NewDefaultClientGetter(false, netguard.Policy{}, "https://helix.example.com")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, u := range []string{internal.URL + "/mcp", internal.URL + "/sse", "http://169.254.169.254/mcp"} {
		_, err := getter.NewClient(ctx, agent.Meta{}, nil, &types.AssistantMCP{URL: u})
		require.Error(t, err, u)
	}
	require.Zero(t, hits.Load(), "internal listener was contacted")

	// The Helix API's own address stays reachable for built-in MCP servers.
	self := NewDefaultClientGetter(false, netguard.Policy{}, internal.URL)
	_, _ = self.NewClient(ctx, agent.Meta{}, nil, &types.AssistantMCP{URL: internal.URL + "/mcp"})
	require.NotZero(t, hits.Load(), "trusted server URL was not contacted")
}

// Creating a client must not reconfigure the process-wide http.DefaultClient.
func TestDefaultClientGetter_DoesNotMutateDefaultClient(t *testing.T) {
	before := http.DefaultClient.Transport
	defer func() { http.DefaultClient.Transport = before }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	getter := &DefaultClientGetter{TLSSkipVerify: true}
	_, _ = getter.NewClient(context.Background(), agent.Meta{}, nil, &types.AssistantMCP{URL: srv.URL + "/mcp"})
	require.True(t, http.DefaultClient.Transport == before, "http.DefaultClient.Transport was replaced")
}
