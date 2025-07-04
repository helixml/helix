//go:build oauth_integration
// +build oauth_integration

package skills_test

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog"
)

// SessionTestingFramework provides utilities for validating agent session responses
type SessionTestingFramework struct {
	logger zerolog.Logger
}

// NewSessionTestingFramework creates a new session testing framework
func NewSessionTestingFramework(logger zerolog.Logger) *SessionTestingFramework {
	return &SessionTestingFramework{
		logger: logger,
	}
}

// ValidateResponse validates an agent response against expected criteria
func (f *SessionTestingFramework) ValidateResponse(response string, criteria ValidationCriteria) ValidationResult {
	f.logger.Info().
		Str("response_length", fmt.Sprintf("%d", len(response))).
		Str("criteria_name", criteria.Name).
		Msg("Validating agent response")

	result := ValidationResult{
		Criteria: criteria,
		Response: response,
		Passed:   true,
		Issues:   make([]string, 0),
	}

	// Check for required keywords
	for _, keyword := range criteria.RequiredKeywords {
		if !strings.Contains(strings.ToLower(response), strings.ToLower(keyword)) {
			result.Passed = false
			result.Issues = append(result.Issues, fmt.Sprintf("Missing required keyword: %s", keyword))
		}
	}

	// Check for forbidden keywords
	for _, keyword := range criteria.ForbiddenKeywords {
		if strings.Contains(strings.ToLower(response), strings.ToLower(keyword)) {
			result.Passed = false
			result.Issues = append(result.Issues, fmt.Sprintf("Contains forbidden keyword: %s", keyword))
		}
	}

	// Check minimum length
	if criteria.MinLength > 0 && len(response) < criteria.MinLength {
		result.Passed = false
		result.Issues = append(result.Issues, fmt.Sprintf("Response too short: %d < %d", len(response), criteria.MinLength))
	}

	// Check maximum length
	if criteria.MaxLength > 0 && len(response) > criteria.MaxLength {
		result.Passed = false
		result.Issues = append(result.Issues, fmt.Sprintf("Response too long: %d > %d", len(response), criteria.MaxLength))
	}

	// Check for generic/mock responses
	if criteria.RejectGenericResponses {
		if f.isGenericResponse(response) {
			result.Passed = false
			result.Issues = append(result.Issues, "Response appears to be generic/mock")
		}
	}

	f.logger.Info().
		Bool("passed", result.Passed).
		Int("issues_count", len(result.Issues)).
		Msg("Response validation completed")

	return result
}

// isGenericResponse checks if a response appears to be generic or mock
func (f *SessionTestingFramework) isGenericResponse(response string) bool {
	lowerResponse := strings.ToLower(response)

	genericIndicators := []string{
		"i don't have access",
		"i cannot access",
		"i'm not able to",
		"i don't have the ability",
		"i can't actually",
		"i cannot actually",
		"as an ai",
		"i'm an ai",
		"i don't have real-time access",
		"i don't have the capability",
		"i cannot browse",
		"i can't browse",
		"i cannot retrieve",
		"i can't retrieve",
		"mock",
		"test data",
		"example",
		"placeholder",
		"unable to access your actual",
		"i don't have access to your real",
	}

	for _, indicator := range genericIndicators {
		if strings.Contains(lowerResponse, indicator) {
			return true
		}
	}

	return false
}

// ValidateOAuthIntegration validates that OAuth integration is working properly
func (f *SessionTestingFramework) ValidateOAuthIntegration(response string, providerType string) ValidationResult {
	criteria := ValidationCriteria{
		Name:                   fmt.Sprintf("%s OAuth Integration", providerType),
		RequiredKeywords:       []string{},
		ForbiddenKeywords:      []string{"mock", "test", "example", "placeholder"},
		MinLength:              10,
		MaxLength:              5000,
		RejectGenericResponses: true,
	}

	return f.ValidateResponse(response, criteria)
}

// ValidationCriteria defines the criteria for validating agent responses
type ValidationCriteria struct {
	Name                   string
	RequiredKeywords       []string
	ForbiddenKeywords      []string
	MinLength              int
	MaxLength              int
	RejectGenericResponses bool
}

// ValidationResult contains the results of response validation
type ValidationResult struct {
	Criteria ValidationCriteria
	Response string
	Passed   bool
	Issues   []string
}

// GetSummary returns a summary of the validation result
func (r ValidationResult) GetSummary() string {
	if r.Passed {
		return fmt.Sprintf("✓ %s: PASSED", r.Criteria.Name)
	}
	return fmt.Sprintf("✗ %s: FAILED - %s", r.Criteria.Name, strings.Join(r.Issues, ", "))
}
