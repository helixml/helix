package skills

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"math/rand"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
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

// randomDelay creates a random delay between min and max milliseconds
func (h *HumanLikeInteractor) randomDelay(minMs, maxMs int) time.Duration {
	if minMs >= maxMs {
		return time.Duration(minMs) * time.Millisecond
	}
	randomMs := minMs + int(time.Now().UnixNano()%int64(maxMs-minMs))
	return time.Duration(randomMs) * time.Millisecond
}

// moveMouseToElement simulates human-like mouse movement to an element
func (h *HumanLikeInteractor) moveMouseToElement(page *rod.Page, element *rod.Element) error {
	h.logger.Info().Msg("Moving mouse to element with human-like movement")

	// Get element shape for positioning
	shape, err := element.Shape()
	if err != nil {
		return fmt.Errorf("failed to get element shape: %w", err)
	}

	// Get the first quad from the shape
	if len(shape.Quads) == 0 {
		return fmt.Errorf("element has no shape quads")
	}

	// Calculate center position from the first quad
	quad := shape.Quads[0]
	centerX := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	centerY := (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	// Add small random offset to make it more human-like
	offsetX := centerX + float64((time.Now().UnixNano()%11)-5) // Random offset -5 to +5
	offsetY := centerY + float64((time.Now().UnixNano()%11)-5)

	// Move mouse to element with human-like curve
	return page.Mouse.MoveTo(proto.Point{X: offsetX, Y: offsetY})
}

// typeStringHumanLike types a string character by character with human-like timing
func (h *HumanLikeInteractor) typeStringHumanLike(page *rod.Page, text string) error {
	h.logger.Info().Str("text_length", fmt.Sprintf("%d", len(text))).Msg("Typing string with human-like timing")

	for i, char := range text {
		// Random delay between keystrokes (50-150ms)
		delay := h.randomDelay(50, 150)
		if i > 0 {
			time.Sleep(delay)
		}

		// Type individual character
		err := page.Keyboard.Press(input.Key(char))
		if err != nil {
			return fmt.Errorf("failed to type character at position %d: %w", i, err)
		}
	}

	return nil
}

// waitForFieldReadiness waits for a field to be ready for interaction
func (h *HumanLikeInteractor) waitForFieldReadiness(element *rod.Element, fieldType string) error {
	h.logger.Info().Str("field_type", fieldType).Msg("Waiting for field to be ready for interaction")

	// Wait for element to be visible
	for i := 0; i < 10; i++ {
		visible, err := element.Visible()
		if err != nil {
			return fmt.Errorf("failed to check visibility: %w", err)
		}
		if visible {
			break
		}
		time.Sleep(h.randomDelay(200, 400))
	}

	// Wait for element to be interactable (simplified check)
	// Skip interactable check as it's causing API issues
	time.Sleep(h.randomDelay(500, 1000)) // Just wait a bit longer instead

	// Additional wait for field stabilization
	time.Sleep(h.randomDelay(300, 600))

	return nil
}

// simulateHumanFieldInteraction simulates human-like field interaction
func (h *HumanLikeInteractor) simulateHumanFieldInteraction(page *rod.Page, element *rod.Element, value string, fieldType string) error {
	h.logger.Info().Str("field_type", fieldType).Msg("Starting human-like field interaction")

	// Step 1: Wait for field readiness
	err := h.waitForFieldReadiness(element, fieldType)
	if err != nil {
		return fmt.Errorf("field not ready: %w", err)
	}

	// Step 2: Move mouse to element (human-like)
	err = h.moveMouseToElement(page, element)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to move mouse to element, continuing")
	}

	// Step 3: Random delay before clicking
	time.Sleep(h.randomDelay(100, 300))

	// Step 4: Click to focus
	err = element.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to click element, trying focus instead")
		err = element.Focus()
		if err != nil {
			return fmt.Errorf("failed to focus element: %w", err)
		}
	}

	// Step 5: Wait for focus to take effect
	time.Sleep(h.randomDelay(100, 250))

	// Step 6: Clear existing content (human-like)
	err = h.clearFieldHumanLike(page, element)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to clear field, continuing")
	}

	// Step 7: Wait before typing
	time.Sleep(h.randomDelay(150, 300))

	// Step 8: Type value character by character
	err = h.typeStringHumanLike(page, value)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Human-like typing failed, trying fallback input")

		// Fallback: Use Rod's input method
		err = element.Input(value)
		if err != nil {
			return fmt.Errorf("all input methods failed: %w", err)
		}
	}

	// Step 9: Wait after typing
	time.Sleep(h.randomDelay(100, 250))

	// Step 10: Simulate field blur to trigger validation
	err = h.simulateFieldBlur(element)
	if err != nil {
		h.logger.Warn().Err(err).Msg("Failed to simulate field blur")
	}

	return nil
}

