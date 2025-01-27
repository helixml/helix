//go:build integration
// +build integration

package smoke

import (
	"testing"

	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateIntegrationApp(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(helper.GetServerURL())
	defer page.MustClose()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.BrowseToAppsPage(t, page)
	helper.CreateNewApp(t, page)

	helper.LogStep(t, "Clicking on the Integrations tab")
	page.MustElementX(`//button[text() = 'Integrations']`).MustClick()

	helper.LogStep(t, "Adding API Tool")
	page.MustElementX(`//button[text() = 'Add API Tool']`).MustWaitInteractable().MustClick()

	helper.LogStep(t, "Selecting example Schema")
	page.MustElementX(`//label[text() = 'Example Schemas']/..`).MustWaitInteractable().MustClick()

	helper.LogStep(t, "Selecting Exchange Rates Schema")
	page.MustElementX(`//li[text() = 'Exchange Rates']`).MustWaitInteractable().MustClick()

	helper.LogStep(t, "Clicking on Save button")
	page.MustElementX(`//button[@id = 'submitButton' and text() = 'Save']`).MustWaitInteractable().MustClick()

	helper.SaveApp(t, page)

	helper.LogStep(t, "Testing the app")
	page.MustElementX(`//textarea[@id='textEntry']`).MustWaitInteractable().MustInput("what is the USD GBP rate")
	page.MustElementX(`//button[@id='sendButton']`).MustWaitInteractable().MustClick()

	helper.WaitForHelixResponse(ctx, t, page)
}
