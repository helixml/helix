package skills

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// BrowserOAuthAutomator handles generic OAuth browser automation flows
type BrowserOAuthAutomator struct {
	browser *rod.Browser
	logger  zerolog.Logger

	// Provider-specific configuration
	config BrowserOAuthConfig
}

// BrowserOAuthConfig contains provider-specific browser automation configuration
type BrowserOAuthConfig struct {
	ProviderName            string
	LoginUsernameSelector   string
	LoginPasswordSelector   string
	LoginButtonSelector     string
	AuthorizeButtonSelector string
	CallbackURLPattern      string
	DeviceVerificationCheck func(url string) bool
	TwoFactorHandler        TwoFactorHandler
}

// TwoFactorHandler interface for handling different types of 2FA
type TwoFactorHandler interface {
	IsRequired(page *rod.Page) bool
	Handle(page *rod.Page, automator *BrowserOAuthAutomator) error
}

// ScreenshotTaker interface for taking screenshots during automation
type ScreenshotTaker interface {
	TakeScreenshot(page *rod.Page, stepName string)
}

// HumanLikeInteractor provides human-like automation methods
type HumanLikeInteractor struct {
	logger zerolog.Logger
}

// NewHumanLikeInteractor creates a new human-like interactor
func NewHumanLikeInteractor(logger zerolog.Logger) *HumanLikeInteractor {
	return &HumanLikeInteractor{
		logger: logger,
	}
}

// NewBrowserOAuthAutomator creates a new browser OAuth automator
func NewBrowserOAuthAutomator(browser *rod.Browser, logger zerolog.Logger, config BrowserOAuthConfig) *BrowserOAuthAutomator {
	return &BrowserOAuthAutomator{
		browser: browser,
		logger:  logger,
		config:  config,
	}
}

// PerformOAuthFlow performs the complete OAuth authorization flow using browser automation
func (a *BrowserOAuthAutomator) PerformOAuthFlow(authURL, state, username, password string, screenshotTaker ScreenshotTaker) (string, error) {
	a.logger.Info().
		Str("auth_url", authURL).
		Str("state", state).
		Str("provider", a.config.ProviderName).
		Msg("Starting browser automation for OAuth")

	// Create a new page for the OAuth flow with overall timeout
	a.logger.Info().Msg("Creating new browser page for OAuth flow")
	page, err := a.browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create browser page: %w", err)
	}
	defer page.Close()

	// Set overall timeout for page operations
	page = page.Timeout(30 * time.Second)

	// Navigate to OAuth authorization URL
	a.logger.Info().Str("url", authURL).Msg("Navigating to OAuth authorization URL")
	err = page.Navigate(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to OAuth URL: %w", err)
	}

	// Wait for page to load with timeout
	a.logger.Info().Msg("Waiting for page to load")
	err = page.Timeout(15 * time.Second).WaitLoad()
	if err != nil {
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Page loaded successfully")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_loaded")

	// Add enhanced screenshot capture for OAuth flow debugging
	if enhancedScreenshotTaker, ok := screenshotTaker.(interface {
		TakeTimedScreenshot(page *rod.Page, stepName string)
		StartAutoScreenshots(page *rod.Page, stepName string)
	}); ok {
		enhancedScreenshotTaker.TakeTimedScreenshot(page, "oauth_flow_started")
		enhancedScreenshotTaker.StartAutoScreenshots(page, "oauth_flow")
	}

	// Check if we need to login
	a.logger.Info().Msg("Checking if login is required")
	loginRequired, err := a.checkLoginRequired(page)
	if err != nil {
		return "", fmt.Errorf("failed to check login requirement: %w", err)
	}

	a.logger.Info().Bool("login_required", loginRequired).Msg("Login requirement check completed")

	if loginRequired {
		a.logger.Info().Msg("Login required - starting login process")

		// Take enhanced screenshot before login
		if enhancedScreenshotTaker, ok := screenshotTaker.(interface {
			TakeTimedScreenshot(page *rod.Page, stepName string)
		}); ok {
			enhancedScreenshotTaker.TakeTimedScreenshot(page, "before_login")
		}

		err = a.performLogin(page, username, password, screenshotTaker)
		if err != nil {
			// Take enhanced screenshot on login failure
			if enhancedScreenshotTaker, ok := screenshotTaker.(interface {
				TakeTimedScreenshot(page *rod.Page, stepName string)
			}); ok {
				enhancedScreenshotTaker.TakeTimedScreenshot(page, "login_failed")
			}
			return "", fmt.Errorf("failed to perform login: %w", err)
		}

		// Take enhanced screenshot after successful login
		if enhancedScreenshotTaker, ok := screenshotTaker.(interface {
			TakeTimedScreenshot(page *rod.Page, stepName string)
		}); ok {
			enhancedScreenshotTaker.TakeTimedScreenshot(page, "after_login")
		}

		// Handle 2FA if required
		if a.config.TwoFactorHandler != nil && a.config.TwoFactorHandler.IsRequired(page) {
			a.logger.Info().Msg("Two-factor authentication required - handling 2FA")
			err = a.config.TwoFactorHandler.Handle(page, a)
			if err != nil {
				return "", fmt.Errorf("failed to handle 2FA: %w", err)
			}
		}

		// Check if we need to navigate back to OAuth after login/2FA
		a.logger.Info().Msg("Checking if navigation back to OAuth is needed")
		err = a.navigateBackToOAuthIfNeeded(page, authURL, screenshotTaker)
		if err != nil {
			return "", fmt.Errorf("failed to navigate back to OAuth: %w", err)
		}
	}

	// Check if already at callback (OAuth completed)
	a.logger.Info().Msg("Checking if OAuth flow is already completed")
	authCode, completed := a.checkOAuthCompleted(page, state)
	if completed {
		a.logger.Info().Msg("OAuth flow already completed - returning authorization code")
		return authCode, nil
	}

	// Look for and click authorization button
	a.logger.Info().Msg("Starting authorization button search and click")
	err = a.performAuthorization(page, screenshotTaker)
	if err != nil {
		return "", fmt.Errorf("failed to perform authorization: %w", err)
	}

	// Wait for callback with authorization code
	a.logger.Info().Msg("Starting callback wait process")
	return a.waitForCallback(page, state, screenshotTaker)
}