// clearFieldHumanLike clears a field in a human-like way
func (h *HumanLikeInteractor) clearFieldHumanLike(page *rod.Page, element *rod.Element) error {
	h.logger.Info().Msg("Clearing field with human-like interaction")

	// Method 1: Select all and delete
	err := page.Keyboard.Press(input.ControlLeft)
	if err == nil {
		time.Sleep(h.randomDelay(50, 100))
		err = page.Keyboard.Press(input.KeyA)
		if err == nil {
			time.Sleep(h.randomDelay(50, 100))
			err = page.Keyboard.Press(input.Delete)
			if err == nil {
				return nil
			}
		}
	}

	// Method 2: Use Rod's select all
	err = element.SelectAllText()
	if err == nil {
		time.Sleep(h.randomDelay(50, 100))
		err = element.Input("")
		if err == nil {
			return nil
		}
	}

	// Method 3: JavaScript clear
	_, err = element.Eval(`(element) => {
		element.value = '';
		element.dispatchEvent(new Event('input', { bubbles: true }));
		element.dispatchEvent(new Event('change', { bubbles: true }));
	}`)

	return err
}

// simulateFieldBlur simulates field blur to trigger validation
func (h *HumanLikeInteractor) simulateFieldBlur(element *rod.Element) error {
	// Simulate blur event
	_, err := element.Eval(`(element) => {
		element.dispatchEvent(new Event('blur', { bubbles: true }));
		element.dispatchEvent(new Event('focusout', { bubbles: true }));
	}`)
	return err
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
			input = input.Timeout(20 * time.Second) // Increased from 2s to 20s for stealth mode

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
			button = button.Timeout(30 * time.Second) // Increased from 2s to 30s for button operations

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
	if a.config.ProviderName == "google" || a.config.ProviderName == "microsoft" || a.config.ProviderName == "atlassian" {
		// Google, Microsoft, and Atlassian: Two-step process (email → Continue/Next → password → Login)
		a.logger.Info().Str("provider", a.config.ProviderName).Msg("Using two-step login flow")

		// Step 1: Handle email input (first step for Google/Microsoft/Atlassian)
		err := a.handleEmailInput(page, username, screenshotTaker)
		if err != nil {
			return fmt.Errorf("failed to handle email input: %w", err)
		}

		// Step 2: Handle password input (second step for Google/Microsoft/Atlassian)
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
	usernameElement = usernameElement.Timeout(15 * time.Second)

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

// fillPasswordField fills the password field using stealth mode (shared by both login flows)
func (a *BrowserOAuthAutomator) fillPasswordField(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Filling password field with stealth mode")

	// Take screenshot before starting password interaction
	screenshotTaker.TakeScreenshot(page, "before_password_interaction")

	// Find password field with extended timeout
	passwordField, err := page.Timeout(30 * time.Second).Element(a.config.LoginPasswordSelector)
	if err != nil {
		return fmt.Errorf("failed to find password field: %w", err)
	}

	// Implement stealth mode automation for password field
	err = a.stealthPasswordInput(page, passwordField, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to enter password with stealth methods: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, "after_password_interaction")
	a.logger.Info().Msg("Successfully filled password field with stealth mode")
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
		elements, err := page.Timeout(30 * time.Second).Elements(selector) // Increased from 3s to 30s
		if err == nil && len(elements) > 0 {
			// Check each element to find the one with "Next" text
			for _, element := range elements {
				buttonText, textErr := element.Timeout(15 * time.Second).Text() // Increased from 1s to 15s
				if textErr == nil && (strings.ToLower(strings.TrimSpace(buttonText)) == "next" || strings.ToLower(strings.TrimSpace(buttonText)) == "continue") {
					a.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found Next/Continue button using class pattern")
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

// handlePasswordInput handles the password input step with advanced anti-detection
func (a *BrowserOAuthAutomator) handlePasswordInput(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Attempting to interact with password field using multiple approaches")

	// Take screenshot before starting password interaction
	screenshotTaker.TakeScreenshot(page, "before_password_interaction")

	// Find password field with extended timeout
	passwordField, err := page.Timeout(30 * time.Second).Element(a.config.LoginPasswordSelector)
	if err != nil {
		return fmt.Errorf("failed to find password field: %w", err)
	}

	// Implement stealth mode automation for password field
	err = a.stealthPasswordInput(page, passwordField, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to enter password with stealth methods: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, "after_password_interaction")

	// Click login/continue button after password entry
	a.logger.Info().Msg("Password entered successfully, clicking login button")
	err = a.clickLoginButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	// Wait for navigation after login
	return a.waitForNavigation(page, screenshotTaker)
}

// stealthPasswordInput implements stealth mode password entry with maximum anti-detection
func (a *BrowserOAuthAutomator) stealthPasswordInput(page *rod.Page, passwordField *rod.Element, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Implementing stealth mode password input with anti-detection")

	// Step 1: Wait for natural user behavior timing
	time.Sleep(time.Duration(2000+rand.Intn(3000)) * time.Millisecond) // 2-5 seconds

	// Step 2: Disable automation detection flags
	err := a.disableAutomationDetection(page)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to disable automation detection")
	}

	// Step 3: Simulate realistic mouse movement patterns
	err = a.simulateRealisticMouseMovement(page, passwordField)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to simulate realistic mouse movement")
	}

	// Step 4: Focus the password field (simpler than stealth click for input fields)
	a.logger.Info().Msg("Focusing password field with realistic behavior")

	// Create a context with timeout for focus operations
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try multiple focus approaches
	focusCtx, focusCancel := context.WithTimeout(ctx, 15*time.Second)
	err = passwordField.Context(focusCtx).Focus()
	focusCancel()
	if err != nil {
		a.logger.Warn().Err(err).Msg("Simple focus failed, trying click")
		// Fallback to single click if focus fails
		clickCtx, clickCancel := context.WithTimeout(ctx, 15*time.Second)
		err = passwordField.Context(clickCtx).Click(proto.InputMouseButtonLeft, 1)
		clickCancel()
		if err != nil {
			a.logger.Warn().Err(err).Msg("Click focus failed, using JavaScript focus")
			// Final fallback to JavaScript focus
			jsCtx, jsCancel := context.WithTimeout(ctx, 15*time.Second)
			_, err = passwordField.Context(jsCtx).Eval(`(element) => {
				element.focus();
				element.click();
			}`)
			jsCancel()
			if err != nil {
				a.logger.Error().Err(err).Msg("All focus methods failed")
			}
		}
	}

	// Step 5: Wait for field to become active (human-like pause)
	time.Sleep(time.Duration(1000+rand.Intn(2000)) * time.Millisecond) // 1-3 seconds

	// Step 6: Clear field with realistic key combinations
	err = a.clearFieldRealistic(page, passwordField)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to clear field realistically")
	}

	// Step 7: Type password with realistic human timing
	err = a.typePasswordRealistic(page, passwordField, password)
	if err != nil {
		return fmt.Errorf("all password input methods failed: %w", err)
	}

	// Step 8: Simulate field validation behavior
	err = a.simulateFieldValidation(page, passwordField)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Failed to simulate field validation")
	}

	return nil
}

// disableAutomationDetection removes automation detection flags
func (a *BrowserOAuthAutomator) disableAutomationDetection(page *rod.Page) error {
	a.logger.Info().Msg("Disabling automation detection flags")

	// Remove webdriver flags and automation indicators
	_, err := page.Eval(`() => {
		// Remove webdriver property
		delete navigator.webdriver;
		
		// Override automation detection
		Object.defineProperty(navigator, 'webdriver', {
			get: () => false,
		});
		
		// Remove automation user agent indicators
		Object.defineProperty(navigator, 'userAgent', {
			get: () => navigator.userAgent.replace(/HeadlessChrome|Chrome.*--headless/g, 'Chrome'),
		});
		
		// Override plugins to appear like real browser
		Object.defineProperty(navigator, 'plugins', {
			get: () => [1, 2, 3, 4, 5],
		});
		
		// Override languages
		Object.defineProperty(navigator, 'languages', {
			get: () => ['en-US', 'en'],
		});
		
		// Override platform
		Object.defineProperty(navigator, 'platform', {
			get: () => 'Linux x86_64',
		});
		
		// Remove Chrome automation extensions
		if (window.chrome && window.chrome.runtime) {
			window.chrome.runtime.onConnect = undefined;
			window.chrome.runtime.onMessage = undefined;
		}
		
		// Override permission API
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
		);
		
		return 'automation detection disabled';
	}`)

	return err
}

// simulateRealisticMouseMovement simulates human-like mouse movement
func (a *BrowserOAuthAutomator) simulateRealisticMouseMovement(page *rod.Page, element *rod.Element) error {
	a.logger.Info().Msg("Simulating realistic mouse movement patterns")

	// Create a context with a generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Get element position with context
	shapeCtx, shapeCancel := context.WithTimeout(ctx, 30*time.Second)
	shape, err := element.Context(shapeCtx).Shape()
	shapeCancel()
	if err != nil {
		return fmt.Errorf("failed to get element shape: %w", err)
	}

	if len(shape.Quads) == 0 {
		return fmt.Errorf("element has no shape quads")
	}

	// Calculate target position
	quad := shape.Quads[0]
	targetX := (quad[0] + quad[2] + quad[4] + quad[6]) / 4
	targetY := (quad[1] + quad[3] + quad[5] + quad[7]) / 4

	// Simulate realistic mouse movement with bezier curve
	currentX, currentY := 100.0, 100.0 // Starting position

	// Create multiple waypoints for realistic movement
	waypoints := []struct{ x, y float64 }{
		{currentX + (targetX-currentX)*0.3, currentY + (targetY-currentY)*0.2},
		{currentX + (targetX-currentX)*0.7, currentY + (targetY-currentY)*0.8},
		{targetX + float64(rand.Intn(5)-2), targetY + float64(rand.Intn(5)-2)}, // Small random offset
	}

	// Move through waypoints with realistic timing
	for _, point := range waypoints {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("mouse movement timeout: %w", ctx.Err())
		default:
		}

		err = page.Mouse.MoveTo(proto.Point{X: point.x, Y: point.y})
		if err != nil {
			return fmt.Errorf("failed to move mouse to waypoint: %w", err)
		}
		time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond) // 50-150ms
	}

	return nil
}

