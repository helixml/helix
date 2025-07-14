package skills

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// AtlassianProviderStrategy implements Atlassian-specific OAuth automation behavior
type AtlassianProviderStrategy struct {
	logger zerolog.Logger
}

// NewAtlassianProviderStrategy creates a new Atlassian provider strategy
func NewAtlassianProviderStrategy(logger zerolog.Logger) *AtlassianProviderStrategy {
	return &AtlassianProviderStrategy{
		logger: logger,
	}
}

// ClickNextButton implements Atlassian-specific Next button logic
func (s *AtlassianProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Atlassian Next/Continue button")

	// Set longer timeout for operations - Atlassian pages can be very slow
	page = page.Timeout(90 * time.Second)

	// Atlassian-specific button selectors
	nextSelectors := []string{
		`button[type="submit"][id="login-submit"]`, // Atlassian login submit button
		`button[type="submit"]`,                    // Generic submit button
		`input[type="submit"]`,                     // Input submit button
		`button:contains("Continue")`,              // Continue button
		`button:contains("Log in")`,                // Log in button
		`button:contains("Sign in")`,               // Sign in button
	}

	var lastError error

	for _, selector := range nextSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying Atlassian Next button selector")

		element, err := page.Timeout(30 * time.Second).Element(selector)
		if err != nil {
			lastError = err
			time.Sleep(1 * time.Second)
			continue
		}

		// Check if element is visible and clickable
		visible, visErr := element.Visible()
		if visErr != nil || !visible {
			s.logger.Info().Str("selector", selector).Msg("Element not visible, skipping")
			time.Sleep(1 * time.Second)
			continue
		}

		// Scroll element into view
		err = element.ScrollIntoView()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
		}

		// Click the button
		err = element.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click Next button")
			lastError = err
			time.Sleep(1 * time.Second)
			continue
		}

		s.logger.Info().Str("selector", selector).Msg("Successfully clicked Atlassian Next button")
		screenshotTaker.TakeScreenshot(page, "atlassian_next_button_clicked")

		time.Sleep(3 * time.Second)
		return nil
	}

	return fmt.Errorf("failed to find Atlassian Next button: %w", lastError)
}

// HandleLoginFlow implements Atlassian-specific single-step login flow
func (s *AtlassianProviderStrategy) HandleLoginFlow(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Starting Atlassian single-step login flow")

	// Atlassian uses single-step login (username + password together)
	err := s.handleSingleStepLogin(page, username, password, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to handle Atlassian single-step login: %w", err)
	}

	return nil
}

