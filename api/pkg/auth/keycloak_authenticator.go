package auth

import (
	"context"
	"embed"
	"fmt"

	"github.com/Nerzal/gocloak/v13"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

//go:embed realm.json
var keycloakConfig embed.FS

type KeycloakAuthenticator struct {
	cfg           *config.Keycloak
	gocloak       *gocloak.GoCloak
	userRetriever UserRetriever
	adminConfig   *AdminConfig
}

func NewKeycloakAuthenticator(
	gocloak *gocloak.GoCloak,
	cfg *config.Keycloak,
	token *gocloak.JWT,
	userRetriever UserRetriever,
	adminConfig *AdminConfig,
) (*KeycloakAuthenticator, error) {
	err := setFrontEndClientConfigurations(gocloak, token.AccessToken, cfg)
	if err != nil {
		return nil, err
	}

	if cfg.ClientSecret == "" {
		err = setAPIClientConfigurations(gocloak, token.AccessToken, cfg)
		if err != nil {
			return nil, err
		}
	}

	return &KeycloakAuthenticator{
		cfg:           cfg,
		gocloak:       gocloak,
		adminConfig:   adminConfig,
		userRetriever: userRetriever,
	}, nil
}

func setAPIClientConfigurations(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("client secret not set, looking up client secret")

	idOfClient, err := getIDOfKeycloakClient(gck, token, cfg.Realm, cfg.APIClientID)
	if err != nil {
		return fmt.Errorf("setAPIClientConfigurations: error getting clients: %s", err.Error())
	}

	if idOfClient == "" {
		log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("No configurations found, creating client")
		idOfClient, err = gck.CreateClient(context.Background(), token, cfg.Realm, gocloak.Client{ClientID: &cfg.APIClientID})
		if err != nil {
			return fmt.Errorf("getKeycloakClient: no Keycloak client found, attempt to create client failed with: %s", err.Error())
		}
	}
	creds, err := gck.GetClientSecret(context.Background(), token, cfg.Realm, idOfClient)
	if err != nil {
		return fmt.Errorf("setAPIClientConfigurations: error updating client secret: %s", err.Error())
	}

	cfg.ClientSecret = *creds.Value
	return nil
}

func setFrontEndClientConfigurations(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	log.Info().Str("client_id", cfg.FrontEndClientID).Str("realm", cfg.Realm).Msg("Configuring Frontend client")

	idOfClient, err := getIDOfKeycloakClient(gck, token, cfg.Realm, cfg.FrontEndClientID)
	if err != nil {
		return fmt.Errorf("setFrontEndClientConfigurations: error getting clients: %s", err.Error())
	}

	if idOfClient == "" {
		log.Info().Str("client_id", cfg.FrontEndClientID).Str("realm", cfg.Realm).Msg("No configurations found, creating client")
		frontendClient := gocloak.Client{
			ClientID:                  &cfg.FrontEndClientID,
			BaseURL:                   &cfg.ServerURL,
			RedirectURIs:              &[]string{"*"},
			WebOrigins:                &[]string{"*"},
			DirectAccessGrantsEnabled: addr(true),
			PublicClient:              addr(true),
		}
		_, err = gck.CreateClient(context.Background(), token, cfg.Realm, frontendClient)
		if err != nil {
			return fmt.Errorf("getKeycloakClient: no Keycloak client found, attempt to create client failed with: %s", err.Error())
		}
	}

	return nil
}

func getIDOfKeycloakClient(gck *gocloak.GoCloak, token string, realm string, clientName string) (string, error) {
	clients, err := gck.GetClients(context.Background(), token, realm, gocloak.GetClientsParams{ClientID: &clientName})
	if err != nil {
		return "", fmt.Errorf("getKeycloakClient: error getting clients: %s", err.Error())
	}

	for _, client := range clients {
		return *client.ID, nil
	}
	return "", nil
}

func (k *KeycloakAuthenticator) ValidateAndReturnUser(ctx context.Context, token string) (*types.User, error) {
	j, _, err := k.gocloak.DecodeAccessToken(ctx, token, k.cfg.Realm)
	if err != nil {
		return nil, fmt.Errorf("DecodeAccessToken: invalid or malformed token: %s", err.Error())
	}

	result, err := k.gocloak.RetrospectToken(ctx, token, k.cfg.APIClientID, k.cfg.ClientSecret, k.cfg.Realm)
	if err != nil {
		log.Warn().
			Err(err).
			Str("token", token).
			Str("client_id", k.cfg.APIClientID).
			Str("realm", k.cfg.Realm).
			Msg("failed getting admin token")
		return nil, fmt.Errorf("RetrospectToken: invalid or malformed token: %w", err)
	}

	if !*result.Active {
		return nil, fmt.Errorf("invalid or expired token")
	}

	sub, err := j.Claims.GetSubject()
	if err != nil {
		return nil, fmt.Errorf("subject missing from token: %w", ErrNoUserIDFound)
	}

	user, err := k.userRetriever.GetUserByID(ctx, sub)

	if err != nil {
		return nil, fmt.Errorf("unable to fetch user information: %w", err)
	}

	account := account{userID: user.ID}

	return &types.User{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email,
		FullName:  user.FullName,
		Token:     token,
		TokenType: types.TokenTypeKeycloak,
		Type:      types.OwnerTypeUser,
		Admin:     account.isAdmin(k.adminConfig),
	}, nil
}

func addr[T any](t T) *T { return &t }

// Compile-time interface check:
var _ Authenticator = (*KeycloakAuthenticator)(nil)
var _ OIDCAuthenticator = (*KeycloakAuthenticator)(nil)
