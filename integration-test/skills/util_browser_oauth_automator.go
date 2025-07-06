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

	// Create a new page for the OAuth flow
	page, err := a.browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return "", fmt.Errorf("failed to create browser page: %w", err)
	}
	defer page.Close()

	// Navigate to OAuth authorization URL
	a.logger.Info().Str("url", authURL).Msg("Navigating to OAuth authorization URL")
	err = page.Navigate(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to OAuth URL: %w", err)
	}

	// Wait for page to load
	err = page.WaitLoad()
	if err != nil {
		return "", fmt.Errorf("failed to wait for page load: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_loaded")

	// Check if we need to login
	loginRequired, err := a.checkLoginRequired(page)
	if err != nil {
		return "", fmt.Errorf("failed to check login requirement: %w", err)
	}

	if loginRequired {
		err = a.performLogin(page, username, password, screenshotTaker)
		if err != nil {
			return "", fmt.Errorf("failed to perform login: %w", err)
		}

		// Handle 2FA if required
		if a.config.TwoFactorHandler != nil && a.config.TwoFactorHandler.IsRequired(page) {
			err = a.config.TwoFactorHandler.Handle(page, a)
			if err != nil {
				return "", fmt.Errorf("failed to handle 2FA: %w", err)
			}
		}

		// Check if we need to navigate back to OAuth after login/2FA
		err = a.navigateBackToOAuthIfNeeded(page, authURL, screenshotTaker)
		if err != nil {
			return "", fmt.Errorf("failed to navigate back to OAuth: %w", err)
		}
	}

	// Check if already at callback (OAuth completed)
	authCode, completed := a.checkOAuthCompleted(page, state)
	if completed {
		return authCode, nil
	}

	// Look for and click authorization button
	err = a.performAuthorization(page, screenshotTaker)
	if err != nil {
		return "", fmt.Errorf("failed to perform authorization: %w", err)
	}

	// Wait for callback with authorization code
	return a.waitForCallback(page, state, screenshotTaker)
}

// checkLoginRequired checks if login is required
func (a *BrowserOAuthAutomator) checkLoginRequired(page *rod.Page) (bool, error) {
	a.logger.Info().Msg("Checking if login is required")

	// Wait a moment for the page to fully load
	time.Sleep(2 * time.Second)

	// Check if we need to login first
	loginElement, _ := page.Element(a.config.LoginUsernameSelector)
	return loginElement != nil, nil
}

// performLogin handles the login process
func (a *BrowserOAuthAutomator) performLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Login required - filling in credentials")

	// Fill in username
	usernameElement, err := page.Element(a.config.LoginUsernameSelector)
	if err != nil {
		return fmt.Errorf("failed to find username field: %w", err)
	}

	err = usernameElement.Input(username)
	if err != nil {
		return fmt.Errorf("failed to enter username: %w", err)
	}

	// Fill in password
	passwordElement := page.MustElement(a.config.LoginPasswordSelector)
	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter password: %w", err)
	}

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_filled")

	// Click login button
	loginButton := page.MustElement(a.config.LoginButtonSelector)
	err = loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	a.logger.Info().Msg("Clicked login button")

	// Wait for login navigation
	return a.waitForNavigation(page, screenshotTaker)
}

// waitForNavigation waits for page navigation after login
func (a *BrowserOAuthAutomator) waitForNavigation(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	a.logger.Info().Msg("Waiting for page navigation after login")

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_button_clicked")

	// Wait for URL to change (indicating navigation started)
	currentURL := page.MustInfo().URL
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	navigationStarted := false
	for !navigationStarted {
		select {
		case <-timeout:
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_login_timeout")
			return fmt.Errorf("timeout waiting for login navigation")
		case <-ticker.C:
			newURL := page.MustInfo().URL
			if newURL != currentURL {
				a.logger.Info().Str("old_url", currentURL).Str("new_url", newURL).Msg("Navigation detected")
				navigationStarted = true
			}
		}
	}

	// Wait for page to fully load after navigation
	a.logger.Info().Msg("Waiting for page to fully load")
	page.MustWaitLoad()

	// Additional small wait to ensure all dynamic content loads
	time.Sleep(2 * time.Second)

	screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_auth_page_after_login")

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

	if needsRedirect {
		a.logger.Info().Str("current_url", currentURL).Msg("Redirected away from OAuth, navigating back to OAuth URL")

		// Navigate back to the OAuth authorization URL
		err := page.Navigate(authURL)
		if err != nil {
			return fmt.Errorf("failed to re-navigate to OAuth URL: %w", err)
		}

		// Wait for page to load
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
	// First try the configured selector
	authButtonElement, err := page.Element(a.config.AuthorizeButtonSelector)
	if err == nil {
		// Check if the button text indicates it's an authorization button
		buttonText, textErr := authButtonElement.Text()
		if textErr == nil {
			buttonTextLower := strings.ToLower(buttonText)
			if strings.Contains(buttonTextLower, "authorize") && !strings.Contains(buttonTextLower, "cancel") && !strings.Contains(buttonTextLower, "deny") {
				a.logger.Info().Str("button_text", buttonText).Msg("Found authorization button with expected text")
				return authButtonElement, nil
			}
		}
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
		elements, err := page.Elements(selector)
		if err != nil {
			continue
		}

		for _, element := range elements {
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
				a.logger.Info().Str("button_text", buttonText).Str("selector", selector).Msg("Examining button")

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
			}
		}
	}

	return nil, fmt.Errorf("no suitable authorization button found")
}

// waitForCallback waits for redirect to callback URL with authorization code
func (a *BrowserOAuthAutomator) waitForCallback(page *rod.Page, state string, screenshotTaker ScreenshotTaker) (string, error) {
	a.logger.Info().Msg("Waiting for OAuth callback redirect")

	// Wait for navigation to callback URL (with authorization code)
	timeout := time.After(20 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			screenshotTaker.TakeScreenshot(page, a.config.ProviderName+"_oauth_timeout")
			currentURL := page.MustInfo().URL
			return "", fmt.Errorf("timeout waiting for OAuth callback, current URL: %s", currentURL)
		case <-ticker.C:
			currentURL := page.MustInfo().URL
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
