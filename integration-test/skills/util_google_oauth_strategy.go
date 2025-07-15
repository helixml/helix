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

	// Take screenshot before looking for Next button
	screenshotTaker.TakeScreenshot(page, "google_next_button_search_start")

	// Use a temporary page context to avoid affecting the main page timeout
	// Google OAuth pages can be slow, so use a longer timeout
	nextPage := page.Timeout(45 * time.Second)

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
			elements, err := nextPage.Timeout(20 * time.Second).Elements(selector)
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
			element, err := nextPage.Timeout(20 * time.Second).Element(selector)
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

	// Take screenshot before looking for Authorize button
	screenshotTaker.TakeScreenshot(page, "google_authorize_button_search_start")

	// Use a temporary page context to avoid affecting the main page timeout
	// OAuth consent pages can be slower, so use a longer timeout
	authPage := page.Timeout(45 * time.Second)

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
			elements, err := authPage.Timeout(20 * time.Second).Elements(selector)
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
			element, err := authPage.Timeout(20 * time.Second).Element(selector)
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

	// Take screenshot at start of email input step
	screenshotTaker.TakeScreenshot(page, "google_email_step_start")

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
		screenshotTaker.TakeScreenshot(page, "google_email_field_not_found")
		return fmt.Errorf("could not find Google email field")
	}

	// Take screenshot after finding email field
	screenshotTaker.TakeScreenshot(page, "google_email_field_found")

	err = emailField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_email_field_click_failed")
		return fmt.Errorf("could not click email field: %w", err)
	}

	err = emailField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in email field: %w", err)
	}

	err = emailField.Input(username)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_email_input_failed")
		return fmt.Errorf("could not input email: %w", err)
	}

	// Take screenshot after filling email field
	screenshotTaker.TakeScreenshot(page, "google_email_filled")

	// Click Next button to proceed to password step
	s.logger.Info().Msg("Attempting to click Next button after email")
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_next_button_click_failed")
		return fmt.Errorf("failed to click Next button after email: %w", err)
	}

	// Take screenshot after clicking Next button
	screenshotTaker.TakeScreenshot(page, "google_next_button_clicked")

	// Wait for next page to load
	time.Sleep(2 * time.Second)
	screenshotTaker.TakeScreenshot(page, "google_after_email_next")

	// Check if we're on an account selection page
	// Handle account selection gracefully - if it fails, continue with normal flow
	err = s.handleAccountSelection(page, username, screenshotTaker)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Account selection failed, continuing with normal password flow")
		// Don't return error - let the normal password flow continue
	}

	// Wait for password page to load
	time.Sleep(2 * time.Second)
	screenshotTaker.TakeScreenshot(page, "google_password_page")

	return nil
}

