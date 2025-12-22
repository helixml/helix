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
	// Provider type: "github", "gitlab", "ado", "bitbucket"
	ProviderType ExternalRepositoryType `json:"provider_type"`
	// Personal Access Token or App Password for authentication
	Token string `json:"token"`
	// Username for authentication (required for Bitbucket)
	Username string `json:"username,omitempty"`
	// Organization URL (required for Azure DevOps)
	OrganizationURL string `json:"organization_url,omitempty"`
	// Base URL for self-hosted instances (for GitHub Enterprise, GitLab Enterprise, or Bitbucket Server)
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

	// Username for authentication (required for Bitbucket, stored encrypted)
	AuthUsername string `json:"-" gorm:"type:text"`

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
	// Username for authentication (required for Bitbucket)
	AuthUsername    string                 `json:"auth_username,omitempty"`
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

// ServiceConnectionType represents the type of service connection
type ServiceConnectionType string

const (
	ServiceConnectionTypeGitHubApp            ServiceConnectionType = "github_app"
	ServiceConnectionTypeADOServicePrincipal  ServiceConnectionType = "ado_service_principal"
)

// ServiceConnection represents a service-to-service authentication configuration
// These are admin-configured connections that can be used across multiple repositories
// Examples: GitHub Apps, Azure DevOps Service Principals
type ServiceConnection struct {
	ID        string         `json:"id" gorm:"primaryKey;type:uuid"`
	CreatedAt time.Time      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time      `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Organization that owns this connection (admin-only, org-scoped)
	OrganizationID string `json:"organization_id" gorm:"index"`

	// Display name for the connection
	Name        string `json:"name" gorm:"not null"`
	Description string `json:"description"`

	// Connection type determines which credentials are used
	Type ServiceConnectionType `json:"type" gorm:"not null;type:text;index"`

	// Provider type for filtering (github, ado, etc.)
	ProviderType ExternalRepositoryType `json:"provider_type" gorm:"not null;type:text;index"`

	// GitHub App credentials (encrypted at rest)
	GitHubAppID          int64  `json:"github_app_id,omitempty"`
	GitHubInstallationID int64  `json:"github_installation_id,omitempty"`
	GitHubPrivateKey     string `json:"-" gorm:"type:text"` // PEM-encoded, sensitive

	// Azure DevOps Service Principal credentials (encrypted at rest)
	ADOOrganizationURL string `json:"ado_organization_url,omitempty"`
	ADOTenantID        string `json:"ado_tenant_id,omitempty"`
	ADOClientID        string `json:"ado_client_id,omitempty"`
	ADOClientSecret    string `json:"-" gorm:"type:text"` // Sensitive

	// Base URL for enterprise/self-hosted instances
	BaseURL string `json:"base_url,omitempty"`

	// Connection status
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// TableName returns the table name for ServiceConnection
func (ServiceConnection) TableName() string {
	return "service_connections"
}

// BeforeCreate sets default values for new service connections
func (c *ServiceConnection) BeforeCreate(_ *gorm.DB) error {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate sets updated_at before updating service connections
func (c *ServiceConnection) BeforeUpdate(_ *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

// ServiceConnectionCreateRequest is the request body for creating a service connection
type ServiceConnectionCreateRequest struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Type        ServiceConnectionType `json:"type"`

	// GitHub App fields
	GitHubAppID          int64  `json:"github_app_id,omitempty"`
	GitHubInstallationID int64  `json:"github_installation_id,omitempty"`
	GitHubPrivateKey     string `json:"github_private_key,omitempty"`

	// Azure DevOps Service Principal fields
	ADOOrganizationURL string `json:"ado_organization_url,omitempty"`
	ADOTenantID        string `json:"ado_tenant_id,omitempty"`
	ADOClientID        string `json:"ado_client_id,omitempty"`
	ADOClientSecret    string `json:"ado_client_secret,omitempty"`

	// Base URL for enterprise/self-hosted instances
	BaseURL string `json:"base_url,omitempty"`
}

// ServiceConnectionUpdateRequest is the request body for updating a service connection
type ServiceConnectionUpdateRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	// GitHub App fields (only update if provided)
	GitHubAppID          *int64  `json:"github_app_id,omitempty"`
	GitHubInstallationID *int64  `json:"github_installation_id,omitempty"`
	GitHubPrivateKey     *string `json:"github_private_key,omitempty"`

	// Azure DevOps Service Principal fields (only update if provided)
	ADOOrganizationURL *string `json:"ado_organization_url,omitempty"`
	ADOTenantID        *string `json:"ado_tenant_id,omitempty"`
	ADOClientID        *string `json:"ado_client_id,omitempty"`
	ADOClientSecret    *string `json:"ado_client_secret,omitempty"`

	// Base URL for enterprise/self-hosted instances
	BaseURL *string `json:"base_url,omitempty"`
}

// ServiceConnectionResponse is the API response for a service connection (hides sensitive fields)
type ServiceConnectionResponse struct {
	ID             string                 `json:"id"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	OrganizationID string                 `json:"organization_id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Type           ServiceConnectionType  `json:"type"`
	ProviderType   ExternalRepositoryType `json:"provider_type"`

	// GitHub App (non-sensitive fields only)
	GitHubAppID          int64 `json:"github_app_id,omitempty"`
	GitHubInstallationID int64 `json:"github_installation_id,omitempty"`
	HasGitHubPrivateKey  bool  `json:"has_github_private_key,omitempty"`

	// Azure DevOps Service Principal (non-sensitive fields only)
	ADOOrganizationURL string `json:"ado_organization_url,omitempty"`
	ADOTenantID        string `json:"ado_tenant_id,omitempty"`
	ADOClientID        string `json:"ado_client_id,omitempty"`
	HasADOClientSecret bool   `json:"has_ado_client_secret,omitempty"`

	BaseURL      string     `json:"base_url,omitempty"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

// ToResponse converts a ServiceConnection to a ServiceConnectionResponse (hiding sensitive fields)
func (c *ServiceConnection) ToResponse() *ServiceConnectionResponse {
	return &ServiceConnectionResponse{
		ID:                   c.ID,
		CreatedAt:            c.CreatedAt,
		UpdatedAt:            c.UpdatedAt,
		OrganizationID:       c.OrganizationID,
		Name:                 c.Name,
		Description:          c.Description,
		Type:                 c.Type,
		ProviderType:         c.ProviderType,
		GitHubAppID:          c.GitHubAppID,
		GitHubInstallationID: c.GitHubInstallationID,
		HasGitHubPrivateKey:  c.GitHubPrivateKey != "",
		ADOOrganizationURL:   c.ADOOrganizationURL,
		ADOTenantID:          c.ADOTenantID,
		ADOClientID:          c.ADOClientID,
		HasADOClientSecret:   c.ADOClientSecret != "",
		BaseURL:              c.BaseURL,
		LastTestedAt:         c.LastTestedAt,
		LastError:            c.LastError,
	}
}
