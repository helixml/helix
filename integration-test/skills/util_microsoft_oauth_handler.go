package skills

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// MicrosoftOAuthHandler handles Microsoft-specific OAuth flow complexities
type MicrosoftOAuthHandler struct {
	logger zerolog.Logger
}

// NewMicrosoftOAuthHandler creates a new Microsoft OAuth handler
func NewMicrosoftOAuthHandler(logger zerolog.Logger) *MicrosoftOAuthHandler {
	return &MicrosoftOAuthHandler{
		logger: logger,
	}
}

// IsRequired checks if Microsoft-specific handling is required
func (h *MicrosoftOAuthHandler) IsRequired(page *rod.Page) bool {
	currentURL := page.MustInfo().URL
	return h.IsRequiredForURL(currentURL)
}

// IsRequiredForURL checks if Microsoft-specific handling is required for a given URL
func (h *MicrosoftOAuthHandler) IsRequiredForURL(url string) bool {
	// Check if we're on a Microsoft authentication page
	isMicrosoftAuthPage := strings.Contains(url, "login.microsoftonline.com") ||
		strings.Contains(url, "login.live.com") ||
		strings.Contains(url, "account.microsoft.com") ||
		strings.Contains(url, "login.windows.net")

	if isMicrosoftAuthPage {
		h.logger.Info().Str("url", url).Msg("Microsoft authentication page detected")
	}

	return isMicrosoftAuthPage
}

// Handle performs Microsoft-specific OAuth flow handling
func (h *MicrosoftOAuthHandler) Handle(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling Microsoft OAuth flow")

	currentURL := page.MustInfo().URL
	h.logger.Info().Str("url", currentURL).Msg("Current Microsoft page")

	// Handle enterprise authentication
	if h.IsEnterpriseAuthenticationRequired(currentURL) {
		h.logger.Info().Str("url", currentURL).Msg("Microsoft enterprise authentication page detected")
		return h.handleEnterpriseAuthentication(page)
	}

	// Handle various Microsoft OAuth states
	if strings.Contains(currentURL, "/oauth2/v2.0/authorize") ||
		strings.Contains(currentURL, "/oauth2/authorize") {
		return h.handleAuthorizationPage(page)
	}

	if strings.Contains(currentURL, "/login") {
		return h.handleLoginPage(page)
	}

	if strings.Contains(currentURL, "/kmsi") {
		return h.handleKeepMeSignedIn(page)
	}

	if strings.Contains(currentURL, "/consent") {
		return h.handleConsentPage(page)
	}

	// Generic handling for other Microsoft auth pages
	return h.handleGenericMicrosoftAuth(page)
}

// IsEnterpriseAuthenticationRequired checks if the URL indicates enterprise authentication
func (h *MicrosoftOAuthHandler) IsEnterpriseAuthenticationRequired(url string) bool {
	enterprisePatterns := []string{
		"login.microsoftonline.com/common/login",
		"login.microsoftonline.com/organizations/login",
		"login.microsoftonline.com/consumers/login",
	}

	for _, pattern := range enterprisePatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}

	return false
}

