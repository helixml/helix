package skills

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

// GitHubDeviceVerificationHandler handles GitHub's device verification process
type GitHubDeviceVerificationHandler struct {
	gmailService           *gmail.Service
	logger                 zerolog.Logger
	gmailCredentialsBase64 string
}

// NewGitHubDeviceVerificationHandler creates a new GitHub device verification handler
func NewGitHubDeviceVerificationHandler(gmailCredentialsBase64 string, logger zerolog.Logger) (*GitHubDeviceVerificationHandler, error) {
	handler := &GitHubDeviceVerificationHandler{
		gmailCredentialsBase64: gmailCredentialsBase64,
		logger:                 logger,
	}

	err := handler.setupGmailService()
	if err != nil {
		return nil, fmt.Errorf("failed to setup Gmail service: %w", err)
	}

	return handler, nil
}

// IsRequired checks if device verification is required
func (h *GitHubDeviceVerificationHandler) IsRequired(page *rod.Page) bool {
	currentURL := page.MustInfo().URL

	// Precise device verification detection based on actual GitHub URLs
	isDeviceVerificationPage := strings.Contains(currentURL, "/sessions/verified-device") ||
		strings.Contains(currentURL, "/login/device") ||
		strings.HasSuffix(currentURL, "/session")

	if isDeviceVerificationPage {
		h.logger.Info().Str("url", currentURL).Msg("GitHub device verification page detected")
	}

	return isDeviceVerificationPage
}

// Handle performs the device verification process
func (h *GitHubDeviceVerificationHandler) Handle(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling GitHub device verification")

	currentURL := page.MustInfo().URL

	// Handle different types of device verification pages
	if strings.Contains(currentURL, "/sessions/verified-device") {
		return h.handleVerifiedDevicePage(page, nil)
	}

	if strings.HasSuffix(currentURL, "/session") {
		return h.handleSessionPage(page, nil)
	}

	if strings.Contains(currentURL, "/login/device") {
		return h.handleTraditionalDeviceVerification(page, nil)
	}

	// Fallback to traditional handling
	h.logger.Warn().Str("url", currentURL).Msg("Unexpected device verification URL pattern, using traditional handling")
	return h.handleTraditionalDeviceVerification(page, nil)
}

// setupGmailService initializes the Gmail API service for device verification
func (h *GitHubDeviceVerificationHandler) setupGmailService() error {
	h.logger.Info().Msg("Setting up Gmail service for device verification")

	// Decode base64 credentials
	credentials, err := base64.StdEncoding.DecodeString(h.gmailCredentialsBase64)
	if err != nil {
		return fmt.Errorf("failed to decode Gmail credentials: %w", err)
	}

	// Create Gmail service with service account credentials and domain-wide delegation
	ctx := context.Background()

	// Parse the service account credentials
	config, err := google.JWTConfigFromJSON(credentials, gmail.GmailReadonlyScope)
	if err != nil {
		return fmt.Errorf("failed to parse Gmail credentials: %w", err)
	}

	// Set the subject to impersonate the test@helix.ml user
	config.Subject = "test@helix.ml"

	// Create HTTP client with the JWT config
	client := config.Client(ctx)

	// Create Gmail service
	service, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return fmt.Errorf("failed to create Gmail service: %w", err)
	}

	h.gmailService = service
	h.logger.Info().Msg("Gmail service setup completed successfully")
	return nil
}

