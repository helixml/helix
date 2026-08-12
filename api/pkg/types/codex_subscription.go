package types

import "time"

// CodexSubscription stores a user's or organization's ChatGPT credentials for Codex CLI.
type CodexSubscription struct {
	ID                   string     `json:"id" gorm:"primaryKey"`
	Created              time.Time  `json:"created"`
	Updated              time.Time  `json:"updated"`
	OwnerID              string     `json:"owner_id" gorm:"not null;index"`
	OwnerType            OwnerType  `json:"owner_type" gorm:"not null"`
	Name                 string     `json:"name"`
	EncryptedCredentials string     `json:"-" gorm:"type:text;not null"`
	AccountID            string     `json:"account_id"`
	AuthMode             string     `json:"auth_mode"`
	Status               string     `json:"status"`
	LastRefreshedAt      *time.Time `json:"last_refreshed_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
	CreatedBy            string     `json:"created_by" gorm:"not null"`
}

// CodexAuthTokens is the token set persisted by Codex CLI in auth.json.
type CodexAuthTokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

// CodexAuthCredentials is Codex CLI's file-based credential cache.
type CodexAuthCredentials struct {
	AuthMode     string          `json:"auth_mode"`
	OpenAIAPIKey *string         `json:"OPENAI_API_KEY"`
	Tokens       CodexAuthTokens `json:"tokens"`
	LastRefresh  time.Time       `json:"last_refresh"`
}

type CreateCodexSubscriptionRequest struct {
	Name        string               `json:"name"`
	OwnerType   OwnerType            `json:"owner_type"`
	OwnerID     string               `json:"owner_id,omitempty"`
	Credentials CodexAuthCredentials `json:"credentials"`
}
