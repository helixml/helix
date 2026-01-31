package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// GitHubProviderStrategy implements GitHub-specific OAuth automation behavior
type GitHubProviderStrategy struct {
	logger zerolog.Logger
}

// NewGitHubProviderStrategy creates a new GitHub provider strategy
func NewGitHubProviderStrategy(logger zerolog.Logger) *GitHubProviderStrategy {
	return &GitHubProviderStrategy{
		logger: logger,
	}
}

// ClickNextButton implements GitHub-specific Next button logic (not typically used in GitHub flow)
func (s *GitHubProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for GitHub Next/Continue button")

	// Use a temporary page context to avoid affecting the main page timeout
	nextPage := page.Timeout(30 * time.Second)

	// GitHub-specific Next/Continue button selectors
	nextSelectors := []string{
		`button[type="submit"]`,
		`input[type="submit"][value="Continue"]`,
		`input[type="submit"][value="Next"]`,
		`button:contains("Continue")`,
		`button:contains("Next")`,
	}

	var lastError error
	for _, selector := range nextSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Next button selector")

		element, err := nextPage.Timeout(10 * time.Second).Element(selector)
		if err != nil {
			lastError = err
			time.Sleep(1 * time.Second)
			continue
		}

		// Click the button
		err = element.ScrollIntoView()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
		}

		err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click Next button")
			lastError = err
			time.Sleep(1 * time.Second)
			continue
		}

		s.logger.Info().Str("selector", selector).Msg("Successfully clicked GitHub Next button")
		screenshotTaker.TakeScreenshot(page, "github_next_button_clicked")

		time.Sleep(2 * time.Second)
		return nil
	}

	return fmt.Errorf("failed to find GitHub Next button: %w", lastError)
}

// HandleLoginFlow implements GitHub-specific single-step login flow
func (s *GitHubProviderStrategy) HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Starting GitHub single-step login flow")

	// GitHub uses single-step login (username + password together)
	err := s.handleSingleStepLogin(page, username, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle GitHub single-step login: %w", err)
	}

	return nil
}

// handleSingleStepLogin handles GitHub's single-step login process
func (s *GitHubProviderStrategy) handleSingleStepLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling GitHub single-step login")

	// Fill username field
	usernameSelectors := []string{
		`input[name="login"]`,
		`input[id="login_field"]`,
		`input[type="text"][placeholder*="username"]`,
		`input[type="email"]`,
	}

	var usernameField *rod.Element
	var err error

	for _, selector := range usernameSelectors {
		usernameField, err = page.Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found GitHub username field")
			break
		}
	}

	if usernameField == nil {
		return fmt.Errorf("could not find GitHub username field")
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
	passwordSelectors := []string{
		`input[name="password"]`,
		`input[id="password"]`,
		`input[type="password"]`,
	}

	var passwordField *rod.Element

	for _, selector := range passwordSelectors {
		passwordField, err = page.Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found GitHub password field")
			break
		}
	}

	if passwordField == nil {
		return fmt.Errorf("could not find GitHub password field")
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

	screenshotTaker.TakeScreenshot(page, "github_credentials_entered")

	// Click login button
	loginSelectors := []string{
		`input[type="submit"][value="Sign in"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:contains("Sign in")`,
		`button:contains("Log in")`,
	}

	var loginButton *rod.Element

	for _, selector := range loginSelectors {
		loginButton, err = page.Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found GitHub login button")
			break
		}
	}

	if loginButton == nil {
		return fmt.Errorf("could not find GitHub login button")
	}

	err = loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click login button: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, "github_login_button_clicked")

	// Wait for login to process
	time.Sleep(3 * time.Second)

	return nil
}

// HandleAuthorization implements GitHub-specific authorization page handling
func (s *GitHubProviderStrategy) HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling GitHub authorization page")

	return s.ClickAuthorizeButton(page, screenshotTaker)
}

