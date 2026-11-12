package knowledge

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/types"
)

// Knowledge web sources are user input fetched by the API server and indexed
// for the user to read back, so they must not reach internal or metadata
// addresses, directly or through a redirect.
func TestDownloadDirectlyRejectsInternalDestinations(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("internal secret"))
	}))
	defer internal.Close()

	_, port, err := net.SplitHostPort(internal.Listener.Addr().String())
	require.NoError(t, err)

	r, err := New(&config.ServerConfig{}, nil, nil, nil, nil, &browser.Browser{}, nil)
	require.NoError(t, err)

	k := &types.Knowledge{Source: types.KnowledgeSource{Web: &types.KnowledgeSourceWeb{}}}
	for _, u := range []string{
		internal.URL,
		"http://localhost:" + port,
		"http://169.254.169.254/latest/meta-data/",
	} {
		_, err := r.downloadDirectly(context.Background(), k, u)
		require.Error(t, err, u)
	}
	require.Zero(t, hits.Load(), "internal listener was contacted")
}