// performStealthClick performs realistic click with multiple attempts
func (a *BrowserOAuthAutomator) performStealthClick(page *rod.Page, element *rod.Element) error {
	a.logger.Info().Msg("Performing stealth click with realistic behavior")

	// Create a context with a generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Try multiple click approaches
	for attempt := 0; attempt < 3; attempt++ {
		a.logger.Info().Int("attempt", attempt+1).Msg("Attempting stealth click")

		// Add small delay between attempts
		if attempt > 0 {
			time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("stealth click timeout: %w", ctx.Err())
		default:
		}

		// Try direct click first
		clickCtx, clickCancel := context.WithTimeout(ctx, 30*time.Second)
		err := element.Context(clickCtx).Click(proto.InputMouseButtonLeft, 1)
		clickCancel()
		if err == nil {
			a.logger.Info().Msg("Direct click successful")
			return nil
		}

		// Try focus then click
		focusCtx, focusCancel := context.WithTimeout(ctx, 30*time.Second)
		err = element.Context(focusCtx).Focus()
		focusCancel()
		if err == nil {
			time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
			clickCtx2, clickCancel2 := context.WithTimeout(ctx, 30*time.Second)
			err = element.Context(clickCtx2).Click(proto.InputMouseButtonLeft, 1)
			clickCancel2()
			if err == nil {
				a.logger.Info().Msg("Focus then click successful")
				return nil
			}
		}

		// Try JavaScript click
		jsCtx, jsCancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = element.Context(jsCtx).Eval(`(element) => {
			element.focus();
			element.click();
			element.dispatchEvent(new MouseEvent('click', {
				bubbles: true,
				cancelable: true,
				view: window,
				button: 0,
				buttons: 1,
				clientX: element.getBoundingClientRect().left + element.getBoundingClientRect().width / 2,
				clientY: element.getBoundingClientRect().top + element.getBoundingClientRect().height / 2
			}));
		}`)
		jsCancel()
		if err == nil {
			a.logger.Info().Msg("JavaScript click successful")
			return nil
		}

		a.logger.Warn().Err(err).Int("attempt", attempt+1).Msg("Click attempt failed")
	}

	return fmt.Errorf("all click attempts failed")
}

