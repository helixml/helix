package skills

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
		s.logger.Warn().Err(err).Msg("Error expanding error details during site selection")
	}

	// Try more targeted selectors for the dropdown button/trigger
	s.logger.Info().Msg("Looking for site dropdown trigger")

	dropdownSelectors := []string{
		`div.site-selector`,                   // React Select container
		`div.css-1vrwmt7-control`,             // React Select control element
		`.site-selector .css-1vrwmt7-control`, // More specific React Select control
		`div:contains("Choose a site")`,       // Div with the dropdown text
		`select`,                              // Standard select element (fallback)
		`button[aria-haspopup="listbox"]`,     // Button with dropdown role (fallback)
		`div[role="button"]`,                  // Div acting as button (fallback)
	}

	var dropdownTrigger *rod.Element
	var lastError error

	for _, selector := range dropdownSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying dropdown selector")

		element, err := page.Timeout(10 * time.Second).Element(selector)
		if err != nil {
			lastError = err
			continue
		}

		// Check if element is visible and clickable
		visible, visErr := element.Visible()
		if visErr != nil || !visible {
			s.logger.Info().Str("selector", selector).Msg("Element not visible, skipping")
			continue
		}

		// For text-based selectors, ensure the text actually contains "Choose a site"
		if strings.Contains(selector, "Choose a site") {
			text, textErr := element.Text()
			if textErr != nil || !strings.Contains(text, "Choose a site") {
				s.logger.Info().Str("selector", selector).Str("text", text).Msg("Element text doesn't match, skipping")
				continue
			}
		}

		dropdownTrigger = element
		s.logger.Info().Str("selector", selector).Msg("Found dropdown trigger")
		break
	}

	if dropdownTrigger == nil {
		s.logger.Warn().Err(lastError).Msg("Site dropdown trigger not found")
		if screenshotTaker != nil {
			screenshotTaker.TakeScreenshot(page, "atlassian_03_dropdown_not_found")
		}
		return fmt.Errorf("site dropdown trigger not found: %w", lastError)
	}

	// Screenshot 3: Before clicking dropdown
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_03_before_dropdown_click")
	}

	// DEBUG: Dump HTML structure around the dropdown area
	s.logger.Info().Msg("=== DUMPING HTML STRUCTURE AROUND DROPDOWN ===")
	htmlContent, err := page.MustWaitLoad().Eval(`() => {
		// Find elements that might be the dropdown
		const selectors = [
			'select',
			'button[aria-haspopup="listbox"]',
			'button[aria-expanded="false"]',
			'button[aria-expanded="true"]',
			'div[role="button"]',
			'*[class*="dropdown"]',
			'*[class*="select"]',
			'*:contains("Choose a site")',
		];
		
		let result = "=== DROPDOWN AREA HTML STRUCTURE ===\n";
		
		// Try to find dropdown-related elements
		for (let selector of selectors) {
			try {
				let elements;
				if (selector.includes(':contains')) {
					// For contains selector, search manually
					elements = Array.from(document.querySelectorAll('*')).filter(el => 
						el.textContent && el.textContent.includes('Choose a site')
					);
				} else {
					elements = document.querySelectorAll(selector);
				}
				
				if (elements.length > 0) {
					result += "\n--- SELECTOR: " + selector + " ---\n";
					for (let i = 0; i < Math.min(elements.length, 3); i++) {
						let el = elements[i];
						result += "Element " + i + ":\n";
						result += "  Tag: " + el.tagName + "\n";
						result += "  Text: " + (el.textContent || '').trim().substring(0, 100) + "\n";
						result += "  Class: " + (el.className || '') + "\n";
						result += "  ID: " + (el.id || '') + "\n";
						result += "  Aria-expanded: " + (el.getAttribute('aria-expanded') || 'none') + "\n";
						result += "  Aria-haspopup: " + (el.getAttribute('aria-haspopup') || 'none') + "\n";
						result += "  HTML: " + el.outerHTML.substring(0, 200) + "...\n";
						result += "  Parent HTML: " + (el.parentElement ? el.parentElement.outerHTML.substring(0, 200) : 'none') + "...\n";
						result += "\n";
					}
				}
			} catch (e) {
				result += "Error with selector " + selector + ": " + e.message + "\n";
			}
		}
		
		return result;
	}`)

	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to dump HTML structure")
	} else {
		s.logger.Info().Str("html_structure", htmlContent.Value.Str()).Msg("Dropdown HTML structure")
	}
	s.logger.Info().Msg("=== END HTML STRUCTURE DUMP ===")

	// Click the dropdown trigger to open it
	s.logger.Info().Msg("Clicking dropdown trigger to open site selection")
	err = dropdownTrigger.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to click dropdown trigger")
		return fmt.Errorf("failed to click dropdown trigger: %w", err)
	}

	// Wait for dropdown to open
	time.Sleep(2 * time.Second)

	// Screenshot 4: After clicking dropdown (should show options)
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_04_dropdown_opened")
	}

	// DEBUG: Dump HTML structure again after clicking to see if it changed
	s.logger.Info().Msg("=== DUMPING HTML STRUCTURE AFTER CLICKING ===")
	htmlContentAfter, err := page.MustWaitLoad().Eval(`() => {
		// Check if dropdown is now open
		let result = "=== DROPDOWN STATE AFTER CLICK ===\n";
		
		// Look for opened dropdown indicators
		const openSelectors = [
			'button[aria-expanded="true"]',
			'*[class*="open"]',
			'*[class*="expanded"]',
			'ul[role="listbox"]',
			'div[role="listbox"]',
			'option',
			'li[role="option"]',
		];
		
		for (let selector of openSelectors) {
			try {
				let elements = document.querySelectorAll(selector);
				if (elements.length > 0) {
					result += "\n--- OPEN SELECTOR: " + selector + " (Found " + elements.length + " elements) ---\n";
					for (let i = 0; i < Math.min(elements.length, 5); i++) {
						let el = elements[i];
						result += "Element " + i + ":\n";
						result += "  Tag: " + el.tagName + "\n";
						result += "  Text: " + (el.textContent || '').trim().substring(0, 100) + "\n";
						result += "  Class: " + (el.className || '') + "\n";
						result += "  ID: " + (el.id || '') + "\n";
						result += "  Visible: " + (el.offsetWidth > 0 && el.offsetHeight > 0) + "\n";
						result += "  HTML: " + el.outerHTML.substring(0, 200) + "...\n";
						result += "\n";
					}
				}
			} catch (e) {
				result += "Error with selector " + selector + ": " + e.message + "\n";
			}
		}
		
		return result;
	}`)

	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to dump HTML structure after click")
	} else {
		s.logger.Info().Str("html_structure_after", htmlContentAfter.Value.Str()).Msg("Dropdown HTML structure after click")
	}
	s.logger.Info().Msg("=== END HTML STRUCTURE DUMP AFTER CLICK ===")

	// DEBUG: Log all available options in the dropdown
	s.logger.Info().Msg("=== DEBUGGING DROPDOWN OPTIONS ===")
	allOptions, err := page.Elements("option, li, div, span, button, a")
	if err == nil {
		s.logger.Info().Int("total_elements", len(allOptions)).Msg("Found elements in dropdown area")
		for i, option := range allOptions {
			if i > 50 { // Limit output to avoid spam
				s.logger.Info().Msg("... (truncated remaining elements)")
				break
			}

			text, textErr := option.Text()
			if textErr != nil {
				text = "(error getting text)"
			}

			// Skip empty text elements
			if strings.TrimSpace(text) == "" {
				continue
			}

			// Get element attributes
			tagName, _ := option.Eval("() => this.tagName")
			className, _ := option.Attribute("class")
			dataValue, _ := option.Attribute("data-value")
			value, _ := option.Attribute("value")

			classStr := ""
			if className != nil {
				classStr = *className
			}
			dataValueStr := ""
			if dataValue != nil {
				dataValueStr = *dataValue
			}
			valueStr := ""
			if value != nil {
				valueStr = *value
			}

			s.logger.Info().
				Int("index", i).
				Str("tag", tagName.Value.Str()).
				Str("text", text).
				Str("class", classStr).
				Str("data-value", dataValueStr).
				Str("value", valueStr).
				Msg("Dropdown option found")
		}
	} else {
		s.logger.Warn().Err(err).Msg("Failed to get dropdown options for debugging")
	}
	s.logger.Info().Msg("=== END DROPDOWN OPTIONS DEBUG ===")

	// Now look for the specific site option in the dropdown
	s.logger.Info().Str("site_name", s.siteName).Msg("Looking for specific site in dropdown options")

	// Try various selectors for dropdown options
	optionSelectors := []string{
		// React Select specific selectors based on debug output
		fmt.Sprintf(`div[id="react-select-2-option-0"]:contains("%s")`, s.siteName), // First option
		fmt.Sprintf(`div[id="react-select-2-option-1"]:contains("%s")`, s.siteName), // Second option
		fmt.Sprintf(`div[id*="react-select-2-option"]:contains("%s")`, s.siteName),  // Any option
		fmt.Sprintf(`div.css-1npl3hl-option:contains("%s")`, s.siteName),            // Option class from debug
		fmt.Sprintf(`div[role="option"]:contains("%s")`, s.siteName),                // React Select option with role
		fmt.Sprintf(`.css-1n7v3ny-option:contains("%s")`, s.siteName),               // React Select option class
		fmt.Sprintf(`div[id*="option"]:contains("%s")`, s.siteName),                 // React Select option with ID
		fmt.Sprintf(`div.css-1ep3b46-option:contains("%s")`, s.siteName),            // Another React Select option class
		// Standard selectors as fallback
		fmt.Sprintf(`option:contains("%s")`, s.siteName), // Standard option element
		fmt.Sprintf(`li:contains("%s")`, s.siteName),     // List item option
		fmt.Sprintf(`div:contains("%s")`, s.siteName),    // Div option
		fmt.Sprintf(`span:contains("%s")`, s.siteName),   // Span option
		fmt.Sprintf(`button:contains("%s")`, s.siteName), // Button option
		fmt.Sprintf(`a:contains("%s")`, s.siteName),      // Link option
		fmt.Sprintf(`[data-value="%s"]`, s.siteName),     // Data attribute
		fmt.Sprintf(`[value="%s"]`, s.siteName),          // Value attribute
	}

	var siteOption *rod.Element

	for _, selector := range optionSelectors {
		s.logger.Info().Str("selector", selector).Msg("Trying site option selector")

		element, err := page.Timeout(10 * time.Second).Element(selector)
		if err != nil {
			continue
		}

		// Check if element is visible and clickable
		visible, visErr := element.Visible()
		if visErr != nil || !visible {
			s.logger.Info().Str("selector", selector).Msg("Site option not visible, skipping")
			continue
		}

		// For text-based selectors, verify the text contains the site name
		if strings.Contains(selector, ":contains(") {
			text, textErr := element.Text()
			if textErr != nil || !strings.Contains(text, s.siteName) {
				s.logger.Info().Str("selector", selector).Str("text", text).Msg("Site option text doesn't match, skipping")
				continue
			}
		}

		siteOption = element
		s.logger.Info().Str("selector", selector).Msg("Found site option")
		break
	}

	// If no exact match found, try partial matches
	if siteOption == nil {
		s.logger.Info().Str("site_name", s.siteName).Msg("No exact match found, trying partial matches")

		// Try different variations of the site name
		siteVariations := []string{
			s.siteName, // Original: "helixml-confluence"
			strings.Replace(s.siteName, "-", " ", -1),        // "helixml confluence"
			strings.ToLower(s.siteName),                      // "helixml-confluence"
			strings.ToUpper(s.siteName),                      // "HELIXML-CONFLUENCE"
			cases.Title(language.English).String(s.siteName), // "Helixml-Confluence"
		}

		// Also try just the base name without suffix
		if strings.Contains(s.siteName, "-") {
			parts := strings.Split(s.siteName, "-")
			if len(parts) > 0 {
				siteVariations = append(siteVariations, parts[0]) // "helixml"
			}
		}

		for _, variation := range siteVariations {
			s.logger.Info().Str("variation", variation).Msg("Trying site name variation")

			partialSelectors := []string{
				// React Select specific selectors based on debug output
				fmt.Sprintf(`div[id="react-select-2-option-0"]:contains("%s")`, variation),
				fmt.Sprintf(`div[id="react-select-2-option-1"]:contains("%s")`, variation),
				fmt.Sprintf(`div[id*="react-select-2-option"]:contains("%s")`, variation),
				fmt.Sprintf(`div.css-1npl3hl-option:contains("%s")`, variation),
				fmt.Sprintf(`div[role="option"]:contains("%s")`, variation),
				fmt.Sprintf(`.css-1n7v3ny-option:contains("%s")`, variation),
				fmt.Sprintf(`div[id*="option"]:contains("%s")`, variation),
				fmt.Sprintf(`div.css-1ep3b46-option:contains("%s")`, variation),
				// Standard selectors
				fmt.Sprintf(`option:contains("%s")`, variation),
				fmt.Sprintf(`li:contains("%s")`, variation),
				fmt.Sprintf(`div:contains("%s")`, variation),
				fmt.Sprintf(`span:contains("%s")`, variation),
				fmt.Sprintf(`button:contains("%s")`, variation),
				fmt.Sprintf(`a:contains("%s")`, variation),
			}

			for _, selector := range partialSelectors {
				element, err := page.Timeout(5 * time.Second).Element(selector)
				if err != nil {
					continue
				}

				visible, visErr := element.Visible()
				if visErr != nil || !visible {
					continue
				}

				text, textErr := element.Text()
				if textErr != nil {
					continue
				}

				// Check if the text contains the variation (case-insensitive)
				if strings.Contains(strings.ToLower(text), strings.ToLower(variation)) {
					siteOption = element
					s.logger.Info().Str("variation", variation).Str("text", text).Str("selector", selector).Msg("Found site option with partial match")
					break
				}
			}

			if siteOption != nil {
				break
			}
		}
	}

	// Try direct click on first option as fallback
	if siteOption == nil {
		s.logger.Info().Msg("No exact or partial match found, trying to click first option")
		firstOptionSelectors := []string{
			`div[id="react-select-2-option-0"]`,                  // First option by ID
			`div[role="option"][aria-selected="false"]`,          // First unselected option
			`div[role="option"]:first-child`,                     // First option child
			`div.css-1npl3hl-option:first-child`,                 // First option with class
			`div[role="listbox"] div[role="option"]:first-child`, // First option in listbox
		}

		for _, selector := range firstOptionSelectors {
			s.logger.Info().Str("selector", selector).Msg("Trying first option selector")
			elements, err := page.Elements(selector)
			if err != nil {
				s.logger.Debug().Err(err).Str("selector", selector).Msg("Failed to find elements")
				continue
			}

			if len(elements) > 0 {
				s.logger.Info().Str("selector", selector).Msg("Found first option, clicking it")
				siteOption = elements[0]
				break
			}
		}
	}

	if siteOption == nil {
		s.logger.Warn().Str("site_name", s.siteName).Msg("Site option not found in dropdown")
		if screenshotTaker != nil {
			screenshotTaker.TakeScreenshot(page, "atlassian_05_site_option_not_found")
		}
		return fmt.Errorf("site option '%s' not found in dropdown", s.siteName)
	}

	// Screenshot 5: Before clicking site option
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_05_before_site_selection")
	}

	// Click the site option to select it
	s.logger.Info().Str("site_name", s.siteName).Msg("Clicking site option to select it")
	err = siteOption.Timeout(10*time.Second).Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		s.logger.Warn().Err(err).Str("site_name", s.siteName).Msg("Failed to click site option")
		return fmt.Errorf("failed to click site option '%s': %w", s.siteName, err)
	}

	// Wait for selection to process
	time.Sleep(2 * time.Second)

	// Screenshot 6: After site selection
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_06_site_selected")
	}

	s.logger.Info().Str("site_name", s.siteName).Msg("Successfully selected site from dropdown")
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
	if err := s.expandErrorDetails(page); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to expand error details")
	}

	// Skip clicking individual permission buttons - modern OAuth consent pages don't require this
	s.logger.Info().Msg("Skipping individual permission button clicks - proceeding directly to Accept button")

	// Screenshot 10: Before looking for accept button
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_10_before_accept_button")
	}

	// For Atlassian consent pages, we may need to click a final "Accept" button
	s.logger.Info().Msg("Looking for final Accept button on Atlassian consent page")

	// Screenshot 11: Before looking for accept button
	if screenshotTaker != nil {
		screenshotTaker.TakeScreenshot(page, "atlassian_11_looking_for_accept_button")
	}

	acceptSelectors := []string{
		`button[type="submit"]`, // Generic submit button
		`input[type="submit"]`,  // Input submit button
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
	s.logger.Info().Str("html", html).Msg("HTML content")
	s.logger.Info().Msg("--- HTML END ---")

	// Return error to exit test immediately
	return fmt.Errorf("error page detected - 'Something went wrong' found on page")
}
