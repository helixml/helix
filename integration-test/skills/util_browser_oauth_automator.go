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
	ProviderStrategy        ProviderStrategy // New: Provider-specific automation strategy
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

// ProviderStrategy interface for provider-specific browser automation logic
type ProviderStrategy interface {
	// ClickNextButton implements provider-specific logic for clicking Next/Continue buttons
	ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error

	// HandleLoginFlow implements provider-specific login flow (single-step vs two-step)
	HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error

	// HandleAuthorization implements provider-specific authorization page handling
	HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error
}

// DefaultProviderStrategy provides generic OAuth automation behavior
type DefaultProviderStrategy struct {
	config BrowserOAuthConfig
	logger zerolog.Logger
}

// NewDefaultProviderStrategy creates a default provider strategy
func NewDefaultProviderStrategy(config BrowserOAuthConfig, logger zerolog.Logger) *DefaultProviderStrategy {
	return &DefaultProviderStrategy{
		config: config,
		logger: logger,
	}
}

// ClickNextButton implements generic Next button clicking logic
func (s *DefaultProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Next button")

	// Use a temporary page context to avoid affecting the main page timeout
	nextPage := page.Timeout(10 * time.Second)

	// First try direct CSS selectors
	nextSelectors := []string{
		`input[id="idSIButton9"]`,            // Microsoft-specific Next button ID
		`input[type="submit"][value="Next"]`, // Input style (Microsoft/general)
		`button[id="identifierNext"]`,        // Old Google style (with ID)
		`button[type="submit"]`,              // Generic submit button
		`input[type="submit"]`,               // Generic submit input
	}

	var nextButton *rod.Element
	for _, selector := range nextSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Next button selector")
		element, err := nextPage.Timeout(3 * time.Second).Element(selector)
		if err == nil && element != nil {
			// For submit buttons, check if they contain "Next" text or value
			if strings.Contains(selector, "submit") && !strings.Contains(selector, "idSIButton9") {
				var buttonText string
				if text, textErr := element.Text(); textErr == nil && text != "" {
					buttonText = text
				} else if value, valueErr := element.Attribute("value"); valueErr == nil && value != nil {
					buttonText = *value
				}

				if buttonText != "" {
					buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
					if buttonTextLower == "next" || buttonTextLower == "continue" {
						s.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found Next button by selector and text match")
						nextButton = element
						break
					}
				}
			} else {
				s.logger.Info().Str("selector", selector).Msg("Found Next button by direct selector")
				nextButton = element
				break
			}
		}
	}

	if nextButton == nil {
		return fmt.Errorf("failed to find Next button")
	}

	// Set timeout for element operations
	nextButton = nextButton.Timeout(5 * time.Second)

	err := nextButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Next button: %w", err)
	}

	s.logger.Info().Msg("Successfully clicked Next button")
	screenshotTaker.TakeScreenshot(page, s.config.ProviderName+"_next_button_clicked")
	return nil
}

// HandleLoginFlow implements generic login flow (detects single vs two-step automatically)
func (s *DefaultProviderStrategy) HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Starting generic login flow")

	// Detect provider type and handle appropriate login flow
	if s.config.ProviderName == "google" || s.config.ProviderName == "microsoft" {
		// Google and Microsoft: Two-step process (email → Next → password → Next)
		s.logger.Info().Str("provider", s.config.ProviderName).Msg("Using two-step login flow")

		// Step 1: Handle email input (first step for Google/Microsoft)
		err := s.handleEmailInput(page, username, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle email input: %w", err)
		}

		// Step 2: Handle password input (second step for Google/Microsoft)
		err = s.handlePasswordInput(page, password, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle password input: %w", err)
		}
	} else {
		// All other providers: Single-step process (username + password → Sign in)
		s.logger.Info().Str("provider", s.config.ProviderName).Msg("Using single-step login flow")

		err := s.handleSingleStepLogin(page, username, password, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle single-step login: %w", err)
		}
	}

	return nil
}