// clearFieldRealistic clears field with realistic key combinations
func (a *BrowserOAuthAutomator) clearFieldRealistic(page *rod.Page, element *rod.Element) error {
	a.logger.Info().Msg("Clearing field with realistic key combinations")

	// Create a context with a generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Simulate realistic field clearing
	approaches := []func() error{
		// Method 1: Triple click to select all, then type
		func() error {
			clickCtx, clickCancel := context.WithTimeout(ctx, 30*time.Second)
			defer clickCancel()
			err := element.Context(clickCtx).Click(proto.InputMouseButtonLeft, 3) // Triple click
			if err != nil {
				return err
			}
			time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
			return nil
		},
		// Method 2: Ctrl+A then Delete
		func() error {
			err := page.Keyboard.Press(input.ControlLeft)
			if err != nil {
				return err
			}
			time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
			err = page.Keyboard.Press(input.KeyA)
			if err != nil {
				return err
			}
			time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
			err = page.Keyboard.Press(input.Delete)
			return err
		},
		// Method 3: JavaScript clear
		func() error {
			jsCtx, jsCancel := context.WithTimeout(ctx, 30*time.Second)
			defer jsCancel()
			_, err := element.Context(jsCtx).Eval(`(element) => {
				element.focus();
				element.select();
				element.value = '';
				element.dispatchEvent(new Event('input', { bubbles: true }));
				element.dispatchEvent(new Event('change', { bubbles: true }));
			}`)
			return err
		},
	}

	// Try each approach
	for i, approach := range approaches {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("clear field timeout: %w", ctx.Err())
		default:
		}

		err := approach()
		if err == nil {
			a.logger.Info().Int("method", i+1).Msg("Field cleared successfully")
			return nil
		}
		a.logger.Warn().Err(err).Int("method", i+1).Msg("Clear method failed")
	}

	return fmt.Errorf("all field clearing methods failed")
}