// getDeviceVerificationCode reads the latest device verification email and extracts the code
func (h *GitHubDeviceVerificationHandler) getDeviceVerificationCode() (string, error) {
	h.logger.Info().Msg("Searching for GitHub device verification email")

	// Search for emails from GitHub with device verification
	query := "from:noreply@github.com subject:device verification"
	listCall := h.gmailService.Users.Messages.List("test@helix.ml").Q(query).MaxResults(5)

	messages, err := listCall.Do()
	if err != nil {
		return "", fmt.Errorf("failed to search for device verification emails: %w", err)
	}

	if len(messages.Messages) == 0 {
		return "", fmt.Errorf("no device verification emails found")
	}

	h.logger.Info().Int("message_count", len(messages.Messages)).Msg("Found device verification emails")

	// Get the most recent message
	messageID := messages.Messages[0].Id
	message, err := h.gmailService.Users.Messages.Get("test@helix.ml", messageID).Do()
	if err != nil {
		return "", fmt.Errorf("failed to get device verification email: %w", err)
	}

	// Extract email body
	emailBody := ""
	if message.Payload.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(message.Payload.Body.Data)
		if err == nil {
			emailBody = string(decoded)
		}
	}

	// Check parts for the body if main body is empty
	if emailBody == "" && len(message.Payload.Parts) > 0 {
		for _, part := range message.Payload.Parts {
			if part.MimeType == "text/plain" && part.Body.Data != "" {
				decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
				if err == nil {
					emailBody = string(decoded)
					break
				}
			}
		}
	}

	if emailBody == "" {
		return "", fmt.Errorf("could not extract email body from device verification email")
	}

	h.logger.Info().Str("email_body", emailBody[:min(len(emailBody), 200)]+"...").Msg("Device verification email content")

	// Extract verification code using regex
	// GitHub device verification codes are typically 6 digits
	codeRegex := regexp.MustCompile(`\b(\d{6})\b`)
	matches := codeRegex.FindAllStringSubmatch(emailBody, -1)

	if len(matches) == 0 {
		return "", fmt.Errorf("could not find verification code in email body")
	}

	// Return the first 6-digit code found
	verificationCode := matches[0][1]
	h.logger.Info().Str("verification_code", verificationCode).Msg("Extracted device verification code")

	return verificationCode, nil
}

// handleVerifiedDevicePage handles GitHub's /sessions/verified-device page
func (h *GitHubDeviceVerificationHandler) handleVerifiedDevicePage(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling GitHub verified device page")

	// Wait for the page to fully load
	time.Sleep(3 * time.Second)

	// Look for device verification code input field
	codeInputSelectors := []string{
		`input[name="otp"]`,
		`input[id="otp"]`,
		`input[name="device_verification_code"]`,
		`input[placeholder*="verification"]`,
		`input[placeholder*="code"]`,
		`input[type="text"]`, // Fallback for generic text inputs
	}

	var codeInput *rod.Element
	var err error

	// Try each selector until we find the input field
	for _, selector := range codeInputSelectors {
		codeInput, err = page.Element(selector)
		if err == nil && codeInput != nil {
			h.logger.Info().Str("selector", selector).Msg("Found device verification code input field")
			break
		}
	}

	// If we found a code input, handle device verification
	if codeInput != nil {
		return h.handleDeviceVerificationInput(page, codeInput, nil)
	}

	// If no code input found, look for continue buttons or other actions
	continueSelectors := []string{
		`button:contains("Continue")`,
		`button:contains("Submit")`,
		`button[type="submit"]`,
		`input[type="submit"]`,
		`button:contains("Send code")`,
		`button:contains("Send")`,
	}

	var continueButton *rod.Element
	for _, selector := range continueSelectors {
		continueButton, err = page.Element(selector)
		if err == nil && continueButton != nil {
			h.logger.Info().Str("selector", selector).Msg("Found continue/submit button on verified device page")
			break
		}
	}

	if continueButton != nil {
		err = continueButton.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to click continue button on verified device page")
		} else {
			h.logger.Info().Msg("Clicked continue button on verified device page")
			// Wait for navigation or form submission
			time.Sleep(5 * time.Second)
		}
		return nil
	}

	// If no specific actions found, wait and check for automatic redirect
	h.logger.Info().Msg("No specific action required on verified device page, waiting for automatic redirect")
	time.Sleep(5 * time.Second)

	return nil
}

