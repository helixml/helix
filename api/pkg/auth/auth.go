package auth

import (
	"context"

	"github.com/Nerzal/gocloak/v13"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type Authenticator interface {
	GetUserByID(ctx context.Context, userID string) (*types.UserDetails, error)
}

type KeycloakAuthenticator struct {
	cfg     *config.Keycloak
	gocloak *gocloak.GoCloak
}

func NewKeycloakAuthenticator(cfg *config.Keycloak) (*KeycloakAuthenticator, error) {
	gck := gocloak.NewClient(cfg.URL)

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

func (k *KeycloakAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.UserDetails, error) {
	token, err := k.gocloak.LoginAdmin(ctx, k.cfg.Username, k.cfg.Password, k.cfg.AdminRealm)
	if err != nil {
		return nil, err
	}

	user, err := k.gocloak.GetUserByID(ctx, token.AccessToken, k.cfg.Realm, userID)
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
