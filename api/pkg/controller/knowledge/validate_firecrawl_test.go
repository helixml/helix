package knowledge

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

func firecrawlKnowledge(apiURL, apiKey string) *types.AssistantKnowledge {
	return &types.AssistantKnowledge{
		Name: "docs",
		Source: types.KnowledgeSource{Web: &types.KnowledgeSourceWeb{
			URLs:    []string{"https://example.com/"},
			Crawler: &types.WebsiteCrawler{Firecrawl: &types.Firecrawl{APIURL: apiURL, APIKey: apiKey}},
		}},
	}
}

// A custom Firecrawl api_url is refused at save time when it needs the
// server's key or points at a disallowed destination, not only at crawl time.
func TestValidate_FirecrawlAPIURL(t *testing.T) {
	cfg := &config.ServerConfig{}
	require.Error(t, Validate(cfg, firecrawlKnowledge("https://firecrawl.example.com", "")), "custom URL without its own key")
	for _, u := range []string{"http://127.0.0.1:3002", "http://169.254.169.254/", "file:///etc/passwd", "http://localhost:3002"} {
		require.Error(t, Validate(cfg, firecrawlKnowledge(u, "key")), u)
	}
	require.NoError(t, Validate(cfg, firecrawlKnowledge("", "key")), "default (operator) Firecrawl URL")

	cfg.Tools.AllowPrivateNetworks = true
	require.NoError(t, Validate(cfg, firecrawlKnowledge("http://127.0.0.1:3002", "key")), "self-hosted Firecrawl allowed by the operator")
}
