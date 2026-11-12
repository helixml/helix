package crawler

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

func firecrawlKnowledge(apiURL, apiKey string) *types.Knowledge {
	return &types.Knowledge{Source: types.KnowledgeSource{Web: &types.KnowledgeSourceWeb{
		URLs: []string{"https://example.com/"},
		Crawler: &types.WebsiteCrawler{Firecrawl: &types.Firecrawl{
			APIURL: apiURL,
			APIKey: apiKey,
		}},
	}}}
}

func strictTransport() http.RoundTripper {
	t := http.DefaultTransport.(*http.Transport).Clone()
	netguard.Policy{}.GuardTransport(t)
	return t
}

// A user-set Firecrawl API URL is contacted from the API server, so it must
// go through the outbound policy like any other user-supplied URL.
func TestFirecrawl_UserAPIURLFollowsOutboundPolicy(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewCrawler(nil, firecrawlKnowledge(srv.URL, "user-key"), nil, strictTransport())
	if err == nil {
		_, err = c.Crawl(context.Background())
	}
	require.Error(t, err)
	require.Zero(t, hits.Load(), "firecrawl request reached a loopback address")
}

// Without its own key, a user-chosen Firecrawl URL would be sent the
// server's FIRECRAWL_API_KEY.
func TestFirecrawl_UserAPIURLRequiresOwnKey(t *testing.T) {
	t.Setenv("FIRECRAWL_API_KEY", "server-key")
	var gotAuth atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := NewCrawler(nil, firecrawlKnowledge(srv.URL, ""), nil, nil)
	if err == nil {
		_, err = c.Crawl(context.Background())
	}
	require.Error(t, err)
	require.Nil(t, gotAuth.Load(), "server API key was sent to a user-chosen URL")
}
