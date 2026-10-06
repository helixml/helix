package crawler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const deterministicHTML = `<!doctype html><html><head><title>Deterministic Crawler Fixture</title><meta name="description" content="A local crawler fixture."></head><body><article><h1>Deterministic Crawler Fixture</h1><p>This deterministic page exercises the real Chrome navigation and readability extraction path without relying on public DNS.</p><p>Its <strong>browser-backed markdown</strong> output proves HTML conversion still runs.</p></article></body></html>`

func startCrawlerFixture(t *testing.T, cfg *config.ServerConfig) string {
	t.Helper()
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	require.NoError(t, err)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(deterministicHTML))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { require.NoError(t, server.Shutdown(context.Background())) })

	host := "127.0.0.1"
	if cfg.RAG.Crawler.LauncherEnabled {
		launcherURL, err := url.Parse(cfg.RAG.Crawler.LauncherURL)
		require.NoError(t, err)
		conn, err := net.Dial("udp", launcherURL.Host)
		require.NoError(t, err)
		host = conn.LocalAddr().(*net.UDPAddr).IP.String()
		require.NoError(t, conn.Close())
	}
	port := listener.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://%s/", net.JoinHostPort(host, fmt.Sprint(port)))
}

func TestDefault_Crawl(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)
	fixtureURL := startCrawlerFixture(t, &cfg)
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{fixtureURL},
				Crawler: &types.WebsiteCrawler{
					Enabled: false,
				},
				Excludes: []string{"searchbot/*"},
			},
		},
	}

	browserManager, err := browser.New(&cfg)
	require.NoError(t, err)

	updateProgress := func(progress types.KnowledgeProgress) {
		t.Logf("progress: %+v", progress)
	}

	d, err := NewDefault(browserManager, k, updateProgress)
	require.NoError(t, err)

	d.disableDomainCheck = true
	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)
	require.Len(t, docs, 1)
	doc := docs[0]
	require.Equal(t, 200, doc.StatusCode)
	require.Equal(t, fixtureURL, doc.SourceURL)
	require.Equal(t, "Deterministic Crawler Fixture", doc.Title)
	require.Contains(t, doc.Description, "local crawler fixture")
	require.Contains(t, doc.Content, "real Chrome navigation")
	require.Contains(t, doc.Content, "**browser-backed markdown**")
	require.NotContains(t, doc.Content, "<strong>")
}

func TestDefault_CrawlSingle_Slow(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}

	// Use a URL that will timeout - 192.0.2.1 is a TEST-NET address
	// that's guaranteed to be non-routable (RFC 5737)
	timeoutURL := "http://192.0.2.1:8080"

	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{timeoutURL},
				Crawler: &types.WebsiteCrawler{
					Enabled: false, // Will do single URL
				},
			},
		},
	}

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	browserManager, err := browser.New(&cfg)
	require.NoError(t, err)

	updateProgress := func(progress types.KnowledgeProgress) {
		t.Logf("progress: %+v", progress)
	}

	d, err := NewDefault(browserManager, k, updateProgress)
	require.NoError(t, err)

	// Disable domain checking for test URL
	// colly's AllowedDomains doesn't work with IP addresses
	d.disableDomainCheck = true

	// Set a short timeout to avoid waiting too long for the non-routable address
	d.pageTimeout = 100 * time.Millisecond

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, len(docs))

	// Check that the message is set indicating an error (timeout or connection refused)
	assert.NotEmpty(t, docs[0].Message)
	// The error can be either timeout or connection error depending on network configuration
	assert.True(t,
		strings.Contains(docs[0].Message, "context deadline exceeded") ||
			strings.Contains(docs[0].Message, "error") ||
			strings.Contains(docs[0].Message, "ERR_"),
		"Expected error message but got: %s", docs[0].Message)
}

func TestDefault_ParseWithCodeBlock_WithReadability(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				Crawler: &types.WebsiteCrawler{
					Readability: true,
				},
			},
		},
	}

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	browserManager, err := browser.New(&cfg)
	require.NoError(t, err)

	updateProgress := func(progress types.KnowledgeProgress) {
		t.Logf("progress: %+v", progress)
	}

	d, err := NewDefault(browserManager, k, updateProgress)
	require.NoError(t, err)

	content, err := os.ReadFile("../readability/testdata/example_code_block.html")
	require.NoError(t, err)

	doc, err := d.convertToMarkdown(context.Background(), &types.CrawledDocument{
		Content: string(content),
	})
	require.NoError(t, err)

	// Assert specific lines
	assert.Contains(t, doc.Content, "Webhook Relay detects multipart/formdata requests and automatically")
	assert.Contains(t, doc.Content, `Content-Disposition: form-data; name="username"`)
	assert.Contains(t, doc.Content, "local encoded_payload, err = json.encode(json_payload)")
}

func TestDefault_ConvertHTMLToMarkdown(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				Crawler: &types.WebsiteCrawler{
					Readability: true,
				},
			},
		},
	}

	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	browserManager, err := browser.New(&cfg)
	require.NoError(t, err)

	updateProgress := func(progress types.KnowledgeProgress) {
		t.Logf("progress: %+v", progress)
	}

	d, err := NewDefault(browserManager, k, updateProgress)
	require.NoError(t, err)

	ctx := context.Background()

	b, err := browserManager.GetBrowser()
	require.NoError(t, err)

	fixtureURL := "data:text/html;charset=utf-8," + url.PathEscape(deterministicHTML)
	doc, err := d.crawlWithBrowser(ctx, b, fixtureURL)
	require.NoError(t, err)

	assert.Equal(t, 200, doc.StatusCode)
	assert.Contains(t, doc.Content, "browser-backed markdown")
}