// checkLoginRequired checks if login is required
func (a *BrowserOAuthAutomator) checkLoginRequired(page *rod.Page) (bool, error) {
	a.logger.Info().
		Str("username_selector", a.config.LoginUsernameSelector).
		Msg("Checking if login is required")

	// Set timeout for operations
	page = page.Timeout(10 * time.Second)

	// Wait a moment for the page to fully load
	a.logger.Info().Msg("Waiting 2 seconds for page to fully load")
	time.Sleep(2 * time.Second)

	// Check if we need to login first
	loginElement, err := page.Element(a.config.LoginUsernameSelector)
	loginRequired := loginElement != nil

	if err != nil {
		a.logger.Info().Err(err).Msg("Login element not found - login not required")
	} else {
		a.logger.Info().Msg("Login element found - login required")
	}

	a.logger.Info().Bool("login_required", loginRequired).Msg("Login requirement check completed")
	return loginRequired, nil
}

// performLogin handles the login process with intelligent field detection
func (a *BrowserOAuthAutomator) performLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Login required - starting login process")

	// Fill in username
	a.logger.Info().Msg("Step 1: Entering username/email")
	usernameElement, err := page.Element(a.config.LoginUsernameSelector)
	if err != nil {
		return fmt.Errorf("failed to find username field: %w", err)
	}

	err = usernameElement.Input(username)
	if err != nil {
		return fmt.Errorf("failed to enter username: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_username_entered")

	// Try to find password field immediately first
	a.logger.Info().Msg("Step 2: Looking for password field")
	passwordElement, err := page.Element(a.config.LoginPasswordSelector)
	if err != nil {
		a.logger.Info().Msg("Password field not immediately available, trying to click Next button")
	} else {
		// Check if password field is visible (Google hides it until Next is clicked)
		visible, visErr := passwordElement.Visible()
		if visErr != nil || !visible {
			a.logger.Info().Msg("Password field found but not visible, trying to click Next button")
			passwordElement = nil // Reset so we go through the Next click logic
			err = fmt.Errorf("password field not visible")
		}
	}

	if err != nil {

		// Try to click a Next/Continue button to proceed to password field
		// Use modern Google button class patterns as they work better than old ID selectors
		nextButtonSelectors := []string{
			// Modern Google Material Design button class patterns (these work!)
			"button.VfPpkd-LgbsSe.nCP5yc.AjY5Oe.DuMIQc",
			"button.VfPpkd-LgbsSe.nCP5yc",
			"button.VfPpkd-LgbsSe",
			// Fallback to configured selectors
		}

		// Add configured selectors as fallback
		configuredSelectors := strings.Split(a.config.LoginButtonSelector, ",")
		for _, selector := range configuredSelectors {
			nextButtonSelectors = append(nextButtonSelectors, strings.TrimSpace(selector))
		}

		var nextClicked bool
		a.logger.Info().Int("selector_count", len(nextButtonSelectors)).Msg("Trying Next button selectors (Google class patterns first)")
		for _, selector := range nextButtonSelectors {
			a.logger.Info().Str("selector", selector).Msg("Trying Next button selector")

			// Use shorter timeout for modern class patterns, longer for old selectors
			timeout := 3 * time.Second
			if strings.Contains(selector, "VfPpkd") {
				timeout = 5 * time.Second // Google class patterns may need more time
			}

			nextButton, nextErr := page.Timeout(timeout).Element(selector)
			if nextErr == nil {
				a.logger.Info().Str("selector", selector).Msg("Found Next button element")

				// Check if button is visible and enabled
				visible, visErr := nextButton.Visible()
				if visErr != nil || !visible {
					a.logger.Warn().Str("selector", selector).Err(visErr).Msg("Next button not visible")
					continue
				}

				// For Google class patterns, find button with Next/Continue text
				if strings.Contains(selector, "VfPpkd") {
					buttonText, textErr := nextButton.Text()
					if textErr == nil {
						buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
						if buttonTextLower == "next" || buttonTextLower == "continue" || buttonTextLower == "weiter" {
							a.logger.Info().Str("selector", selector).Str("text", buttonText).Msg("Found Next button with correct text")
						} else {
							a.logger.Info().Str("selector", selector).Str("text", buttonText).Msg("Button found but wrong text, skipping")
							continue
						}
					}
				}

				clickErr := nextButton.Click(proto.InputMouseButtonLeft, 1)
				if clickErr == nil {
					a.logger.Info().Str("selector", selector).Msg("Successfully clicked Next button")
					nextClicked = true
					break
				} else {
					a.logger.Warn().Str("selector", selector).Err(clickErr).Msg("Failed to click Next button")
				}
			} else {
				a.logger.Info().Str("selector", selector).Err(nextErr).Msg("Next button selector not found")
			}
		}

		if nextClicked {
			// Wait for password field to appear after clicking Next
			a.logger.Info().Msg("Waiting for password field to appear after Next click")
			time.Sleep(2 * time.Second)

			// Try to find password field again
			for attempts := 0; attempts < 5; attempts++ {
				passwordElement, err = page.Element(a.config.LoginPasswordSelector)
				if err == nil {
					break
				}
				a.logger.Info().Int("attempt", attempts+1).Msg("Password field not yet available, waiting...")
				time.Sleep(1 * time.Second)
			}
		}

		if err != nil {
			return fmt.Errorf("failed to find password field: %w", err)
		}
	}

	// Fill in password
	a.logger.Info().Msg("Entering password")

	// Wait for password field to be ready for input
	time.Sleep(1 * time.Second)

	// Check if password field is enabled and visible
	visible, err := passwordElement.Visible()
	if err != nil {
		return fmt.Errorf("failed to check password field visibility: %w", err)
	}
	if !visible {
		return fmt.Errorf("password field is not visible")
	}

	// Focus on password field first
	err = passwordElement.Focus()
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to focus password field, continuing anyway")
	}

	// Try to clear the field first
	err = passwordElement.SelectAllText()
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to select all text in password field, continuing anyway")
	}

	// Enter password
	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter password: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_password_entered")

	// Click login button (could be the same button as Next for some flows)
	a.logger.Info().Msg("Step 3: Clicking login button")

	// Use the same Google class patterns for the final login button
	loginButtonSelectors := []string{
		// Modern Google Material Design button class patterns (these work!)
		"button.VfPpkd-LgbsSe.nCP5yc.AjY5Oe.DuMIQc",
		"button.VfPpkd-LgbsSe.nCP5yc",
		"button.VfPpkd-LgbsSe",
		// Fallback to configured selectors
	}

	// Add configured selectors as fallback
	configuredLoginSelectors := strings.Split(a.config.LoginButtonSelector, ",")
	for _, selector := range configuredLoginSelectors {
		loginButtonSelectors = append(loginButtonSelectors, strings.TrimSpace(selector))
	}

	var loginButton *rod.Element
	a.logger.Info().Int("selector_count", len(loginButtonSelectors)).Msg("Trying login button selectors (Google class patterns first)")
	for _, selector := range loginButtonSelectors {
		a.logger.Info().Str("selector", selector).Msg("Trying login button selector")

		// Use appropriate timeout
		timeout := 3 * time.Second
		if strings.Contains(selector, "VfPpkd") {
			timeout = 5 * time.Second
		}

		element, elemErr := page.Timeout(timeout).Element(selector)
		if elemErr == nil {
			a.logger.Info().Str("selector", selector).Msg("Found login button element")

			// Check if button is visible and enabled
			visible, visErr := element.Visible()
			if visErr != nil || !visible {
				a.logger.Warn().Str("selector", selector).Err(visErr).Msg("Login button not visible")
				continue
			}

			// For Google class patterns, find button with login/sign in text
			if strings.Contains(selector, "VfPpkd") {
				buttonText, textErr := element.Text()
				if textErr == nil {
					buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
					if buttonTextLower == "sign in" || buttonTextLower == "next" || buttonTextLower == "continue" ||
						buttonTextLower == "anmelden" || buttonTextLower == "weiter" {
						a.logger.Info().Str("selector", selector).Str("text", buttonText).Msg("Found login button with correct text")
						loginButton = element
						break
					} else {
						a.logger.Info().Str("selector", selector).Str("text", buttonText).Msg("Button found but wrong text, skipping")
						continue
					}
				}
			} else {
				loginButton = element
				break
			}
		} else {
			a.logger.Info().Str("selector", selector).Err(elemErr).Msg("Login button selector not found")
		}
	}

	if loginButton == nil {
		return fmt.Errorf("failed to find login button")
	}

	err = loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	a.logger.Info().Msg("Login process completed successfully")

	// Wait for login navigation
	return a.waitForNavigation(page, screenshotTaker)
}