// handleEmailInput handles the first step of two-step login (email input)
func (s *DefaultProviderStrategy) handleEmailInput(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling email input (first step)")

	// Find and fill email field
	emailField, err := page.Element(s.config.LoginUsernameSelector)
	if err != nil {
		return fmt.Errorf("could not find email field: %w", err)
	}

	err = emailField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click email field: %w", err)
	}

	err = emailField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in email field: %w", err)
	}

	err = emailField.Input(username)
	if err != nil {
		return fmt.Errorf("could not input email: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, s.config.ProviderName+"_email_entered")

	// Click Next button to proceed to password step
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Next button after email: %w", err)
	}

	// Wait for password page to load
	time.Sleep(2 * time.Second)
	screenshotTaker.TakeScreenshot(page, s.config.ProviderName+"_password_page")

	return nil
}

// handlePasswordInput handles the second step of two-step login (password input)
func (s *DefaultProviderStrategy) handlePasswordInput(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling password input (second step)")

	// Find and fill password field
	passwordField, err := page.Element(s.config.LoginPasswordSelector)
	if err != nil {
		return fmt.Errorf("could not find password field: %w", err)
	}

	err = passwordField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click password field: %w", err)
	}

	err = passwordField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in password field: %w", err)
	}

	err = passwordField.Input(password)
	if err != nil {
		return fmt.Errorf("could not input password: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, s.config.ProviderName+"_password_entered")

	// Click Next/Sign in button to proceed
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Next button after password: %w", err)
	}

	return nil
}

// handleSingleStepLogin handles single-step login (username + password in one step)
func (s *DefaultProviderStrategy) handleSingleStepLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling single-step login")

	// Fill username field
	usernameField, err := page.Element(s.config.LoginUsernameSelector)
	if err != nil {
		return fmt.Errorf("could not find username field: %w", err)
	}

	err = usernameField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click username field: %w", err)
	}

	err = usernameField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in username field: %w", err)
	}

	err = usernameField.Input(username)
	if err != nil {
		return fmt.Errorf("could not input username: %w", err)
	}

	// Fill password field
	passwordField, err := page.Element(s.config.LoginPasswordSelector)
	if err != nil {
		return fmt.Errorf("could not find password field: %w", err)
	}

	err = passwordField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click password field: %w", err)
	}

	err = passwordField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in password field: %w", err)
	}

	err = passwordField.Input(password)
	if err != nil {
		return fmt.Errorf("could not input password: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, s.config.ProviderName+"_credentials_entered")

	// Click login button
	loginButton, err := page.Element(s.config.LoginButtonSelector)
	if err != nil {
		return fmt.Errorf("could not find login button: %w", err)
	}

	err = loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click login button: %w", err)
	}

	return nil
}

// HandleAuthorization implements generic authorization handling
func (s *DefaultProviderStrategy) HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling authorization with generic strategy")

	// Find authorization button using configured selector
	authButtonElement, err := page.Element(s.config.AuthorizeButtonSelector)
	if err != nil {
		return fmt.Errorf("failed to find authorization button using selector %s: %w", s.config.AuthorizeButtonSelector, err)
	}

	s.logger.Info().Msg("Found authorization button")

	// Click the authorize button
	s.logger.Info().Msg("Clicking authorize button")
	err = authButtonElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click authorize button: %w", err)
	}

	// Take screenshot after successful authorization
	screenshotTaker.TakeScreenshot(page, "authorization_button_clicked")

	return nil
}

