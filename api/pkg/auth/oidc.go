package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2"
)

var (
	ErrInvalidConfig = errors.New("invalid config")
)

type OIDCClient struct {
	cfg          OIDCConfig
	provider     *oidc.Provider
	oauth2Config *oauth2.Config
	adminConfig  *AdminConfig
	store        store.Store
}

type OIDCConfig struct {
	ProviderURL  string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AdminUserIDs []string
	AdminUserSrc config.AdminSrcType
	Audience     string
	Scopes       []string
	Store        store.Store
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
			AdminUserSrc: cfg.AdminUserSrc,
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
		provider, err := oidc.NewProvider(context.Background(), c.cfg.ProviderURL)
		if err != nil {
			return nil, err
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
		log.Trace().Str("client_id", c.cfg.ClientID).Str("redirect_url", c.cfg.RedirectURL).Interface("endpoints", provider.Endpoint()).Msg("Getting oauth2 config")
		c.oauth2Config = &oauth2.Config{
			ClientID:     c.cfg.ClientID,
			ClientSecret: c.cfg.ClientSecret,
			RedirectURL:  c.cfg.RedirectURL,
			Scopes:       c.cfg.Scopes,
			Endpoint:     provider.Endpoint(),
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
	return oauth2Config.AuthCodeURL(state, oidc.Nonce(nonce))
}

// Exchange converts an authorization code into tokens
func (c *OIDCClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	log.Info().Str("code", code).Msg("Exchanging code for token")
	oauth2Config, err := c.getOauth2Config()
	if err != nil {
		return nil, err
	}
	log.Info().Str("code", code).Msg("Exchanged code for token")
	return oauth2Config.Exchange(ctx, code)
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
	provider, err := c.getProvider()
	if err != nil {
		return nil, err
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID: c.cfg.Audience,
	})
	_, err = verifier.Verify(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid access token: %w", err)
	}

	userInfo, err := c.GetUserInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid access token (could not get user): %w", err)
	}

	user, err := c.store.GetUser(ctx, &store.GetUserQuery{
		Email: userInfo.Email,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid access token (could not get user): %w", err)
	}

	account := account{userInfo: userInfo}

	return &types.User{
		ID:          userInfo.Subject,
		Username:    userInfo.Subject,
		Email:       userInfo.Email,
		FullName:    userInfo.Name,
		Token:       accessToken,
		TokenType:   types.TokenTypeOIDC,
		Type:        types.OwnerTypeUser,
		Admin:       account.isAdmin(c.adminConfig),
		SB:          user.SB,
		Deactivated: user.Deactivated,
	}, nil
}

type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

type UserInfo struct {
	Email      string `json:"email"`
	Name       string `json:"name"`
	Subject    string `json:"sub"`
	Admin      bool   `json:"admin"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
}
