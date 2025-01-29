//go:build integration
// +build integration

package smoke

import (
	"testing"

	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateRagApp(t *testing.T) {
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

	helper.LogStep(t, "Adding knowledge")
	page.MustElementX(`//button[text() = 'Knowledge']`).MustClick()

	helper.LogStep(t, "Adding knowledge source")
	page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustWaitInteractable().MustClick()
	page.MustElementX(`//input[@value = 'filestore']`).MustWaitInteractable().MustClick()
	page.MustElementX(`//input[@type = 'text']`).MustWaitInteractable().MustInput("test hr-guide.pdf")
	page.MustElementX(`//button[text() = 'Add']`).MustWaitInteractable().MustClick()

	helper.LogStep(t, "Getting the upload file input")
	upload := page.MustElementX("//input[@type = 'file']")

	helper.LogStep(t, "Uploading the file")
	wait1 := page.MustWaitRequestIdle()
	upload.MustSetFiles(helper.TestPDFFile)
	wait1()

	helper.LogStep(t, "Saving the app")
	helper.SaveApp(t, page)

	helper.LogStep(t, "Reloading the page")
	page.MustReload()
	helper.LogStep(t, "Waiting for knowledge source to be ready")
	page.MustElementX(`//span[contains(text(), 'ready')]`)

	helper.LogStep(t, "Testing the app")
	page.MustElementX(`//textarea[@id='textEntry']`).MustWaitInteractable().MustInput("do you have a shoe policy")
	page.MustElementX(`//button[@id='sendButton']`).MustWaitInteractable().MustClick()

	helper.WaitForHelixResponse(ctx, t, page)
}
