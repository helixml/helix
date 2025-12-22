package types

import (
	"time"

	"gorm.io/gorm"
)

// OAuthProviderType represents the type of OAuth provider
type OAuthProviderType string

const (
	OAuthProviderTypeUnknown      OAuthProviderType = ""
	OAuthProviderTypeAtlassian    OAuthProviderType = "atlassian"
	OAuthProviderTypeGoogle       OAuthProviderType = "google"
	OAuthProviderTypeMicrosoft    OAuthProviderType = "microsoft"
	OAuthProviderTypeGitHub       OAuthProviderType = "github"
	OAuthProviderTypeGitLab       OAuthProviderType = "gitlab"
	OAuthProviderTypeAzureDevOps  OAuthProviderType = "azure_devops"
	OAuthProviderTypeSlack        OAuthProviderType = "slack"
	OAuthProviderTypeLinkedIn     OAuthProviderType = "linkedin"
	OAuthProviderTypeHubSpot      OAuthProviderType = "hubspot"
	OAuthProviderTypeCustom       OAuthProviderType = "custom"
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

	// Common fields for all providers
	ClientID     string `json:"client_id" gorm:"not null"`
	ClientSecret string `json:"client_secret" gorm:"type:text"`

	// OAuth 2.0 fields
	AuthURL      string `json:"auth_url"`
	TokenURL     string `json:"token_url"`
	UserInfoURL  string `json:"user_info_url"`
	CallbackURL  string `json:"callback_url"`
	DiscoveryURL string `json:"discovery_url"`

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

	// OAuth token fields
	AccessToken  string    `json:"access_token" gorm:"not null;type:text"`
	RefreshToken string    `json:"refresh_token" gorm:"type:text"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scopes       []string  `json:"scopes" gorm:"type:text;serializer:json"`

	// User details from the provider
	ProviderUserID    string         `json:"provider_user_id"`
	ProviderUserEmail string         `json:"provider_user_email"`
	ProviderUsername  string         `json:"provider_username"`
	Profile           *OAuthUserInfo `json:"profile" gorm:"type:text;serializer:json"`
	Metadata          string         `json:"metadata" gorm:"type:text"`
}

type OAuthConnectionTestResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`

	ProviderDetails map[string]any `json:"provider_details"` // Returned from the provider itself
}

// OAuthRequestToken temporarily stores OAuth state parameters during authorization code flow
type OAuthRequestToken struct {
	ID          string    `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UserID      string    `json:"user_id" gorm:"not null;index"`
	ProviderID  string    `json:"provider_id" gorm:"not null;index;type:uuid"`
	Token       string    `json:"token"` // For compatibility with existing records
	State       string    `json:"state" gorm:"index"`
	RedirectURL string    `json:"redirect_url" gorm:"type:text"`
	Metadata    string    `json:"metadata" gorm:"type:text"` // JSON metadata (e.g., organization_url for ADO)
	ExpiresAt   time.Time `json:"expires_at" gorm:"not null;index"`
}

// OAuthUserInfo represents standardized user information from any OAuth provider
type OAuthUserInfo struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Username    string `json:"username"`     // Provider-specific username (e.g., GitHub login)
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
	Raw         string `json:"raw"` // Raw JSON response from provider
}

// BeforeCreate sets default values for new records
func (p *OAuthProvider) BeforeCreate(_ *gorm.DB) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating
func (p *OAuthProvider) BeforeUpdate(_ *gorm.DB) error {
	p.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate sets default values for new connections
func (c *OAuthConnection) BeforeCreate(_ *gorm.DB) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating connections
func (c *OAuthConnection) BeforeUpdate(_ *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// BeforeCreate sets default values for new request tokens
func (t *OAuthRequestToken) BeforeCreate(_ *gorm.DB) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now()
	}
	return nil
}

// BrowseRemoteRepositoriesRequest is the request body for browsing remote repositories using PAT
type BrowseRemoteRepositoriesRequest struct {
	// Provider type: "github", "gitlab", "ado"
	ProviderType ExternalRepositoryType `json:"provider_type"`
	// Personal Access Token for authentication
	Token string `json:"token"`
	// Organization URL (required for Azure DevOps)
	OrganizationURL string `json:"organization_url,omitempty"`
	// Base URL for self-hosted instances (for GitHub Enterprise or GitLab Enterprise)
	BaseURL string `json:"base_url,omitempty"`
}

// GitProviderConnection represents a user's PAT-based connection to a git provider
// This is separate from OAuthConnection which uses OAuth flow
type GitProviderConnection struct {
	ID        string         `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// User who owns this connection (PAT is personal, not org-level)
	UserID string `json:"user_id" gorm:"not null;index"`

	// Provider type: github, gitlab, ado
	ProviderType ExternalRepositoryType `json:"provider_type" gorm:"not null;type:text"`

	// Display name for the connection (e.g., "My GitHub Account")
	Name string `json:"name"`

	// Personal Access Token (encrypted at rest)
	Token string `json:"-" gorm:"not null;type:text"`

	// For Azure DevOps: organization URL
	OrganizationURL string `json:"organization_url,omitempty"`

	// For GitHub Enterprise or GitLab Enterprise: base URL (empty = github.com/gitlab.com)
	BaseURL string `json:"base_url,omitempty"`

	// User info from the provider (cached from last successful auth)
	Username  string `json:"username,omitempty"`
	Email     string `json:"email,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`

	// Last successful connection test
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
}

// GitProviderConnectionCreateRequest is the request body for creating a PAT connection
type GitProviderConnectionCreateRequest struct {
	ProviderType    ExternalRepositoryType `json:"provider_type"`
	Name            string                 `json:"name,omitempty"`
	Token           string                 `json:"token"`
	OrganizationURL string                 `json:"organization_url,omitempty"`
	BaseURL         string                 `json:"base_url,omitempty"`
}

// BeforeCreate sets default values for new git provider connections
func (c *GitProviderConnection) BeforeCreate(_ *gorm.DB) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating git provider connections
func (c *GitProviderConnection) BeforeUpdate(_ *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}