// waitForNavigation waits for page navigation to complete
func (a *BrowserOAuthAutomator) waitForNavigation(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Waiting for page navigation after login")

	// Take screenshot before navigation
	screenshotTaker.TakeScreenshot(page, "navigation_start")

	// Record current URL
	initialURL := page.MustInfo().URL
	a.logger.Info().Str("initial_url", initialURL).Msg("Starting navigation wait from this URL")

	// Wait for navigation with timeout
	timeout := 60 * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return fmt.Errorf("navigation timeout after %v", timeout)
		}

		// Check current URL
		currentURL := page.MustInfo().URL
		if currentURL != initialURL {
			a.logger.Info().Str("old_url", initialURL).Str("new_url", currentURL).Int("seconds_waited", int(time.Since(start).Seconds())).Msg("Navigation detected")

			// Wait for page to fully load
			a.logger.Info().Msg("Navigation detected - waiting for page to fully load")
			time.Sleep(5 * time.Second)

			// Final URL check
			finalURL := page.MustInfo().URL
			a.logger.Info().Str("final_url", finalURL).Msg("Page navigation and load completed")

			break
		}

		// Small delay before checking again
		time.Sleep(1 * time.Second)
	}

	return nil
}

// navigateBackToOAuthIfNeeded navigates back to OAuth authorization if needed
func (a *BrowserOAuthAutomator) navigateBackToOAuthIfNeeded(page *rod.Page, authURL string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Checking if navigation back to OAuth is needed")

	currentURL := page.MustInfo().URL

	// Check if we need to navigate back to OAuth
	if !strings.Contains(currentURL, "oauth") && !strings.Contains(currentURL, "authorize") {
		a.logger.Info().Str("current_url", currentURL).Str("auth_url", authURL).Msg("Not on OAuth page, navigating back to authorization")

		// Navigate back to OAuth URL
		err := page.Navigate(authURL)
		if err != nil {
			return fmt.Errorf("failed to navigate back to OAuth URL: %w", err)
		}

		// Wait for page to load
		time.Sleep(3 * time.Second)
		screenshotTaker.TakeScreenshot(page, "navigate_back_to_oauth")

		a.logger.Info().Msg("Successfully navigated back to OAuth")
	} else {
		a.logger.Info().Msg("Already on OAuth page, no navigation needed")
	}

	return nil
}

