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

	// Check if we need to login
	a.logger.Info().Msg("Checking if login is required")
	loginRequired, err := a.checkLoginRequired(page)
	if err != nil {
		return "", fmt.Errorf("failed to check login requirement: %w", err)
	}

	a.logger.Info().Bool("login_required", loginRequired).Msg("Login requirement check completed")

	if loginRequired {
		a.logger.Info().Msg("Login required - starting login process")
		err = a.performLogin(page, username, password, screenshotTaker)
		if err != nil {
			return "", fmt.Errorf("failed to perform login: %w", err)
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

// debugDumpPageElements dumps all available HTML elements for debugging
func (a *BrowserOAuthAutomator) debugDumpPageElements(page *rod.Page, stepName string) {
	a.logger.Info().Str("step", stepName).Msg("=== DEBUGGING PAGE ELEMENTS ===")

	// Set timeout for all operations
	page = page.Timeout(10 * time.Second)

	// Get page HTML with timeout
	html, err := page.HTML()
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to get page HTML")
		return
	}

	// Log current URL
	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Current page URL")

	// Find all input elements with timeout
	inputs, err := page.Elements("input")
	if err == nil {
		a.logger.Info().Int("count", len(inputs)).Msg("Found input elements")
		for i, input := range inputs {
			if i >= 10 { // Limit to first 10 elements
				break
			}

			// Add timeout to attribute operations
			input = input.Timeout(2 * time.Second)

			inputType, _ := input.Attribute("type")
			inputName, _ := input.Attribute("name")
			inputID, _ := input.Attribute("id")
			inputClass, _ := input.Attribute("class")
			inputPlaceholder, _ := input.Attribute("placeholder")
			inputValue, _ := input.Attribute("value")

			// Safe dereference with nil checks
			typeStr := ""
			if inputType != nil {
				typeStr = *inputType
			}
			nameStr := ""
			if inputName != nil {
				nameStr = *inputName
			}
			idStr := ""
			if inputID != nil {
				idStr = *inputID
			}
			classStr := ""
			if inputClass != nil {
				classStr = *inputClass
			}
			placeholderStr := ""
			if inputPlaceholder != nil {
				placeholderStr = *inputPlaceholder
			}
			valueStr := ""
			if inputValue != nil {
				valueStr = *inputValue
			}

			a.logger.Info().
				Int("index", i).
				Str("type", typeStr).
				Str("name", nameStr).
				Str("id", idStr).
				Str("class", classStr).
				Str("placeholder", placeholderStr).
				Str("value", valueStr).
				Msg("Input element found")
		}
	} else {
		a.logger.Error().Err(err).Msg("Failed to find input elements")
	}

	// Find all button elements with timeout
	buttons, err := page.Elements("button")
	if err == nil {
		a.logger.Info().Int("count", len(buttons)).Msg("Found button elements")
		for i, button := range buttons {
			if i >= 10 { // Limit to first 10 elements
				break
			}

			// Add timeout to attribute operations
			button = button.Timeout(2 * time.Second)

			buttonType, _ := button.Attribute("type")
			buttonID, _ := button.Attribute("id")
			buttonClass, _ := button.Attribute("class")
			buttonText, _ := button.Text()

			// Safe dereference with nil checks
			typeStr := ""
			if buttonType != nil {
				typeStr = *buttonType
			}
			idStr := ""
			if buttonID != nil {
				idStr = *buttonID
			}
			classStr := ""
			if buttonClass != nil {
				classStr = *buttonClass
			}

			a.logger.Info().
				Int("index", i).
				Str("type", typeStr).
				Str("id", idStr).
				Str("class", classStr).
				Str("text", buttonText).
				Msg("Button element found")

		}
	} else {
		a.logger.Error().Err(err).Msg("Failed to find button elements")
	}

	// Log a portion of the HTML for manual inspection
	if len(html) > 2000 {
		a.logger.Info().Str("html_snippet", html[:2000]+"...").Msg("HTML snippet (first 2000 chars)")
	} else {
		a.logger.Info().Str("html_snippet", html).Msg("Full HTML")
	}

	a.logger.Info().Msg("=== END DEBUG ELEMENTS ===")
}

// performLogin handles the login process (different flows for different providers)
func (a *BrowserOAuthAutomator) performLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Login required - starting login process")

	// Debug: dump page elements
	a.debugDumpPageElements(page, "login_start")

	// Detect provider type and handle appropriate login flow
	if a.config.ProviderName == "google" || a.config.ProviderName == "microsoft" {
		// Google and Microsoft: Two-step process (email → Next → password → Next)
		a.logger.Info().Str("provider", a.config.ProviderName).Msg("Using two-step login flow")

		// Step 1: Handle email input (first step for Google/Microsoft)
		err := a.handleEmailInput(page, username, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle email input: %w", err)
		}

		// Step 2: Handle password input (second step for Google/Microsoft)
		err = a.handlePasswordInput(page, password, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle password input: %w", err)
		}
	} else {
		// GitHub and other providers: Single-step process (username + password → Sign in)
		a.logger.Info().Str("provider", a.config.ProviderName).Msg("Using single-step login flow")

		err := a.handleSingleStepLogin(page, username, password, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle single-step login: %w", err)
		}
	}

	return nil
}

