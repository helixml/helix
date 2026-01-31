package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// GoogleOAuthHandler handles Google-specific OAuth flow complexities
type GoogleOAuthHandler struct {
	logger zerolog.Logger
}

// NewGoogleOAuthHandler creates a new Google OAuth handler
func NewGoogleOAuthHandler(logger zerolog.Logger) *GoogleOAuthHandler {
	return &GoogleOAuthHandler{
		logger: logger,
	}
}

// IsRequired checks if Google-specific handling is required
func (h *GoogleOAuthHandler) IsRequired(page *rod.Page) bool {
	currentURL := page.MustInfo().URL
	return h.IsRequiredForURL(currentURL)
}

// IsRequiredForURL checks if Google-specific handling is required for a given URL
func (h *GoogleOAuthHandler) IsRequiredForURL(url string) bool {
	// Exclude OAuth consent pages - these should be handled by the provider strategy
	if strings.Contains(url, "/signin/oauth/consent") ||
		strings.Contains(url, "/o/oauth2/") {
		h.logger.Info().Str("url", url).Msg("Google OAuth consent page detected - will be handled by provider strategy")
		return false
	}

	// Check if we're on a Google authentication page that needs 2FA handling
	isGoogleAuthPage := strings.Contains(url, "accounts.google.com") ||
		strings.Contains(url, "myaccount.google.com") ||
		strings.Contains(url, "google.com/signin")

	if isGoogleAuthPage {
		h.logger.Info().Str("url", url).Msg("Google authentication page detected")
	}

	return isGoogleAuthPage
}

// Handle performs Google-specific OAuth flow handling
func (h *GoogleOAuthHandler) Handle(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling Google OAuth flow")

	currentURL := page.MustInfo().URL
	h.logger.Info().Str("url", currentURL).Msg("Current Google page")

	// Handle various Google OAuth states
	if strings.Contains(currentURL, "/signin/v2/identifier") {
		return h.handleEmailStep(page)
	}

	if strings.Contains(currentURL, "/signin/v2/challenge/pwd") {
		return h.handlePasswordStep(page)
	}

	if strings.Contains(currentURL, "/signin/v2/challenge/") {
		return h.handle2FAChallenge(page)
	}

	// Handle OAuth consent pages (both old and new Google patterns)
	if strings.Contains(currentURL, "/o/oauth2/") ||
		strings.Contains(currentURL, "/signin/oauth/consent") ||
		strings.Contains(currentURL, "/signin/oauth/id") {
		return h.handleOAuthConsent(page)
	}

	// Generic handling for other Google auth pages
	return h.handleGenericGoogleAuth(page)
}

// handleEmailStep handles the email input step of Google's login
func (h *GoogleOAuthHandler) handleEmailStep(page *rod.Page) error {
	h.logger.Info().Msg("Handling Google email input step")

	// Wait for page to fully load
	time.Sleep(2 * time.Second)

	// Find email input field
	emailSelectors := []string{
		`input[type="email"]`,
		`input[name="identifier"]`,
		`input[id="identifierId"]`,
		`input[autocomplete="username"]`,
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
		return fmt.Errorf("could not find email input field")
	}

	// Check if email input is already filled
	currentValue, err := emailInput.Text()
	if err == nil && currentValue != "" {
		h.logger.Info().Str("email", currentValue).Msg("Email field already filled")
	} else {
		h.logger.Info().Msg("Email field needs to be filled by main automation")
		// Return nil to let the main automation handle the email input
		return nil
	}

	// Find and click Next button
	nextSelectors := []string{
		`button[id="identifierNext"]`,
		`input[type="submit"][value="Next"]`,
		`button[type="submit"]`,
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

		// Wait for navigation to password step
		time.Sleep(3 * time.Second)
	}

	return nil
}

