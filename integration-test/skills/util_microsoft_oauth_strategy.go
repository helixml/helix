package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// MicrosoftProviderStrategy implements Microsoft-specific OAuth automation
type MicrosoftProviderStrategy struct {
	logger zerolog.Logger
}

// NewMicrosoftProviderStrategy creates a Microsoft provider strategy
func NewMicrosoftProviderStrategy(logger zerolog.Logger) *MicrosoftProviderStrategy {
	return &MicrosoftProviderStrategy{
		logger: logger,
	}
}

// ClickNextButton implements Microsoft-specific Next button clicking logic
func (s *MicrosoftProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Microsoft Next button")

	// Use a temporary page context to avoid affecting the main page timeout
	nextPage := page.Timeout(10 * time.Second)

	// Microsoft-specific button selectors (prioritize Microsoft patterns)
	nextSelectors := []string{
		`input[id="idSIButton9"]`,               // Microsoft-specific Next button ID
		`input[type="submit"][value="Next"]`,    // Microsoft input style
		`input[type="submit"][value="Sign in"]`, // Microsoft sign in button
		`button[id="identifierNext"]`,           // Google style (for compatibility)
		`button[type="submit"]`,                 // Generic submit button
	}

	var nextButton *rod.Element
	for _, selector := range nextSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Microsoft Next button selector")
		element, err := nextPage.Timeout(3 * time.Second).Element(selector)
		if err == nil && element != nil {
			// For Microsoft input elements, check value attribute
			if strings.Contains(selector, "input") && !strings.Contains(selector, "idSIButton9") {
				if value, valueErr := element.Attribute("value"); valueErr == nil && value != nil {
					valueText := strings.ToLower(strings.TrimSpace(*value))
					if valueText == "next" || valueText == "sign in" || valueText == "continue" {
						s.logger.Info().Str("selector", selector).Str("value", *value).Msg("Found Microsoft Next button by value")
						nextButton = element
						break
					}
				}
			} else {
				s.logger.Info().Str("selector", selector).Msg("Found Microsoft Next button by direct selector")
				nextButton = element
				break
			}
		}
	}

	// If direct selectors failed, try Microsoft-specific JavaScript approach
	if nextButton == nil {
		s.logger.Info().Msg("Direct selectors failed, using Microsoft-specific JavaScript to click Next button")

		// Microsoft-specific JavaScript that handles input[type="submit"] elements correctly
		jsCode := `
			(function() {
				// Check both button elements and input elements (Microsoft uses input[type="submit"])
				var buttons = document.querySelectorAll('button');
				var inputs = document.querySelectorAll('input[type="submit"]');
				
				// First check input elements (Microsoft's primary pattern)
				for (var i = 0; i < inputs.length; i++) {
					var text = (inputs[i].value || '').trim().toLowerCase();
					if (text === 'next' || text === 'sign in' || text === 'continue') {
						inputs[i].click();
						return 'success';
					}
				}
				
				// Then check button elements
				for (var i = 0; i < buttons.length; i++) {
					var text = (buttons[i].textContent || '').trim().toLowerCase();
					if (text === 'next' || text === 'sign in' || text === 'continue') {
						buttons[i].click();
						return 'success';
					}
				}
				
				return 'no_next_button_found';
			})();
		`

		result, err := page.Eval(jsCode)
		if err != nil {
			s.logger.Error().Err(err).Msg("Failed to execute Microsoft JavaScript click")
		} else {
			resultStr := result.Value.String()
			if resultStr == "success" {
				s.logger.Info().Msg("Successfully clicked Microsoft Next button using JavaScript")
				screenshotTaker.TakeScreenshot(page, "microsoft_next_button_clicked")
				return nil
			}
			s.logger.Warn().Str("result", resultStr).Msg("Microsoft JavaScript click failed or unexpected result")
		}

		return fmt.Errorf("failed to find Microsoft Next button")
	}

	// Click the found button
	nextButton = nextButton.Timeout(5 * time.Second)
	err := nextButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Microsoft Next button: %w", err)
	}

	s.logger.Info().Msg("Successfully clicked Microsoft Next button")
	screenshotTaker.TakeScreenshot(page, "microsoft_next_button_clicked")
	return nil
}

