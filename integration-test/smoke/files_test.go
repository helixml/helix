package smoke

import (
	"testing"
	"time"

	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

const folderName = "smoke"

func TestUploadPDFFile(t *testing.T) {
	t.Parallel()
	ctx := helper.SetTestTimeout(t, 30*time.Second)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(helper.GetServerURL() + "/files")
	defer page.MustClose()
	page.MustWaitLoad()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.LogStep(t, "Creating test folder")
	elements := page.MustElementsX(`//button[contains(text(), 'Create Folder')]`)
	require.Len(t, elements, 1, "create folder button not found")
	elements[0].MustClick()

	helper.LogStep(t, "Typing folder name")
	textarea := page.MustElement("input[type='text']")
	textarea.MustInput(folderName)

	helper.LogStep(t, "Clicking Submit button")
	sendButton := page.MustElement("#submitButton")
	sendButton.MustClick()

	helper.LogStep(t, "Clicking on the folder")
	folder := page.MustElementX(`//a[contains(text(), '` + folderName + `')]`)
	folder.MustClick()

	helper.LogStep(t, "Waiting for page to stabilize")
	page.MustWaitStable()

	helper.LogStep(t, "Uploading file")
	upload := page.MustElement("input[type='file']")
	upload.MustSetFiles(helper.TestPDFFile)
	page.MustReload()

	helper.LogStep(t, "Verifying file exists")
	file := page.MustElementX(`//a[contains(text(), 'hr-guide.pdf')]`)
	require.NotNil(t, file, "uploaded file should be visible")
	file.MustClick()
}
