package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// OAuth2Provider implements the Provider interface for generic OAuth 2.0 providers
type OAuth2Provider struct {
	config      *types.OAuthProvider
	store       store.Store
	oauthConfig *oauth2.Config
	provider    *oidc.Provider
	verifier    *oidc.IDTokenVerifier
}

// NewOAuth2Provider creates a new OAuth 2.0 provider
func NewOAuth2Provider(ctx context.Context, config *types.OAuthProvider, store store.Store) (Provider, error) {
	// Always use OAuth 2.0 regardless of what is specified in the config

	var provider *oidc.Provider
	var verifier *oidc.IDTokenVerifier
	var err error

	// If discovery URL is provided, use it to set up OIDC provider
	if config.DiscoveryURL != "" {
		provider, err = oidc.NewProvider(ctx, config.DiscoveryURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create OIDC provider: %w", err)
		}

		verifier = provider.Verifier(&oidc.Config{
			ClientID: config.ClientID,
		})
	}

	// Create OAuth2 config
	oauthConfig := &oauth2.Config{
		ClientID:     config.ClientID,
		ClientSecret: config.ClientSecret,
		RedirectURL:  config.CallbackURL,
		Scopes:       config.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  config.AuthURL,
			TokenURL: config.TokenURL,
		},
	}

	return &OAuth2Provider{
		config:      config,
		store:       store,
		oauthConfig: oauthConfig,
		provider:    provider,
		verifier:    verifier,
	}, nil
}

// GetID returns the provider ID
func (p *OAuth2Provider) GetID() string {
	return p.config.ID
}

// GetName returns the provider name
func (p *OAuth2Provider) GetName() string {
	return p.config.Name
}

// GetType returns the provider type
func (p *OAuth2Provider) GetType() types.OAuthProviderType {
	return p.config.Type
}

