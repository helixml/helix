//go:build integration
// +build integration

package smoke

import (
	"testing"
	"time"

	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateIntegrationApp(t *testing.T) {
	ctx := helper.SetTestTimeout(t, 60*time.Second)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(helper.GetServerURL())
	defer page.MustClose()
	page.MustWaitLoad()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.BrowseToAppsPage(t, page)
	helper.CreateNewApp(t, page)

	helper.LogStep(t, "Clicking on the Integrations tab")
	page.MustElementX(`//button[text() = 'Integrations']`).MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Adding API Tool")
	page.MustElementX(`//button[text() = 'Add API Tool']`).MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Selecting example Schema")
	page.MustElementX(`//label[text() = 'Example Schemas']/..`).MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Selecting Exchange Rates Schema")
	page.MustElementX(`//li[text() = 'Exchange Rates']`).MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Clicking on Save button")
	page.MustElementX(`//button[@id = 'submitButton' and text() = 'Save']`).MustClick()

	helper.SaveApp(t, page)
	helper.MustReload(t, page)

	helper.LogStep(t, "Testing the app")
	page.MustElement("#textEntry").MustInput("what is the USD GBP rate")
	page.MustElement("#sendButton").MustClick()

	helper.WaitForHelixResponse(ctx, t, page)
}
