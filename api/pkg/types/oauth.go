package types

import (
	"time"

	"gorm.io/gorm"
)

// OAuthVersion represents the OAuth protocol version
type OAuthVersion string

const (
	OAuthVersionUnknown OAuthVersion = ""
	OAuthVersion1       OAuthVersion = "1.0a"
	OAuthVersion2       OAuthVersion = "2.0"
)

// OAuthProviderType represents the type of OAuth provider
type OAuthProviderType string

const (
	OAuthProviderTypeUnknown   OAuthProviderType = ""
	OAuthProviderTypeAtlassian OAuthProviderType = "atlassian"
	OAuthProviderTypeGoogle    OAuthProviderType = "google"
	OAuthProviderTypeMicrosoft OAuthProviderType = "microsoft"
	OAuthProviderTypeGitHub    OAuthProviderType = "github"
	OAuthProviderTypeCustom    OAuthProviderType = "custom"
)

// OAuthProvider represents an OAuth provider configuration
type OAuthProvider struct {
	ID          string            `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time         `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time         `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt    `json:"deleted_at" gorm:"index"`
	Name        string            `json:"name" gorm:"not null"`
	Description string            `json:"description"`
	Type        OAuthProviderType `json:"type" gorm:"not null;type:text"`
	Version     OAuthVersion      `json:"version" gorm:"not null;default:'2.0';type:text"`

	// Common fields for all providers
	ClientID     string `json:"client_id" gorm:"not null"`
	ClientSecret string `json:"client_secret" gorm:"type:text"`

	// OAuth 2.0 specific fields
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	UserInfoURL  string `json:"user_info_url"`
	CallbackURL  string `json:"callback_url"`
	DiscoveryURL string `json:"discovery_url"`

	// OAuth 1.0a specific fields
	RequestTokenURL string `json:"request_token_url"`
	AccessTokenURL  string `json:"access_token_url"`
	AuthorizeURL    string `json:"authorize_url"`
	BaseURL         string `json:"base_url"` // For Atlassian server

	// RSA keys for OAuth 1.0a
	PublicKey  string `json:"public_key" gorm:"type:text"`
	PrivateKey string `json:"private_key" gorm:"type:text"`

	// Who created/owns this provider
	CreatorID   string    `json:"creator_id" gorm:"not null;index"`
	CreatorType OwnerType `json:"creator_type" gorm:"not null;type:text"`

	// Misc configuration
	Scopes  []string `json:"scopes" gorm:"type:text;serializer:json"`
	Enabled bool     `json:"enabled" gorm:"not null;default:true"`
}

// OAuthConnection represents a user's connection to an OAuth provider
type OAuthConnection struct {
	ID         string         `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt  time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"index"`
	UserID     string         `json:"user_id" gorm:"not null;index"`
	ProviderID string         `json:"provider_id" gorm:"not null;index;type:uuid"`

	// Provider is a reference to the OAuth provider
	Provider OAuthProvider `json:"provider" gorm:"foreignKey:ProviderID"`

	// OAuth 1.0a fields
	AccessToken string `json:"access_token" gorm:"not null;type:text"`
	TokenSecret string `json:"token_secret" gorm:"type:text"`

	// OAuth 2.0 fields
	RefreshToken string    `json:"refresh_token" gorm:"type:text"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes" gorm:"type:text;serializer:json"`

	// User details from the provider
	ProviderUserID    string         `json:"provider_user_id"`
	ProviderUserEmail string         `json:"provider_user_email"`
	ProviderUsername  string         `json:"provider_username"`
	Profile           *OAuthUserInfo `json:"profile" gorm:"type:text;serializer:json"`
	Metadata          string         `json:"metadata" gorm:"type:text"`

	// Unique constraint
	UniqueConnection string `json:"unique_connection" gorm:"uniqueIndex"`
}

// OAuthRequestToken temporarily stores OAuth 1.0a request tokens during flow
// or OAuth 2.0 state parameters during authorization code flow
type OAuthRequestToken struct {
	ID          string    `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UserID      string    `json:"user_id" gorm:"not null;index"`
	ProviderID  string    `json:"provider_id" gorm:"not null;index;type:uuid"`
	Token       string    `json:"token"`
	TokenSecret string    `json:"token_secret" gorm:"type:text"`
	State       string    `json:"state" gorm:"index"`
	RedirectURL string    `json:"redirect_url" gorm:"type:text"`
	ExpiresAt   time.Time `json:"expires_at" gorm:"not null;index"`
}

// OAuthUserInfo represents standardized user information from any OAuth provider
type OAuthUserInfo struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Raw         string `json:"raw"` // Raw JSON response from provider
}

// BeforeCreate sets default values for new records
func (p *OAuthProvider) BeforeCreate(tx *gorm.DB) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating
func (p *OAuthProvider) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate sets default values for new connections
func (c *OAuthConnection) BeforeCreate(tx *gorm.DB) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating connections
func (c *OAuthConnection) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate sets default values for new request tokens
func (t *OAuthRequestToken) BeforeCreate(tx *gorm.DB) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	return nil
}
