package auth

import (
	"context"
	"fmt"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.UserDetails, error)
	ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error)
}

type KeycloakAuthenticator struct {
	cfg     *config.Keycloak
	gocloak *gocloak.GoCloak
}

func NewKeycloakAuthenticator(cfg *config.Keycloak) (*KeycloakAuthenticator, error) {
	gck := gocloak.NewClient(cfg.URL)

	log.Info().Str("keycloak_url", cfg.URL).Msg("connecting to keycloak")

	token, err := gck.LoginAdmin(context.Background(), cfg.Username, cfg.Password, cfg.AdminRealm)
	if err != nil {
		return nil, err
	}
	// Test token
	_, err = gck.GetServerInfo(context.Background(), token.AccessToken)
	if err != nil {
		return nil, err
	}

	return &KeycloakAuthenticator{
		cfg:     cfg,
		gocloak: gck,
	}, nil
}

func (k *KeycloakAuthenticator) getAdminToken(ctx context.Context) (*gocloak.JWT, error) {
	token, err := k.gocloak.LoginAdmin(ctx, k.cfg.Username, k.cfg.Password, k.cfg.AdminRealm)
	if err != nil {
		return nil, err
	}
	return token, nil
}

func (k *KeycloakAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.UserDetails, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := k.gocloak.GetUserByID(ctx, adminToken.AccessToken, k.cfg.Realm, userID)
	if err != nil {
		return nil, err
	}

	return &types.UserDetails{
		ID:        gocloak.PString(user.ID),
		Username:  gocloak.PString(user.Username),
		Email:     gocloak.PString(user.Email),
		FirstName: gocloak.PString(user.FirstName),
		LastName:  gocloak.PString(user.LastName),
	}, nil
}

func (k *KeycloakAuthenticator) ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("error accessing keycloak, token error: %w", err)
	}

	j, _, err := k.gocloak.DecodeAccessToken(ctx, token, k.cfg.Realm)
	if err != nil {
		return nil, fmt.Errorf("DecodeAccessToken: invalid or malformed token: %s", err.Error())
	}

	result, err := k.gocloak.RetrospectToken(ctx, token, k.cfg.ClientID, adminToken.AccessToken, k.cfg.Realm)
	if err != nil {
		return nil, fmt.Errorf("RetrospectToken: invalid or malformed token: %w", err)
	}

	if !*result.Active {
		return nil, fmt.Errorf("invalid or expired token")
	}

	return j, nil
}