// ClickAuthorizeButton implements GitHub-specific authorization button detection and clicking
func (s *GitHubProviderStrategy) ClickAuthorizeButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for GitHub authorization button")

	// Use a temporary page context to avoid affecting the main page timeout
	// GitHub auth pages can be slower, so use a longer timeout
	authPage := page.Timeout(45 * time.Second)

	// GitHub-specific authorization button selectors
	authSelectors := []string{
		// GitHub app authorization buttons
		`button[type="submit"][value="Authorize"]`,
		`input[type="submit"][value="Authorize"]`,
		`button[name="authorize"]`,
		`input[name="authorize"]`,

		// GitHub OAuth app authorization
		`button[data-octo-click="oauth_application_authorization"]`,
		`button[form="app-authorization"]`,

		// Generic GitHub buttons with authorization text
		`button:contains("Authorize")`,
		`button:contains("Grant access")`,
		`button:contains("Allow")`,

		// Form submit buttons (GitHub often uses forms)
		`form[id="app-authorization"] button[type="submit"]`,
		`form button[type="submit"]`,

		// Fallback selectors
		`button[type="submit"]`,
		`input[type="submit"]`,
	}

	var lastError error
	for _, selector := range authSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying GitHub authorization button selector")

		// For :contains selectors, need special handling
		if strings.Contains(selector, ":contains(") {
			// Extract the text to search for
			text := strings.TrimPrefix(selector, "button:contains(\"")
			text = strings.TrimSuffix(text, "\")")

			// Find all buttons and check their text
			buttons, err := authPage.Timeout(20 * time.Second).Elements("button")
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second)
				continue
			}

			for _, button := range buttons {
				buttonText, textErr := button.Timeout(5 * time.Second).Text()
				if textErr == nil {
					if strings.Contains(strings.ToLower(buttonText), strings.ToLower(text)) {
						s.logger.Info().Str("text", text).Str("button_text", buttonText).Msg("Found GitHub authorization button by text")

						err = button.ScrollIntoView()
						if err != nil {
							s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
						}

						err = button.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
						if err != nil {
							s.logger.Warn().Err(err).Str("text", text).Msg("Failed to click authorization button")
							lastError = err
							continue
						}

						s.logger.Info().Str("text", text).Msg("Successfully clicked GitHub authorization button")
						screenshotTaker.TakeScreenshot(page, "github_authorize_button_clicked")

						time.Sleep(2 * time.Second)
						return nil
					}
				}
			}
		} else {
			// Standard selector
			element, err := authPage.Timeout(20 * time.Second).Element(selector)
			if err != nil {
				lastError = err
				time.Sleep(1 * time.Second)
				continue
			}

			// For generic submit buttons, check if they're in an authorization context
			if selector == `button[type="submit"]` || selector == `input[type="submit"]` {
				// Check if the button is in an authorization form or has authorization-related text
				text, textErr := element.Timeout(5 * time.Second).Text()
				if textErr == nil {
					textLower := strings.ToLower(strings.TrimSpace(text))
					if !strings.Contains(textLower, "authorize") &&
						!strings.Contains(textLower, "allow") &&
						!strings.Contains(textLower, "grant") &&
						!strings.Contains(textLower, "accept") {
						s.logger.Info().Str("selector", selector).Str("text", text).Msg("Button found but doesn't contain authorization text")
						time.Sleep(1 * time.Second)
						continue
					}
				}
			}

			// Try to click the element
			s.logger.Info().Str("selector", selector).Msg("Found GitHub authorization button, attempting to click")

			err = element.ScrollIntoView()
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
			}

			err = element.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click authorization button")
				lastError = err
				time.Sleep(1 * time.Second)
				continue
			}

			s.logger.Info().Str("selector", selector).Msg("Successfully clicked GitHub authorization button")
			screenshotTaker.TakeScreenshot(page, "github_authorize_button_clicked")

			time.Sleep(2 * time.Second)
			return nil
		}
	}

	// If no selector worked, return the last error
	return fmt.Errorf("failed to find GitHub authorization button: %w", lastError)
}