// handleSingleStepLogin handles Atlassian's single-step login process
func (s *AtlassianProviderStrategy) handleSingleStepLogin(page *rod.Page, username, password string, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Atlassian single-step login")

	// Set longer timeout for all operations - Atlassian pages can be very slow
	page = page.Timeout(90 * time.Second)

	// Fill username field
	usernameSelectors := []string{
		`input[type="email"]`,
		`input[name="username"]`,
		`input[id="username"]`,
		`input[placeholder*="email"]`,
		`input[placeholder*="Email"]`,
	}

	var usernameField *rod.Element
	var err error

	for _, selector := range usernameSelectors {
		usernameField, err = page.Timeout(30 * time.Second).Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found Atlassian username field")
			break
		}
	}

	if usernameField == nil {
		return fmt.Errorf("could not find Atlassian username field")
	}

	err = usernameField.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
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

	s.logger.Info().Msg("Successfully entered Atlassian username")

	// Atlassian uses a two-step login flow: username first, then click Continue, then password
	s.logger.Info().Msg("Clicking Continue button to proceed to password step")

	// Click the Continue button to proceed to the password step
	err = s.ClickNextButton(page, screenshotTaker)
	if err != nil {
		return fmt.Errorf("failed to click Continue button after username: %w", err)
	}

	// Wait for the password page to load
	time.Sleep(3 * time.Second)
	s.logger.Info().Msg("Waiting for password page to load after clicking Continue")

	// Fill password field with more specific selector and better error handling
	passwordSelectors := []string{
		`input[id="password"]`,           // Most specific - use the ID from debug output
		`input[name="password"]`,         // Fallback to name
		`input[type="password"]`,         // Generic type selector
		`input[placeholder*="password"]`, // Placeholder match
		`input[placeholder*="Password"]`, // Capitalized placeholder match
	}

	var passwordField *rod.Element

	for _, selector := range passwordSelectors {
		passwordField, err = page.Timeout(30 * time.Second).Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found Atlassian password field")
			break
		}
	}

	if passwordField == nil {
		return fmt.Errorf("could not find Atlassian password field")
	}

	// Add extra wait and more careful handling for password field click
	s.logger.Info().Msg("Attempting to click Atlassian password field with extended timeout and visibility checks")
	time.Sleep(2 * time.Second) // Give the page more time to settle

	// Wait for password field to become visible (with timeout)
	var visible bool
	var visErr error

	// Try waiting for visibility for up to 30 seconds
	for i := 0; i < 30; i++ {
		visible, visErr = passwordField.Visible()
		if visErr == nil && visible {
			s.logger.Info().Int("wait_seconds", i).Msg("Password field is now visible")
			break
		}
		if i < 29 {
			s.logger.Info().Int("attempt", i+1).Bool("visible", visible).Msg("Password field not yet visible, waiting...")
			time.Sleep(1 * time.Second)
		}
	}

	if visErr != nil {
		s.logger.Warn().Err(visErr).Msg("Failed to check password field visibility")
	} else if !visible {
		s.logger.Warn().Msg("Password field is still not visible after waiting, trying to make it visible")
		err = passwordField.ScrollIntoView()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to scroll password field into view")
		}
		time.Sleep(1 * time.Second)
	}

	s.logger.Info().Bool("visible", visible).Msg("Password field visibility status")

	// Try clicking with a fresh element reference and different approaches
	err = passwordField.Timeout(60*time.Second).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Warn().Err(err).Msg("First click attempt failed, trying alternative method")

		// Try alternative click methods
		err = passwordField.Focus()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to focus password field")
		} else {
			s.logger.Info().Msg("Successfully focused password field")
		}

		// Try clicking again after focus
		err = passwordField.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			return fmt.Errorf("could not click password field after multiple attempts: %w", err)
		}
	}

	err = passwordField.SelectAllText()
	if err != nil {
		return fmt.Errorf("could not select all text in password field: %w", err)
	}

	err = passwordField.Input(password)
	if err != nil {
		return fmt.Errorf("could not input password: %w", err)
	}

	s.logger.Info().Msg("Successfully entered Atlassian password")
	screenshotTaker.TakeScreenshot(page, "atlassian_credentials_entered")

	// Click login button
	loginSelectors := []string{
		`button[type="submit"][id="login-submit"]`,
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:contains("Continue")`,
		`button:contains("Log in")`,
		`button:contains("Sign in")`,
	}

	var loginButton *rod.Element

	for _, selector := range loginSelectors {
		loginButton, err = page.Timeout(30 * time.Second).Element(selector)
		if err == nil {
			s.logger.Info().Str("selector", selector).Msg("Found Atlassian login button")
			break
		}
	}

	if loginButton == nil {
		return fmt.Errorf("could not find Atlassian login button")
	}

	err = loginButton.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("could not click login button: %w", err)
	}

	s.logger.Info().Msg("Successfully clicked Atlassian login button")
	screenshotTaker.TakeScreenshot(page, "atlassian_login_button_clicked")

	// Wait for login to process
	time.Sleep(5 * time.Second)

	return nil
}

// HandleAuthorization implements Atlassian-specific authorization page handling
func (s *AtlassianProviderStrategy) HandleAuthorization(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Handling Atlassian authorization page")

	return s.ClickAuthorizeButton(page, screenshotTaker)
}

// ClickAuthorizeButton implements Atlassian-specific authorization button detection and clicking
func (s *AtlassianProviderStrategy) ClickAuthorizeButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Atlassian Authorize button")

	// Set longer timeout for operations
	page = page.Timeout(90 * time.Second)

	// First, wait for the page to fully load and take a screenshot for debugging
	time.Sleep(3 * time.Second)
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_consent_page_loaded")
	}

	// Debug: dump all buttons on the consent page
	buttons, err := page.Elements("button")
	if err == nil {
		s.logger.Info().Int("count", len(buttons)).Msg("Found button elements on consent page")
		for i, button := range buttons {
			buttonType, _ := button.Attribute("type")
			buttonID, _ := button.Attribute("id")
			buttonClass, _ := button.Attribute("class")
			buttonText, _ := button.Text()
			buttonTestID, _ := button.Attribute("data-testid")

			// Safe string dereferencing to avoid nil pointer crashes
			typeStr := ""
			if buttonType != nil {
				typeStr = *buttonType
			}
			idStr := ""
			if buttonID != nil {
				idStr = *buttonID
			}
			classStr := ""
			if buttonClass != nil {
				classStr = *buttonClass
			}
			testIDStr := ""
			if buttonTestID != nil {
				testIDStr = *buttonTestID
			}

			s.logger.Info().
				Int("index", i).
				Str("type", typeStr).
				Str("id", idStr).
				Str("class", classStr).
				Str("text", buttonText).
				Str("data-testid", testIDStr).
				Msg("Button element found on consent page")
		}
	}

	// Atlassian-specific authorization button selectors - updated based on actual consent page
	authSelectors := []string{
		`button[data-testid="permission-button"]`,      // Primary Atlassian consent permission buttons
		`button[id*="permission-btn"]`,                 // Atlassian permission buttons by ID pattern
		`button[class*="css-ypfr33"]`,                  // Atlassian permission buttons by class
		`button[data-testid*="permission"]`,            // Any button with permission in test id
		`button[data-testid="consent-approve-button"]`, // Fallback: main consent approve button
		`button[data-testid="approve-button"]`,         // Fallback: alternative consent button
		`button[data-testid*="approve"]`,               // Any button with approve in test id
		`button[data-testid*="consent"]`,               // Any button with consent in test id
		`button:contains("Accept")`,                    // Accept button
		`button:contains("Allow")`,                     // Allow button
		`button:contains("Authorize")`,                 // Authorize button
		`button:contains("Approve")`,                   // Approve button
		`button:contains("Continue")`,                  // Continue button
		`button[type="submit"]`,                        // Generic submit button
		`input[type="submit"]`,                         // Input submit button
		`button[id*="authorize"]`,                      // Button with authorize in ID
		`button[id*="approve"]`,                        // Button with approve in ID
		`button[class*="authorize"]`,                   // Button with authorize in class
		`button[class*="approve"]`,                     // Button with approve in class
	}

	// Special handling for Atlassian consent: need to click ALL permission buttons first
	permissionSelector := `button[data-testid="permission-button"]`
	s.logger.Info().Str("selector", permissionSelector).Msg("Looking for ALL Atlassian permission buttons")

	permissionButtons, err := page.Elements(permissionSelector)
	if err == nil && len(permissionButtons) > 0 {
		s.logger.Info().Int("count", len(permissionButtons)).Msg("Found multiple permission buttons - need to click ALL of them")

		clickedCount := 0
		for i, button := range permissionButtons {
			// Check if this button is visible and clickable
			visible, visErr := button.Visible()
			if visErr != nil || !visible {
				s.logger.Info().Int("index", i).Msg("Permission button not visible, skipping")
				continue
			}

			s.logger.Info().Int("index", i).Msg("Clicking permission button")

			err = button.ScrollIntoView()
			if err != nil {
				s.logger.Warn().Err(err).Int("index", i).Msg("Failed to scroll permission button into view")
			}

			err = button.Timeout(20*time.Second).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				s.logger.Warn().Err(err).Int("index", i).Msg("Failed to click permission button")
				continue
			}

			s.logger.Info().Int("index", i).Msg("Successfully clicked permission button")
			clickedCount++
			time.Sleep(1 * time.Second) // Brief pause between clicks
		}

		if clickedCount > 0 {
			s.logger.Info().Int("clicked", clickedCount).Msg("Finished clicking all permission buttons")
			screenshotTaker.TakeScreenshot(page, "atlassian_authorize_button_clicked")

			// Wait longer for all permission selections to register and JavaScript to enable Accept button
			s.logger.Info().Msg("Waiting for JavaScript to process permission selections and enable Accept button")
			time.Sleep(10 * time.Second)
		} else {
			s.logger.Warn().Msg("No permission buttons were clickable")
		}
	} else {
		s.logger.Info().Msg("No permission buttons found with data-testid, trying other authorization selectors")

		// Fallback to other selectors if permission buttons not found
		for _, selector := range authSelectors[1:] { // Skip the first one since we already tried permission buttons
			s.logger.Info().Str("selector", selector).Msg("Trying Atlassian authorization button selector")

			element, err := page.Timeout(20 * time.Second).Element(selector)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// For generic submit buttons, check if text contains authorization-related words
			if selector == `button[type="submit"]` || selector == `input[type="submit"]` {
				text, textErr := element.Timeout(5 * time.Second).Text()
				if textErr == nil {
					textLower := strings.ToLower(strings.TrimSpace(text))
					if !strings.Contains(textLower, "accept") && !strings.Contains(textLower, "allow") &&
						!strings.Contains(textLower, "authorize") && !strings.Contains(textLower, "continue") {
						s.logger.Info().Str("selector", selector).Str("text", text).Msg("Button found but doesn't contain authorization text")
						time.Sleep(1 * time.Second)
						continue
					}
				}
			}

			// Check if element is visible and clickable
			visible, visErr := element.Visible()
			if visErr != nil || !visible {
				s.logger.Info().Str("selector", selector).Msg("Element not visible, skipping")
				time.Sleep(1 * time.Second)
				continue
			}

			// Try to click the element
			s.logger.Info().Str("selector", selector).Msg("Found authorization button, attempting to click")

			err = element.ScrollIntoView()
			if err != nil {
				s.logger.Warn().Err(err).Msg("Failed to scroll element into view, trying click anyway")
			}

			err = element.Timeout(20*time.Second).Click(proto.InputMouseButtonLeft, 1)
			if err != nil {
				s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click authorization button")
				time.Sleep(1 * time.Second)
				continue
			}

			s.logger.Info().Str("selector", selector).Msg("Successfully clicked Atlassian authorization button")
			screenshotTaker.TakeScreenshot(page, "atlassian_authorize_button_clicked")

			// Wait a moment for the click to register
			time.Sleep(3 * time.Second)
			break
		}
	}

	// For Atlassian consent pages, after clicking permission buttons, we may need to click a final "Accept" button
	s.logger.Info().Msg("Looking for final Accept button on Atlassian consent page")
	acceptSelectors := []string{
		`button[type="submit"]:contains("Accept")`,
		`button[class*="css-9r91db"]`, // Specific class from debug output
		`button:contains("Accept")`,
		`input[type="submit"][value="Accept"]`,
		`button[type="submit"]`,
	}

	for _, acceptSelector := range acceptSelectors {
		s.logger.Info().Str("selector", acceptSelector).Msg("Trying Accept button selector")

		acceptElement, err := page.Timeout(15 * time.Second).Element(acceptSelector)
		if err != nil {
			s.logger.Info().Str("selector", acceptSelector).Msg("Accept button not found with this selector")
			continue
		}

		// Check if element is visible and clickable with multiple attempts
		visible := false
		for attempt := 0; attempt < 5; attempt++ {
			visErr := error(nil)
			visible, visErr = acceptElement.Visible()
			if visErr == nil && visible {
				break
			}
			s.logger.Info().Int("attempt", attempt+1).Str("selector", acceptSelector).Msg("Accept button not visible yet, waiting...")
			time.Sleep(2 * time.Second)
		}

		if !visible {
			s.logger.Info().Str("selector", acceptSelector).Msg("Accept button not visible after multiple attempts, skipping")
			continue
		}

		// Try to click the Accept button with multiple strategies
		s.logger.Info().Str("selector", acceptSelector).Msg("Found visible Accept button, attempting multiple click strategies")

		// Strategy 1: Standard click with scroll
		err = acceptElement.ScrollIntoView()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to scroll Accept button into view")
		}

		// Wait a moment after scroll
		time.Sleep(1 * time.Second)

		err = acceptElement.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if err == nil {
			s.logger.Info().Str("selector", acceptSelector).Msg("Successfully clicked Atlassian Accept button with standard click")
			screenshotTaker.TakeScreenshot(page, "atlassian_accept_button_clicked")
			time.Sleep(5 * time.Second)
			return nil
		}
		s.logger.Warn().Err(err).Str("selector", acceptSelector).Msg("Standard click failed, trying alternative strategies")

		// Strategy 2: Focus then click
		err = acceptElement.Focus()
		if err != nil {
			s.logger.Warn().Err(err).Msg("Failed to focus Accept button")
		}
		time.Sleep(500 * time.Millisecond)

		err = acceptElement.Timeout(30*time.Second).Click(proto.InputMouseButtonLeft, 1)
		if err == nil {
			s.logger.Info().Str("selector", acceptSelector).Msg("Successfully clicked Atlassian Accept button with focus+click")
			screenshotTaker.TakeScreenshot(page, "atlassian_accept_button_clicked")
			time.Sleep(5 * time.Second)
			return nil
		}
		s.logger.Warn().Err(err).Str("selector", acceptSelector).Msg("Focus+click failed, trying JavaScript click")

		// Strategy 3: JavaScript click
		_, err = acceptElement.Eval("() => this.click()")
		if err == nil {
			s.logger.Info().Str("selector", acceptSelector).Msg("Successfully clicked Atlassian Accept button with JavaScript")
			screenshotTaker.TakeScreenshot(page, "atlassian_accept_button_clicked")
			time.Sleep(5 * time.Second)
			return nil
		}
		s.logger.Warn().Err(err).Str("selector", acceptSelector).Msg("JavaScript click failed")

		// Strategy 4: Submit the form if this is a submit button
		if strings.Contains(acceptSelector, "submit") {
			_, err = acceptElement.Eval("() => { if (this.form) { this.form.submit(); } }")
			if err == nil {
				s.logger.Info().Str("selector", acceptSelector).Msg("Successfully submitted form via Accept button")
				screenshotTaker.TakeScreenshot(page, "atlassian_accept_button_clicked")
				time.Sleep(5 * time.Second)
				return nil
			}
			s.logger.Warn().Err(err).Str("selector", acceptSelector).Msg("Form submit failed")
		}
	}

	s.logger.Info().Msg("No Accept button found or clickable, assuming permission button click was sufficient")
	return nil
}