// handleSingleStepLogin handles single-step login (username + password → Sign in)
func (a *BrowserOAuthAutomator) handleSingleStepLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Handling single-step login")

	// Set timeout for operations
	page = page.Timeout(15 * time.Second)

	// Step 1: Fill username field
	err := a.fillUsernameField(page, username, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to fill username field: %w", err)
	}

	// Step 2: Fill password field
	err = a.fillPasswordField(page, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to fill password field: %w", err)
	}

	// Step 3: Click login button
	err = a.clickLoginButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	// Wait for navigation after login
	return a.waitForNavigation(page, screenshotTaker)
}

// fillUsernameField fills the username field (shared by both login flows)
func (a *BrowserOAuthAutomator) fillUsernameField(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Filling username field")

	// Try to find username field using multiple selectors
	usernameSelectors := strings.Split(a.config.LoginUsernameSelector, ", ")
	var usernameElement *rod.Element
	var err error

	for _, selector := range usernameSelectors {
		selector = strings.TrimSpace(selector)
		usernameElement, err = page.Element(selector)
		if err == nil && usernameElement != nil {
			a.logger.Info().Str("selector", selector).Msg("Found username field")
			break
		}
	}

	if usernameElement == nil {
		a.debugDumpPageElements(page, "username_field_not_found")
		return fmt.Errorf("failed to find username field using selectors: %s", a.config.LoginUsernameSelector)
	}

	// Set timeout for element operations
	usernameElement = usernameElement.Timeout(5 * time.Second)

	// Clear any existing content and enter username
	err = usernameElement.SelectAllText()
	if err == nil {
		err = usernameElement.Input("")
		if err != nil {
			a.logger.Warn().Err(err).Msg("Failed to clear username field")
		}
	}

	err = usernameElement.Input(username)
	if err != nil {
		return fmt.Errorf("failed to enter username: %w", err)
	}

	a.logger.Info().Str("username", username).Msg("Successfully entered username")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_username_filled")

	return nil
}

// fillPasswordField fills the password field (shared by both login flows)
func (a *BrowserOAuthAutomator) fillPasswordField(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Filling password field")

	// Try to find password field using multiple selectors
	passwordSelectors := strings.Split(a.config.LoginPasswordSelector, ", ")
	var passwordElement *rod.Element
	var err error

	for _, selector := range passwordSelectors {
		selector = strings.TrimSpace(selector)
		passwordElement, err = page.Element(selector)
		if err == nil && passwordElement != nil {
			a.logger.Info().Str("selector", selector).Msg("Found password field")
			break
		}
	}

	if passwordElement == nil {
		a.debugDumpPageElements(page, "password_field_not_found")
		return fmt.Errorf("failed to find password field using selectors: %s", a.config.LoginPasswordSelector)
	}

	// Set timeout for element operations
	passwordElement = passwordElement.Timeout(5 * time.Second)

	// Clear any existing content and enter password
	err = passwordElement.SelectAllText()
	if err == nil {
		err = passwordElement.Input("")
		if err != nil {
			a.logger.Warn().Err(err).Msg("Failed to clear password field")
		}
	}

	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter password: %w", err)
	}

	a.logger.Info().Msg("Successfully entered password")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_password_filled")

	return nil
}

