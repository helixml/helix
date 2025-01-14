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
	page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustClick()
	page.MustElement(`input[value=filestore]`).MustClick()
	page.MustElement(`input[type=text]`).MustInput("test hr-guide.pdf")
	page.MustElementX(`//button[text() = 'Add']`).MustClick()

	helper.SaveApp(t, page)

	helper.LogStep(t, "Clicking on the upload file button")
	upload := page.MustElement("input[type='file']")

	helper.LogStep(t, "Uploading the file")
	wait1 := page.MustWaitRequestIdle()
	upload.MustSetFiles(helper.TestPDFFile)
	wait1()

	helper.LogStep(t, "Double checking that the file is present in the knowledge")
	moreButton := page.MustElement(`[data-testid='ExpandMoreIcon']`)
	moreButton.MustClick()
	knowledgeSources := page.MustElementsX(`//span[text() = 'hr-guide.pdf']`)
	require.Equal(t, 1, len(knowledgeSources), "knowledge source should be present")

	helper.WaitForKnowledgeReady(t, page)

	helper.LogStep(t, "Testing the app")
	page.MustElement("#textEntry").MustInput("do you have a shoe policy")
	page.MustElement("#sendButton").MustClick()

	helper.WaitForHelixResponse(ctx, t, page)
}
