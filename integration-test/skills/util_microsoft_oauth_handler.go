package skills

import (
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