// handlePasswordStep handles the password input step of Google's login
func (h *GoogleOAuthHandler) handlePasswordStep(page *rod.Page) error {
	h.logger.Info().Msg("Handling Google password input step")

	// Wait for password page to load
	time.Sleep(2 * time.Second)

	// Find password input field
	passwordSelectors := []string{
		`input[type="password"]`,
		`input[name="password"]`,
		`input[autocomplete="current-password"]`,
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
		// Let main automation handle password input
		h.logger.Info().Msg("Password field not found, letting main automation handle it")
		return nil
	}

	// Check if password field is already filled
	currentValue, err := passwordInput.Property("value")
	if err == nil && currentValue.String() != "" {
		h.logger.Info().Msg("Password field already filled")

		// Find and click Next/Sign in button
		nextSelectors := []string{
			`button[id="passwordNext"]`,
			`input[type="submit"][value="Next"]`,
			`input[type="submit"][value="Sign in"]`,
			`input[type="submit"]`,
			`button[type="submit"]`,
		}

		var nextButton *rod.Element
		for _, selector := range nextSelectors {
			nextButton, err = page.Element(selector)
			if err == nil && nextButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found Next/Sign in button")
				break
			}
		}

		if nextButton != nil {
			err = nextButton.Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				return fmt.Errorf("failed to click Next/Sign in button: %w", err)
			}

			h.logger.Info().Msg("Clicked Next/Sign in button after password input")
			time.Sleep(3 * time.Second)
		}
	}

	return nil
}

// handle2FAChallenge handles Google's 2FA challenges
func (h *GoogleOAuthHandler) handle2FAChallenge(page *rod.Page) error {
	h.logger.Info().Msg("Handling Google 2FA challenge")

	// Wait for 2FA page to load
	time.Sleep(3 * time.Second)

	// Look for various 2FA inputs
	twoFASelectors := []string{
		`input[type="tel"]`,     // Phone verification
		`input[name="totpPin"]`, // TOTP code
		`input[placeholder*="code"]`,
		`input[placeholder*="verification"]`,
		`input[id*="code"]`,
	}

	var twoFAInput *rod.Element
	var err error

	for _, selector := range twoFASelectors {
		twoFAInput, err = page.Element(selector)
		if err == nil && twoFAInput != nil {
			h.logger.Info().Str("selector", selector).Msg("Found 2FA input field")
			break
		}
	}

	if twoFAInput == nil {
		// Look for alternative 2FA options or skip buttons
		skipSelectors := []string{
			`button[value="Skip"]`,
			`input[type="submit"][value="Skip"]`,
			`button[value="Not now"]`,
			`a[href*="skip"]`,
		}

		for _, selector := range skipSelectors {
			skipButton, err := page.Element(selector)
			if err == nil && skipButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found skip button for 2FA")
				err = skipButton.Click(proto.InputMouseButtonLeft, 1)
				if err == nil {
					h.logger.Info().Msg("Clicked skip button for 2FA")
					time.Sleep(3 * time.Second)
					return nil
				}
			}
		}

		return fmt.Errorf("no 2FA input found and no skip option available")
	}

	// For now, we'll return an error for 2FA as it requires manual intervention
	// In a real implementation, you'd need to integrate with a 2FA provider
	h.logger.Warn().Msg("2FA detected but not implemented - this may require manual intervention")
	return fmt.Errorf("2FA challenge detected but automatic handling not implemented")
}