// handleSessionPage handles GitHub's new session verification flow
func (h *GitHubDeviceVerificationHandler) handleSessionPage(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling GitHub session page")

	// Wait for the session page to fully load
	time.Sleep(3 * time.Second)

	// Look for various elements that might be on the session page
	// Check for device verification code input first
	deviceCodeInput, err := page.Element(`input[name="otp"], input[id="otp"], input[name="device_verification_code"], input[placeholder*="verification"], input[placeholder*="code"]`)
	if err == nil && deviceCodeInput != nil {
		h.logger.Info().Msg("Found device verification code input on session page")
		return h.handleDeviceVerificationInput(page, deviceCodeInput, nil)
	}

	// Check for continue/submit buttons that might advance the session
	continueButton, err := page.Element(`button:contains("Continue"), button:contains("Submit"), button[type="submit"], input[type="submit"]`)
	if err == nil && continueButton != nil {
		h.logger.Info().Msg("Found continue/submit button on session page")

		err = continueButton.Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			h.logger.Error().Err(err).Msg("Failed to click continue button")
		} else {
			h.logger.Info().Msg("Clicked continue button on session page")
			// Wait for navigation
			time.Sleep(3 * time.Second)
		}
		return nil
	}

	// Check if there are any forms to submit
	forms, err := page.Elements("form")
	if err == nil && len(forms) > 0 {
		h.logger.Info().Int("form_count", len(forms)).Msg("Found forms on session page")

		// Try to find a submit button in any form
		for i, form := range forms {
			submitBtn, err := form.Element(`button[type="submit"], input[type="submit"]`)
			if err == nil && submitBtn != nil {
				h.logger.Info().Int("form_index", i).Msg("Found submit button in form")

				err = submitBtn.Click(proto.InputMouseButtonLeft, 1)
				if err == nil {
					h.logger.Info().Msg("Successfully clicked submit button in form")
					time.Sleep(3 * time.Second)
					return nil
				}
			}
		}
	}

	// If no specific elements found, wait and continue - the session page might be temporary
	h.logger.Info().Msg("No specific action required on session page, waiting for automatic redirect")
	time.Sleep(5 * time.Second)

	return nil
}

// handleTraditionalDeviceVerification handles the traditional device verification flow
func (h *GitHubDeviceVerificationHandler) handleTraditionalDeviceVerification(page *rod.Page, _ *BrowserOAuthAutomator) error {
	h.logger.Info().Msg("Handling traditional device verification")

	// More specific verification code input field selector (exclude generic text inputs)
	codeInputSelector := `input[name="otp"], input[id="otp"], input[name="device_verification_code"], input[placeholder*="verification"], input[placeholder*="code"]`
	codeInput, err := page.Element(codeInputSelector)
	if err != nil {
		return fmt.Errorf("failed to find verification code input field: %w", err)
	}

	return h.handleDeviceVerificationInput(page, codeInput, nil)
}

// handleDeviceVerificationInput handles entering device verification codes
func (h *GitHubDeviceVerificationHandler) handleDeviceVerificationInput(page *rod.Page, codeInput *rod.Element, _ *BrowserOAuthAutomator) error {
	// Get device verification code from Gmail
	h.logger.Info().Msg("Waiting for device verification email...")

	var verificationCode string
	var err error

	// Wait up to 60 seconds for the device verification email
	for i := 0; i < 12; i++ {
		time.Sleep(5 * time.Second)
		verificationCode, err = h.getDeviceVerificationCode()
		if err == nil {
			break
		}
		h.logger.Info().Err(err).Int("attempt", i+1).Msg("Device verification email not found yet, retrying...")
	}

	if err != nil {
		return fmt.Errorf("failed to get device verification code after 60 seconds: %w", err)
	}

	// Enter the verification code
	h.logger.Info().Str("code", verificationCode).Msg("Entering device verification code")
	err = codeInput.Input(verificationCode)
	if err != nil {
		return fmt.Errorf("failed to enter verification code: %w", err)
	}

	// Find and click the verify button
	verifyButtonSelector := `button[type="submit"], input[type="submit"], button:contains("Verify")`
	verifyButton, err := page.Element(verifyButtonSelector)
	if err != nil {
		return fmt.Errorf("failed to find verify button: %w", err)
	}

	err = verifyButton.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("failed to click verify button: %w", err)
	}

	h.logger.Info().Msg("Device verification submitted")

	// Wait for navigation after device verification
	time.Sleep(3 * time.Second)

	return nil
}

// IsGitHubDeviceVerificationPage checks if a URL is a GitHub device verification page
func IsGitHubDeviceVerificationPage(url string) bool {
	return strings.Contains(url, "/sessions/verified-device") ||
		strings.Contains(url, "/login/device") ||
		strings.HasSuffix(url, "/session")
}
