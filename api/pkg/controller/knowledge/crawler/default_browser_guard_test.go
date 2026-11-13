package crawler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/controller/knowledge/browser/browsertest"
	"github.com/helixml/helix/api/pkg/types"
)

const internalSecret = "INTERNAL-METADATA-VALUE"

// refuseHost is a stand-in outbound policy: it refuses one host:port.
type refuseHost struct {
	host string
	next http.RoundTripper
}

func (r refuseHost) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == r.host {
		return nil, errors.New("destination not allowed")
	}
	return r.next.RoundTrip(req)
}

// The crawler fetches a page through the guarded transport and then renders
// it in Chrome. Chrome must not be able to reach a destination the transport
// refuses, whether the page redirects there with script or reads it with fetch.
func TestDefault_BrowserRenderFollowsOutboundPolicy(t *testing.T) {
	b := browsertest.New(t)

	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>internal</title></head><body><article><h1>Internal</h1><p>` +
			internalSecret + ` this paragraph is long enough for readability to keep it as the main content.</p></article></body></html>`))
	}))
	defer internal.Close()
	internalURL, err := url.Parse(internal.URL)
	require.NoError(t, err)

	pages := map[string]string{
		"script redirect": `<script>location.replace("` + internal.URL + `/latest/meta-data")</script>`,
		"fetch into DOM":  `<script>fetch("` + internal.URL + `/").then(r => r.text()).then(t => { document.getElementById("out").textContent = t })</script>`,
	}
	for name, script := range pages {
		t.Run(name, func(t *testing.T) {
			public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(`<!doctype html><html><head><title>Public</title></head><body><article><h1>Public</h1>` +
					`<p>This public page is long enough for readability to treat it as the article body.</p><p id="out"></p></article>` +
					script + `</body></html>`))
			}))
			defer public.Close()

			k := &types.Knowledge{Source: types.KnowledgeSource{Web: &types.KnowledgeSourceWeb{
				URLs:    []string{public.URL + "/"},
				Crawler: &types.WebsiteCrawler{Enabled: false},
			}}}
			d, err := NewDefault(b, k, func(types.KnowledgeProgress) {})
			require.NoError(t, err)
			d.disableDomainCheck = true
			d.transport = refuseHost{host: internalURL.Host, next: http.DefaultTransport}

			docs, err := d.Crawl(context.Background())
			require.NoError(t, err)
			require.Len(t, docs, 1)
			for _, doc := range docs {
				require.False(t, strings.Contains(doc.Content, internalSecret), "browser reached a refused destination: %q", doc.Content)
				if name == "fetch into DOM" { // the redirect case ends on Chrome's error page
					require.Contains(t, doc.Content, "This public page")
				}
			}
		})
	}
}
