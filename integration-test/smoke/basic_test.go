package smoke

import (
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestHomepageLoad(t *testing.T) {
	t.Parallel()
	browser := createBrowser()
	defer browser.MustClose()

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()

	helper.LogStep(t, "Waiting for page load")
	page.MustWaitLoad()

	page.MustElement("body")

	helper.LogStep(t, "Verifying page loaded successfully")
	require.True(t, page.MustHas("body"), "homepage failed to load properly")
}

func TestLoginFlow(t *testing.T) {
	t.Parallel()
	browser := createBrowser()
	defer browser.MustClose()

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()
	page.MustWaitLoad()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")
}
