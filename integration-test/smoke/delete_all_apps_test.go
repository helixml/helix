//go:build integration
// +build integration

package smoke

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestDeleteAllApps(t *testing.T) {
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

	helper.LogStep(t, "Waiting for at least one app")
	err = rod.Try(func() {
		page.Timeout(10 * time.Second).MustElement(`[data-testid='DeleteIcon']`)
	})
	if errors.Is(err, context.DeadlineExceeded) {
		helper.LogStep(t, "No apps found, skipping")
		t.SkipNow()
	} else if err != nil {
		t.Fatalf("error waiting for at least one app: %v", err)
	}

	helper.LogStep(t, "Deleting all apps, please wait...")
	// Get all private rows
	rows := page.MustElementsX(`//tbody//tr//p[contains(text(), 'Private')]`)
	for i, row := range rows {
		helper.LogStep(t, fmt.Sprintf("Deleting app %d of %d", i+1, len(rows)))
		deleteButton := row.MustElementX(`//*[name()='svg' and @data-testid='DeleteIcon']`)

		wait := page.MustWaitRequestIdle()
		deleteButton.MustClick()

		// Wait for and type delete into the modal
		page.MustElementX(`//input[@type='text']`).MustWaitInteractable().MustInput("delete")
		page.MustElementX(`//button[text() = 'Confirm']`).MustWaitInteractable().MustClick()
		wait()
	}
}
