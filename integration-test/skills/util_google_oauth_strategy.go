package skills

import (
	"fmt"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// GoogleProviderStrategy implements Google-specific OAuth automation
type GoogleProviderStrategy struct {
	logger zerolog.Logger
}

// NewGoogleProviderStrategy creates a Google provider strategy
func NewGoogleProviderStrategy(logger zerolog.Logger) *GoogleProviderStrategy {
	return &GoogleProviderStrategy{
		logger: logger,
	}
}

// ClickNextButton implements Google-specific Next button clicking logic
func (s *GoogleProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Google Next button")

	// Set timeout for operations
	page = page.Timeout(10 * time.Second)

	// Google-specific button selectors
	nextSelectors := []string{
		`button[id="identifierNext"]`,        // Google Next button ID
		`button[type="submit"]`,              // Generic submit button
		`input[type="submit"][value="Next"]`, // Input style Next button
		`button:contains("Next")`,            // Button containing "Next" text
	}

	var nextButton *rod.Element
	for _, selector := range nextSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Google Next button selector")
		element, err := page.Timeout(3 * time.Second).Element(selector)
		if err == nil && element != nil {
			s.logger.Info().Str("selector", selector).Msg("Found Google Next button")
			nextButton = element
			break
		}
	}

	// If direct selectors failed, try Google-specific JavaScript approach
	if nextButton == nil {
		s.logger.Info().Msg("Direct selectors failed, using Google-specific JavaScript to click Next button")

		// Google-specific JavaScript that handles button elements
		jsCode := `
			(function() {
				// Check button elements (Google's primary pattern)
				var buttons = document.querySelectorAll('button');
				
				// First check for specific Google button IDs
				var nextButton = document.getElementById('identifierNext');
				if (nextButton) {
					nextButton.click();
					return 'success';
				}
				
				// Then check button text content
				for (var i = 0; i < buttons.length; i++) {
					var text = (buttons[i].textContent || '').trim().toLowerCase();
					if (text === 'next' || text === 'continue') {
						buttons[i].click();
						return 'success';
					}
				}
				
				return 'no_next_button_found';
			})();
		`

		result, err := page.Eval(jsCode)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to execute Google JavaScript click")
		} else {
			resultStr := result.Value.String()
			if resultStr == "success" {
				s.logger.Info().Msg("Successfully clicked Google Next button using JavaScript")
				screenshotTaker.TakeScreenshot(page, "google_next_button_clicked")
				return nil
			}
			s.logger.Warn().Str("result", resultStr).Msg("Google JavaScript click failed or unexpected result")
		}

		return fmt.Errorf("failed to find Google Next button")
	}

	// Click the found button
	nextButton = nextButton.Timeout(5 * time.Second)
	err := nextButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Google Next button: %w", err)
	}

	s.logger.Info().Msg("Successfully clicked Google Next button")
	screenshotTaker.TakeScreenshot(page, "google_next_button_clicked")
	return nil
}

