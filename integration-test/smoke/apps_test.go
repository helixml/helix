package smoke

import (
	"testing"
	"time"

	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestCreateRagApp(t *testing.T) {
	t.Parallel()
	browser := createBrowser()
	defer browser.MustClose()

	page := browser.
		DefaultDevice(devices.LaptopWithHiDPIScreen.Landscape()).
		MustPage(helper.GetServerURL())
	defer page.MustClose()
	page.MustWaitLoad()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	helper.LogStep(t, "Browsing to the apps page")
	page.MustElement("button[aria-controls='menu-appbar']").MustClick()
	page.MustElementX(`//li[contains(text(), 'Your Apps')]`).MustClick()

	helper.LogStep(t, "Creating a new app")
	page.MustElement("#new-app-button").MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Save initial app")
	appName := "smoke-" + time.Now().Format("20060102150405")
	page.MustElement("#app-name").MustInput(appName)
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
	page.MustWaitStable()

	helper.LogStep(t, "Adding knowledge")
	page.MustElementX(`//button[text() = 'Knowledge']`).MustClick()

	helper.LogStep(t, "Adding knowledge source")
	page.MustElementX(`//button[text() = 'Add Knowledge Source']`).MustClick()
	page.MustElement(`input[value=filestore]`).MustClick()
	page.MustElement(`input[type=text]`).MustInput(folderName)
	page.MustElementX(`//button[text() = 'Add']`).MustClick()

	helper.LogStep(t, "Save the app again")
	page.MustElementX(`//button[text() = 'Save']`).MustClick()
	page.MustWaitStable()
}
