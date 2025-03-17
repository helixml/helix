package oauth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/tidwall/gjson"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// AtlassianProvider implements the Provider interface for Atlassian
type AtlassianProvider struct {
	config      *types.OAuthProvider
	store       store.Store
	privateKey  *rsa.PrivateKey
	oauthConfig *oauth1.Config
}

// NewAtlassianProvider creates a new Atlassian provider
func NewAtlassianProvider(ctx context.Context, config *types.OAuthProvider, store store.Store) (Provider, error) {
	if config.Type != types.OAuthProviderTypeAtlassian {
		return nil, fmt.Errorf("invalid provider type: %s", config.Type)
	}

	if config.Version != types.OAuthVersion1 {
		return nil, fmt.Errorf("invalid OAuth version for Atlassian: %s", config.Version)
	}

	// Parse the RSA private key
	if config.PrivateKey == "" {
		return nil, errors.New("private key is required for Atlassian OAuth")
	}

	block, _ := pem.Decode([]byte(config.PrivateKey))
	if block == nil {
		return nil, errors.New("failed to parse PEM block containing private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Create the OAuth1 config
	oauthConfig := &oauth1.Config{
		ConsumerKey:    config.ClientID,
		ConsumerSecret: "", // Not used with RSA-SHA1
		CallbackURL:    config.CallbackURL,
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: config.RequestTokenURL,
			AuthorizeURL:    config.AuthorizeURL,
			AccessTokenURL:  config.TokenURL,
		},
		Signer: &oauth1.RSASigner{
			PrivateKey: privateKey,
		},
	}

	return &AtlassianProvider{
		config:      config,
		store:       store,
		privateKey:  privateKey,
		oauthConfig: oauthConfig,
	}, nil
}

// GetID returns the provider ID
func (p *AtlassianProvider) GetID() string {
	return p.config.ID
}

// GetName returns the provider name
func (p *AtlassianProvider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *AtlassianProvider) GetType() types.OAuthProviderType {
	return p.config.Type
}

// GetVersion returns the OAuth version
func (p *AtlassianProvider) GetVersion() types.OAuthVersion {
	return p.config.Version
}

// GetUserInfo gets the user information from Atlassian
func (p *AtlassianProvider) GetUserInfo(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthUserInfo, error) {
	return p.getUserInfo(connection.AccessToken, connection.TokenSecret)
}

// GetAuthorizationURL generates the authorization URL for the OAuth flow
func (p *AtlassianProvider) GetAuthorizationURL(ctx context.Context, userID, redirectURL string) (string, error) {
	// Create a request token
	requestToken, requestSecret, err := p.oauthConfig.RequestToken()
	if err != nil {
		return "", fmt.Errorf("failed to get request token: %w", err)
	}

	// Store the request token and secret
	tokenObj := &types.OAuthRequestToken{
		UserID:      userID,
		ProviderID:  p.config.ID,
		Token:       requestToken,
		TokenSecret: requestSecret,
		RedirectURL: redirectURL,
		ExpiresAt:   time.Now().Add(30 * time.Minute), // Request tokens typically expire quickly
	}

	tokenObj, err = p.store.CreateOAuthRequestToken(ctx, tokenObj)
	if err != nil {
		return "", fmt.Errorf("failed to store request token: %w", err)
	}

	// Generate the authorization URL
	authURL, err := p.oauthConfig.AuthorizationURL(requestToken)
	if err != nil {
		return "", fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	return authURL.String(), nil
}

// CompleteAuthorization exchanges the authorization code for an access token
func (p *AtlassianProvider) CompleteAuthorization(ctx context.Context, userID, verifier string) (*types.OAuthConnection, error) {
	// Get the most recent request token for this user and provider
	requestTokens, err := p.store.GetOAuthRequestToken(ctx, userID, p.config.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get request token: %w", err)
	}

	if len(requestTokens) == 0 {
		return nil, errors.New("no request token found")
	}

	// Use the most recent request token
	requestToken := requestTokens[0]

	// Delete the request token as it's no longer needed
	defer p.store.DeleteOAuthRequestToken(ctx, requestToken.ID)

	// Check if the request token has expired
	if time.Now().After(requestToken.ExpiresAt) {
		return nil, errors.New("request token has expired")
	}

	// Exchange the request token for an access token
	accessToken, accessSecret, err := p.oauthConfig.AccessToken(requestToken.Token, requestToken.TokenSecret, verifier)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	// Get user information
	userInfo, err := p.getUserInfo(accessToken, accessSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Create the connection
	connection := &types.OAuthConnection{
		UserID:         userID,
		ProviderID:     p.config.ID,
		AccessToken:    accessToken,
		TokenSecret:    accessSecret,
		RefreshToken:   "",          // OAuth 1.0a doesn't use refresh tokens
		ExpiresAt:      time.Time{}, // OAuth 1.0a tokens don't expire unless revoked
		Scopes:         []string{},
		ProviderUserID: userInfo.ID,
		Profile:        userInfo,
	}

	return connection, nil
}

// RefreshTokenIfNeeded refreshes the token if needed
func (p *AtlassianProvider) RefreshTokenIfNeeded(ctx context.Context, connection *types.OAuthConnection) error {
	// OAuth 1.0a doesn't use refresh tokens, tokens don't expire unless revoked
	// We can check if the token is still valid by making a test request

	// Try to get user info to test if token is still valid
	userInfo, err := p.getUserInfo(connection.AccessToken, connection.TokenSecret)
	if err != nil {
		// If we get an error, the token might be invalid
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Update user info in the connection
	connection.Profile = userInfo

	return nil
}

// getUserInfo gets the user information from Atlassian
func (p *AtlassianProvider) getUserInfo(token, tokenSecret string) (*types.OAuthUserInfo, error) {
	// Create an HTTP client with the OAuth1 token
	config := &oauth1.Config{
		ConsumerKey:    p.config.ClientID,
		ConsumerSecret: "", // Not used with RSA-SHA1
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: p.config.RequestTokenURL,
			AuthorizeURL:    p.config.AuthorizeURL,
			AccessTokenURL:  p.config.TokenURL,
		},
		Signer: &oauth1.RSASigner{
			PrivateKey: p.privateKey,
		},
	}

	httpClient := config.Client(oauth1.NoContext, &oauth1.Token{
		Token:       token,
		TokenSecret: tokenSecret,
	})

	// Make a request to get user info
	userInfoURL := p.config.UserInfoURL
	if userInfoURL == "" {
		// Default to Atlassian's user info endpoint if not specified
		baseURL := strings.TrimSuffix(p.config.AuthorizeURL, "/authorize")
		userInfoURL = fmt.Sprintf("%s/rest/api/latest/myself", baseURL)
	}

	resp, err := httpClient.Get(userInfoURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read user info response: %w", err)
	}

	// Parse the response
	userInfo := &types.OAuthUserInfo{
		ID:          gjson.GetBytes(body, "accountId").String(),
		Email:       gjson.GetBytes(body, "emailAddress").String(),
		Name:        gjson.GetBytes(body, "displayName").String(),
		DisplayName: gjson.GetBytes(body, "displayName").String(),
		AvatarURL:   gjson.GetBytes(body, "avatarUrls.48x48").String(),
		Raw:         string(body),
	}

	if userInfo.ID == "" {
		return nil, errors.New("user ID not found in response")
	}

	return userInfo, nil
}

// MakeAuthorizedRequest makes a request to the provider's API with authorization
func (p *AtlassianProvider) MakeAuthorizedRequest(ctx context.Context, connection *types.OAuthConnection, method, url string, body io.Reader) (*http.Response, error) {
	// Create an HTTP client with the OAuth1 token
	config := &oauth1.Config{
		ConsumerKey:    p.config.ClientID,
		ConsumerSecret: "", // Not used with RSA-SHA1
		Endpoint: oauth1.Endpoint{
			RequestTokenURL: p.config.RequestTokenURL,
			AuthorizeURL:    p.config.AuthorizeURL,
			AccessTokenURL:  p.config.TokenURL,
		},
		Signer: &oauth1.RSASigner{
			PrivateKey: p.privateKey,
		},
	}

	httpClient := config.Client(oauth1.NoContext, &oauth1.Token{
		Token:       connection.AccessToken,
		TokenSecret: connection.TokenSecret,
	})

	// Make the request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" || method == "PATCH" {
		req.Header.Set("Content-Type", "application/json")
	}

	return httpClient.Do(req)
}
