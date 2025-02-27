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
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
)

func TestDeleteAllApps(t *testing.T) {
	ctx := helper.CreateContext(t)

	browser := createBrowser(ctx)
	defer browser.MustClose()

	page := createPage(browser)

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
	for {
		// Get current private rows - refresh on each iteration
		rows := page.MustElementsX(`//tbody//tr//p[contains(text(), 'Private')]`)
		if len(rows) == 0 {
			break
		}

		rod.Try(func() {
			// Always work with the first row since previous ones get deleted
			row := rows[0]
			helper.LogStep(t, fmt.Sprintf("Deleting app (remaining: %d)", len(rows)))
			deleteButton := row.MustElementX(`//*[name()='svg' and @data-testid='DeleteIcon']`)

			wait := page.MustWaitRequestIdle()
			deleteButton.MustWaitVisible().MustClick()

			// Wait for and type delete into the modal
			page.MustElementX(`//input[@type='text']`).MustWaitVisible().MustInput("delete")
			page.MustElementX(`//button[text() = 'Confirm']`).MustWaitVisible().MustClick()
			wait()

			// Do a hard reload on every delete to ensure the page is updated
			page.MustReload()
			page.Timeout(10 * time.Second).MustElement(`[data-testid='DeleteIcon']`)
		})
	}
}
