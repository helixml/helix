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

	if cfg.ClientSecret == "" {
		log.Info().Str("client_id", cfg.ClientID).Str("realm", cfg.Realm).Msg("client secret not set, looking up client secret")

		// Lookup
		clients, err := gck.GetClients(context.Background(), token.AccessToken, cfg.Realm, gocloak.GetClientsParams{})
		if err != nil {
			return nil, fmt.Errorf("GetClients: error getting clients: %s", err.Error())
		}

		for _, client := range clients {
			if client.ClientID != nil && *client.ClientID == cfg.ClientID {
				creds, err := gck.GetClientSecret(context.Background(), token.AccessToken, cfg.Realm, *client.ID)
				if err != nil {
					return nil, fmt.Errorf("GetClientSecret: error getting client secret: %s", err.Error())
				}
				cfg.ClientSecret = *creds.Value

				log.Info().Str("client_id", cfg.ClientID).Str("realm", cfg.Realm).Msg("found client secret")

				break
			}
		}
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
	fmt.Sprintln("expires in: ", token.ExpiresIn)
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
	j, _, err := k.gocloak.DecodeAccessToken(ctx, token, k.cfg.Realm)
	if err != nil {
		return nil, fmt.Errorf("DecodeAccessToken: invalid or malformed token: %s", err.Error())
	}

	result, err := k.gocloak.RetrospectToken(ctx, token, k.cfg.ClientID, k.cfg.ClientSecret, k.cfg.Realm)
	if err != nil {
		log.Warn().
			Err(err).
			Str("token", token).
			Str("client_id", k.cfg.ClientID).
			Str("realm", k.cfg.Realm).
			Msg("failed getting admin token")
		return nil, fmt.Errorf("RetrospectToken: invalid or malformed token: %w", err)
	}

	if !*result.Active {
		return nil, fmt.Errorf("invalid or expired token")
	}

	return j, nil
}
