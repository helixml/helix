package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/helixml/helix/api/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowser_Get(t *testing.T) {
	cfg := &config.ServerConfig{}
	cfg.RAG.Crawler.ChromeURL = "http://127.0.0.1:9222"

	browserManager := New(cfg)

	browser, err := browserManager.Get()
	require.NoError(t, err)

	assert.NotNil(t, browser)

	defer browserManager.Put(browser)

	page, err := browser.Page(proto.TargetCreateTarget{URL: "https://docs.helix.ml/"})
	require.NoError(t, err)
	assert.NotNil(t, page)

	defer page.Close()

	body, err := page.HTML()
	require.NoError(t, err)

	assert.Contains(t, body, "Helix")
}
