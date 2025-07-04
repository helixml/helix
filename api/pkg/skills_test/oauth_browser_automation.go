//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/rs/zerolog"
)

// OAuthBrowserAutomation handles browser automation for OAuth flows
type OAuthBrowserAutomation struct {
	browser           *rod.Browser
	ctx               context.Context
	logger            zerolog.Logger
	screenshotCounter int
	testTimestamp     string
	testResultsDir    string
	oauthManager      *oauth.Manager
}

// NewOAuthBrowserAutomation creates a new browser automation instance
func NewOAuthBrowserAutomation(browser *rod.Browser, ctx context.Context, logger zerolog.Logger, testTimestamp string, testResultsDir string, oauthManager *oauth.Manager) *OAuthBrowserAutomation {
	return &OAuthBrowserAutomation{
		browser:        browser,
		ctx:            ctx,
		logger:         logger,
		testTimestamp:  testTimestamp,
		testResultsDir: testResultsDir,
		oauthManager:   oauthManager,
	}
}

// StartOAuthFlow starts the OAuth flow and returns auth URL and state
func (ba *OAuthBrowserAutomation) StartOAuthFlow(userID, providerID, callbackURL string) (string, string, error) {
	ba.logger.Info().Msg("Starting OAuth flow via OAuth manager")

	authURL, err := ba.oauthManager.StartOAuthFlow(ba.ctx, userID, providerID, callbackURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to start OAuth flow: %w", err)
	}

	// Extract state parameter from the auth URL
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse auth URL: %w", err)
	}

	state := parsedURL.Query().Get("state")
	if state == "" {
		return "", "", fmt.Errorf("no state parameter in auth URL")
	}

	return authURL, state, nil
}

// CompleteOAuthFlow completes the OAuth flow using the authorization code
func (ba *OAuthBrowserAutomation) CompleteOAuthFlow(userID, providerID, authCode string) error {
	ba.logger.Info().Msg("Completing OAuth flow via OAuth manager")

	_, err := ba.oauthManager.CompleteOAuthFlow(ba.ctx, userID, providerID, authCode)
	if err != nil {
		return fmt.Errorf("failed to complete OAuth flow: %w", err)
	}

	return nil
}

// TakeScreenshot captures a screenshot and saves it with the test timestamp
func (ba *OAuthBrowserAutomation) TakeScreenshot(page *rod.Page, stepName string) {
	ba.screenshotCounter++
	filename := filepath.Join(ba.testResultsDir, fmt.Sprintf("oauth_e2e_%s_step_%02d_%s.png",
		ba.testTimestamp, ba.screenshotCounter, stepName))

	data, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})

	if err != nil {
		ba.logger.Error().Err(err).Str("filename", filename).Msg("Failed to take screenshot")
		return
	}

	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		ba.logger.Error().Err(err).Str("filename", filename).Msg("Failed to save screenshot")
	} else {
		ba.logger.Info().Str("filename", filename).Msg("Screenshot saved")
	}
}

// CreatePage creates a new browser page for OAuth automation
func (ba *OAuthBrowserAutomation) CreatePage() (*rod.Page, error) {
	page, err := ba.browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create browser page: %w", err)
	}
	return page, nil
}

// NavigateToOAuthURL navigates to the OAuth authorization URL and waits for page load
func (ba *OAuthBrowserAutomation) NavigateToOAuthURL(page *rod.Page, authURL string) error {
	ba.logger.Info().Str("url", authURL).Msg("Navigating to OAuth authorization URL")

	err := page.Navigate(authURL)
	if err != nil {
		return fmt.Errorf("failed to navigate to OAuth URL: %w", err)
	}

	// Wait for page to load
	err = page.WaitLoad()
	if err != nil {
		return fmt.Errorf("failed to wait for page load: %w", err)
	}

	ba.TakeScreenshot(page, "oauth_page_loaded")
	return nil
}

