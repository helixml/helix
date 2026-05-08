package types

import (
	"time"

	"github.com/lib/pq"
)

// ClaudeSubscription represents a user's or organization's Claude subscription credentials.
// Claude OAuth tokens only work through Claude Code (not as generic Anthropic API keys),
// so this is a specialized provider type separate from ProviderEndpoint.
type ClaudeSubscription struct {
	ID                   string         `json:"id" gorm:"primaryKey"`
	Created              time.Time      `json:"created"`
	Updated              time.Time      `json:"updated"`
	OwnerID              string         `json:"owner_id" gorm:"not null;index"`
	OwnerType            OwnerType      `json:"owner_type" gorm:"not null"` // "user" or "org"
	Name                 string         `json:"name"`
	EncryptedCredentials string         `json:"-" gorm:"type:text;not null"` // AES-256-GCM encrypted credentials JSON
	CredentialType       string         `json:"credential_type" gorm:"default:'oauth'"` // "oauth" or "setup_token"
	SubscriptionType     string         `json:"subscription_type"`                       // "max", "pro"
	RateLimitTier        string         `json:"rate_limit_tier"`
	Scopes               pq.StringArray `json:"scopes" gorm:"type:text[]"`
	AccessTokenExpiresAt time.Time      `json:"access_token_expires_at"`
	Status               string         `json:"status"`                    // "active", "expired", "error"
	LastRefreshedAt      *time.Time     `json:"last_refreshed_at,omitempty"`
	LastError            string         `json:"last_error,omitempty"`
	CreatedBy            string         `json:"created_by" gorm:"not null"`
}

// ClaudeOAuthCredentials contains the raw OAuth credentials from Claude's credentials file.
// These are stored encrypted at rest and only decrypted when needed by containers.
type ClaudeOAuthCredentials struct {
	AccessToken      string   `json:"accessToken"`
	RefreshToken     string   `json:"refreshToken"`
	ExpiresAt        int64    `json:"expiresAt"` // Unix milliseconds
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscriptionType"`
	RateLimitTier    string   `json:"rateLimitTier"`
}

// ClaudeSetupTokenCredentials stores a token from `claude setup-token`.
// This is an opaque long-lived OAuth token injected as CLAUDE_CODE_OAUTH_TOKEN.
type ClaudeSetupTokenCredentials struct {
	SetupToken string `json:"setupToken"`
}

// CreateClaudeSubscriptionRequest is the request body for creating a Claude subscription.
type CreateClaudeSubscriptionRequest struct {
	Name        string    `json:"name"`
	OwnerType   OwnerType `json:"owner_type"`              // "user" or "org"
	OwnerID     string    `json:"owner_id,omitempty"`       // Required for org-level, auto-set for user
	SetupToken  string    `json:"setup_token,omitempty"`    // From `claude setup-token` (alternative to credentials)
	Credentials struct {
		ClaudeAiOauth ClaudeOAuthCredentials `json:"claudeAiOauth"`
	} `json:"credentials"`
}