// handleEmailInput handles the email input step (Google's first step)
func (a *BrowserOAuthAutomator) handleEmailInput(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Handling email input step")

	// Set timeout for operations
	page = page.Timeout(15 * time.Second)

	// Fill username field (reusing common function)
	err := a.fillUsernameField(page, username, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to fill username field: %w", err)
	}

	// Try Rod's native element selection using exact class pattern from debug output
	a.logger.Info().Msg("Attempting to find Next button using specific class pattern")

	// Use the exact class pattern we see in debug output for the Next button
	specificSelectors := []string{
		// Modern Google Next button class pattern (from debug output)
		"button.VfPpkd-LgbsSe.VfPpkd-LgbsSe-OWXEXe-k8QpJ.VfPpkd-LgbsSe-OWXEXe-dgl2Hf.nCP5yc.AjY5Oe.DuMIQc.LQeN7.BqKGqe.Jskylb.TrZEUc.lw1w4b",
		// Shorter pattern with key classes
		"button.VfPpkd-LgbsSe.nCP5yc.AjY5Oe.DuMIQc",
		// Even shorter pattern
		"button.VfPpkd-LgbsSe.nCP5yc",
		// Fallback to core Google button class
		"button.VfPpkd-LgbsSe",
	}

	var nextButton *rod.Element
	for _, selector := range specificSelectors {
		// Set shorter timeout for each attempt
		elements, err := page.Timeout(3 * time.Second).Elements(selector)
		if err == nil && len(elements) > 0 {
			// Check each element to find the one with "Next" text
			for _, element := range elements {
				buttonText, textErr := element.Timeout(1 * time.Second).Text()
				if textErr == nil && strings.ToLower(strings.TrimSpace(buttonText)) == "next" {
					a.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found Next button using class pattern")
					nextButton = element
					break
				}
			}
			if nextButton != nil {
				break
			}
		}
	}

	if nextButton != nil {
		a.logger.Info().Msg("Clicking Next button using class pattern")
		nextButton.MustClick()
		a.logger.Info().Msg("Successfully clicked Next button via class pattern")
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_next_button_clicked")
	} else {
		a.logger.Warn().Msg("Class pattern selection failed, falling back to manual button search")
		// Fall back to Rod element selection
		err = a.clickNextButton(page, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to click Next button: %w", err)
		}
	}

	// Wait for navigation to password step
	return a.waitForNavigation(page, screenshotTaker)
}

// handlePasswordInput handles the password input step
func (a *BrowserOAuthAutomator) handlePasswordInput(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Handling password input step")

	// Set timeout for operations
	page = page.Timeout(15 * time.Second)

	// Debug: dump page elements at password step
	a.debugDumpPageElements(page, "password_step")

	// Try to find password field using multiple selectors
	passwordSelectors := strings.Split(a.config.LoginPasswordSelector, ", ")
	var passwordElement *rod.Element
	var err error

	for _, selector := range passwordSelectors {
		selector = strings.TrimSpace(selector)
		passwordElement, err = page.Element(selector)
		if err == nil && passwordElement != nil {
			a.logger.Info().Str("selector", selector).Msg("Found password field")
			break
		}
	}

	if passwordElement == nil {
		a.debugDumpPageElements(page, "password_field_not_found")
		return fmt.Errorf("failed to find password field using selectors: %s", a.config.LoginPasswordSelector)
	}

	// Set timeout for element operations
	passwordElement = passwordElement.Timeout(5 * time.Second)

	// Clear any existing content and enter password
	err = passwordElement.SelectAllText()
	if err == nil {
		err = passwordElement.Input("")
		if err != nil {
			a.logger.Warn().Err(err).Msg("Failed to clear password field")
		}
	}

	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter password: %w", err)
	}

	a.logger.Info().Msg("Successfully entered password")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_password_filled")

	// Click login/next button
	err = a.clickLoginButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	// Wait for navigation after login
	return a.waitForNavigation(page, screenshotTaker)
}

