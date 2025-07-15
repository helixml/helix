package skills

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
)

// AtlassianProviderStrategy implements Atlassian-specific OAuth automation behavior
type AtlassianProviderStrategy struct {
	logger   zerolog.Logger
	siteName string // The site name to select (e.g., "helixml" for Jira, "helixml-confluence" for Confluence)
}

// NewAtlassianProviderStrategy creates a new Atlassian provider strategy
func NewAtlassianProviderStrategy(logger zerolog.Logger, siteName string) *AtlassianProviderStrategy {
	return &AtlassianProviderStrategy{
		logger:   logger,
		siteName: siteName,
	}
}

// ClickNextButton implements Atlassian-specific Next button logic
func (s *AtlassianProviderStrategy) ClickNextButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Atlassian Next/Continue button")

	// Use a temporary page context to avoid affecting the main page timeout
	// Atlassian pages can be very slow, so use a longer timeout
	nextPage := page.Timeout(90 * time.Second)

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

		element, err := nextPage.Timeout(30 * time.Second).Element(selector)
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

	// Use a temporary page context to avoid affecting the main page timeout
	// Atlassian pages can be very slow, so use a longer timeout
	loginPage := page.Timeout(90 * time.Second)

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
		usernameField, err = loginPage.Timeout(30 * time.Second).Element(selector)
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
		passwordField, err = loginPage.Timeout(30 * time.Second).Element(selector)
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
		loginButton, err = loginPage.Timeout(30 * time.Second).Element(selector)
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

	// Step 1: Handle site selection dropdown first
	err := s.HandleSiteSelection(page, screenshotTaker)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to handle site selection, continuing with authorization")
		// Don't fail the entire flow if site selection fails - continue with authorization
	}

	// Step 2: Click authorize button
	return s.ClickAuthorizeButton(page, screenshotTaker)
}

