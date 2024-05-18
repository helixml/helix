package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type KeycloakAuthenticator struct {
	cfg     *config.Keycloak
	gocloak *gocloak.GoCloak

	adminTokenMu      *sync.Mutex
	adminToken        *gocloak.JWT
	adminTokenExpires time.Time
}

func NewKeycloakAuthenticator(cfg *config.Keycloak) (*KeycloakAuthenticator, error) {
	gck := gocloak.NewClient(cfg.URL)

	log.Info().Str("keycloak_url", cfg.URL).Msg("connecting to keycloak...")

	// Retryable connect that waits for keycloak
	token, err := connect(context.Background(), cfg)
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
		cfg:          cfg,
		gocloak:      gck,
		adminTokenMu: &sync.Mutex{},
	}, nil
}

func connect(ctx context.Context, cfg *config.Keycloak) (*gocloak.JWT, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	gck := gocloak.NewClient(cfg.URL)

	for {
		token, err := gck.LoginAdmin(context.Background(), cfg.Username, cfg.Password, cfg.AdminRealm)
		if err != nil {
			log.Warn().Err(err).Msg("failed getting admin token, retrying in 5 seconds....")
			time.Sleep(5 * time.Second)
			continue
		}

		// OK
		return token, nil
	}
}

func (k *KeycloakAuthenticator) getAdminToken(ctx context.Context) (*gocloak.JWT, error) {
	k.adminTokenMu.Lock()
	defer k.adminTokenMu.Unlock()

	if k.adminToken != nil {
		if k.adminTokenExpires.After(time.Now().Add(5 * time.Second)) {
			return k.adminToken, nil
		}
	}

	token, err := k.gocloak.LoginAdmin(ctx, k.cfg.Username, k.cfg.Password, k.cfg.AdminRealm)
	if err != nil {
		return nil, err
	}

	k.adminToken = token
	k.adminTokenExpires = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)

	fmt.Println("XXX expires in: ", token.ExpiresIn)
	return token, nil
}

func (k *KeycloakAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.User, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := k.gocloak.GetUserByID(ctx, adminToken.AccessToken, k.cfg.Realm, userID)
	if err != nil {
		return nil, err
	}

	return &types.User{
		ID:       gocloak.PString(user.ID),
		Username: gocloak.PString(user.Username),
		Email:    gocloak.PString(user.Email),
		FullName: fmt.Sprintf("%s %s", gocloak.PString(user.FirstName), gocloak.PString(user.LastName)),
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

// Compile-time interface check:
var _ Authenticator = (*KeycloakAuthenticator)(nil)