// checkOAuthCompleted checks if OAuth flow has completed
func (a *BrowserOAuthAutomator) checkOAuthCompleted(page *rod.Page, state string) (string, bool) {
	a.logger.Info().Msg("Checking if OAuth flow is already completed")

	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Current page URL")

	// Check if we're already at callback URL
	if strings.Contains(currentURL, "callback") {
		a.logger.Info().Msg("OAuth flow completed - already at callback URL")

		// Extract authorization code from URL
		parsedURL, err := url.Parse(currentURL)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to parse callback URL")
			return "", false
		}

		code := parsedURL.Query().Get("code")
		if code != "" {
			a.logger.Info().Str("auth_code", code[:10]+"...").Str("state", state).Msg("Successfully extracted authorization code from callback URL")
			return code, true
		}
	}

	return "", false
}

// performAuthorization performs OAuth authorization
func (a *BrowserOAuthAutomator) performAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Checking for authorization button")

	// Look for authorization button
	authButton, err := a.findAuthorizationButton(page)
	if err != nil {
		a.logger.Info().Msg("No authorization button found - may already be authorized")
		return nil
	}

	// Click authorization button
	err = authButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click authorization button: %w", err)
	}

	a.logger.Info().Msg("Clicked authorization button")
	screenshotTaker.TakeScreenshot(page, "authorization_clicked")

	return nil
}

