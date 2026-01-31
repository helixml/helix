package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

var (
	ErrInvalidConfig    = errors.New("invalid config")
	ErrProviderNotReady = errors.New("OIDC provider not ready")
)

type OIDCClient struct {
	cfg          OIDCConfig
	provider     *oidc.Provider
	oauth2Config *oauth2.Config
	adminConfig  *AdminConfig
	store        store.Store
}

var _ OIDC = &OIDCClient{}

type OIDCConfig struct {
	ProviderURL  string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AdminUserIDs []string // List of admin user IDs, or contains "all" for dev mode
	Audience     string
	Scopes       []string
	Store        store.Store
	// ExpectedIssuer allows the OIDC provider to return a different issuer than the ProviderURL.
	// This is useful when the API connects to Keycloak via an internal URL (e.g., keycloak:8080)
	// but Keycloak is configured with an external URL (e.g., localhost:8180) for browser access.
	// If set, the OIDC client will accept tokens with this issuer even though discovery
	// was done via ProviderURL.
	ExpectedIssuer string
	// TokenURL overrides the token endpoint from OIDC discovery.
	// Useful when the API needs an internal URL for token exchange while discovery
	// returns browser-accessible URLs.
	TokenURL string
}

func NewOIDCClient(ctx context.Context, cfg OIDCConfig) (*OIDCClient, error) {
	// Validate the other fields
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("%w: client ID is required", ErrInvalidConfig)
	}
	if cfg.ClientSecret == "" {
		return nil, fmt.Errorf("%w: client secret is required", ErrInvalidConfig)
	}
	if cfg.RedirectURL == "" {
		return nil, fmt.Errorf("%w: redirect URL is required", ErrInvalidConfig)
	}
	if cfg.ProviderURL == "" {
		return nil, fmt.Errorf("%w: provider URL is required", ErrInvalidConfig)
	}

	client := &OIDCClient{
		cfg: cfg,
		adminConfig: &AdminConfig{
			AdminUserIDs: cfg.AdminUserIDs,
		},
		store: cfg.Store,
	}

	// Start a go routine to periodically check if the provider is available
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
				_, err := client.getProvider()
				if err != nil {
					log.Warn().Err(err).Msg("Failed to communicate with OIDC provider, retrying in 1s...")
					continue
				}
				_, err = client.getOauth2Config()
				if err != nil {
					log.Warn().Err(err).Msg("Failed to prepare oauth2 config, retrying in 1s...")
					continue
				}
				log.Info().Msg("Successfully connected to OIDC provider")
				return
			}
		}
	}()

	return client, nil
}

func (c *OIDCClient) getProvider() (*oidc.Provider, error) {
	if c.provider == nil {
		log.Trace().Str("provider_url", c.cfg.ProviderURL).Msg("Getting provider")

		// If ExpectedIssuer is set, use InsecureIssuerURLContext to allow the provider
		// to return a different issuer than the discovery URL. This is needed when
		// the API connects to Keycloak via an internal URL but Keycloak is configured
		// with an external URL for browser access.
		ctx := context.Background()
		if c.cfg.ExpectedIssuer != "" {
			log.Info().
				Str("discovery_url", c.cfg.ProviderURL).
				Str("expected_issuer", c.cfg.ExpectedIssuer).
				Msg("Using InsecureIssuerURLContext to allow different issuer")
			ctx = oidc.InsecureIssuerURLContext(ctx, c.cfg.ExpectedIssuer)
		}

		provider, err := oidc.NewProvider(ctx, c.cfg.ProviderURL)
		if err != nil {
			// Wrap error to indicate provider not ready (used to return 503 instead of 401)
			return nil, fmt.Errorf("%w: %v", ErrProviderNotReady, err)
		}
		c.provider = provider
	}
	return c.provider, nil
}