// typePasswordRealistic types password with realistic human behavior
func (a *BrowserOAuthAutomator) typePasswordRealistic(page *rod.Page, element *rod.Element, password string) error {
	a.logger.Info().Msg("Typing password with realistic human behavior")

	// Create a context with a generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Simulate realistic typing patterns
	for i, char := range password {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return fmt.Errorf("password typing timeout: %w", ctx.Err())
		default:
		}

		if i > 0 {
			// Realistic inter-character delay (human typing speed)
			delay := time.Duration(80+rand.Intn(120)) * time.Millisecond // 80-200ms
			time.Sleep(delay)
		}

		// Occasionally make "mistakes" to appear more human
		if rand.Float64() < 0.02 && i > 0 { // 2% chance of backspace
			err := page.Keyboard.Press(input.Backspace)
			if err == nil {
				time.Sleep(time.Duration(100+rand.Intn(200)) * time.Millisecond)
			}
		}

		// Type the character
		err := page.Keyboard.Type(input.Key(char))
		if err != nil {
			a.logger.Warn().Err(err).Int("position", i).Msg("Character typing failed, trying fallback")

			// Fallback: Use element input with context
			inputCtx, inputCancel := context.WithTimeout(ctx, 30*time.Second)
			err = element.Context(inputCtx).Input(string(char))
			inputCancel()
			if err != nil {
				a.logger.Warn().Err(err).Int("position", i).Msg("Fallback input failed")

				// Final fallback: JavaScript with context
				jsCtx, jsCancel := context.WithTimeout(ctx, 30*time.Second)
				_, err = element.Context(jsCtx).Eval(`(element, char) => {
					element.value += char;
					element.dispatchEvent(new Event('input', { bubbles: true }));
				}`, string(char))
				jsCancel()
				if err != nil {
					return fmt.Errorf("failed to type character at position %d: %w", i, err)
				}
			}
		}
	}

	// Simulate realistic post-typing behavior
	time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond) // 200-500ms pause

	a.logger.Info().Msg("Password typing completed successfully")
	return nil
}