// handleAccountSelection handles the Google account selection page that may appear after email input
func (s *GoogleProviderStrategy) handleAccountSelection(page *rod.Page, username string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Checking for Google account selection page")

	// Set overall timeout for account selection to prevent it from running too long
	startTime := time.Now()
	accountSelectionTimeout := 30 * time.Second

	// Check if we're on an account selection page
	currentURL := page.MustInfo().URL
	if !strings.Contains(currentURL, "accounts.google.com") {
		s.logger.Info().Str("url", currentURL).Msg("Not on Google accounts page, skipping account selection")
		return nil
	}

	// Take screenshot before account selection validation
	screenshotTaker.TakeScreenshot(page, "google_account_selection_validation")

	// Additional validation: check for account selection page indicators
	// Look for text that would only appear on account selection pages
	bodyElement, bodyErr := page.Timeout(2 * time.Second).Element("body")
	if bodyErr == nil {
		pageText, textErr := bodyElement.Text()
		if textErr == nil {
			pageTextLower := strings.ToLower(pageText)
			// If we don't see typical account selection text, assume we're on password page
			if !strings.Contains(pageTextLower, "choose an account") &&
				!strings.Contains(pageTextLower, "select an account") &&
				!strings.Contains(pageTextLower, "which account") &&
				!strings.Contains(pageTextLower, "choose your account") {
				s.logger.Info().Str("url", currentURL).Msg("No account selection text found, assuming password page")
				screenshotTaker.TakeScreenshot(page, "google_no_account_selection_text")
				return nil
			}
			s.logger.Info().Msg("Found account selection text, proceeding with account selection")
			screenshotTaker.TakeScreenshot(page, "google_account_selection_confirmed")
		}
	}

	// Look for account selection indicators - use only very specific selectors
	// to avoid matching elements on password page or other pages
	accountSelectors := []string{
		// Modern Google account selection UI selectors (2024+)
		`div[data-email]`,                    // Account divs with email data attribute
		`div[data-identifier]`,               // Account divs with identifier data attribute
		`button[data-email]`,                 // Account buttons with email
		`div[data-account-id]`,               // Account divs with account ID
		`li[data-account-id]`,                // Account list items with account ID
		`div[jscontroller][data-email]`,      // JS controller divs with email
		`div[jscontroller][data-identifier]`, // JS controller divs with identifier

		// Additional selectors for modern Google UI
		`div[role="listitem"]`,   // Account list items
		`li[role="option"]`,      // Account options in dropdown
		`div[role="button"]`,     // Account buttons
		`div[data-value]`,        // Account divs with data-value
		`div[jsname]`,            // JS name divs (Google internal)
		`div[jsaction*="click"]`, // Clickable divs with jsaction

		// Fallback selectors for account elements
		`div[aria-label*="account"]`,    // Account divs with aria-label
		`div[aria-label*="Account"]`,    // Account divs with aria-label (capitalized)
		`button[aria-label*="account"]`, // Account buttons with aria-label
		`div[title*="@"]`,               // Elements with email in title
		`div[data-identifier*="@"]`,     // Elements with email in data-identifier
		`div[data-email*="@"]`,          // Elements with email in data-email
	}

	var accountElements []*rod.Element

	// Try to find account elements with reduced timeout to avoid accumulating delays
	for _, selector := range accountSelectors {
		// Check overall timeout
		if time.Since(startTime) > accountSelectionTimeout {
			s.logger.Warn().Msg("Account selection timeout exceeded, skipping")
			break
		}

		elements, elemErr := page.Timeout(2 * time.Second).Elements(selector)
		if elemErr == nil && len(elements) > 0 {
			s.logger.Info().Str("selector", selector).Int("count", len(elements)).Msg("Found account selection elements")
			accountElements = elements
			break
		}
	}

	if len(accountElements) == 0 {
		s.logger.Info().Msg("No account selection elements found, assuming password page")
		screenshotTaker.TakeScreenshot(page, "google_no_account_elements_found")
		return nil
	}

	// Take screenshot after finding account elements
	screenshotTaker.TakeScreenshot(page, "google_account_elements_found")

	// Enhanced account matching logic
	s.logger.Info().Str("target_username", username).Msg("Looking for account matching target username")

	// Look for the account matching our username
	for i, element := range accountElements {
		// Check overall timeout
		if time.Since(startTime) > accountSelectionTimeout {
			s.logger.Warn().Msg("Account selection timeout exceeded during account matching")
			break
		}

		s.logger.Info().Int("element_index", i).Msg("Checking account element")

		// Method 1: Check if element text contains our email
		text, textErr := element.Text()
		if textErr == nil && text != "" {
			s.logger.Info().Str("element_text", text).Msg("Found element text")
			if strings.Contains(strings.ToLower(text), strings.ToLower(username)) {
				s.logger.Info().Str("text", text).Str("username", username).Msg("Found matching account by text, clicking")
				return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_text")
			}
		}

		// Method 2: Check data-email attribute
		dataEmail, dataErr := element.Attribute("data-email")
		if dataErr == nil && dataEmail != nil && *dataEmail != "" {
			s.logger.Info().Str("data_email", *dataEmail).Msg("Found data-email attribute")
			if strings.EqualFold(*dataEmail, username) {
				s.logger.Info().Str("data-email", *dataEmail).Str("username", username).Msg("Found matching account by data-email, clicking")
				return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_data_email")
			}
		}

		// Method 3: Check data-identifier attribute
		dataIdentifier, identifierErr := element.Attribute("data-identifier")
		if identifierErr == nil && dataIdentifier != nil && *dataIdentifier != "" {
			s.logger.Info().Str("data_identifier", *dataIdentifier).Msg("Found data-identifier attribute")
			if strings.EqualFold(*dataIdentifier, username) {
				s.logger.Info().Str("data-identifier", *dataIdentifier).Str("username", username).Msg("Found matching account by data-identifier, clicking")
				return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_data_identifier")
			}
		}

		// Method 4: Check for child elements that might contain the email
		childElements, childErr := element.Elements("*")
		if childErr == nil {
			for _, child := range childElements {
				childText, childTextErr := child.Text()
				if childTextErr == nil && childText != "" {
					if strings.Contains(strings.ToLower(childText), strings.ToLower(username)) {
						s.logger.Info().Str("child_text", childText).Str("username", username).Msg("Found matching account by child text, clicking")
						return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_child_text")
					}
				}
			}
		}

		// Method 5: Check aria-label for email
		ariaLabel, ariaErr := element.Attribute("aria-label")
		if ariaErr == nil && ariaLabel != nil && *ariaLabel != "" {
			s.logger.Info().Str("aria_label", *ariaLabel).Msg("Found aria-label attribute")
			if strings.Contains(strings.ToLower(*ariaLabel), strings.ToLower(username)) {
				s.logger.Info().Str("aria-label", *ariaLabel).Str("username", username).Msg("Found matching account by aria-label, clicking")
				return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_aria_label")
			}
		}

		// Method 6: Check title attribute for email
		title, titleErr := element.Attribute("title")
		if titleErr == nil && title != nil && *title != "" {
			s.logger.Info().Str("title", *title).Msg("Found title attribute")
			if strings.Contains(strings.ToLower(*title), strings.ToLower(username)) {
				s.logger.Info().Str("title", *title).Str("username", username).Msg("Found matching account by title, clicking")
				return s.clickAccountElement(element, screenshotTaker, "google_account_selected_by_title")
			}
		}
	}

	// If we couldn't find the exact account, try clicking the first account
	if len(accountElements) > 0 {
		s.logger.Info().Msg("Could not find exact account match, trying first account")
		return s.clickAccountElement(accountElements[0], screenshotTaker, "google_first_account_selected")
	}

	s.logger.Info().Msg("No suitable account found or failed to click, continuing with flow")
	return nil
}