func (c *OIDCClient) getOauth2Config() (*oauth2.Config, error) {
	if c.oauth2Config == nil {
		provider, err := c.getProvider()
		if err != nil {
			log.Error().Err(err).Msg("Failed to get provider")
			return nil, err
		}
		endpoint := provider.Endpoint()

		// Override token URL for internal API access
		// If TokenURL is set explicitly, use it (useful when discovery returns browser URLs
		// but API needs internal URLs, e.g., Keycloak behind a proxy)
		// IMPORTANT: Do NOT auto-derive Keycloak-style URLs - this breaks Google and other
		// standard OIDC providers. Only override if explicitly configured.
		if c.cfg.TokenURL != "" && c.cfg.TokenURL != endpoint.TokenURL {
			log.Info().
				Str("original_token_url", endpoint.TokenURL).
				Str("override_token_url", c.cfg.TokenURL).
				Msg("Overriding token endpoint URL with explicit OIDC_TOKEN_URL")
			endpoint.TokenURL = c.cfg.TokenURL
		}

		log.Trace().Str("client_id", c.cfg.ClientID).Str("redirect_url", c.cfg.RedirectURL).Interface("endpoints", endpoint).Msg("Getting oauth2 config")
		c.oauth2Config = &oauth2.Config{
			ClientID:     c.cfg.ClientID,
			ClientSecret: c.cfg.ClientSecret,
			RedirectURL:  c.cfg.RedirectURL,
			Scopes:       c.cfg.Scopes,
			Endpoint:     endpoint,
		}
	}
	return c.oauth2Config, nil
}

// GetAuthURL returns the authorization URL for the OIDC login flow
func (c *OIDCClient) GetAuthURL(state, nonce string) string {
	oauth2Config, err := c.getOauth2Config()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get oauth2 config")
		return ""
	}
	// Add prompt=select_account to force the account picker (useful for Google)
	// This ensures users can choose which account to use instead of auto-selecting
	return oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.SetAuthURLParam("prompt", "select_account"))
}

// Exchange converts an authorization code into tokens
func (c *OIDCClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	// Log truncated code for debugging (avoid logging full code for security)
	codePreview := code
	if len(code) > 20 {
		codePreview = code[:20] + "..."
	}
	log.Info().Str("code", codePreview).Msg("Exchanging code for token")
	oauth2Config, err := c.getOauth2Config()
	if err != nil {
		return nil, err
	}
	log.Info().
		Str("redirect_url", oauth2Config.RedirectURL).
		Str("client_id", oauth2Config.ClientID).
		Str("token_endpoint", oauth2Config.Endpoint.TokenURL).
		Msg("Token exchange config")
	token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		log.Error().Err(err).Str("redirect_url", oauth2Config.RedirectURL).Msg("Token exchange failed")
		return nil, err
	}
	log.Info().Msg("Token exchange successful")
	return token, nil
}

// VerifyIDToken verifies the ID token and returns the claims
func (c *OIDCClient) VerifyIDToken(ctx context.Context, token *oauth2.Token) (*oidc.IDToken, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		return nil, errors.New("no id_token field in oauth2 token")
	}
	provider, err := c.getProvider()
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID: c.cfg.ClientID,
	})
	return verifier.Verify(ctx, rawIDToken)
}

func (c *OIDCClient) VerifyAccessToken(ctx context.Context, accessToken string) error {
	provider, err := c.getProvider()
	if err != nil {
		return err
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID: c.cfg.Audience,
	})
	_, err = verifier.Verify(ctx, accessToken)
	if err != nil {
		return err
	}
	return nil
}

func (c *OIDCClient) RefreshAccessToken(ctx context.Context, refreshToken string) (*oauth2.Token, error) {
	oauth2Config, err := c.getOauth2Config()
	if err != nil {
		return nil, err
	}

	return oauth2Config.TokenSource(ctx, &oauth2.Token{RefreshToken: refreshToken}).Token()
}

// GetUserInfo retrieves user information using the access token
func (c *OIDCClient) GetUserInfo(ctx context.Context, accessToken string) (*UserInfo, error) {
	provider, err := c.getProvider()
	if err != nil {
		return nil, err
	}
	userInfo, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	}))
	if err != nil {
		return nil, err
	}

	var claims UserInfo
	if err := userInfo.Claims(&claims); err != nil {
		return nil, err
	}

	return &claims, nil
}

// GetLogoutURL returns the URL to log out from the OIDC provider
func (c *OIDCClient) GetLogoutURL() (string, error) {
	provider, err := c.getProvider()
	if err != nil {
		return "", err
	}
	// Get the provider's OpenID configuration
	oidcConfig := &struct {
		EndSessionEndpoint string `json:"end_session_endpoint"`
	}{}
	err = provider.Claims(oidcConfig)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get provider end session endpoint")
		return "", fmt.Errorf("failed to get provider end session endpoint: %w", err)
	}

	return oidcConfig.EndSessionEndpoint, nil
}