// NewBrowserOAuthAutomator creates a new browser OAuth automator
func NewBrowserOAuthAutomator(browser *rod.Browser, logger zerolog.Logger, config BrowserOAuthConfig) *BrowserOAuthAutomator {
	// If no strategy is provided, use the default strategy
	if config.ProviderStrategy == nil {
		config.ProviderStrategy = NewDefaultProviderStrategy(config, logger)
	}

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

	// Set longer overall timeout for page operations - increased for Atlassian OAuth reliability
	// Google OAuth now includes account selection which adds significant processing time
	page = page.Timeout(180 * time.Second)

	// Navigate to OAuth authorization URL
	a.logger.Info().Str("url", authURL).Msg("Navigating to OAuth authorization URL")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_before_navigation")

	err = page.Navigate(authURL)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_navigation_failed")
		return "", fmt.Errorf("failed to navigate to OAuth URL: %w", err)
	}

	// Wait for page to load with longer timeout - increased from 15 seconds
	// Use a temporary page context to avoid affecting the main page timeout
	a.logger.Info().Msg("Waiting for page to load")
	pageLoadPage := page.Timeout(30 * time.Second)
	err = pageLoadPage.WaitLoad()
	if err != nil {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_page_load_failed")
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Page loaded successfully")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_loaded")

	// Check if we need to login
	a.logger.Info().Msg("Checking if login is required")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_before_login_check")

	loginRequired, err := a.checkLoginRequired(page)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_check_failed")
		return "", fmt.Errorf("failed to check login requirement: %w", err)
	}

	a.logger.Info().Bool("login_required", loginRequired).Msg("Login requirement check completed")

	if loginRequired {
		a.logger.Info().Msg("Login required - starting login process")
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_required")

		err = a.performLogin(page, username, password, screenshotTaker)
		if err != nil {
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_failed")
			return "", fmt.Errorf("failed to perform login: %w", err)
		}

		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_completed")
	} else {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_not_required")
	}

	// Check if we need to navigate back to OAuth after login/2FA
	a.logger.Info().Msg("Checking if navigation back to OAuth is needed")
	err = a.navigateBackToOAuthIfNeeded(page, authURL, screenshotTaker)
	if err != nil {
		return "", fmt.Errorf("failed to navigate back to OAuth: %w", err)
	}

	// Check if already at callback (OAuth completed)
	a.logger.Info().Msg("Checking if OAuth flow is already completed")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_before_oauth_completion_check")

	authCode, completed := a.checkOAuthCompleted(page, state)
	if completed {
		a.logger.Info().Msg("OAuth flow already completed - returning authorization code")
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_already_completed")
		return authCode, nil
	}

	// Look for and click authorization button
	a.logger.Info().Msg("Starting authorization button search and click")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_before_authorization")

	err = a.performAuthorization(page, screenshotTaker)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_authorization_failed")
		return "", fmt.Errorf("failed to perform authorization: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_authorization_completed")

	// Wait for callback with authorization code
	a.logger.Info().Msg("Starting callback wait process")
	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_before_callback_wait")

	return a.waitForCallback(page, state, screenshotTaker)
}

