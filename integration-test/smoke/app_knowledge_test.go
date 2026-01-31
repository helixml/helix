//go:build integration
// +build integration

package smoke

import (
	"fmt"
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateRagApp(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := createPage(browser)

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.BrowseToAppsPage(t, page)
	helper.CreateNewApp(t, page)

	helper.LogStep(t, "Adding knowledge")
	page.MustElementX(`//button[text() = 'Knowledge']`).MustClick()

	helper.LogStep(t, "Adding knowledge source")
	page.MustWaitStable()
	page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustWaitVisible().MustClick()
	page.MustElementX(`//input[@value = 'filestore']`).MustWaitVisible().MustClick()
	page.MustElementX(`//input[@type = 'text']`).MustWaitVisible().MustInput("test hr-guide.pdf")
	page.MustElementX(`//button[text() = 'Add']`).MustWaitVisible().MustClick()

	testFile := helper.GetTestPDFFile()
	helper.LogStep(t, fmt.Sprintf("Uploading the file %s", testFile))
	upload := page.MustElementX("//input[@type = 'file']").MustWaitVisible()
	upload.MustSetFiles(testFile)

	// Wait for the file to be uploaded
	helper.LogStep(t, "Waiting for file to be uploaded")
	page.Race().ElementX(fmt.Sprintf(`//span[contains(text(), "%s")]`, helper.TestPDFFilename)).MustDo()

	helper.LogStep(t, "Saving the app")
	helper.SaveApp(t, page)

	helper.LogStep(t, "Reloading the page")
	page.MustReload()
	helper.LogStep(t, "Waiting for knowledge source to be ready")
	page.MustElementX(`//span[contains(text(), 'ready')]`)

	helper.TestApp(ctx, t, page, "do you have a shoe policy")
}