// WaitForCallback waits for OAuth callback and extracts authorization code
func (ba *OAuthBrowserAutomation) WaitForCallback(page *rod.Page, expectedState string, timeoutSeconds int) (string, error) {
	ba.logger.Info().Msg("Waiting for OAuth callback redirect")

	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			ba.TakeScreenshot(page, "oauth_callback_timeout")
			currentURL := page.MustInfo().URL
			return "", fmt.Errorf("timeout waiting for OAuth callback, current URL: %s", currentURL)
		case <-ticker.C:
			currentURL := page.MustInfo().URL
			if strings.Contains(currentURL, "/api/v1/oauth/callback") || strings.Contains(currentURL, "code=") {
				ba.logger.Info().Str("callback_url", currentURL).Msg("OAuth callback received")
				ba.TakeScreenshot(page, "oauth_callback_received")

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
				if callbackState != expectedState {
					return "", fmt.Errorf("state mismatch: expected %s, got %s", expectedState, callbackState)
				}

				ba.logger.Info().
					Str("auth_code", authCode[:10]+"...").
					Str("state", callbackState).
					Msg("Successfully extracted authorization code from callback")

				return authCode, nil
			}
		}
	}
}

// PerformLogin performs login on a provider's login page
func (ba *OAuthBrowserAutomation) PerformLogin(page *rod.Page, username, password string, loginConfig LoginConfig) error {
	ba.logger.Info().Msg("Performing login on provider page")

	// Check if we need to login
	loginElement, err := page.Element(loginConfig.UsernameSelector)
	if err != nil {
		// Already logged in, no login required
		ba.logger.Info().Msg("Already logged in - no login form found")
		return nil
	}

	ba.logger.Info().Msg("Login required - filling in credentials")

	// Fill in username
	err = loginElement.Input(username)
	if err != nil {
		return fmt.Errorf("failed to enter username: %w", err)
	}

	// Fill in password
	passwordElement, err := page.Element(loginConfig.PasswordSelector)
	if err != nil {
		return fmt.Errorf("failed to find password field: %w", err)
	}

	err = passwordElement.Input(password)
	if err != nil {
		return fmt.Errorf("failed to enter password: %w", err)
	}

	ba.TakeScreenshot(page, "login_filled")

	// Click login button
	loginButton, err := page.Element(loginConfig.LoginButtonSelector)
	if err != nil {
		return fmt.Errorf("failed to find login button: %w", err)
	}

	err = loginButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}

	ba.logger.Info().Msg("Clicked login button")
	ba.TakeScreenshot(page, "login_button_clicked")

	// Wait for login to complete
	ba.logger.Info().Msg("Waiting for page navigation after login")
	return ba.waitForNavigation(page, 10)
}

// ClickAuthorizeButton finds and clicks the OAuth authorization button
func (ba *OAuthBrowserAutomation) ClickAuthorizeButton(page *rod.Page, buttonSelector string) error {
	ba.logger.Info().Msg("Looking for authorization button")
	ba.TakeScreenshot(page, "authorization_page")

	// Wait for authorization button to be present
	authButtonElement, err := page.Element(buttonSelector)
	if err != nil {
		ba.TakeScreenshot(page, "auth_button_not_found")
		currentURL := page.MustInfo().URL
		return fmt.Errorf("could not find authorization button on page %s: %w", currentURL, err)
	}

	ba.logger.Info().Msg("Found authorization button")

	// Click the authorize button
	ba.logger.Info().Msg("Clicking authorize button")
	err = authButtonElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click authorize button: %w", err)
	}

	ba.TakeScreenshot(page, "authorize_button_clicked")
	return nil
}

// waitForNavigation waits for page navigation to complete
func (ba *OAuthBrowserAutomation) waitForNavigation(page *rod.Page, timeoutSeconds int) error {
	currentURL := page.MustInfo().URL
	timeout := time.After(time.Duration(timeoutSeconds) * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			ba.TakeScreenshot(page, "navigation_timeout")
			return fmt.Errorf("timeout waiting for page navigation")
		case <-ticker.C:
			newURL := page.MustInfo().URL
			if newURL != currentURL {
				ba.logger.Info().Str("old_url", currentURL).Str("new_url", newURL).Msg("Navigation detected")

				// Wait for page to fully load
				page.MustWaitLoad()
				time.Sleep(2 * time.Second) // Additional wait for dynamic content

				return nil
			}
		}
	}
}


// GetGitHubLoginConfig returns the login configuration for GitHub
func GetGitHubLoginConfig() LoginConfig {
	return LoginConfig{
		UsernameSelector:    `input[name="login"]`,
		PasswordSelector:    `input[name="password"]`,
		LoginButtonSelector: `input[type="submit"][value="Sign in"], button[type="submit"]`,
	}
}
