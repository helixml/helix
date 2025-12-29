package oauth

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// Provider defines the interface for all OAuth providers (both OAuth 1.0a and OAuth 2.0)
type Provider interface {
	// Basic provider information
	GetID() string
	GetName() string
	GetType() types.OAuthProviderType

	// OAuth flow
	// metadata is optional JSON string with provider-specific data (e.g., organization_url for Azure DevOps)
	GetAuthorizationURL(ctx context.Context, userID, redirectURL, metadata string) (string, error)
	CompleteAuthorization(ctx context.Context, userID, code string) (*types.OAuthConnection, error)

	// Token management
	RefreshTokenIfNeeded(ctx context.Context, connection *types.OAuthConnection) error

	// User information
	GetUserInfo(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthUserInfo, error)

	// API interactions
	MakeAuthorizedRequest(ctx context.Context, connection *types.OAuthConnection, method, url string, body io.Reader) (*http.Response, error)
}

// ProviderConfig contains common configuration for OAuth providers
type ProviderConfig struct {
	ID              string
	Name            string
	Description     string
	CreatorID       string
	CreatorType     string
	Enabled         bool
	ClientID        string
	ClientSecret    string
	RedirectURL     string
	Scopes          []string
	CallbackURL     string
	AuthorizeURL    string
	TokenURL        string
	UserInfoURL     string
	DiscoveryURL    string
	PrivateKey      string
	RequestTokenURL string
}

// Connection represents a user's connection to an OAuth provider
type Connection struct {
	UserID        string
	ProviderID    string
	AccessToken   string
	RefreshToken  string
	ExpiresAt     time.Time
	Scopes        []string
	Profile       *types.OAuthUserInfo
	ProviderType  types.OAuthProviderType
	LastRefreshed time.Time
}