func (c *OIDCClient) ValidateUserToken(ctx context.Context, accessToken string) (*types.User, error) {
	// Note: We intentionally don't verify the access token as a JWT here.
	// The go-oidc Verifier is designed for ID tokens, not access tokens.
	// Keycloak access tokens have different claims (aud="account" vs client_id).
	// Instead, we validate the token by calling the userinfo endpoint - if the
	// token is invalid or expired, that call will fail.
	userInfo, err := c.GetUserInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid access token (could not get user info): %w", err)
	}

	// Try to get the user from the database by their OIDC subject ID
	user, err := c.store.GetUser(ctx, &store.GetUserQuery{
		ID: userInfo.Subject,
	})
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("invalid access token (database error): %w", err)
	}

	// Extract full name from userinfo
	fullName := userInfo.Name
	if fullName == "" && userInfo.GivenName != "" && userInfo.FamilyName != "" {
		fullName = userInfo.GivenName + " " + userInfo.FamilyName
	}
	if fullName == "" {
		fullName = userInfo.Email
	}

	// If user doesn't exist, create them (first login after OIDC registration)
	isNewUser := user == nil
	if isNewUser {
		log.Info().
			Str("subject", userInfo.Subject).
			Str("email", userInfo.Email).
			Msg("Creating new user from OIDC token")

		user, err = c.store.CreateUser(ctx, &types.User{
			ID:        userInfo.Subject,
			Username:  userInfo.Subject,
			Email:     userInfo.Email,
			FullName:  fullName,
			CreatedAt: time.Now(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	}

	// Auto-join organization by email domain (OIDC only, with verified email)
	if userInfo.EmailVerified && userInfo.Email != "" {
		c.tryAutoJoinOrganization(ctx, userInfo.Subject, userInfo.Email)
	}

	// Determine admin status:
	// - If user ID is in ADMIN_USER_IDS list (or "all" is set): admin
	// - Otherwise use the database admin field
	isAdmin := c.adminConfig.IsUserInAdminList(userInfo.Subject) || user.Admin

	return &types.User{
		ID:          userInfo.Subject,
		Username:    userInfo.Subject,
		Email:       userInfo.Email,
		FullName:    fullName,
		Token:       accessToken,
		TokenType:   types.TokenTypeOIDC,
		Type:        types.OwnerTypeUser,
		Admin:       isAdmin,
		SB:          user.SB,
		Deactivated: user.Deactivated,
	}, nil
}

// tryAutoJoinOrganization attempts to add the user to an organization based on their email domain
func (c *OIDCClient) tryAutoJoinOrganization(ctx context.Context, userID, email string) {
	// Extract domain from email
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return
	}
	domain := strings.ToLower(parts[1])

	// Look up organization by domain
	org, err := c.store.GetOrganizationByDomain(ctx, domain)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			log.Warn().Err(err).Str("domain", domain).Msg("error looking up organization by domain")
		}
		return // No org with this domain or error
	}

	// Check if user is already a member
	_, err = c.store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
		OrganizationID: org.ID,
		UserID:         userID,
	})
	if err == nil {
		return // Already a member
	}
	if !errors.Is(err, store.ErrNotFound) {
		log.Warn().Err(err).Str("org_id", org.ID).Str("user_id", userID).Msg("error checking organization membership")
		return
	}

	// Create membership with member role
	_, err = c.store.CreateOrganizationMembership(ctx, &types.OrganizationMembership{
		OrganizationID: org.ID,
		UserID:         userID,
		Role:           types.OrganizationRoleMember,
	})
	if err != nil {
		log.Error().Err(err).Str("org_id", org.ID).Str("user_id", userID).Str("domain", domain).Msg("failed to auto-join user to organization")
		return
	}

	log.Info().
		Str("org_id", org.ID).
		Str("org_name", org.Name).
		Str("user_id", userID).
		Str("email", email).
		Str("domain", domain).
		Msg("user auto-joined organization via email domain")
}

type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

type UserInfo struct {
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Subject       string `json:"sub"`
	Admin         bool   `json:"admin"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
}
