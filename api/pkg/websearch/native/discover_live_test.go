//go:build live

package native

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/searxng"
	"github.com/helixml/helix/api/pkg/websearch"
)

// Run against a real SearXNG:
//
//	SEARXNG_URL=http://searxng:8080 go test -tags live -run TestLive ./pkg/websearch/native/
func liveDiscoverer(t *testing.T) *SearXNGDiscoverer {
	base := os.Getenv("SEARXNG_URL")
	if base == "" {
		t.Skip("SEARXNG_URL not set")
	}
	return NewSearXNGDiscoverer(searxng.NewClient(base, strings.Split(os.Getenv("SEARXNG_ENGINES"), ",")))
}

func TestLiveDiscover_PagesMergesAndFiltersDomains(t *testing.T) {
	d := liveDiscoverer(t)
	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries:        []string{"golang generics tutorial", "go type parameters guide"},
		NumResults:     25,
		ExcludeDomains: []string{"youtube.com"},
		Country:        "US",
	})
	require.NoError(t, err)
	t.Logf("%d candidates, warnings %+v", len(out.Candidates), out.Warnings)
	require.Greater(t, len(out.Candidates), 10, "paging should collect more than one page")
	seen := map[string]bool{}
	for _, c := range out.Candidates {
		require.False(t, seen[c.URL], "duplicate %s", c.URL)
		seen[c.URL] = true
		require.NotContains(t, c.URL, "youtube.com")
	}
	for _, c := range out.Candidates[:5] {
		t.Logf("  %s | %s", c.URL, c.Title)
	}
}

func TestLiveDiscover_IncludeDomainsAndNewsDates(t *testing.T) {
	d := liveDiscoverer(t)
	out, err := d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"golang release"}, NumResults: 5, IncludeDomains: []string{"go.dev", "github.com/golang"},
	})
	require.NoError(t, err)
	t.Logf("include: %d candidates", len(out.Candidates))
	for _, c := range out.Candidates {
		t.Logf("  %s", c.URL)
		require.True(t, strings.Contains(c.URL, "go.dev") || strings.Contains(c.URL, "github.com/golang"), c.URL)
	}

	start := time.Now().AddDate(0, -1, 0)
	out, err = d.Discover(context.Background(), &DiscoverRequest{
		Queries: []string{"nvidia"}, NumResults: 10, Category: websearch.CategoryNews, StartPublishedDate: &start,
	})
	require.NoError(t, err)
	t.Logf("news: %d candidates, warnings %+v", len(out.Candidates), out.Warnings)
	for _, c := range out.Candidates {
		if c.PublishedDate != nil {
			require.False(t, c.PublishedDate.Before(start), "%s %s", c.URL, c.PublishedDate)
		}
		t.Logf("  %v %s", c.PublishedDate, c.URL)
	}
}
