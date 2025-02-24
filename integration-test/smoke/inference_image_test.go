//go:build integration
// +build integration

package smoke

import (
	"testing"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestImageInference(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := browser.MustPage(helper.GetServerURL())
	defer page.MustClose()

	err := helper.PerformLogin(t, page)
	require.NoError(t, err, "login should succeed")

	err = helper.StartNewImageSession(t, page)
	require.NoError(t, err, "starting new image session should succeed")

	helper.SendMessage(t, page, "a beautiful image of a yorkshire rose")

	helper.LogStep(t, "Waiting for image to be generated")
	page.MustElementX(`//main//img`)
}