// handleOAuthConsent handles Google's OAuth consent screen
func (h *GoogleOAuthHandler) handleOAuthConsent(page *rod.Page) error {
	h.logger.Info().Msg("Handling Google OAuth consent screen")

	// Wait for consent page to load
	time.Sleep(2 * time.Second)

	// First try the same successful class pattern approach that worked for Next/Login buttons
	h.logger.Info().Msg("Attempting to find consent button using same class pattern as other Google buttons")

	// Use the same class patterns that successfully worked for Next/Login buttons
	specificSelectors := []string{
		// Same Google button class pattern that worked for Next/Login
		"button.VfPpkd-LgbsSe.VfPpkd-LgbsSe-OWXEXe-k8QpJ.VfPpkd-LgbsSe-OWXEXe-dgl2Hf.nCP5yc.AjY5Oe.DuMIQc.LQeN7.BqKGqe.Jskylb.TrZEUc.lw1w4b",
		// Shorter pattern with key classes
		"button.VfPpkd-LgbsSe.nCP5yc.AjY5Oe.DuMIQc",
		// Even shorter pattern
		"button.VfPpkd-LgbsSe.nCP5yc",
		// Fallback to core Google button class
		"button.VfPpkd-LgbsSe",
	}

	var allowButton *rod.Element
	for _, selector := range specificSelectors {
		// Set shorter timeout for each attempt
		elements, err := page.Timeout(3 * time.Second).Elements(selector)
		if err == nil && len(elements) > 0 {
			// Check each element to find one with consent-related text
			for _, element := range elements {
				buttonText, textErr := element.Timeout(1 * time.Second).Text()
				if textErr == nil {
					buttonTextLower := strings.ToLower(strings.TrimSpace(buttonText))
					// Look for consent/permission buttons
					if buttonTextLower == "allow" || buttonTextLower == "continue" ||
						buttonTextLower == "accept" || buttonTextLower == "grant" ||
						strings.Contains(buttonTextLower, "allow") || strings.Contains(buttonTextLower, "continue") {
						h.logger.Info().Str("selector", selector).Str("button_text", buttonText).Msg("Found consent button using class pattern")
						allowButton = element
						break
					}
				}
			}
			if allowButton != nil {
				break
			}
		}
	}

	// Fallback to original selectors if class patterns don't work
	if allowButton == nil {
		h.logger.Warn().Msg("Class pattern approach failed, falling back to original selectors")
		allowSelectors := []string{
			`input[type="submit"][value="Allow"]`,
			`input[type="submit"][value="Continue"]`,
			`button[value="Allow"]`,
			`button[value="Continue"]`,
			`button[data-l*="allow"]`,
			`button[id*="submit"]`,
			`button[type="submit"]`,
		}

		var err error
		for _, selector := range allowSelectors {
			allowButton, err = page.Element(selector)
			if err == nil && allowButton != nil {
				h.logger.Info().Str("selector", selector).Msg("Found OAuth consent button using fallback selector")
				break
			}
		}
	}

	if allowButton == nil {
		return fmt.Errorf("could not find OAuth consent button")
	}

	err := allowButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click OAuth consent button: %w", err)
	}

	h.logger.Info().Msg("Clicked OAuth consent button")
	time.Sleep(3 * time.Second)

	return nil
}

// handleGenericGoogleAuth handles other Google authentication pages
func (h *GoogleOAuthHandler) handleGenericGoogleAuth(page *rod.Page) error {
	h.logger.Info().Msg("Handling generic Google authentication page")

	// Wait for page to load
	time.Sleep(2 * time.Second)

	// Look for common Google authentication buttons
	buttonSelectors := []string{
		`input[type="submit"][value="Continue"]`,
		`input[type="submit"][value="Next"]`,
		`input[type="submit"][value="Allow"]`,
		`button[value="Continue"]`,
		`button[value="Next"]`,
		`button[value="Allow"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	for _, selector := range buttonSelectors {
		button, err := page.Element(selector)
		if err == nil && button != nil {
			h.logger.Info().Str("selector", selector).Msg("Found button on Google auth page")

			err = button.Click(proto.InputMouseButtonLeft, 1)
			if err == nil {
				h.logger.Info().Str("selector", selector).Msg("Clicked button on Google auth page")
				time.Sleep(3 * time.Second)
				return nil
			}
		}
	}

	h.logger.Info().Msg("No suitable buttons found on generic Google auth page")
	return nil
}