// findAuthorizationButton finds the authorization button on the page
func (a *BrowserOAuthAutomator) findAuthorizationButton(page *rod.Page) (*rod.Element, error) {
	a.logger.Info().Msg("Looking for authorization button")

	// Common authorization button selectors
	authSelectors := []string{
		a.config.AuthorizeButtonSelector,
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:contains("Authorize")`,
		`button:contains("Allow")`,
		`button:contains("Grant")`,
		`button:contains("Accept")`,
		`button:contains("Continue")`,
		`button:contains("Confirm")`,
		`input[value="Authorize"]`,
		`input[value="Allow"]`,
		`input[value="Grant"]`,
		`input[value="Accept"]`,
		`input[value="Continue"]`,
		`input[value="Confirm"]`,
	}

	for _, selector := range authSelectors {
		if selector == "" {
			continue
		}

		a.logger.Info().Str("selector", selector).Msg("Trying authorization button selector")

		element, err := page.Element(selector)
		if err == nil && element != nil {
			a.logger.Info().Str("selector", selector).Msg("Found authorization button")
			return element, nil
		}
	}

	// Try finding by text content
	buttons, err := page.Elements("button")
	if err == nil {
		for _, button := range buttons {
			text, err := button.Text()
			if err == nil {
				lowerText := strings.ToLower(strings.TrimSpace(text))
				if lowerText == "authorize" || lowerText == "allow" || lowerText == "grant" || lowerText == "accept" || lowerText == "continue" || lowerText == "confirm" {
					a.logger.Info().Str("text", text).Msg("Found authorization button by text")
					return button, nil
				}
			}
		}
	}

	// Try finding inputs by value
	inputs, err := page.Elements("input")
	if err == nil {
		for _, input := range inputs {
			value, err := input.Attribute("value")
			if err == nil && value != nil {
				lowerValue := strings.ToLower(strings.TrimSpace(*value))
				if lowerValue == "authorize" || lowerValue == "allow" || lowerValue == "grant" || lowerValue == "accept" || lowerValue == "continue" || lowerValue == "confirm" {
					a.logger.Info().Str("value", *value).Msg("Found authorization input by value")
					return input, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("authorization button not found")
}

// waitForCallback waits for OAuth callback
func (a *BrowserOAuthAutomator) waitForCallback(page *rod.Page, state string, _ ScreenshotTaker) (string, error) {
	a.logger.Info().Msg("Waiting for OAuth callback")

	timeout := 60 * time.Second
	start := time.Now()

	for {
		if time.Since(start) > timeout {
			return "", fmt.Errorf("callback timeout after %v", timeout)
		}

		// Check if we're at callback URL
		code, completed := a.checkOAuthCompleted(page, state)
		if completed {
			a.logger.Info().Msg("OAuth flow already completed - returning authorization code")
			return code, nil
		}

		// Small delay before checking again
		time.Sleep(1 * time.Second)
	}
}