// simulateFieldValidation simulates field validation behavior
func (a *BrowserOAuthAutomator) simulateFieldValidation(page *rod.Page, element *rod.Element) error {
	a.logger.Info().Msg("Simulating field validation behavior")

	// Create a context with a generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Simulate tab out and back in (common user behavior)
	err := page.Keyboard.Press(input.Tab)
	if err == nil {
		time.Sleep(time.Duration(300+rand.Intn(400)) * time.Millisecond) // 300-700ms
		err = page.Keyboard.Press(input.ShiftLeft)
		if err == nil {
			time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
			err = page.Keyboard.Press(input.Tab)
			if err == nil {
				time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
			}
		}
	}

	// Trigger validation events with context
	validationCtx, validationCancel := context.WithTimeout(ctx, 30*time.Second)
	defer validationCancel()
	_, err = element.Context(validationCtx).Eval(`(element) => {
		element.dispatchEvent(new Event('blur', { bubbles: true }));
		element.dispatchEvent(new Event('focusout', { bubbles: true }));
		element.dispatchEvent(new Event('change', { bubbles: true }));
		element.dispatchEvent(new Event('input', { bubbles: true }));
	}`)

	return err
}

// clickNextButton clicks the Next/Continue button (for Google's, Microsoft's, and Atlassian's email step)
func (a *BrowserOAuthAutomator) clickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Looking for Next/Continue button")

	// Create a context with very generous timeout for ALL operations
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second) // 3 minutes total
	defer cancel()

	// Set the page context to use our generous timeout
	page = page.Context(ctx)

	var nextButton *rod.Element

	// First try direct CSS selectors
	nextButtonSelectors := []string{
		`button:contains("Next")`, `button:contains("Continue")`,
		`input[type="submit"][value*="Next"]`, `input[type="submit"][value*="Continue"]`,
		`button[type="submit"]`, `input[type="submit"]`,
	}

	var err error
	for _, selector := range nextButtonSelectors {
		elements, err := page.Elements(selector)
		if err == nil && len(elements) > 0 {
			for _, element := range elements {
				// Get element text using context-aware operations
				buttonText, textErr := element.Text()
				if textErr == nil {
					buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
					if buttonTextLower == "next" || buttonTextLower == "continue" {
						a.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found Next/Continue button using direct selector")
						nextButton = element
						break
					}
				}
			}
		}
		if nextButton != nil {
			break
		}
	}

	if nextButton == nil {
		a.debugDumpPageElements(page, "next_button_not_found")
		return fmt.Errorf("failed to find Next/Continue button")
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

	// Create a context with a very generous timeout for all operations
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Simplify - focus on the exact button we know exists from debug output
	a.logger.Info().Msg("Attempting to find the exact login-submit button")

	// Try the exact selectors we know should work, one by one with contexts
	exactSelectors := []string{
		`button[id="login-submit"]`, // Exact match from debug output
		`#login-submit`,             // CSS ID selector
		`button[type="submit"]`,     // Type-based selector
		`input[type="submit"]`,      // Alternative input type
	}

	var loginButton *rod.Element
	for i, selector := range exactSelectors {
		a.logger.Info().Int("selector_index", i).Str("selector", selector).Msg("Trying exact login button selector")

		// Use context-based timeout for element finding
		elemCtx, elemCancel := context.WithTimeout(ctx, 45*time.Second)
		element, err := page.Context(elemCtx).Element(selector)
		elemCancel()

		if err == nil && element != nil {
			a.logger.Info().Str("selector", selector).Msg("Found login button!")
			loginButton = element
			break
		} else {
			a.logger.Warn().Err(err).Str("selector", selector).Msg("Selector failed")
		}
	}

	if loginButton != nil {
		a.logger.Info().Msg("Attempting to click login button with stealth mode")

		// Use our proven stealth click function that worked for other elements
		err := a.performStealthClick(page, loginButton)
		if err == nil {
			a.logger.Info().Msg("Login button clicked successfully!")
			return nil
		} else {
			a.logger.Warn().Err(err).Msg("Stealth click failed, trying simple click")

			// Fallback to simple click
			clickCtx, clickCancel := context.WithTimeout(ctx, 30*time.Second)
			err = loginButton.Context(clickCtx).Click(proto.InputMouseButtonLeft, 1)
			clickCancel()
			if err == nil {
				a.logger.Info().Msg("Simple click succeeded!")
				return nil
			} else {
				a.logger.Warn().Err(err).Msg("Simple click failed, trying JavaScript click")

				// Final fallback to JavaScript click
				jsCtx, jsCancel := context.WithTimeout(ctx, 30*time.Second)
				_, err = loginButton.Context(jsCtx).Eval(`(element) => {
					console.log('Clicking button via JavaScript');
					element.focus();
					element.click();
					// Also dispatch a mouse click event
					element.dispatchEvent(new MouseEvent('click', {
						bubbles: true,
						cancelable: true,
						view: window
					}));
				}`)
				jsCancel()
				if err == nil {
					a.logger.Info().Msg("JavaScript click succeeded!")
					return nil
				} else {
					a.logger.Error().Err(err).Msg("All click methods failed")
				}
			}
		}
	}

	// If we get here, no button was found or clicked successfully
	a.logger.Error().Msg("Could not find or click login button")

	// Add final debugging but with very short timeout to avoid hanging
	debugCtx, debugCancel := context.WithTimeout(ctx, 5*time.Second)
	currentURL, _ := page.Context(debugCtx).Eval(`() => window.location.href`)
	debugCancel()
	if currentURL != nil {
		a.logger.Info().Str("current_url", fmt.Sprintf("%v", currentURL.Value)).Msg("Current page URL for debugging")
	}

	return fmt.Errorf("failed to find or click login button after trying all methods")
}