// HandleSiteSelection handles the "Use app on" dropdown selection for Atlassian sites
func (s *AtlassianProviderStrategy) HandleSiteSelection(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Str("site_name", s.siteName).Msg("Handling Atlassian site selection dropdown")

	// Screenshot 1: Initial page state before any site selection
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_01_initial_page")
	}

	// Wait for page to load
	time.Sleep(2 * time.Second)

	// Screenshot 2: After page load, before looking for dropdown
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_02_page_loaded")
	}

	// Check if this is an error page and dump HTML if needed
	s.logger.Info().Msg("Checking for error page during site selection")
	err := s.expandErrorDetails(page)
	if err != nil {
		return fmt.Errorf("error page detected during site selection: %w", err)
	}

	// Look for the site selection dropdown using multiple strategies
	dropdownSelectors := []string{
		`button[role="combobox"]`,
		`div[role="combobox"]`,
		`[data-testid="site-selector"]`,
		`[data-testid="site-picker"]`,
		`select[name="site"]`,
		`select[name="resourceId"]`,
		`button[aria-haspopup="true"]`,
		`div[aria-haspopup="true"]`,
	}

	var dropdownElement *rod.Element
	var foundSelector string

	// Try to find dropdown by selectors first
	for _, selector := range dropdownSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying site dropdown selector")
		element, err := page.Timeout(5 * time.Second).Element(selector)
		if err == nil {
			dropdownElement = element
			foundSelector = selector
			s.logger.Info().Str("selector", selector).Msg("Found site dropdown element")
			break
		}
	}

	// If not found by selectors, try to find by text content
	if dropdownElement == nil {
		s.logger.Info().Msg("Dropdown not found by selectors, trying to find by text content")

		// Find all buttons and divs that might contain the dropdown text
		allElements, err := page.Elements(`button, div`)
		if err == nil {
			for _, element := range allElements {
				// Add nil check for element
				if element == nil {
					continue
				}

				text, err := element.Text()
				if err == nil && (strings.Contains(text, "Choose a site") || strings.Contains(text, "Use app on")) {
					// Validate that the element is actually interactable
					visible, visErr := element.Visible()
					if visErr != nil || !visible {
						s.logger.Info().Str("text", text).Msg("Found text but element not visible, skipping")
						continue
					}

					// Check if element is clickable (has click handlers or is a button/div with role)
					tagName, tagErr := element.Property("tagName")
					role, roleErr := element.Attribute("role")

					tagStr := "unknown"
					if tagErr == nil {
						tagStr = strings.ToLower(tagName.String())
					}

					roleStr := ""
					if roleErr == nil && role != nil {
						roleStr = *role
					}

					// Only accept elements that are likely to be clickable dropdowns
					if tagStr == "button" || strings.Contains(roleStr, "combobox") || strings.Contains(roleStr, "button") {
						dropdownElement = element
						foundSelector = "text-based"
						s.logger.Info().Str("text", text).Str("tag", tagStr).Str("role", roleStr).Msg("Found dropdown by text content")
						break
					} else {
						s.logger.Info().Str("text", text).Str("tag", tagStr).Str("role", roleStr).Msg("Found text but element not clickable, skipping")
					}
				}
			}
		}
	}

	if dropdownElement == nil {
		s.logger.Warn().Msg("Site dropdown not found, site may already be selected")
		// Screenshot for debugging when dropdown not found
		if screenshotTaker != nil {
			screenshotTaker.TakeScreenshot(page, "atlassian_03_dropdown_not_found")
		}
		return nil
	}

	// Screenshot 3: After finding dropdown element, before clicking it
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_03_dropdown_found")
	}

	// Additional safety check to ensure dropdown element is still valid
	if dropdownElement == nil {
		s.logger.Error().Msg("Dropdown element became nil after finding it")
		return fmt.Errorf("dropdown element became nil")
	}

	// Debug: Log dropdown element properties with safe access
	dropdownText, textErr := dropdownElement.Text()
	if textErr != nil {
		s.logger.Warn().Err(textErr).Msg("Failed to get dropdown text")
		dropdownText = "unable to get text"
	}

	dropdownTagName, tagErr := dropdownElement.Property("tagName")
	dropdownClasses, classErr := dropdownElement.Attribute("class")

	// Safe string conversion with error checks
	tagName := "unknown"
	if tagErr == nil {
		tagName = dropdownTagName.String()
	}

	classes := "unknown"
	if classErr == nil && dropdownClasses != nil {
		classes = *dropdownClasses
	}

	s.logger.Info().
		Str("selector", foundSelector).
		Str("text", dropdownText).
		Str("tag_name", tagName).
		Str("classes", classes).
		Msg("Dropdown element details")

	// Click the dropdown to open it
	s.logger.Info().Msg("Clicking site dropdown to open it")
	err = dropdownElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		// Try scrolling to element first, then clicking
		s.logger.Info().Msg("Regular click failed, trying to scroll to element first")
		err = dropdownElement.ScrollIntoView()
		if err == nil {
			time.Sleep(1 * time.Second)
			err = dropdownElement.Click(proto.InputMouseButtonLeft, 1)
		}

		if err != nil {
			// Screenshot for debugging click failure
			if screenshotTaker != nil {
				screenshotTaker.TakeScreenshot(page, "atlassian_04_dropdown_click_failed")
			}
			return fmt.Errorf("failed to click dropdown: %w", err)
		}
	}

	// Screenshot 4: Immediately after clicking dropdown
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_04_dropdown_clicked")
	}

	// Wait for dropdown to open and options to load
	time.Sleep(3 * time.Second)

	// Screenshot 5: After waiting for dropdown to open
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_05_dropdown_opened")
	}

	// Find the site option by text content using Rod's proper methods
	s.logger.Info().Str("site_name", s.siteName).Msg("Looking for site option by text content")

	var siteElement *rod.Element

	// Strategy 1: Look for elements that contain the site name
	allElements, err := page.Elements(`li, div, option, span, button`)
	if err == nil {
		for _, element := range allElements {
			text, err := element.Text()
			if err == nil && strings.Contains(text, s.siteName) {
				// Additional check: make sure it's clickable and visible
				visible, err := element.Visible()
				if err == nil && visible {
					siteElement = element
					s.logger.Info().Str("text", text).Msg("Found site option by text content")
					break
				}
			}
		}
	}

	// Strategy 2: If not found, try aria-label or data attributes
	if siteElement == nil {
		s.logger.Info().Msg("Site not found by text, trying aria-label and data attributes")

		attrSelectors := []string{
			fmt.Sprintf(`[aria-label*="%s"]`, s.siteName),
			fmt.Sprintf(`[data-value*="%s"]`, s.siteName),
			fmt.Sprintf(`[data-site*="%s"]`, s.siteName),
			fmt.Sprintf(`[title*="%s"]`, s.siteName),
		}

		for _, selector := range attrSelectors {
			element, err := page.Timeout(2 * time.Second).Element(selector)
			if err == nil {
				siteElement = element
				s.logger.Info().Str("selector", selector).Msg("Found site option by attribute")
				break
			}
		}
	}

	// Strategy 3: If still not found, log all available options for debugging
	if siteElement == nil {
		s.logger.Warn().Str("site_name", s.siteName).Msg("Site not found, logging all available options")

		allOptions, err := page.Elements(`li, div, option, span`)
		if err == nil {
			for i, option := range allOptions {
				if i > 30 { // Limit to avoid spam
					break
				}
				text, err := option.Text()
				if err == nil && text != "" && len(text) < 200 {
					visible, _ := option.Visible()
					s.logger.Info().
						Int("index", i).
						Str("text", text).
						Bool("visible", visible).
						Msg("Available option")
				}
			}
		}

		// Screenshot for debugging when site not found
		if screenshotTaker != nil {
			screenshotTaker.TakeScreenshot(page, "atlassian_06_site_not_found")
		}
		return fmt.Errorf("site '%s' not found in dropdown options", s.siteName)
	}

	// Screenshot 6: After finding site option, before clicking it
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_06_site_option_found")
	}

	// Debug: Log site element properties
	siteText, _ := siteElement.Text()
	siteTagName, _ := siteElement.Property("tagName")
	siteClasses, _ := siteElement.Attribute("class")
	s.logger.Info().
		Str("text", siteText).
		Str("tag_name", siteTagName.String()).
		Str("classes", *siteClasses).
		Msg("Site element details")

	// Click the site option
	s.logger.Info().Str("site", s.siteName).Msg("Clicking site option")

	err = siteElement.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		// Try scrolling to element first, then clicking
		s.logger.Info().Msg("Regular click failed, trying to scroll to element first")
		err = siteElement.ScrollIntoView()
		if err == nil {
			time.Sleep(1 * time.Second)
			err = siteElement.Click(proto.InputMouseButtonLeft, 1)
		}

		if err != nil {
			// Screenshot for debugging click failure
			if screenshotTaker != nil {
				screenshotTaker.TakeScreenshot(page, "atlassian_07_site_click_failed")
			}
			return fmt.Errorf("failed to click site option: %w", err)
		}
	}

	// Screenshot 7: Immediately after clicking site option
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_07_site_clicked")
	}

	// Wait for selection to register
	time.Sleep(2 * time.Second)

	// Screenshot 8: After waiting for selection to register
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_08_site_selected")
	}

	s.logger.Info().Str("site", s.siteName).Msg("Successfully selected site")
	return nil
}

