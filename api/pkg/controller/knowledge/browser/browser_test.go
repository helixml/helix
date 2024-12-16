package browser

import (
	"testing"

	"github.com/go-rod/rod/lib/proto"
	"github.com/helixml/helix/api/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowser_Get(t *testing.T) {
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	browserManager, err := New(&cfg)
	require.NoError(t, err)

	browser, err := browserManager.GetBrowser()
	require.NoError(t, err)

	assert.NotNil(t, browser)

	page, err := browser.Page(proto.TargetCreateTarget{URL: "https://docs.helix.ml/"})
	require.NoError(t, err)
	assert.NotNil(t, page)

	defer page.Close()

	err = page.WaitLoad()
	require.NoError(t, err)

	body, err := page.HTML()
	require.NoError(t, err)

	assert.Contains(t, body, "Helix")
}

func TestBrowser_BrowsePages(t *testing.T) {
	cfg, err := config.LoadServerConfig()
	require.NoError(t, err)

	browserManager, err := New(&cfg)
	require.NoError(t, err)

	browser, err := browserManager.GetBrowser()
	require.NoError(t, err)

	page1, err := browserManager.GetPage(browser, proto.TargetCreateTarget{URL: "https://docs.helix.ml/"})
	require.NoError(t, err)
	assert.NotNil(t, page1)

	err = page1.WaitLoad()
	require.NoError(t, err)

	body, err := page1.HTML()
	require.NoError(t, err)

	assert.Contains(t, body, "Helix")

	browserManager.PutPage(page1)

	page2, err := browserManager.GetPage(browser, proto.TargetCreateTarget{URL: "https://docs.helix.ml/helix/help/"})
	require.NoError(t, err)

	err = page2.WaitLoad()
	require.NoError(t, err)

	body, err = page2.HTML()
	require.NoError(t, err)

	assert.Contains(t, body, "Commercial Support")

	browserManager.PutPage(page2)

	// Destroy the browser
	err = browserManager.PutBrowser(browser)
	require.NoError(t, err)
}
