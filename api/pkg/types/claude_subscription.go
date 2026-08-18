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
	EncryptedCredentials string         `json:"-" gorm:"type:text;not null"`            // AES-256-GCM encrypted credentials JSON
	CredentialType       string         `json:"credential_type" gorm:"default:'oauth'"` // "oauth" or "setup_token"
	SubscriptionType     string         `json:"subscription_type"`                      // "max", "pro"
	RateLimitTier        string         `json:"rate_limit_tier"`
	Scopes               pq.StringArray `json:"scopes" gorm:"type:text[]"`
	AccessTokenExpiresAt time.Time      `json:"access_token_expires_at"`
	Status               string         `json:"status"` // "active", "expired", "error"

	// DelegatedOrgIDs lists the organizations whose agent sessions may
	// authenticate with this subscription on the owner's behalf, even when the
	// session itself is owned by someone else (a service account dispatching
	// work for this person — see SpecTask.CredentialOwnerID).
	//
	// This is the consent gate. Without it, any member who can create a task
	// could name another user as credential owner and spend their Claude quota.
	// Empty (the default) means the subscription is only ever used for sessions
	// its owner owns, which is the pre-existing behaviour.
	DelegatedOrgIDs pq.StringArray `json:"delegated_org_ids" gorm:"type:text[]"`

	// AccountEmail is the email of the Claude account the stored token
	// authenticates as, fetched from Anthropic's /api/oauth/profile. It is the
	// identity that gets billed and can differ from the Helix user/org (OwnerID)
	// that connected the subscription. Best-effort: empty until a valid probe
	// has enriched the row.
	AccountEmail       string `json:"account_email"`
	AccountDisplayName string `json:"account_display_name"`

	LastRefreshedAt *time.Time `json:"last_refreshed_at,omitempty"`
	LastValidatedAt *time.Time `json:"last_validated_at,omitempty"` // last time the token was liveness-probed against Anthropic
	LastError       string     `json:"last_error,omitempty"`
	CreatedBy       string     `json:"created_by" gorm:"not null"`
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
	Name      string    `json:"name"`
	OwnerType OwnerType `json:"owner_type"`         // "user" or "org"
	OwnerID   string    `json:"owner_id,omitempty"` // Required for org-level, auto-set for user
	// OrganizationID identifies the org whose Claude Code harness is enabled
	// after connection. It is independent from subscription ownership.
	OrganizationID string `json:"organization_id,omitempty"`
	SetupToken     string `json:"setup_token,omitempty"` // From `claude setup-token` (alternative to credentials)
	// AccountEmail is the self-reported email of the Claude account the token
	// belongs to. Anthropic's /api/oauth/profile only serves account identity
	// to tokens holding the user:profile scope — setup tokens never do, so for
	// those the user must report it (or leave it empty). A successful profile
	// fetch during validation overwrites this with the authoritative value.
	AccountEmail     string `json:"account_email,omitempty"`
	SubscriptionType string `json:"subscription_type,omitempty"` // Optional: fills the plan for tokens whose credentials carry none (setup tokens)
	RateLimitTier    string `json:"rate_limit_tier,omitempty"`   // Optional: e.g. "20x"
	Credentials      struct {
		ClaudeAiOauth ClaudeOAuthCredentials `json:"claudeAiOauth"`
	} `json:"credentials"`
}
