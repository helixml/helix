//go:build integration
// +build integration

package smoke

import (
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateIntegrationApp(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := createPage(browser)

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.BrowseToAppsPage(t, page)
	helper.CreateNewApp(t, page)

	helper.LogStep(t, "Clicking on the Integrations tab")
	page.MustElementX(`//button[text() = 'Integrations']`).MustWaitVisible().MustClick()

	helper.LogStep(t, "Adding API Tool")
	page.MustElementX(`//button[text() = 'Add API Tool']`).MustWaitVisible().MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Selecting example Schema")
	page.MustElementX(`//label[text() = 'Example Schemas']/..`).MustWaitVisible().MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Selecting Exchange Rates Schema")
	page.MustElementX(`//li[text() = 'Exchange Rates']`).MustWaitVisible().MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Clicking on Save button")
	page.MustElementX(`//button[@id = 'submitButton' and text() = 'Save']`).MustWaitVisible().MustClick()

	helper.SaveApp(t, page)

	helper.TestApp(ctx, t, page, "what is the USD GBP rate")
}
