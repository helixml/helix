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
	k := &types.Knowledge{
		Source: types.KnowledgeSource{
			Web: &types.KnowledgeSourceWeb{
				URLs: []string{"https://docs.helix.ml/helix"},
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
		appsText              = `When I submit a request that uses an App, it hangs`
		privateDeploymentText = `This section describes how to install the control plane using Docker`
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

			assert.Equal(t, "https://docs.helix.ml/helix/develop/apps/", doc.SourceURL)
		}
		if strings.Contains(doc.Content, privateDeploymentText) {
			privateDeploymentTextFound = true

			assert.Equal(t, "https://docs.helix.ml/helix/private-deployment/manual-install/docker/", doc.SourceURL)
		}
	}

	require.True(t, appsTextFound, "apps text not found")
	require.True(t, privateDeploymentTextFound, "private deployment text not found")

	t.Logf("docs: %d", len(docs))
}

func TestDefault_CrawlSingle(t *testing.T) {
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

	// Setting very short timeout to force the page to timeout
	d.pageTimeout = 5 * time.Millisecond

	docs, err := d.Crawl(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, len(docs))

	// Check that the message is set
	assert.NotEmpty(t, docs[0].Message)
	assert.Contains(t, docs[0].Message, "context deadline exceeded")
}

func TestDefault_ParseWithCodeBlock_WithReadability(t *testing.T) {
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
