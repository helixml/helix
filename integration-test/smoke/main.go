package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// TestCase represents a single smoke test
type TestCase struct {
	Name        string
	Description string
	Timeout     time.Duration
	Run         func(*rod.Browser) error
}

// Add this helper function at the top level
func logStep(step string) {
	fmt.Printf("  ⏩ %s\n", step)
}

// Add this struct for cookie serialization
type StoredCookies struct {
	URL     string                      `json:"url"`
	Cookies []*proto.NetworkCookieParam `json:"cookies"`
	Time    time.Time                   `json:"time"`
}

// Add these helper functions
func getCookiePath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "helix-smoke-test-cookies.json")
}

func saveCookies(page *rod.Page) error {
	logStep("Saving cookies for future tests")

	cookies := page.MustCookies(fmt.Sprintf("%s/auth/realms/helix/", getServerURL()))

	logStep(fmt.Sprintf("Found %d total cookies", len(cookies)))

	// Only save if we found auth cookies
	if len(cookies) == 0 {
		return fmt.Errorf("no auth cookies found after login")
	}

	stored := StoredCookies{
		URL:     getServerURL(),
		Cookies: convertCookies(cookies),
		Time:    time.Now(),
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	return os.WriteFile(getCookiePath(), data, 0600)
}

func loadCookies(page *rod.Page) error {
	data, err := os.ReadFile(getCookiePath())
	if err != nil {
		return fmt.Errorf("no saved cookies found: %w", err)
	}

	var stored StoredCookies
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	// Check if cookies are expired (24 hours)
	if time.Since(stored.Time) > 24*time.Hour {
		return fmt.Errorf("cookies are expired")
	}

	// Check if cookies are for the right URL
	if stored.URL != getServerURL() {
		return fmt.Errorf("cookies are for different URL")
	}

	for _, cookie := range stored.Cookies {
		// Ensure the cookie has the correct domain
		if cookie.Domain == "" {
			serverURL := getServerURL()
			parsedURL, err := url.Parse(serverURL)
			if err == nil {
				cookie.Domain = parsedURL.Host
			}
		}

		if err := page.SetCookies([]*proto.NetworkCookieParam{cookie}); err != nil {
			return fmt.Errorf("failed to set cookie: %w", err)
		}
	}
	return nil
}

// Add this conversion function
func convertCookies(cookies []*proto.NetworkCookie) []*proto.NetworkCookieParam {
	var params []*proto.NetworkCookieParam
	for _, c := range cookies {
		params = append(params, &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
			Expires:  c.Expires,
		})
	}
	return params
}

// Modify the Login test to save cookies
func performLogin(browser *rod.Browser) error {
	page := browser.MustPage(getServerURL())
	page.MustWaitLoad()

	// Try to load existing cookies first
	if err := loadCookies(page); err != nil {
		logStep(fmt.Sprintf("Need to login: %v", err))

		logStep("Looking for login button")
		loginBtn := page.MustElement("button[id='login-button']")

		logStep("Clicking login button")
		loginBtn.MustClick()

		logStep("Waiting for login page to load")
		page.MustWaitLoad()

		logStep("Getting credentials from environment")
		username := os.Getenv("HELIX_USER")
		password := os.Getenv("HELIX_PASSWORD")

		if username == "" || password == "" {
			return fmt.Errorf("HELIX_USER and HELIX_PASSWORD environment variables must be set")
		}

		logStep("Entering username")
		page.MustElement("input[type='text']").MustInput(username)

		logStep("Entering password")
		page.MustElement("input[type='password']").MustInput(password)

		logStep("Clicking submit button")
		page.MustElement("input[type='submit']").MustClick()

		logStep("Waiting for navigation after login")
		page.MustWaitStable()

		logStep(fmt.Sprintf("Verifying successful login of %s", username))
		xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
		if len(page.MustElementsX(xpath)) == 0 {
			return fmt.Errorf("login failed - username not found after login")
		}

		// Save cookies for future tests
		if err := saveCookies(page); err != nil {
			logStep(fmt.Sprintf("Warning: Failed to save cookies: %v", err))
		}
	} else {
		// Verify the cookies worked
		logStep("Verifying cookie-based login")
		page.MustReload()
		page.MustWaitStable()

		username := os.Getenv("HELIX_USER")
		xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
		if len(page.MustElementsX(xpath)) == 0 {
			// Take a screenshot
			page.MustScreenshot(filepath.Join(os.TempDir(), "helix-smoke-test-screenshot.png"))
			logStep("Screenshot saved to " + filepath.Join(os.TempDir(), "helix-smoke-test-screenshot.png"))

			return fmt.Errorf("cookie login failed - username not found after loading cookies")
		}
	}

	return nil
}