// HandleLoginFlow implements Microsoft-specific two-step login flow
func (s *MicrosoftProviderStrategy) HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Starting Microsoft two-step login flow")

	// Step 1: Handle email input
	err := s.handleEmailInput(page, username, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle Microsoft email input: %w", err)
	}

	// Step 2: Handle password input
	err = s.handlePasswordInput(page, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle Microsoft password input: %w", err)
	}

	return nil
}

// HandleAuthorization implements Microsoft-specific authorization handling
func (s *MicrosoftProviderStrategy) HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Microsoft authorization")

	// Find authorization button using Microsoft-specific logic
	authButtonElement, err := s.findMicrosoftAuthorizationButton(page)
	if err != nil {
		return fmt.Errorf("failed to find Microsoft authorization button: %w", err)
	}

	s.logger.Info().Msg("Found Microsoft authorization button")

	// Click the authorize button
	s.logger.Info().Msg("Clicking Microsoft authorize button")
	err = authButtonElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click Microsoft authorize button: %w", err)
	}

	// Take screenshot after successful authorization
	screenshotTaker.TakeScreenshot(page, "microsoft_authorize_button_clicked")

	return nil
}

// Helper methods for Microsoft strategy

func (s *MicrosoftProviderStrategy) handleEmailInput(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Microsoft email input step")

	// Fill username field using Microsoft selectors
	usernameElement, err := page.Element(`input[type="email"], input[name="loginfmt"]`)
	if err != nil {
		return fmt.Errorf("failed to find Microsoft username field: %w", err)
	}

	err = usernameElement.Input(username)
	if err != nil {
		return fmt.Errorf("failed to enter Microsoft username: %w", err)
	}

	s.logger.Info().Str("username", username).Msg("Successfully entered Microsoft username")
	screenshotTaker.TakeScreenshot(page, "microsoft_username_filled")

	// Wait a moment for form validation
	time.Sleep(1 * time.Second)

	// Click Next button to proceed to password step
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Next button after Microsoft username: %w", err)
	}

	return nil
}

func (s *MicrosoftProviderStrategy) handlePasswordInput(page *rod.Page, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Microsoft password input step")

	// Wait for password page to load
	time.Sleep(2 * time.Second)

	// Fill password field using Microsoft selectors
	passwordElement, err := page.Element(`input[type="password"], input[name="passwd"]`)
	if err != nil {
		return fmt.Errorf("failed to find Microsoft password field: %w", err)
	}

	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter Microsoft password: %w", err)
	}

	s.logger.Info().Msg("Successfully entered Microsoft password")
	screenshotTaker.TakeScreenshot(page, "microsoft_password_filled")

	// Click Sign in button
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Sign in button after Microsoft password: %w", err)
	}

	return nil
}

func (s *MicrosoftProviderStrategy) findMicrosoftAuthorizationButton(page *rod.Page) (*rod.Element, error) {
	s.logger.Info().Msg("Finding Microsoft authorization button")

	// Microsoft-specific authorization button selectors
	authSelectors := []string{
		`input[type="submit"][value="Accept"]`,
		`input[type="submit"][value="Allow"]`,
		`input[id="idSIButton9"]`, // Microsoft consent button
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	for _, selector := range authSelectors {
		element, err := page.Timeout(3 * time.Second).Element(selector)
		if err == nil && element != nil {
			// For input elements, check value attribute
			if strings.Contains(selector, "input") && !strings.Contains(selector, "idSIButton9") {
				if value, valueErr := element.Attribute("value"); valueErr == nil && value != nil {
					valueText := strings.ToLower(strings.TrimSpace(*value))
					if strings.Contains(valueText, "accept") || strings.Contains(valueText, "allow") || strings.Contains(valueText, "authorize") {
						s.logger.Info().Str("selector", selector).Str("value", *value).Msg("Found Microsoft authorization button by value")
						return element, nil
					}
				}
			} else {
				s.logger.Info().Str("selector", selector).Msg("Found Microsoft authorization button by direct selector")
				return element, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to find Microsoft authorization button")
}
