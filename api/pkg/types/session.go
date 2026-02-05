package types

import (
	"time"
)

// Note: AuthProvider type and constants (AuthProviderRegular, AuthProviderOIDC)
// are already defined in authz.go - we reuse them here.

// UserSession represents an authenticated user session.
// This is the core of the BFF (Backend-For-Frontend) authentication system.
//
// The frontend only sees the session ID via an HttpOnly cookie.
// All token management (OIDC refresh, etc.) happens transparently on the backend.
//
// Similar pattern to OAuthConnection but specifically for user authentication.
type UserSession struct {
	ID        string    `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// User who owns this session
	UserID string `json:"user_id" gorm:"not null;index"`

	// Auth provider used: "regular" or "oidc"
	AuthProvider AuthProvider `json:"auth_provider" gorm:"not null;type:text"`

	// Session expiry (30 days from creation by default)
	ExpiresAt time.Time `json:"expires_at" gorm:"not null;index"`

	// For OIDC sessions: store the refresh token so backend can refresh access tokens
	// These are never exposed to the frontend (json:"-")
	OIDCRefreshToken string    `json:"-" gorm:"column:oidc_refresh_token;type:text"`
	OIDCAccessToken  string    `json:"-" gorm:"column:oidc_access_token;type:text"`
	OIDCTokenExpiry  time.Time `json:"-" gorm:"column:oidc_token_expiry"`

	// Optional metadata for security/audit
	UserAgent  string    `json:"user_agent,omitempty" gorm:"type:text"`
	IPAddress  string    `json:"ip_address,omitempty"`
	LastUsedAt time.Time `json:"last_used_at"`
}

// TableName returns the table name for UserSession
func (UserSession) TableName() string {
	return "user_sessions"
}

// IsExpired returns true if the session has expired
func (s *UserSession) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// NeedsOIDCRefresh returns true if the OIDC access token needs to be refreshed
// We refresh if the token expires within the next 5 minutes
func (s *UserSession) NeedsOIDCRefresh() bool {
	if s.AuthProvider != AuthProviderOIDC {
		return false
	}
	if s.OIDCRefreshToken == "" {
		return false
	}
	// Refresh if token expires within 5 minutes
	return time.Now().Add(5 * time.Minute).After(s.OIDCTokenExpiry)
}

// UpdateOIDCTokens updates the OIDC tokens after a refresh
func (s *UserSession) UpdateOIDCTokens(accessToken, refreshToken string, expiry time.Time) {
	s.OIDCAccessToken = accessToken
	if refreshToken != "" {
		s.OIDCRefreshToken = refreshToken
	}
	s.OIDCTokenExpiry = expiry
	s.UpdatedAt = time.Now()
}

// Touch updates the LastUsedAt timestamp
func (s *UserSession) Touch() {
	s.LastUsedAt = time.Now()
}

// SessionInfo is the public session information returned to the frontend
// It does NOT include any tokens - just session metadata
type SessionInfo struct {
	ID           string           `json:"id"`
	UserID       string           `json:"user_id"`
	AuthProvider AuthProvider `json:"auth_provider"`
	CreatedAt    time.Time        `json:"created_at"`
	ExpiresAt    time.Time        `json:"expires_at"`
}

// ToSessionInfo converts a UserSession to public SessionInfo
func (s *UserSession) ToSessionInfo() *SessionInfo {
	return &SessionInfo{
		ID:           s.ID,
		UserID:       s.UserID,
		AuthProvider: s.AuthProvider,
		CreatedAt:    s.CreatedAt,
		ExpiresAt:    s.ExpiresAt,
	}
}