// clickAccountElement is a helper method to click an account element with proper error handling
func (s *GoogleProviderStrategy) clickAccountElement(element *rod.Element, screenshotTaker ScreenshotTaker, screenshotName string) error {
	err := element.ScrollIntoView()
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to scroll account element into view")
	}

	// Try to click the element
	err = element.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to click account element, trying alternative click")
		// Try clicking with different approach
		element.MustClick()
		s.logger.Info().Msg("Used alternative click method")
	}

	s.logger.Info().Msg("Successfully clicked account selection")

	// Take screenshot by getting the page from the element
	page := element.Page()
	screenshotTaker.TakeScreenshot(page, screenshotName)

	// Wait for the page to navigate after account selection
	time.Sleep(3 * time.Second)
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

	// Take screenshot at start of password input step
	screenshotTaker.TakeScreenshot(page, "google_password_step_start")

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
		screenshotTaker.TakeScreenshot(page, "google_password_field_not_found")
		return fmt.Errorf("could not find Google password field")
	}

	// Take screenshot after finding password field
	screenshotTaker.TakeScreenshot(page, "google_password_field_found")

	err = passwordField.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_password_field_click_failed")
		return fmt.Errorf("could not click password field: %w", err)
	}

	err = passwordField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in password field: %w", err)
	}

	err = passwordField.Input(password)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_password_input_failed")
		return fmt.Errorf("could not input password: %w", err)
	}

	// Take screenshot after filling password field
	screenshotTaker.TakeScreenshot(page, "google_password_filled")

	// Click Next/Sign in button to proceed
	s.logger.Info().Msg("Attempting to click Next/Sign in button after password")
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		screenshotTaker.TakeScreenshot(page, "google_password_next_button_click_failed")
		return fmt.Errorf("failed to click Next button after password: %w", err)
	}

	// Take screenshot after clicking Next/Sign in button
	screenshotTaker.TakeScreenshot(page, "google_password_next_button_clicked")

	return nil
}
