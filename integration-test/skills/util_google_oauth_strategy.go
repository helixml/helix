package skills

import (
	"fmt"
	"strings"
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

	// Set longer timeout for operations - increased from 10 seconds for better reliability
	page = page.Timeout(45 * time.Second)

	// Use modern Google button selectors based on debug output - prioritize class-based selectors
	nextButtonSelectors := []string{
		// Modern Google button class patterns first (from actual debug output)
		`button.VfPpkd-LgbsSe.nCP5yc`, // Core Google button classes that appear consistently
		`button.VfPpkd-LgbsSe`,        // Fallback to core Google button class
		`button[id="identifierNext"]`, // Legacy Google identifier Next button
		`button[id="passwordNext"]`,   // Legacy Google password Next button
		`button[type="submit"]`,       // Generic submit button
		`input[type="submit"]`,        // Input submit button
	}

	var lastError error
	for _, selector := range nextButtonSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Next button selector")

		if strings.Contains(selector, "VfPpkd-LgbsSe") {
			// For class-based selectors, find all matching elements and check for "Next" text
			elements, err := page.Timeout(20 * time.Second).Elements(selector)
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time
				continue
			}

			// Check each element for "Next" text
			for _, element := range elements {
				text, textErr := element.Timeout(10 * time.Second).Text()
				if textErr == nil {
					textLower := strings.ToLower(strings.TrimSpace(text))
					if textLower == "next" || textLower == "continue" {
						s.logger.Info().Str("selector", selector).Str("text", text).Msg("Found Google Next button by class and text")

						// Click this button
						err = element.ScrollIntoView()
						if err != nil {
							s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
						}

						err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
						if err != nil {
							s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click Next button")
							lastError = err
							break // Try next selector
						}

						s.logger.Info().Str("selector", selector).Msg("Successfully clicked Google Next button")
						screenshotTaker.TakeScreenshot(page, "google_next_button_clicked")

						// Wait a moment for the click to register
						time.Sleep(2 * time.Second)
						return nil
					}
				}
			}
		} else {
			// For ID and submit selectors, try direct match
			element, err := page.Timeout(20 * time.Second).Element(selector)
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time
				continue
			}

			// For generic submit buttons, verify text contains "Next"
			if strings.Contains(selector, "submit") {
				text, textErr := element.Timeout(5 * time.Second).Text()
				if textErr != nil || !strings.Contains(strings.ToLower(text), "next") {
					s.logger.Info().Str("selector", selector).Str("text", text).Msg("Button found but doesn't contain 'Next' text")
					time.Sleep(1 * time.Second) // Reduced sleep time
					continue
				}
			}

			// Try to click the element
			s.logger.Info().Str("selector", selector).Msg("Found Next button, attempting to click")

			err = element.ScrollIntoView()
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
			}

			err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click Next button")
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time
				continue
			}

			s.logger.Info().Str("selector", selector).Msg("Successfully clicked Google Next button")
			screenshotTaker.TakeScreenshot(page, "google_next_button_clicked")

			// Wait a moment for the click to register
			time.Sleep(2 * time.Second)
			return nil
		}
	}

	// If no selector worked, return the last error
	return fmt.Errorf("failed to find Next button: %w", lastError)
}

// ClickAuthorizeButton implements Google-specific Authorize button clicking logic
func (s *GoogleProviderStrategy) ClickAuthorizeButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Google Authorize button")

	// Set longer timeout for operations - OAuth consent pages can be slower
	// Use 45 seconds per element operation to allow for page loading delays
	page = page.Timeout(45 * time.Second)

	// Use modern Google button selectors - same pattern that worked for Next button
	authSelectors := []string{
		// Modern Google button class patterns first (same as Next button)
		`button.VfPpkd-LgbsSe.nCP5yc`,         // Core Google button classes
		`button.VfPpkd-LgbsSe`,                // Fallback to core Google button class
		`input[type="submit"][value="Allow"]`, // Legacy input style Allow button
		`button[type="submit"]`,               // Generic submit button
		`button[data-l*="allow"]`,             // Button with allow data attribute
	}

	var lastError error
	for _, selector := range authSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying authorization button selector")

		if strings.Contains(selector, "VfPpkd-LgbsSe") {
			// For class-based selectors, find all matching elements and check for authorization text
			elements, err := page.Timeout(20 * time.Second).Elements(selector)
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time from 3 seconds to 1 second
				continue
			}

			// Check each element for authorization text
			for _, element := range elements {
				text, textErr := element.Timeout(10 * time.Second).Text()
				if textErr == nil {
					textLower := strings.ToLower(strings.TrimSpace(text))
					if textLower == "allow" || textLower == "authorize" ||
						textLower == "consent" || textLower == "continue" || textLower == "accept" {
						s.logger.Info().Str("selector", selector).Str("text", text).Msg("Found Google authorization button by class and text")

						// Click this button
						err = element.ScrollIntoView()
						if err != nil {
							s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
						}

						err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
						if err != nil {
							s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click authorization button")
							lastError = err
							break // Try next selector
						}

						s.logger.Info().Str("selector", selector).Msg("Successfully clicked Google authorization button")
						screenshotTaker.TakeScreenshot(page, "google_authorize_button_clicked")

						// Wait a moment for the click to register
						time.Sleep(2 * time.Second)
						return nil
					}
				}
			}
		} else {
			// For legacy selectors, try direct match
			element, err := page.Timeout(20 * time.Second).Element(selector)
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time from 3 seconds to 1 second
				continue
			}

			// For generic submit buttons, check if text contains allow/consent related words
			if selector == `button[type="submit"]` {
				text, textErr := element.Timeout(5 * time.Second).Text()
				if textErr == nil {
					textLower := strings.ToLower(strings.TrimSpace(text))
					if !strings.Contains(textLower, "allow") && !strings.Contains(textLower, "authorize") &&
						!strings.Contains(textLower, "consent") && !strings.Contains(textLower, "continue") {
						s.logger.Info().Str("selector", selector).Str("text", text).Msg("Button found but doesn't contain authorization text")
						time.Sleep(1 * time.Second) // Reduced sleep time
						continue
					}
				}
			}

			// Try to click the element
			s.logger.Info().Str("selector", selector).Msg("Found authorization button, attempting to click")

			err = element.ScrollIntoView()
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
			}

			err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click authorization button")
				lastError = err
				time.Sleep(1 * time.Second) // Reduced sleep time
				continue
			}

			s.logger.Info().Str("selector", selector).Msg("Successfully clicked Google authorization button")
			screenshotTaker.TakeScreenshot(page, "google_authorize_button_clicked")

			// Wait a moment for the click to register
			time.Sleep(2 * time.Second)
			return nil
		}
	}

	// If no selector worked, return the last error
	return fmt.Errorf("failed to find authorization button: %w", lastError)
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

// HandleAuthorization implements provider-specific authorization page handling
func (s *GoogleProviderStrategy) HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Google authorization page")

	// For Google, we can try to click the authorization/consent button
	return s.ClickAuthorizeButton(page, screenshotTaker)
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