// handleEnterpriseAuthentication handles Microsoft enterprise authentication pages
func (h *MicrosoftOAuthHandler) handleEnterpriseAuthentication(page *rod.Page) error {
	h.logger.Info().Msg("Handling Microsoft enterprise authentication")

	// Wait for page to load
	time.Sleep(3 * time.Second)

	currentURL := page.MustInfo().URL
	h.logger.Info().Str("url", currentURL).Msg("Enterprise authentication page URL")

	// Strategy 1: Try to click the Yes button immediately (we know it's there from page dumps)
	// This is the most direct approach since we consistently see the same Yes button structure
	immediateSelectors := []string{
		`input[type="submit"][id="idSIButton9"][value="Yes"]`, // Exact match from consistent page dumps
		`input[id="idSIButton9"][value="Yes"]`,                // Yes button with id
		`input[type="submit"][value="Yes"]`,                   // Yes button by value
	}

	for _, selector := range immediateSelectors {
		h.logger.Info().Str("selector", selector).Msg("Trying immediate click of enterprise Accept button")
		element, err := page.Element(selector)
		if err == nil && element != nil {
			visible, visErr := element.Visible()
			if visErr == nil && visible {
				h.logger.Info().Str("selector", selector).Msg("Found Accept button - clicking immediately")
				clickErr := element.Click(proto.InputMouseButtonLeft, 1)
				if clickErr == nil {
					h.logger.Info().Str("selector", selector).Msg("Successfully clicked Accept button")
					time.Sleep(3 * time.Second)
					return nil
				}
				h.logger.Error().Str("selector", selector).Err(clickErr).Msg("Failed to click Accept button")
			}
		}
	}

	// Strategy 2: If immediate click failed, do full debugging and attempt fallback approaches
	// Create a fresh context with longer timeout for enterprise auth page interactions
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Create a new page context for element searches
	freshPage := page.Context(ctx)

	// Dump all elements on the page for debugging
	h.dumpPageElements(freshPage, "enterprise_auth")

	// Strategy 3: Look for skip/alternative options
	skipSelectors := []string{
		`a[href*="skip"]`,
		`button[id*="skip"]`,
		`input[value*="Skip"]`,
		`a[id*="skip"]`,
		`button[class*="skip"]`,
		`a[class*="skip"]`,
		`button[id="idBtn_Back"]`, // "Back" button might skip MFA
		`a[href*="alternate"]`,
		`a[href*="other"]`,
		`button[id*="other"]`,
		`a[id*="other"]`,
	}

	for _, selector := range skipSelectors {
		element, err := freshPage.Element(selector)
		if err == nil && element != nil {
			visible, visErr := element.Visible()
			if visErr == nil && visible {
				h.logger.Info().Str("selector", selector).Msg("Found skip/alternative option - clicking")
				clickErr := element.Click(proto.InputMouseButtonLeft, 1)
				if clickErr == nil {
					h.logger.Info().Msg("Successfully clicked skip/alternative option")
					time.Sleep(3 * time.Second)
					return nil
				}
			}
		}
	}

	// Strategy 4: Look for specific Microsoft enterprise buttons (from page dump analysis)
	// We know from the page dump that the button has id="idSIButton9" and value="Yes"
	enterpriseButtonSelectors := []string{
		`input[type="submit"][id="idSIButton9"][value="Yes"]`, // Exact match from page dump
		`input[id="idSIButton9"][value="Yes"]`,                // Yes button with id
		`input[type="submit"][value="Yes"]`,                   // Yes button specifically
		`input[name="idSIButton9"][value="Accept"]`,           // Accept button with name
		`input[type="submit"][value="Accept"]`,                // Accept button specifically
		`input[name="idSIButton9"]`,                           // Microsoft enterprise button (from page dump)
		`input[id="idSIButton9"]`,                             // Microsoft standard button (often "Continue")
		`input[type="submit"][value="Continue"]`,
		`input[type="submit"][value="Allow"]`,
		`input[type="submit"][value="Next"]`,
		`input[type="submit"][value="Sign in"]`,
		`button[value="Continue"]`,
		`button[value="Accept"]`,
		`button[value="Yes"]`,
		`button[value="Allow"]`,
		`button[value="Next"]`,
		`button[value="Sign in"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	for _, selector := range enterpriseButtonSelectors {
		h.logger.Info().Str("selector", selector).Msg("Trying to find enterprise authentication button")
		element, err := freshPage.Element(selector)
		if err != nil {
			h.logger.Info().Str("selector", selector).Err(err).Msg("Could not find element with selector")
			continue
		}
		if element == nil {
			h.logger.Info().Str("selector", selector).Msg("Element is nil")
			continue
		}

		visible, visErr := element.Visible()
		if visErr != nil {
			h.logger.Info().Str("selector", selector).Err(visErr).Msg("Could not check visibility")
			continue
		}
		if !visible {
			h.logger.Info().Str("selector", selector).Msg("Element is not visible")
			continue
		}

		h.logger.Info().Str("selector", selector).Msg("Found visible enterprise authentication button - attempting to click")
		clickErr := element.Click(proto.InputMouseButtonLeft, 1)
		if clickErr != nil {
			h.logger.Error().Str("selector", selector).Err(clickErr).Msg("Failed to click enterprise authentication button")
			continue
		}

		h.logger.Info().Str("selector", selector).Msg("Successfully clicked enterprise authentication button")
		time.Sleep(3 * time.Second)
		return nil
	}

	// Strategy 5: Look for any clickable element with helpful text
	allButtons, err := freshPage.Elements(`button, input[type="submit"], input[type="button"], a`)
	if err == nil {
		for _, button := range allButtons {
			visible, visErr := button.Visible()
			if visErr != nil || !visible {
				continue
			}

			// Try to get button text/value
			buttonText := ""
			if text, textErr := button.Text(); textErr == nil && text != "" {
				buttonText = strings.ToLower(strings.TrimSpace(text))
			} else if value, valueErr := button.Attribute("value"); valueErr == nil && value != nil {
				buttonText = strings.ToLower(strings.TrimSpace(*value))
			}

			// Look for helpful text patterns
			helpfulPatterns := []string{"continue", "skip", "next", "proceed", "allow", "accept", "yes", "ok", "sign in"}
			for _, pattern := range helpfulPatterns {
				if strings.Contains(buttonText, pattern) {
					h.logger.Info().Str("button_text", buttonText).Msg("Found button with helpful text - clicking")
					clickErr := button.Click(proto.InputMouseButtonLeft, 1)
					if clickErr == nil {
						h.logger.Info().Msg("Successfully clicked button with helpful text")
						time.Sleep(3 * time.Second)
						return nil
					}
				}
			}
		}
	}

	h.logger.Info().Msg("No actionable elements found on enterprise authentication page")
	return nil
}

// dumpPageElements dumps page elements for debugging
func (h *MicrosoftOAuthHandler) dumpPageElements(page *rod.Page, stepName string) {
	h.logger.Info().Str("step", stepName).Msg("=== DUMPING PAGE ELEMENTS ===")

	// Get current URL
	currentURL := page.MustInfo().URL
	h.logger.Info().Str("current_url", currentURL).Msg("Current page URL")

	// Get page title (not available in rod, skip for now)
	h.logger.Info().Msg("Page title: [not available via rod API]")

	// Dump input elements
	inputs, err := page.Elements(`input`)
	if err == nil {
		h.logger.Info().Int("count", len(inputs)).Msg("Found input elements")
		for i, input := range inputs {
			if i >= 10 { // Limit to first 10 to avoid spam
				break
			}
			inputType, _ := input.Attribute("type")
			inputName, _ := input.Attribute("name")
			inputID, _ := input.Attribute("id")
			inputClass, _ := input.Attribute("class")
			inputValue, _ := input.Attribute("value")
			inputPlaceholder, _ := input.Attribute("placeholder")

			h.logger.Info().
				Int("index", i).
				Str("type", getStringValue(inputType)).
				Str("name", getStringValue(inputName)).
				Str("id", getStringValue(inputID)).
				Str("class", getStringValue(inputClass)).
				Str("value", getStringValue(inputValue)).
				Str("placeholder", getStringValue(inputPlaceholder)).
				Msg("Input element found")
		}
	}

	// Dump button elements
	buttons, err := page.Elements(`button`)
	if err == nil {
		h.logger.Info().Int("count", len(buttons)).Msg("Found button elements")
		for i, button := range buttons {
			if i >= 10 { // Limit to first 10 to avoid spam
				break
			}
			buttonType, _ := button.Attribute("type")
			buttonID, _ := button.Attribute("id")
			buttonClass, _ := button.Attribute("class")
			buttonText, _ := button.Text()

			h.logger.Info().
				Int("index", i).
				Str("type", getStringValue(buttonType)).
				Str("id", getStringValue(buttonID)).
				Str("class", getStringValue(buttonClass)).
				Str("text", buttonText).
				Msg("Button element found")
		}
	}

	h.logger.Info().Str("step", stepName).Msg("=== END PAGE ELEMENTS DUMP ===")
}

// getStringValue safely gets string value from pointer
func getStringValue(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

// handleAuthorizationPage handles the initial Microsoft authorization page
func (h *MicrosoftOAuthHandler) handleAuthorizationPage(page *rod.Page) error {
	h.logger.Info().Msg("Handling Microsoft authorization page")

	// Wait for page to fully load
	time.Sleep(2 * time.Second)

	// Look for email input field
	emailSelectors := []string{
		`input[type="email"]`,
		`input[name="loginfmt"]`,
		`input[placeholder*="email"]`,
		`input[placeholder*="Email"]`,
		`input[data-bind*="textInput: username"]`,
	}

	var emailInput *rod.Element
	var err error

	for _, selector := range emailSelectors {
		emailInput, err = page.Element(selector)
		if err == nil && emailInput != nil {
			h.logger.Info().Str("selector", selector).Msg("Found email input field")
			break
		}
	}

	if emailInput == nil {
		h.logger.Info().Msg("No email input found, letting main automation handle it")
		return nil
	}

	// Check if email input is already filled
	currentValue, err := emailInput.Property("value")
	if err == nil && currentValue.String() != "" {
		h.logger.Info().Str("email", currentValue.String()).Msg("Email field already filled")

		// Find and click Next button
		nextSelectors := []string{
			`input[type="submit"][value="Next"]`,
			`input[id="idSIButton9"]`,
			`button[type="submit"]`,
			`input[type="submit"]`,
		}

		var nextButton *rod.Element
		for _, selector := range nextSelectors {
			nextButton, err = page.Element(selector)
			if err == nil && nextButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found Next button")
				break
			}
		}

		if nextButton != nil {
			err = nextButton.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return fmt.Errorf("failed to click Next button: %w", err)
			}

			h.logger.Info().Msg("Clicked Next button after email input")
			time.Sleep(3 * time.Second)
		}
	}

	return nil
}

// handleLoginPage handles the Microsoft login page with password input
func (h *MicrosoftOAuthHandler) handleLoginPage(page *rod.Page) error {
	h.logger.Info().Msg("Handling Microsoft login page")

	// Wait for password page to load
	time.Sleep(2 * time.Second)

	// Find password input field
	passwordSelectors := []string{
		`input[type="password"]`,
		`input[name="passwd"]`,
		`input[placeholder*="password"]`,
		`input[placeholder*="Password"]`,
		`input[data-bind*="textInput: password"]`,
	}

	var passwordInput *rod.Element
	var err error

	for _, selector := range passwordSelectors {
		passwordInput, err = page.Element(selector)
		if err == nil && passwordInput != nil {
			h.logger.Info().Str("selector", selector).Msg("Found password input field")
			break
		}
	}

	if passwordInput == nil {
		h.logger.Info().Msg("Password field not found, letting main automation handle it")
		return nil
	}

	// Check if password field is already filled
	currentValue, err := passwordInput.Property("value")
	if err == nil && currentValue.String() != "" {
		h.logger.Info().Msg("Password field already filled")

		// Find and click Sign in button
		signinSelectors := []string{
			`input[type="submit"][value="Sign in"]`,
			`input[id="idSIButton9"]`,
			`button[type="submit"]`,
			`input[type="submit"]`,
		}

		var signinButton *rod.Element
		for _, selector := range signinSelectors {
			signinButton, err = page.Element(selector)
			if err == nil && signinButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found Sign in button")
				break
			}
		}

		if signinButton != nil {
			err = signinButton.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return fmt.Errorf("failed to click Sign in button: %w", err)
			}

			h.logger.Info().Msg("Clicked Sign in button after password input")
			time.Sleep(3 * time.Second)
		}
	}

	return nil
}

// handleKeepMeSignedIn handles the "Keep me signed in" page
func (h *MicrosoftOAuthHandler) handleKeepMeSignedIn(page *rod.Page) error {
	h.logger.Info().Msg("Handling Microsoft 'Keep me signed in' page")

	// Wait for page to load
	time.Sleep(2 * time.Second)

	// Look for "No" button to not stay signed in (more secure for tests)
	noSelectors := []string{
		`input[type="submit"][value="No"]`,
		`input[id="idBtn_Back"]`,
		`button[value="No"]`,
	}

	var noButton *rod.Element
	var err error

	for _, selector := range noSelectors {
		noButton, err = page.Element(selector)
		if err == nil && noButton != nil {
			h.logger.Info().Str("selector", selector).Msg("Found No button")
			break
		}
	}

	if noButton != nil {
		err = noButton.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			return fmt.Errorf("failed to click No button: %w", err)
		}

		h.logger.Info().Msg("Clicked No button for 'Keep me signed in'")
		time.Sleep(3 * time.Second)
	} else {
		// If no "No" button found, look for "Yes" button as fallback
		yesSelectors := []string{
			`input[type="submit"][value="Yes"]`,
			`input[id="idSIButton9"]`,
			`button[value="Yes"]`,
		}

		var yesButton *rod.Element
		for _, selector := range yesSelectors {
			yesButton, err = page.Element(selector)
			if err == nil && yesButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found Yes button")
				break
			}
		}

		if yesButton != nil {
			err = yesButton.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return fmt.Errorf("failed to click Yes button: %w", err)
			}

			h.logger.Info().Msg("Clicked Yes button for 'Keep me signed in'")
			time.Sleep(3 * time.Second)
		}
	}

	return nil
}

// handleConsentPage handles Microsoft's OAuth consent screen
func (h *MicrosoftOAuthHandler) handleConsentPage(page *rod.Page) error {
	h.logger.Info().Msg("Handling Microsoft OAuth consent screen")

	// Wait for consent page to load
	time.Sleep(2 * time.Second)

	// Find Accept/Allow button
	acceptSelectors := []string{
		`input[type="submit"][value="Accept"]`,
		`input[type="submit"][value="Allow"]`,
		`input[id="idSIButton9"]`,
		`button[value="Accept"]`,
		`button[value="Allow"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	var acceptButton *rod.Element
	var err error

	for _, selector := range acceptSelectors {
		acceptButton, err = page.Element(selector)
		if err == nil && acceptButton != nil {
			h.logger.Info().Str("selector", selector).Msg("Found consent button")
			break
		}
	}

	if acceptButton == nil {
		return fmt.Errorf("could not find OAuth consent button")
	}

	err = acceptButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click OAuth consent button: %w", err)
	}

	h.logger.Info().Msg("Clicked OAuth consent button")
	time.Sleep(3 * time.Second)

	return nil
}

// handleGenericMicrosoftAuth handles other Microsoft authentication pages
func (h *MicrosoftOAuthHandler) handleGenericMicrosoftAuth(page *rod.Page) error {
	h.logger.Info().Msg("Handling generic Microsoft authentication page")

	// Wait for page to load
	time.Sleep(2 * time.Second)

	// Look for common Microsoft authentication buttons
	buttonSelectors := []string{
		`input[type="submit"][value="Continue"]`,
		`input[type="submit"][value="Next"]`,
		`input[type="submit"][value="Sign in"]`,
		`input[type="submit"][value="Accept"]`,
		`input[id="idSIButton9"]`,
		`button[value="Continue"]`,
		`button[value="Next"]`,
		`button[value="Sign in"]`,
		`button[value="Accept"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	for _, selector := range buttonSelectors {
		button, err := page.Element(selector)
		if err == nil && button != nil {
			h.logger.Info().Str("selector", selector).Msg("Found button on Microsoft auth page")

			err = button.Click(proto.InputMouseButtonLeft, 1)
			if err == nil {
				h.logger.Info().Str("selector", selector).Msg("Clicked button on Microsoft auth page")
				time.Sleep(3 * time.Second)
				return nil
			}
		}
	}

	h.logger.Info().Msg("No suitable buttons found on generic Microsoft auth page")
	return nil
}