// TestSuite contains all smoke tests to be run
var TestSuite = []TestCase{
	{
		Name:        "Homepage Load",
		Description: "Verifies that the homepage loads successfully",
		Timeout:     10 * time.Second,
		Run: func(browser *rod.Browser) error {
			logStep("Navigating to homepage")
			page := browser.MustPage(getServerURL())

			logStep("Waiting for page load")
			page.MustWaitLoad()

			logStep("Verifying page loaded successfully")
			if !page.MustHas("body") {
				return fmt.Errorf("homepage failed to load properly")
			}
			return nil
		},
	},
	{
		Name:        "Login Flow",
		Description: "Tests the login functionality using credentials or saved cookies",
		Timeout:     10 * time.Second,
		Run:         performLogin,
	},
	{
		Name:        "Start New Session",
		Description: "Tests starting a new chat session after login",
		Timeout:     20 * time.Second,
		Run: func(browser *rod.Browser) error {
			page := browser.MustPage(getServerURL())
			page.MustWaitLoad()

			// Load saved cookies first
			if err := loadCookies(page); err != nil {
				return fmt.Errorf("failed to load cookies: %w", err)
			}

			logStep("Waiting for page to load after cookie restoration")
			page.MustWaitStable()

			// Verify we're logged in
			username := os.Getenv("HELIX_USER")
			xpath := fmt.Sprintf(`//span[contains(text(), '%s')]`, username)
			if len(page.MustElementsX(xpath)) == 0 {
				return fmt.Errorf("not logged in - username not found")
			}

			logStep("Looking for New Session button")
			xpath = fmt.Sprintf(`//span[contains(text(), '%s')]`, "New Session")
			elements := page.MustElementsX(xpath)
			if len(elements) != 1 {
				return fmt.Errorf("new Session button not found")
			}

			newSessionBtn := elements[0]

			logStep("Clicking New Session button")
			newSessionBtn.MustClick()

			logStep("Waiting for page to stabilize")
			page.MustWaitStable()

			logStep("Looking for chat input textarea")
			textarea := page.MustElement("textarea")

			logStep("Typing 'hello helix' into chat input")
			textarea.MustInput("hello helix")

			logStep("Looking for send button")
			sendButton := page.MustElement("#sendButton")
			// Wait for button to be enabled/clickable
			sendButton.MustClick()

			logStep("Waiting for message to be sent")
			page.MustWaitStable()

			logStep("Looking for chat message")
			chatMessage := page.MustElement("div.interactionMessage")
			if chatMessage == nil {
				return fmt.Errorf("chat message not found")
			}

			return nil
		},
	},
	// Add more test cases here as needed
}

func getServerURL() string {
	url := os.Getenv("SERVER_URL")
	if url == "" {
		log.Fatal("SERVER_URL environment variable is not set")
	}
	return url
}

func runTests(browser *rod.Browser) (passed, failed int) {
	for _, test := range TestSuite {
		fmt.Printf("\n📋 Running Test: %s\n", test.Name)
		fmt.Printf("📝 Description: %s\n", test.Description)

		done := make(chan error)
		start := time.Now()

		go func() {
			done <- test.Run(browser)
		}()

		var err error
		select {
		case err = <-done:
			duration := time.Since(start)
			if err != nil {
				fmt.Printf("\n❌ Test Failed (%s)\n", duration)
				fmt.Printf("   Error: %v\n", err)
				failed++
			} else {
				fmt.Printf("\n✅ Test Passed (%s)\n", duration)
				passed++
			}
		case <-time.After(test.Timeout):
			duration := time.Since(start)
			fmt.Printf("\n❌ Test Failed (%s)\n", duration)
			fmt.Printf("   Error: Timeout after %v\n", test.Timeout)
			failed++
		}
	}
	return
}

func main() {
	// Add command line flag
	showBrowser := flag.Bool("show", false, "Show browser during test execution")
	flag.Parse()

	// Launch browser with configurable headless mode
	url := launcher.New().
		Headless(!*showBrowser).
		MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()
	defer browser.MustClose()

	fmt.Println("🚀 Starting Smoke Tests")
	passed, failed := runTests(browser)

	// Print summary
	fmt.Printf("\n=== Test Summary ===\n")
	fmt.Printf("Passed: %d\n", passed)
	fmt.Printf("Failed: %d\n", failed)
	fmt.Printf("Total: %d\n", passed+failed)

	if failed > 0 {
		os.Exit(1)
	}
}
