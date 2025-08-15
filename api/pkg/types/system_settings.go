package types

import (
	"time"
)

// SystemSettings represents global system configuration
// This serves as the fallback/default for all users and organizations
// Future enhancement: Add HuggingFaceToken to Organization and User tables
// with resolution hierarchy: User -> Organization -> System (global)
type SystemSettings struct {
	ID      string    `json:"id" gorm:"primaryKey"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Global Hugging Face configuration (fallback for all users/orgs)
	// Future: This will be the lowest priority in token resolution hierarchy
	HuggingFaceToken string `json:"huggingface_token,omitempty" gorm:"column:huggingface_token"`

	// Future global settings can be added here
}

// SystemSettingsRequest represents the request payload for updating system settings
type SystemSettingsRequest struct {
	HuggingFaceToken *string `json:"huggingface_token,omitempty"`
}

// SystemSettingsResponse represents the response payload for system settings (without sensitive data)
type SystemSettingsResponse struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Sensitive fields are masked
	HuggingFaceTokenSet    bool   `json:"huggingface_token_set"`
	HuggingFaceTokenSource string `json:"huggingface_token_source"` // "database", "environment", or "none"
}

// ToResponse converts SystemSettings to SystemSettingsResponse (masking sensitive data)
func (s *SystemSettings) ToResponse() *SystemSettingsResponse {
	return &SystemSettingsResponse{
		ID:                  s.ID,
		Created:             s.Created,
		Updated:             s.Updated,
		HuggingFaceTokenSet: s.HuggingFaceToken != "",
	}
}

// ToResponseWithSource converts SystemSettings to SystemSettingsResponse with source information
func (s *SystemSettings) ToResponseWithSource(dbToken, envToken string) *SystemSettingsResponse {
	var source string
	var hasToken bool

	if dbToken != "" {
		source = "database"
		hasToken = true
	} else if envToken != "" {
		source = "environment"
		hasToken = true
	} else {
		source = "none"
		hasToken = false
	}

	return &SystemSettingsResponse{
		ID:                     s.ID,
		Created:                s.Created,
		Updated:                s.Updated,
		HuggingFaceTokenSet:    hasToken,
		HuggingFaceTokenSource: source,
	}
}

const (
	// SystemSettingsID is the fixed ID for the single system settings record
	SystemSettingsID = "system"
)