// clickNextButton clicks the Next button (for Google's email step)
func (a *BrowserOAuthAutomator) clickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Looking for Next button")

	// Set timeout for operations
	page = page.Timeout(10 * time.Second)

	// First try direct CSS selectors
	nextSelectors := []string{
		`input[id="idSIButton9"]`,            // Microsoft-specific Next button ID
		`input[type="submit"][value="Next"]`, // Input style (Microsoft/general)
		`button[id="identifierNext"]`,        // Old Google style (with ID)
		`button[id="passwordNext"]`,          // Old Google style (with ID)
		`button[type="submit"]`,              // Submit button
		`input[type="submit"]`,               // Fallback input submit
	}

	var nextButton *rod.Element
	var err error

	for _, selector := range nextSelectors {
		nextButton, err = page.Element(selector)
		if err == nil && nextButton != nil {
			a.logger.Info().Str("selector", selector).Msg("Found Next button")
			break
		}
	}

	// If no direct selector worked, try Google-specific class patterns and text content
	if nextButton == nil {
		a.logger.Info().Msg("Direct selectors failed, trying Google-specific patterns")

		// First try Google's button classes (more efficient than text search)
		googleButtonSelectors := []string{
			`button.VfPpkd-LgbsSe.VfPpkd-LgbsSe-OWXEXe-k8QpJ`, // Google's primary button style
			`button.VfPpkd-LgbsSe.Jskylb`,                     // Google's button with Next styling
			`button.VfPpkd-LgbsSe[jsname]`,                    // Google buttons with jsname
		}

		for _, selector := range googleButtonSelectors {
			buttons, err := page.Elements(selector)
			if err == nil {
				for _, button := range buttons {
					button = button.Timeout(1 * time.Second)
					buttonText, textErr := button.Text()
					if textErr == nil {
						buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
						if buttonTextLower == "next" {
							a.logger.Info().Str("button_text", buttonText).Str("selector", selector).Msg("Found Next button by class and text")
							nextButton = button
							break
						}
					}
				}
			}
			if nextButton != nil {
				break
			}
		}

		// If class-based search failed, use JavaScript to click the Next button directly
		if nextButton == nil {
			a.logger.Info().Msg("Class-based search failed, using JavaScript to click Next button directly")

			// Use JavaScript to click the 4th button (index 3) which is consistently the Next button
			jsCode := `
				if (document.querySelectorAll('button').length > 3) {
					if (document.querySelectorAll('button')[3].textContent.trim().toLowerCase() === 'next') {
						document.querySelectorAll('button')[3].click();
						'success';
					} else {
						'wrong_button_text:' + document.querySelectorAll('button')[3].textContent.trim().toLowerCase();
					}
				} else {
					'not_enough_buttons:' + document.querySelectorAll('button').length;
				}
			`

			result, err := page.Eval(jsCode)
			if err != nil {
				a.logger.Error().Err(err).Msg("Failed to execute JavaScript click")
			}
			if err == nil {
				resultStr := result.Value.String()
				if resultStr == "success" {
					a.logger.Info().Msg("Successfully clicked Next button using JavaScript")
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_next_button_clicked")
					return nil // Success, no need to continue with rod methods
				}
				a.logger.Warn().Str("result", resultStr).Msg("JavaScript click failed or unexpected result")
			}
		}
	}

	if nextButton == nil {
		a.debugDumpPageElements(page, "next_button_not_found")
		return fmt.Errorf("failed to find Next button")
	}

	// Set timeout for element operations
	nextButton = nextButton.Timeout(5 * time.Second)

	err = nextButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Next button: %w", err)
	}

	a.logger.Info().Msg("Successfully clicked Next button")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_next_button_clicked")

	// Add debugging for Microsoft - wait and check for error messages
	if a.config.ProviderName == "microsoft" {
		a.logger.Info().Msg("Waiting 3 seconds for Microsoft page to respond to Next button click")
		time.Sleep(3 * time.Second)

		// Check for error messages
		errorSelectors := []string{
			`.error-message`, `.alert-error`, `.field-error`,
			`[role="alert"]`, `[aria-live="polite"]`, `[data-bind*="error"]`,
			`.has-error`, `.validation-error`, `.login-error`,
		}

		for _, selector := range errorSelectors {
			errorElements, err := page.Elements(selector)
			if err == nil && len(errorElements) > 0 {
				for _, errorElement := range errorElements {
					errorText, textErr := errorElement.Text()
					if textErr == nil && strings.TrimSpace(errorText) != "" {
						a.logger.Error().Str("selector", selector).Str("error", errorText).Msg("Microsoft error detected on page")
					}
				}
			}
		}

		// Check if we're still on the same page (no navigation)
		currentURL := page.MustInfo().URL
		a.logger.Info().Str("current_url", currentURL).Msg("Current URL after Next button click")

		// Take another screenshot to see current state
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_after_next_button_debug")
	}

	return nil
}

