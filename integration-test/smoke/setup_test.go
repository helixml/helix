//go:build integration || launchpad

package smoke

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/devices"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/helixml/helix/integration-test/smoke/helper"
)

// createBrowser creates a new browser instance for testing
func createBrowser(ctx context.Context) *rod.Browser {
	showBrowser := os.Getenv("SHOW_BROWSER")
	externalBrowserURL := os.Getenv("BROWSER_URL")

	var controlURL string
	if externalBrowserURL != "" {
		controlURL = externalBrowserURL
	} else {
		controlURL = launcher.New().
			Headless(showBrowser == "").
			MustLaunch()
	}

	browser := rod.New().
		ControlURL(controlURL).
		Context(ctx).
		MustConnect()

	if externalBrowserURL != "" && showBrowser != "" {
		launcher.Open(browser.ServeMonitor(""))
	}

	return browser
}

func createPage(browser *rod.Browser) *rod.Page {
	// If any single test takes longer than 3 minutes, take a screenshot
	ctx, cancel := context.WithTimeout(browser.GetContext(), 180*time.Second)

	// A custom device screen size so that it works on laptops
	device := devices.LaptopWithHiDPIScreen.Landscape()
	device.Screen.Horizontal.Width = 1200
	device.Screen.Horizontal.Height = 600

	page := browser.
		Context(ctx).
		DefaultDevice(device).
		Trace(true).
		MustPage(helper.GetServerURL())

	go func() {
		<-ctx.Done()
		err := ctx.Err()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				page.MustScreenshotFullPage(fmt.Sprintf("%s.png", time.Now().Format("2006-01-02-15-04-05")))
			}
		}
		_ = page.Close() // Always close the page, ignore if it already closed
		cancel()
	}()

	return page
}