// waitForNavigation waits for page navigation after login
func (a *BrowserOAuthAutomator) waitForNavigation(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Waiting for page navigation after login")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_navigation_start")

	// Microsoft uses dynamic page updates instead of URL navigation
	if a.config.ProviderName == "microsoft" {
		return a.waitForMicrosoftPageTransition(page, screenshotTaker)
	}

	// Wait for URL to change (indicating navigation started) - for Google/GitHub/Atlassian
	currentURL := page.MustInfo().URL
	a.logger.Info().Str("initial_url", currentURL).Msg("Starting navigation wait from this URL")

	// Significantly increase timeout for slow Atlassian pages
	timeout := time.After(60 * time.Second)   // Increased from 10s to 60s
	ticker := time.NewTicker(1 * time.Second) // Check every second instead of 500ms
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
				Int("timeout_seconds", 60).
				Msg("Timeout waiting for login navigation")
			return fmt.Errorf("timeout waiting for login navigation after 60 seconds")
		case <-ticker.C:
			checkCount++
			newURL := page.MustInfo().URL
			if checkCount%5 == 0 { // Log every 5 seconds instead of every 2
				a.logger.Info().
					Str("current_url", newURL).
					Int("check_count", checkCount).
					Int("seconds_elapsed", checkCount).
					Msg("Navigation check - URL still unchanged")

				// Take periodic screenshots during long wait
				screenshotTaker.TakeScreenshot(page, fmt.Sprintf("%s_navigation_wait_%ds", a.config.ProviderName, checkCount))
			}
			if newURL != currentURL {
				a.logger.Info().Str("old_url", currentURL).Str("new_url", newURL).Int("seconds_waited", checkCount).Msg("Navigation detected")
				navigationStarted = true
			}
		}
	}

	// Wait for page to fully load after navigation with longer timeout
	a.logger.Info().Msg("Navigation detected - waiting for page to fully load")
	page.Timeout(60 * time.Second).MustWaitLoad() // Add explicit timeout

	// Additional wait to ensure all dynamic content loads
	a.logger.Info().Msg("Waiting additional 5 seconds for dynamic content to load")
	time.Sleep(5 * time.Second) // Increased from 2s to 5s

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
	passwordField, err := page.Timeout(10 * time.Second).Element(`input[type="password"]:not(.moveOffScreen)`) // Add explicit timeout
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

	// Significantly increase timeout for Microsoft transitions
	timeout := time.After(90 * time.Second)   // Increased from 15s to 90s
	ticker := time.NewTicker(1 * time.Second) // Check every second
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
				Int("timeout_seconds", 90).
				Msg("Timeout waiting for Microsoft page transition")
			return fmt.Errorf("timeout waiting for Microsoft page transition after 90 seconds")
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
		elements, err := page.Timeout(30 * time.Second).Elements(selector) // Increased from 3s to 30s
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
