package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Nerzal/gocloak/v13"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/rs/zerolog/log"
)

type KeycloakClientResult struct {
	Client *gocloak.GoCloak
	Token  *gocloak.JWT
}

func NewGoCloakClient(cfg *config.Keycloak) (*KeycloakClientResult, error) {
	gck := gocloak.NewClient(cfg.KeycloakURL)

	log.Info().Str("keycloak_url", cfg.KeycloakURL).Msg("connecting to keycloak...")

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

	err = setRealmConfigurations(gck, token.AccessToken, cfg)
	if err != nil {
		return nil, err
	}

	return &KeycloakClientResult{
		Client: gck,
		Token:  token,
	}, nil
}

func connect(ctx context.Context, cfg *config.Keycloak) (*gocloak.JWT, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	gck := gocloak.NewClient(cfg.KeycloakURL)

	for {
		select {
		case <-ctx.Done():
		default:
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
}

func setRealmConfigurations(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	realm, err := gck.GetRealm(context.Background(), token, cfg.Realm)
	if err != nil {
		if !strings.Contains(err.Error(), "404") {
			return fmt.Errorf("setRealmConfiguration: failed to get Keycloak realm, attempt to find realm config failed with: %s", err.Error())
		}

		// If user has a different realm configuration, don't try to create it
		// as it might be a legitimate realm
		if cfg.Realm != "helix" {
			return fmt.Errorf("setRealmConfiguration: no Keycloak realm found, error: %s", err.Error())
		}

		// Default configuration, create realm
		log.Info().Str("realm", cfg.Realm).Msg("No configurations found, creating default 'helix' realm")

		f, err := keycloakConfig.Open("realm.json")
		if err != nil {
			return fmt.Errorf("setRealmConfiguration: error opening realm.json: %s", err.Error())
		}
		defer f.Close()

		var keycloakRealmConfig gocloak.RealmRepresentation
		err = json.NewDecoder(f).Decode(&keycloakRealmConfig)
		if err != nil {
			return fmt.Errorf("setRealmConfiguration: error decoding realm.json: %s", err.Error())
		}

		_, err = gck.CreateRealm(context.Background(), token, keycloakRealmConfig)
		if err != nil {
			return fmt.Errorf("setRealmConfiguration: no Keycloak realm found, attempt to create realm failed with: %s", err.Error())
		}
		// OK, get again
		realm, err = gck.GetRealm(context.Background(), token, cfg.Realm)
		if err != nil {
			return fmt.Errorf("setRealmConfiguration: failed to get Keycloak realm, attempt to update realm config failed with: %s", err.Error())
		}
	}

	attributes := *realm.Attributes
	attributes["frontendUrl"] = cfg.KeycloakFrontEndURL
	*realm.Attributes = attributes

	err = gck.UpdateRealm(context.Background(), token, *realm)
	if err != nil {
		return fmt.Errorf("setRealmConfiguration: attempt to update realm config failed with: %s", err.Error())
	}

	log.Info().
		Str("realm", cfg.Realm).
		Str("frontend_url", cfg.KeycloakFrontEndURL).
		Msg("Configured realm")

	return nil
}