// clickLoginButton clicks the login/sign in button
func (a *BrowserOAuthAutomator) clickLoginButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Looking for login button")

	// Set timeout for operations
	page = page.Timeout(10 * time.Second)

	// First try the same successful class pattern approach that worked for the Next button
	a.logger.Info().Msg("Attempting to find login button using same class pattern as Next button")

	// Microsoft and Google-specific button selectors
	specificSelectors := []string{
		// Microsoft-specific patterns
		`input[id="idSIButton9"]`,               // Microsoft Next/Sign in button ID
		`input[type="submit"][value="Sign in"]`, // Microsoft sign in button
		`input[type="submit"][value="Next"]`,    // Microsoft next button
		// Google patterns
		"button.VfPpkd-LgbsSe.VfPpkd-LgbsSe-OWXEXe-k8QpJ.VfPpkd-LgbsSe-OWXEXe-dgl2Hf.nCP5yc.AjY5Oe.DuMIQc.LQeN7.BqKGqe.Jskylb.TrZEUc.lw1w4b",
		// Shorter Google patterns
		"button.VfPpkd-LgbsSe.nCP5yc.AjY5Oe.DuMIQc",
		"button.VfPpkd-LgbsSe.nCP5yc",
		"button.VfPpkd-LgbsSe",
	}

	var loginButton *rod.Element
	for _, selector := range specificSelectors {
		// For Microsoft ID selectors, try direct match first
		if strings.Contains(selector, "idSIButton9") || strings.Contains(selector, "value=") {
			element, err := page.Timeout(3 * time.Second).Element(selector)
			if err == nil && element != nil {
				a.logger.Info().Str("selector", selector).Msg("Found login button using Microsoft selector")
				loginButton = element
				break
			}
		} else {
			// For Google class selectors, check text content
			elements, err := page.Timeout(3 * time.Second).Elements(selector)
			if err == nil && len(elements) > 0 {
				for _, element := range elements {
					buttonText, textErr := element.Timeout(1 * time.Second).Text()
					if textErr == nil {
						buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
						if buttonTextLower == "next" || buttonTextLower == "sign in" {
							a.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found login button using class pattern")
							loginButton = element
							break
						}
					}
				}
				if loginButton != nil {
					break
				}
			}
		}
	}

	if loginButton == nil {
		// Fallback to original selectors
		a.logger.Warn().Msg("Class pattern approach failed, falling back to original selectors")
		loginSelectors := strings.Split(a.config.LoginButtonSelector, ", ")
		var err error

		for _, selector := range loginSelectors {
			selector = strings.TrimSpace(selector)
			loginButton, err = page.Element(selector)
			if err == nil && loginButton != nil {
				a.logger.Info().Str("selector", selector).Msg("Found login button using fallback selector")
				break
			}
		}
	}

	if loginButton == nil {
		a.debugDumpPageElements(page, "login_button_not_found")
		return fmt.Errorf("failed to find login button using selectors: %s", a.config.LoginButtonSelector)
	}

	// Set timeout for element operations
	loginButton = loginButton.Timeout(5 * time.Second)

	err := loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	a.logger.Info().Msg("Successfully clicked login button")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_button_clicked")

	return nil
}

// waitForNavigation waits for page navigation after login
func (a *BrowserOAuthAutomator) waitForNavigation(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Waiting for page navigation after login")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_button_clicked")

	// Microsoft uses dynamic page updates instead of URL navigation
	if a.config.ProviderName == "microsoft" {
		return a.waitForMicrosoftPageTransition(page, screenshotTaker)
	}

	// Wait for URL to change (indicating navigation started) - for Google/GitHub
	currentURL := page.MustInfo().URL
	a.logger.Info().Str("initial_url", currentURL).Msg("Starting navigation wait from this URL")

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	navigationStarted := false
	checkCount := 0
	for !navigationStarted {
		select {
		case <-timeout:
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_timeout")
			a.logger.Error().
				Str("initial_url", currentURL).
				Str("final_url", page.MustInfo().URL).
				Int("check_count", checkCount).
				Msg("Timeout waiting for login navigation")
			return fmt.Errorf("timeout waiting for login navigation")
		case <-ticker.C:
			checkCount++
			newURL := page.MustInfo().URL
			if checkCount%4 == 0 { // Log every 2 seconds
				a.logger.Info().
					Str("current_url", newURL).
					Int("check_count", checkCount).
					Msg("Navigation check - URL still unchanged")
			}
			if newURL != currentURL {
				a.logger.Info().Str("old_url", currentURL).Str("new_url", newURL).Msg("Navigation detected")
				navigationStarted = true
			}
		}
	}

	// Wait for page to fully load after navigation
	a.logger.Info().Msg("Navigation detected - waiting for page to fully load")
	page.MustWaitLoad()

	// Additional small wait to ensure all dynamic content loads
	a.logger.Info().Msg("Waiting additional 2 seconds for dynamic content to load")
	time.Sleep(2 * time.Second)

	finalURL := page.MustInfo().URL
	a.logger.Info().Str("final_url", finalURL).Msg("Page navigation and load completed")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_after_login")

	return nil
}