// ClickAuthorizeButton implements Atlassian-specific authorization button detection and clicking
func (s *AtlassianProviderStrategy) ClickAuthorizeButton(page *rod.Page, screenshotTaker ScreenshotTaker) error {
	s.logger.Info().Msg("Looking for Atlassian Authorize button")

	// Use a temporary page context to avoid affecting the main page timeout
	// Atlassian pages can be very slow, so use a longer timeout
	authPage := page.Timeout(90 * time.Second)

	// Screenshot 9: First, wait for the page to fully load and take a screenshot for debugging
	time.Sleep(3 * time.Second)
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_09_consent_page_loaded")
	}

	// Check if this is an error page and expand any collapsed sections for debugging
	s.logger.Info().Msg("Attempting to expand error details on authorization page")
	s.expandErrorDetails(page)

	// Debug: dump all buttons on the consent page
	buttons, err := authPage.Elements("button")
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

			err = button.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
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
			// Screenshot 10: After clicking all permission buttons
			screenshotTaker.TakeScreenshot(page, "atlassian_10_permissions_clicked")

			// Wait longer for all permission selections to register and JavaScript to enable Accept button
			s.logger.Info().Msg("Waiting for JavaScript to process permission selections and enable Accept button")
			time.Sleep(10 * time.Second)

			// Screenshot 11: After waiting for JavaScript processing
			if screenshotTaker != nil {
				screenshotTaker.TakeScreenshot(page, "atlassian_11_js_processing_complete")
			}

			// For Atlassian consent pages, after clicking permission buttons, we may need to click a final "Accept" button
			s.logger.Info().Msg("Looking for final Accept button on Atlassian consent page")

			// Screenshot 12: Before looking for accept button
			if screenshotTaker != nil {
				screenshotTaker.TakeScreenshot(page, "atlassian_12_looking_for_accept_button")
			}
			acceptSelectors := []string{
				`button[data-testid="permission-button"]`, // Primary Atlassian consent permission buttons (PROVEN WORKING)
				`button[type="submit"]`,                   // Generic submit button (fallback)
				`input[type="submit"]`,                    // Input submit button (fallback)
			}

			var lastError error

			for _, selector := range acceptSelectors {
				s.logger.Info().Str("selector", selector).Msg("Trying Atlassian Accept button selector")

				element, err := authPage.Timeout(30 * time.Second).Element(selector)
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
					s.logger.Warn().Err(err).Str("selector", selector).Msg("Failed to click Accept button")
					lastError = err
					time.Sleep(1 * time.Second)
					continue
				}

				s.logger.Info().Str("selector", selector).Msg("Successfully clicked Atlassian Accept button")
				screenshotTaker.TakeScreenshot(page, "atlassian_accept_button_clicked")

				time.Sleep(3 * time.Second)
				return nil
			}

			return fmt.Errorf("failed to find Atlassian Accept button: %w", lastError)
		}
	}

	return nil
}