// ClickAuthorizeButton implements Google-specific Authorize button clicking logic
func (s *GoogleProviderStrategy) ClickAuthorizeButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Google Authorize button")

	// Set timeout for operations
	page = page.Timeout(10 * time.Second)

	// Google-specific authorization button selectors
	authSelectors := []string{
		`button[id="submit_approve_access"]`,   // Google authorization button ID
		`button:contains("Allow")`,             // Button containing "Allow" text
		`button:contains("Accept")`,            // Button containing "Accept" text
		`button[type="submit"]`,                // Generic submit button
		`input[type="submit"][value="Allow"]`,  // Input style Allow button
		`input[type="submit"][value="Accept"]`, // Input style Accept button
	}

	var authButton *rod.Element
	for _, selector := range authSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Google Authorize button selector")
		element, err := page.Timeout(3 * time.Second).Element(selector)
		if err == nil && element != nil {
			s.logger.Info().Str("selector", selector).Msg("Found Google Authorize button")
			authButton = element
			break
		}
	}

	// If direct selectors failed, try Google-specific JavaScript approach
	if authButton == nil {
		s.logger.Info().Msg("Direct selectors failed, using Google-specific JavaScript to click Authorize button")

		// Google-specific JavaScript that handles authorization buttons
		jsCode := `
			(function() {
				// Check button elements (Google's primary pattern)
				var buttons = document.querySelectorAll('button');
				
				// First check for specific Google button IDs
				var authButton = document.getElementById('submit_approve_access');
				if (authButton) {
					authButton.click();
					return 'success';
				}
				
				// Then check button text content
				for (var i = 0; i < buttons.length; i++) {
					var text = (buttons[i].textContent || '').trim().toLowerCase();
					if (text === 'allow' || text === 'accept' || text === 'authorize') {
						buttons[i].click();
						return 'success';
					}
				}
				
				return 'no_auth_button_found';
			})();
		`

		result, err := page.Eval(jsCode)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to execute Google JavaScript click")
		} else {
			resultStr := result.Value.String()
			if resultStr == "success" {
				s.logger.Info().Msg("Successfully clicked Google Authorize button using JavaScript")
				screenshotTaker.TakeScreenshot(page, "google_authorize_button_clicked")
				return nil
			}
			s.logger.Warn().Str("result", resultStr).Msg("Google JavaScript click failed or unexpected result")
		}

		return fmt.Errorf("failed to find Google Authorize button")
	}

	// Click the found button
	authButton = authButton.Timeout(5 * time.Second)
	err := authButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Google Authorize button: %w", err)
	}

	s.logger.Info().Msg("Successfully clicked Google Authorize button")
	screenshotTaker.TakeScreenshot(page, "google_authorize_button_clicked")
	return nil
}

// HandleLoginFlow implements Google-specific login flow (two-step: email first, then password)
func (s *GoogleProviderStrategy) HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Using Google two-step login flow")

	// Step 1: Handle email input (first step for Google)
	err := s.handleEmailInput(page, username, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle email input: %w", err)
	}

	// Step 2: Handle password input (second step for Google)
	err = s.handlePasswordInput(page, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle password input: %w", err)
	}

	return nil
}

// handleEmailInput handles the first step of Google login (email input)
func (s *GoogleProviderStrategy) handleEmailInput(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling email input (first step)")

	// Find and fill email field using Google-specific selectors
	emailSelectors := []string{
		`input[type="email"]`,
		`input[id="identifierId"]`, // Google email field ID
		`input[name="identifier"]`, // Google email field name
		`input[placeholder*="email"]`,
		`input[placeholder*="Email"]`,
	}

	var emailField *rod.Element
	var err error

	for _, selector := range emailSelectors {
		emailField, err = page.Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found Google email field")
			break
		}
	}

	if emailField == nil {
		return fmt.Errorf("could not find Google email field")
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

	screenshotTaker.TakeScreenshot(page, "google_email_entered")

	// Click Next button to proceed to password step
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Next button after email: %w", err)
	}

	// Wait for password page to load
	time.Sleep(2 * time.Second)
	screenshotTaker.TakeScreenshot(page, "google_password_page")

	return nil
}

// handlePasswordInput handles the second step of Google login (password input)
func (s *GoogleProviderStrategy) handlePasswordInput(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling password input (second step)")

	// Find and fill password field using Google-specific selectors
	passwordSelectors := []string{
		`input[type="password"]`,
		`input[name="password"]`,
		`input[placeholder*="password"]`,
		`input[placeholder*="Password"]`,
	}

	var passwordField *rod.Element
	var err error

	for _, selector := range passwordSelectors {
		passwordField, err = page.Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found Google password field")
			break
		}
	}

	if passwordField == nil {
		return fmt.Errorf("could not find Google password field")
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

	screenshotTaker.TakeScreenshot(page, "google_password_entered")

	// Click Next/Sign in button to proceed
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Next button after password: %w", err)
	}

	return nil
}