// waitForMicrosoftPageTransition waits for Microsoft's dynamic page transition
func (a *BrowserOAuthAutomator) waitForMicrosoftPageTransition(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Waiting for Microsoft page transition")

	// Take screenshot at start of transition
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_page_transition_start")

	// Debug the current page state
	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Current URL at start of Microsoft transition")

	// Detect current step first
	isPasswordStepVisible := false
	passwordField, err := page.Element(`input[type="password"]:not(.moveOffScreen)`)
	if err == nil && passwordField != nil {
		visible, visErr := passwordField.Visible()
		if visErr == nil && visible {
			isPasswordStepVisible = true
		}
	}

	if isPasswordStepVisible {
		a.logger.Info().Msg("Microsoft currently on password step - waiting for transition to next step")
	} else {
		a.logger.Info().Msg("Microsoft already past password step - waiting for transition to final step")
	}

	timeout := time.After(15 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	transitionDetected := false
	checkCount := 0

	for !transitionDetected {
		select {
		case <-timeout:
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_transition_timeout")

			// Debug what's on the page when timeout occurs
			a.logger.Info().Msg("=== DEBUGGING MICROSOFT TRANSITION TIMEOUT ===")
			a.debugDumpPageElements(page, "microsoft_transition_timeout")
			a.logger.Info().Msg("=== END TRANSITION TIMEOUT DEBUG ===")

			a.logger.Error().
				Int("check_count", checkCount).
				Msg("Timeout waiting for Microsoft page transition")
			return fmt.Errorf("timeout waiting for Microsoft page transition")
		case <-ticker.C:
			checkCount++

			if isPasswordStepVisible {
				// We're waiting for email → password transition
				// Check if password field is now visible and active (first transition)
				passwordField, err := page.Element(`input[type="password"]:not(.moveOffScreen)`)
				if err == nil && passwordField != nil {
					// Verify it's actually visible
					visible, visErr := passwordField.Visible()
					if visErr == nil && visible {
						a.logger.Info().Msg("Password field is now visible - Microsoft page transition successful")
						transitionDetected = true
						break
					}
				}
			} else {
				// We're waiting for password → final step transition
				// Check if we've moved past password step to consent/completion

				// Look for consent buttons or success indicators
				consentElements, err := page.Elements(`input[type="submit"][value*="Accept"], input[type="submit"][value*="Allow"], button[type="submit"]`)
				if err == nil && len(consentElements) > 0 {
					a.logger.Info().Msg("Microsoft consent screen detected - transition successful")
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_consent_screen_detected")
					transitionDetected = true
					break
				}

				// Check if we've reached a "Keep me signed in" prompt
				kmsiElements, err := page.Elements(`input[id*="kmsi"], input[type="submit"][value*="Yes"], input[type="submit"][value*="No"]`)
				if err == nil && len(kmsiElements) > 0 {
					a.logger.Info().Msg("Microsoft 'Keep me signed in' prompt detected - transition successful")
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_kmsi_prompt_detected")
					transitionDetected = true
					break
				}

				// Check if we've been redirected to success/error pages
				currentURL := page.MustInfo().URL
				if !strings.Contains(currentURL, "login.microsoftonline.com/common/oauth2/v2.0/authorize") {
					// We've been redirected away from the login page
					a.logger.Info().Str("new_url", currentURL).Msg("Microsoft redirected away from login page - transition successful")
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_redirect_away_from_login")
					transitionDetected = true
					break
				}
			}

			// Also check if we've already reached callback (OAuth completed)
			currentURL := page.MustInfo().URL
			if strings.Contains(currentURL, a.config.CallbackURLPattern) || strings.Contains(currentURL, "code=") {
				a.logger.Info().Msg("OAuth callback detected - Microsoft flow completed")
				transitionDetected = true
				break
			}

			if checkCount%4 == 0 { // Log every 2 seconds
				a.logger.Info().
					Int("check_count", checkCount).
					Msg("Microsoft transition check - waiting for password field")
			}
		}
	}

	// Wait for page to fully update after transition
	a.logger.Info().Msg("Microsoft transition detected - waiting for page to fully update")
	time.Sleep(1 * time.Second)

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_after_transition")

	return nil
}

// navigateBackToOAuthIfNeeded checks if we need to navigate back to OAuth URL after login/2FA
func (a *BrowserOAuthAutomator) navigateBackToOAuthIfNeeded(page *rod.Page, authURL string, screenshotTaker ScreenshotTaker) error {
	currentURL := page.MustInfo().URL

	// Check if we're still on a session/verification page or redirected away from OAuth
	needsRedirect := false
	if a.config.DeviceVerificationCheck != nil {
		needsRedirect = a.config.DeviceVerificationCheck(currentURL) || !strings.Contains(currentURL, "oauth")
	} else {
		needsRedirect = strings.Contains(currentURL, "/session") || !strings.Contains(currentURL, "oauth")
	}

	// Handle Microsoft enterprise authentication pages
	if strings.Contains(currentURL, "login.microsoftonline.com/common/login") {
		a.logger.Info().Str("current_url", currentURL).Msg("Microsoft enterprise authentication page detected - attempting to handle")
		// Don't redirect away, let the Microsoft handler deal with this page
		return nil
	}

	if needsRedirect {
		a.logger.Info().Str("current_url", currentURL).Msg("Redirected away from OAuth, navigating back to OAuth URL")

		// Navigate back to the OAuth authorization URL with timeout
		page = page.Timeout(15 * time.Second)
		err := page.Navigate(authURL)
		if err != nil {
			return fmt.Errorf("failed to re-navigate to OAuth URL: %w", err)
		}

		// Wait for page to load with timeout
		err = page.WaitLoad()
		if err != nil {
			return fmt.Errorf("failed to wait for OAuth page load: %w", err)
		}

		// Additional wait for dynamic content
		time.Sleep(2 * time.Second)
		screenshotTaker.TakeScreenshot(page, "oauth_page_after_session_redirect")
	}

	return nil
}

// checkOAuthCompleted checks if OAuth flow is already completed
func (a *BrowserOAuthAutomator) checkOAuthCompleted(page *rod.Page, state string) (string, bool) {
	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Current page URL")

	// Check if we're already at the callback URL
	if strings.Contains(currentURL, a.config.CallbackURLPattern) || strings.Contains(currentURL, "code=") {
		a.logger.Info().Msg("OAuth flow completed - already at callback URL")

		// Extract authorization code from current URL
		parsedURL, err := url.Parse(currentURL)
		if err != nil {
			a.logger.Error().Err(err).Msg("Failed to parse callback URL")
			return "", false
		}

		authCode := parsedURL.Query().Get("code")
		if authCode == "" {
			a.logger.Error().Str("url", currentURL).Msg("No authorization code in callback URL")
			return "", false
		}

		// Verify state parameter matches
		callbackState := parsedURL.Query().Get("state")
		if callbackState != state {
			a.logger.Error().Str("expected", state).Str("got", callbackState).Msg("State mismatch")
			return "", false
		}

		a.logger.Info().
			Str("auth_code", authCode[:min(len(authCode), 10)]+"...").
			Str("state", callbackState).
			Msg("Successfully extracted authorization code from callback URL")

		return authCode, true
	}

	return "", false
}

// performAuthorization handles clicking the authorization button
func (a *BrowserOAuthAutomator) performAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Looking for authorization button")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_authorization_page")

	// Find authorization button with smart text-based detection
	authButtonElement, err := a.findAuthorizationButton(page)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_button_not_found")
		currentURL := page.MustInfo().URL
		return fmt.Errorf("could not find authorization button on page %s: %w", currentURL, err)
	}

	a.logger.Info().Msg("Found authorization button")

	// Click the authorize button
	a.logger.Info().Msg("Clicking authorize button")
	err = authButtonElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click authorize button: %w", err)
	}

	return nil
}

