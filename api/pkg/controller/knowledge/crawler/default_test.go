package crawler

import (
	"context"
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

func TestDefault_Crawl(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}

	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://docs.helixml.tech/helix"},
				Crawler: &types.WebsiteCrawler{
					Enabled:  true,
					MaxDepth: 200,
				},
				Excludes: []string{"searchbot/*"},
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

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	const (
		appsText              = `how to create, configure, and interact with agents`
		privateDeploymentText = `Install Control Plane pointing at TogetherAI`
	)

	var (
		appsTextFound              bool
		privateDeploymentTextFound bool
	)

	for _, doc := range docs {
		// Uncomment to save the chunks to a file for debugging
		// os.WriteFile(fmt.Sprintf("doc-%s.html", doc.Title), []byte(doc.Content), 0644)

		if strings.Contains(doc.Content, appsText) {
			appsTextFound = true

			assert.Equal(t, "https://docs.helixml.tech/helix/develop/apps/", doc.SourceURL)
		}
		if strings.Contains(doc.Content, privateDeploymentText) {
			privateDeploymentTextFound = true

			assert.Equal(t, "https://docs.helixml.tech/helix/private-deployment/controlplane/", doc.SourceURL)
		}
	}

	require.True(t, appsTextFound, "apps text not found")
	require.True(t, privateDeploymentTextFound, "private deployment text not found")

	t.Logf("docs: %d", len(docs))
}

func TestDefault_CrawlSingle(t *testing.T) {
	// Skip test if Chrome service is not available (e.g., in local development)
	if testing.Short() {
		t.Skip("Skipping crawler test in short mode")
	}
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://www.theguardian.com/uk-news/2024/sep/13/plans-unveiled-for-cheaper-high-speed-alternative-to-scrapped-hs2-northern-leg"},
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

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, len(docs))
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

	doc, err := d.crawlWithBrowser(ctx, b, "https://news.ycombinator.com/news")
	require.NoError(t, err)

	assert.True(t, strings.Contains(doc.Content, "points"))
}
