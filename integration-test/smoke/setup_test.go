//go:build integration || launchpad

package smoke

import (
	"context"
	"os"
	"testing"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
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

func TestMain(m *testing.M) {
	// Just run the tests - each test will create its own browser
	code := m.Run()
	os.Exit(code)
}