// expandErrorDetails checks if we're on an error page and attempts to expand collapsed error details
func (s *AtlassianProviderStrategy) expandErrorDetails(page *rod.Page) error {
	s.logger.Info().Msg("Checking for error page indicators...")

	// Check for error page indicators
	pageText, err := page.MustWaitLoad().Eval(`() => document.body.innerText || document.body.textContent || ''`)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to get page text")
		return err
	}

	text := pageText.Value.Str()
	textLower := strings.ToLower(text)

	// More specific error detection - only trigger on actual error pages
	isErrorPage := strings.Contains(textLower, "something went wrong") ||
		(strings.Contains(textLower, "information for the owner of helix dev") &&
			!strings.Contains(textLower, "is requesting access")) // Exclude consent pages

	if isErrorPage {
		s.logger.Warn().Msg("🚨 ERROR PAGE DETECTED - dumping full HTML for debugging")
		return s.dumpErrorPageHTML(page)
	}

	s.logger.Info().Msg("✅ No error page detected - continuing with OAuth flow")
	return nil
}

// dumpErrorPageHTML dumps the full HTML content of an error page for debugging
func (s *AtlassianProviderStrategy) dumpErrorPageHTML(page *rod.Page) error {
	s.logger.Info().Msg("=== DUMPING ERROR PAGE HTML ===")

	// Get the full HTML content
	htmlContent, err := page.MustWaitLoad().Eval(`() => document.documentElement.outerHTML`)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to get HTML content")
		return fmt.Errorf("failed to get HTML content: %w", err)
	}

	html := htmlContent.Value.Str()
	s.logger.Info().Int("full_html_length", len(html)).Msg("Full HTML content")

	// Write HTML to file for debugging
	filename := fmt.Sprintf("/tmp/helix-oauth-error-page-%d.html", time.Now().Unix())
	err = os.WriteFile(filename, []byte(html), 0644)
	if err != nil {
		s.logger.Warn().Err(err).Str("filename", filename).Msg("Failed to write HTML to file")
	} else {
		s.logger.Info().Str("filename", filename).Msg("HTML content written to file")
	}

	// Also write to console with clear delimiters
	s.logger.Info().Msg("--- HTML START ---")
	// Use fmt.Println to ensure HTML is printed to console
	fmt.Println(html)
	s.logger.Info().Msg("--- HTML END ---")

	// Return error to exit test immediately
	return fmt.Errorf("error page detected - 'Something went wrong' found on page")
}