// findAuthorizationButton finds the correct authorization button by examining text content
func (a *BrowserOAuthAutomator) findAuthorizationButton(page *rod.Page) (*rod.Element, error) {
	a.logger.Info().Str("configured_selector", a.config.AuthorizeButtonSelector).Msg("Starting authorization button search")

	// Set timeout for operations - increased for Microsoft enterprise auth
	page = page.Timeout(60 * time.Second)

	// First try the configured selector
	authButtonElement, err := page.Element(a.config.AuthorizeButtonSelector)
	if err == nil {
		// Check if the button text indicates it's an authorization button
		buttonText, textErr := authButtonElement.Text()
		if textErr == nil {
			buttonTextLower := strings.ToLower(buttonText)
			a.logger.Info().Str("button_text", buttonText).Msg("Found element with configured selector")
			if strings.Contains(buttonTextLower, "authorize") && !strings.Contains(buttonTextLower, "cancel") && !strings.Contains(buttonTextLower, "deny") {
				a.logger.Info().Str("button_text", buttonText).Msg("Found authorization button with expected text")
				return authButtonElement, nil
			}
		} else {
			a.logger.Info().Err(textErr).Msg("Could not get text from configured selector element")
		}
	} else {
		a.logger.Info().Err(err).Msg("Configured selector did not find element")
	}

	// If the configured selector didn't work or found wrong button, try broader search
	a.logger.Info().Msg("Configured selector didn't find suitable button, trying broader search")

	// Look for all buttons and inputs that might be authorization buttons
	buttonSelectors := []string{
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button`,
		`input[type="button"]`,
	}

	for _, selector := range buttonSelectors {
		a.logger.Info().Str("selector", selector).Msg("Searching for buttons with selector")
		elements, err := page.Timeout(30 * time.Second).Elements(selector)
		if err != nil {
			a.logger.Info().Err(err).Str("selector", selector).Msg("Error finding elements with selector")
			continue
		}

		a.logger.Info().Str("selector", selector).Int("count", len(elements)).Msg("Found elements with selector")

		for i, element := range elements {
			// Get button text/value
			buttonText := ""

			// Try getting text content first
			if text, err := element.Text(); err == nil && text != "" {
				buttonText = text
			} else if value, err := element.Attribute("value"); err == nil && value != nil {
				buttonText = *value
			} else if innerHTML, err := element.Property("innerHTML"); err == nil && innerHTML.String() != "" {
				buttonText = innerHTML.String()
			}

			if buttonText != "" {
				buttonTextLower := strings.ToLower(buttonText)
				a.logger.Info().
					Str("button_text", buttonText).
					Str("selector", selector).
					Int("element_index", i).
					Msg("Examining button")

				// Look for authorize-like text while avoiding cancel/deny text
				isAuthorizeButton := (strings.Contains(buttonTextLower, "authorize") ||
					strings.Contains(buttonTextLower, "allow") ||
					strings.Contains(buttonTextLower, "approve") ||
					strings.Contains(buttonTextLower, "grant")) &&
					!strings.Contains(buttonTextLower, "cancel") &&
					!strings.Contains(buttonTextLower, "deny") &&
					!strings.Contains(buttonTextLower, "reject")

				if isAuthorizeButton {
					a.logger.Info().Str("button_text", buttonText).Msg("Found authorization button based on text content")
					return element, nil
				}
			} else {
				a.logger.Info().
					Str("selector", selector).
					Int("element_index", i).
					Msg("Button has no text content")
			}
		}
	}

	a.logger.Error().Msg("No suitable authorization button found after searching all selectors")
	return nil, fmt.Errorf("no suitable authorization button found")
}

// waitForCallback waits for redirect to callback URL with authorization code
func (a *BrowserOAuthAutomator) waitForCallback(page *rod.Page, state string, screenshotTaker ScreenshotTaker) (string, error) {
	a.logger.Info().
		Str("callback_pattern", a.config.CallbackURLPattern).
		Str("expected_state", state).
		Msg("Waiting for OAuth callback redirect")

	// Wait for navigation to callback URL (with authorization code)
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	checkCount := 0
	for {
		select {
		case <-timeout:
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_timeout")
			currentURL := page.MustInfo().URL
			a.logger.Error().
				Str("current_url", currentURL).
				Str("callback_pattern", a.config.CallbackURLPattern).
				Int("check_count", checkCount).
				Msg("Timeout waiting for OAuth callback")
			return "", fmt.Errorf("timeout waiting for OAuth callback, current URL: %s", currentURL)
		case <-ticker.C:
			checkCount++
			currentURL := page.MustInfo().URL

			// Log progress every 4 seconds
			if checkCount%8 == 0 {
				a.logger.Info().
					Str("current_url", currentURL).
					Int("check_count", checkCount).
					Msg("Still waiting for OAuth callback")
			}

			if strings.Contains(currentURL, a.config.CallbackURLPattern) || strings.Contains(currentURL, "code=") {
				a.logger.Info().Str("callback_url", currentURL).Msg("OAuth callback received")
				screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_callback_received")

				// Extract authorization code from callback URL
				parsedURL, err := url.Parse(currentURL)
				if err != nil {
					return "", fmt.Errorf("failed to parse callback URL: %w", err)
				}

				authCode := parsedURL.Query().Get("code")
				if authCode == "" {
					return "", fmt.Errorf("no authorization code in callback URL: %s", currentURL)
				}

				// Verify state parameter matches
				callbackState := parsedURL.Query().Get("state")
				if callbackState != state {
					return "", fmt.Errorf("state mismatch: expected %s, got %s", state, callbackState)
				}

				a.logger.Info().
					Str("auth_code", authCode[:min(len(authCode), 10)]+"...").
					Str("state", callbackState).
					Msg("Successfully extracted authorization code from callback")

				return authCode, nil
			}
		}
	}
}