// checkLoginRequired checks if login is required
func (a *BrowserOAuthAutomator) checkLoginRequired(page *rod.Page) (bool, error) {
	a.logger.Info().
		Str("username_selector", a.config.LoginUsernameSelector).
		Msg("Checking if login is required")

	// Wait a moment for the page to fully load
	a.logger.Info().Msg("Waiting 2 seconds for page to fully load")
	time.Sleep(2 * time.Second)

	// Check if we need to login first
	// Use a temporary page context to avoid affecting the main page timeout
	loginCheckPage := page.Timeout(20 * time.Second)
	loginElement, err := loginCheckPage.Element(a.config.LoginUsernameSelector)
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

	// Use a temporary page context to avoid affecting the main page timeout
	debugPage := page.Timeout(20 * time.Second)

	// Get page HTML with timeout
	html, err := debugPage.HTML()
	if err != nil {
		a.logger.Error().Err(err).Msg("Failed to get page HTML")
		return
	}

	// Log current URL
	currentURL := debugPage.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Msg("Current page URL")

	// Find all input elements with timeout
	inputs, err := debugPage.Elements("input")
	if err == nil {
		a.logger.Info().Int("count", len(inputs)).Msg("Found input elements")
		for i, input := range inputs {
			if i >= 10 { // Limit to first 10 elements
				break
			}

			// Add longer timeout to attribute operations - increased from 2 seconds
			input = input.Timeout(5 * time.Second)

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
	buttons, err := debugPage.Elements("button")
	if err == nil {
		a.logger.Info().Int("count", len(buttons)).Msg("Found button elements")
		for i, button := range buttons {
			if i >= 10 { // Limit to first 10 elements
				break
			}

			// Add longer timeout to attribute operations - increased from 2 seconds
			button = button.Timeout(5 * time.Second)

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

	// Use the provider strategy to handle the login flow
	err := a.config.ProviderStrategy.HandleLoginFlow(page, username, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle login flow: %w", err)
	}

	// Check for MFA immediately after login completes
	// This is crucial for providers like Atlassian that redirect to MFA immediately after login
	if a.config.TwoFactorHandler != nil && a.config.TwoFactorHandler.IsRequired(page) {
		a.logger.Info().Msg("Two-factor authentication required after login - handling MFA")
		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_2fa_required_after_login")

		err = a.config.TwoFactorHandler.Handle(page, a)
		if err != nil {
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_2fa_failed_after_login")
			return fmt.Errorf("failed to handle 2FA after login: %w", err)
		}

		screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_2fa_completed_after_login")
	}

	return nil
}

// navigateBackToOAuthIfNeeded checks if we need to navigate back to OAuth after login/2FA
func (a *BrowserOAuthAutomator) navigateBackToOAuthIfNeeded(page *rod.Page, authURL string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Checking if we need to navigate back to OAuth")

	currentURL := page.MustInfo().URL
	a.logger.Info().Str("current_url", currentURL).Str("auth_url", authURL).Msg("Comparing URLs")

	// Check if we need to navigate back to OAuth flow
	if !strings.Contains(currentURL, a.config.CallbackURLPattern) && !strings.Contains(currentURL, "code=") {
		// If we're not at callback and not already at OAuth page, navigate back
		if !strings.Contains(currentURL, "oauth") && !strings.Contains(currentURL, "authorize") {
			a.logger.Info().Str("auth_url", authURL).Msg("Navigating back to OAuth after login")
			err := page.Navigate(authURL)
			if err != nil {
				return fmt.Errorf("failed to navigate back to OAuth URL: %w", err)
			}
			err = page.WaitLoad()
			if err != nil {
				return fmt.Errorf("failed to wait for OAuth page to load: %w", err)
			}
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_back_to_oauth")
		}
	}

	return nil
}

// checkOAuthCompleted checks if the OAuth flow is already completed
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
	a.logger.Info().Msg("Authorization required - delegating to provider strategy")

	// Use the provider strategy to handle the authorization
	return a.config.ProviderStrategy.HandleAuthorization(page, screenshotTaker)
}

// waitForCallback waits for the OAuth callback and extracts the authorization code
func (a *BrowserOAuthAutomator) waitForCallback(page *rod.Page, state string, screenshotTaker ScreenshotTaker) (string, error) {
	a.logger.Info().Msg("Waiting for OAuth callback with authorization code")

	// Set up a timeout and ticker for polling
	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	checkCount := 0

	for {
		select {
		case <-timeout:
			a.logger.Error().Msg("Timeout waiting for OAuth callback")
			// Take final screenshot for debugging
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_timeout")
			return "", fmt.Errorf("timeout waiting for OAuth callback after 2 minutes")

		case <-ticker.C:
			checkCount++
			currentURL := page.MustInfo().URL

			// Log progress every 4 seconds
			if checkCount%8 == 0 {
				a.logger.Info().
					Str("current_url", currentURL).
					Int("check_count", checkCount).
					Msg("Still waiting for OAuth callback")
				// Take periodic screenshots to show progress
				screenshotTaker.TakeScreenshot(page, fmt.Sprintf("%s_callback_wait_%d", a.config.ProviderName, checkCount))
			}

			if strings.Contains(currentURL, a.config.CallbackURLPattern) || strings.Contains(currentURL, "code=") {
				a.logger.Info().Str("callback_url", currentURL).Msg("OAuth callback received")
				screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_callback_received")

				// Extract authorization code from callback URL
				parsedURL, err := url.Parse(currentURL)
				if err != nil {
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_callback_url_parse_failed")
					return "", fmt.Errorf("failed to parse callback URL: %w", err)
				}

				authCode := parsedURL.Query().Get("code")
				if authCode == "" {
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_no_auth_code_in_callback")
					return "", fmt.Errorf("no authorization code in callback URL: %s", currentURL)
				}

				// Verify state parameter matches
				callbackState := parsedURL.Query().Get("state")
				if callbackState != state {
					screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_state_mismatch")
					return "", fmt.Errorf("state mismatch: expected %s, got %s", state, callbackState)
				}

				a.logger.Info().
					Str("auth_code", authCode[:min(len(authCode), 10)]+"...").
					Str("state", callbackState).
					Msg("Successfully extracted authorization code from callback")

				// Take final success screenshot
				screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_success")
				return authCode, nil
			}
		}
	}
}