// GetAuthorizationURL generates the authorization URL for the OAuth flow
// metadata is optional JSON string with provider-specific data (e.g., organization_url for Azure DevOps)
func (p *OAuth2Provider) GetAuthorizationURL(ctx context.Context, userID, redirectURL, metadata string) (string, error) {
	// Generate a random state
	state, err := p.store.GenerateRandomState(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Store the state in the database with the user ID
	tokenObj := &types.OAuthRequestToken{
		UserID:     userID,
		ProviderID: p.config.ID,
		State:      state,
		Metadata:   metadata,
		ExpiresAt:  time.Now().Add(30 * time.Minute),
	}

	_, err = p.store.CreateOAuthRequestToken(ctx, tokenObj)
	if err != nil {
		return "", fmt.Errorf("failed to store state: %w", err)
	}

	// Use the original redirect URL from the config if none is provided
	if redirectURL == "" {
		redirectURL = p.config.CallbackURL
	}

	// Clone the config to use a custom redirect URL if provided
	oauth2Config := &oauth2.Config{
		ClientID:     p.oauthConfig.ClientID,
		ClientSecret: p.oauthConfig.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       p.oauthConfig.Scopes,
		Endpoint:     p.oauthConfig.Endpoint,
	}

	// Generate the authorization URL
	authURL := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
	// authURL := oauth2Config.AuthCodeURL(state)

	return authURL, nil
}

// CompleteAuthorization exchanges the authorization code for an access token
func (p *OAuth2Provider) CompleteAuthorization(ctx context.Context, userID, code string) (*types.OAuthConnection, error) {
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
	defer func() {
		if err := p.store.DeleteOAuthRequestToken(ctx, requestToken.ID); err != nil {
			log.Error().Err(err).Str("requestTokenID", requestToken.ID).Msg("Failed to delete OAuth request token")
		}
	}()

	// Check if the request token has expired
	if time.Now().After(requestToken.ExpiresAt) {
		return nil, errors.New("request token has expired")
	}

	// Exchange the authorization code for a token
	token, err := p.oauthConfig.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Get user information
	userInfo, err := p.getUserInfo(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	// Create connection record
	connection := &types.OAuthConnection{
		UserID:            userID,
		ProviderID:        p.config.ID,
		AccessToken:       token.AccessToken,
		RefreshToken:      token.RefreshToken,
		ExpiresAt:         token.Expiry,
		Scopes:            p.config.Scopes,
		ProviderUserID:    userInfo.ID,
		ProviderUserEmail: userInfo.Email,
		ProviderUsername:  userInfo.DisplayName,
		Profile:           userInfo,
		Metadata:          requestToken.Metadata, // Transfer metadata from request token (e.g., organization_url for ADO)
	}

	return connection, nil
}

// RefreshTokenIfNeeded refreshes the token if it's expired or about to expire
func (p *OAuth2Provider) RefreshTokenIfNeeded(ctx context.Context, connection *types.OAuthConnection) error {
	// Check if the token needs refreshing (expired or expires soon)
	// Treat zero time value (0001-01-01T00:00:00Z) as non-expiring token
	if connection.ExpiresAt.IsZero() || connection.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
		return nil // Token is still valid
	}

	// Check if we have a refresh token
	if connection.RefreshToken == "" {
		return errors.New("no refresh token available")
	}

	// Create a token for refreshing
	token := &oauth2.Token{
		AccessToken:  connection.AccessToken,
		RefreshToken: connection.RefreshToken,
		Expiry:       connection.ExpiresAt,
	}

	// Create the token source
	tokenSource := p.oauthConfig.TokenSource(ctx, token)

	log.Info().Str("provider_id", p.config.ID).Str("user_id", connection.UserID).Msg("Refreshing token")

	// Refresh the token
	newToken, err := tokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update the connection with the new token
	connection.AccessToken = newToken.AccessToken
	if newToken.RefreshToken != "" {
		connection.RefreshToken = newToken.RefreshToken
	}
	connection.ExpiresAt = newToken.Expiry

	// Get updated user info
	userInfo, err := p.getUserInfo(ctx, newToken)
	if err != nil {
		return fmt.Errorf("failed to get updated user info: %w", err)
	}

	connection.Profile = userInfo

	return nil
}

// getUserInfo gets the user information from the provider
func (p *OAuth2Provider) getUserInfo(ctx context.Context, token *oauth2.Token) (*types.OAuthUserInfo, error) {
	// Create an HTTP client with the token
	client := p.oauthConfig.Client(ctx, token)

	// If OIDC provider is available, try to get user info from it
	if p.provider != nil && p.verifier != nil && token.Extra("id_token") != nil {
		// Parse and verify the ID token
		idToken, err := p.verifier.Verify(ctx, token.Extra("id_token").(string))
		if err != nil {
			return nil, fmt.Errorf("failed to verify ID token: %w", err)
		}

		// Extract claims
		var claims map[string]interface{}
		if err := idToken.Claims(&claims); err != nil {
			return nil, fmt.Errorf("failed to extract claims: %w", err)
		}

		userInfo := &types.OAuthUserInfo{
			ID:          getStringClaim(claims, "sub"),
			Email:       getStringClaim(claims, "email"),
			Name:        getStringClaim(claims, "name"),
			DisplayName: getStringClaim(claims, "name"),
			AvatarURL:   getStringClaim(claims, "picture"),
			Raw:         toJSONString(claims),
		}

		return userInfo, nil
	}

	// Otherwise, use the UserInfo endpoint
	userInfoURL := p.config.UserInfoURL
	if userInfoURL == "" {
		return nil, errors.New("user info URL not specified")
	}

	// If URL contains the token, replace it
	userInfoURL = strings.Replace(userInfoURL, "{token}", token.AccessToken, 1)

	resp, err := client.Get(userInfoURL)
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

	if strings.Contains(userInfoURL, "https://api.hubapi.com") {
		return p.parseHubSpotUserInfo(body)
	}

	// Standard providers

	// Try to parse as JSON
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("failed to parse user info response: %w", err)
	}

	// Extract user info based on provider type
	var id, email, name, displayName, avatarURL string

	switch p.config.Type {
	case types.OAuthProviderTypeGitHub:
		id = getStringValue(data, "id")
		email = getStringValue(data, "email")
		name = getStringValue(data, "name")
		displayName = getStringValue(data, "login")
		avatarURL = getStringValue(data, "avatar_url")
	case types.OAuthProviderTypeGoogle:
		id = getStringValue(data, "sub")
		email = getStringValue(data, "email")
		name = getStringValue(data, "name")
		displayName = getStringValue(data, "name")
		avatarURL = getStringValue(data, "picture")
	case types.OAuthProviderTypeMicrosoft:
		id = getStringValue(data, "id")
		email = getStringValue(data, "userPrincipalName")
		name = getStringValue(data, "displayName")
		displayName = getStringValue(data, "displayName")
	case types.OAuthProviderTypeAtlassian:
		id = getStringValue(data, "account_id")
		email = getStringValue(data, "email")
		name = getStringValue(data, "name")
		displayName = getStringValue(data, "nickname")
		if displayName == "" {
			displayName = name
		}
		avatarURL = getStringValue(data, "picture")
	default:
		// For custom providers, try some common field names
		id = getStringValue(data, "id")
		if id == "" {
			id = getStringValue(data, "sub")
		}
		email = getStringValue(data, "email")
		name = getStringValue(data, "name")
		displayName = getStringValue(data, "display_name")
		if displayName == "" {
			displayName = name
		}
		avatarURL = getStringValue(data, "avatar_url")
		if avatarURL == "" {
			avatarURL = getStringValue(data, "picture")
		}
	}

	// Make sure we have an ID
	if id == "" {
		return nil, errors.New("user ID not found in response")
	}

	userInfo := &types.OAuthUserInfo{
		ID:          id,
		Email:       email,
		Name:        name,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		Raw:         string(body),
	}

	return userInfo, nil
}

// MakeAuthorizedRequest makes a request to the provider's API with authorization
func (p *OAuth2Provider) MakeAuthorizedRequest(ctx context.Context, connection *types.OAuthConnection, method, url string, body io.Reader) (*http.Response, error) {
	// Create a token for the request
	token := &oauth2.Token{
		AccessToken:  connection.AccessToken,
		RefreshToken: connection.RefreshToken,
		Expiry:       connection.ExpiresAt,
	}

	// Create an HTTP client with the token
	client := p.oauthConfig.Client(ctx, token)

	// Make the request
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	if method == "POST" || method == "PUT" || method == "PATCH" {
		req.Header.Set("Content-Type", "application/json")
	}

	return client.Do(req)
}

// Helper functions

// getStringClaim extracts a string claim from the claims map
func getStringClaim(claims map[string]interface{}, key string) string {
	value, ok := claims[key]
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
}

// getStringValue extracts a string value from a map
func getStringValue(data map[string]interface{}, key string) string {
	value, ok := data[key]
	if !ok {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}

// toJSONString converts a map to a JSON string
func toJSONString(data map[string]interface{}) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	return string(bytes)
}

// GetUserInfo gets the user information from the provider
func (p *OAuth2Provider) GetUserInfo(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthUserInfo, error) {
	// Create a token for the request
	token := &oauth2.Token{
		AccessToken:  connection.AccessToken,
		RefreshToken: connection.RefreshToken,
		Expiry:       connection.ExpiresAt,
	}

	return p.getUserInfo(ctx, token)
}
