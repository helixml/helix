//go:build integration
// +build integration

package smoke

import (
	"testing"
)

func TestImageInference(t *testing.T) {
	t.Skip("Skipping image inference test, because it is unreliable on prod because of the lack of machines reserved for image generation")
	return

	// ctx := helper.CreateContext(t)

	// browser := createBrowser(ctx)
	// defer browser.MustClose()

	// page := createPage(browser)

	// err := helper.PerformLogin(t, page)
	// require.NoError(t, err, "login should succeed")

	// err = helper.StartNewImageSession(t, page)
	// require.NoError(t, err, "starting new image session should succeed")

	// helper.SendMessage(t, page, "a beautiful image of a yorkshire rose")

	// helper.LogStep(t, "Waiting for image to be generated")
	// page.WaitElementsMoreThan("main a > img", 0)
}
