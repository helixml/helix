package auth

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Nerzal/gocloak/v13"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

//go:embed realm.json
var keycloakConfig embed.FS

type KeycloakAuthenticator struct {
	cfg     *config.Keycloak
	gocloak *gocloak.GoCloak

	adminTokenMu      *sync.Mutex
	adminToken        *gocloak.JWT
	adminTokenExpires time.Time

	store store.Store
}

func NewKeycloakAuthenticator(cfg *config.Keycloak, store store.Store) (*KeycloakAuthenticator, error) {
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

	err = ensureConfiguration(gck, token.AccessToken, cfg)
	if err != nil {
		return nil, err
	}

	return &KeycloakAuthenticator{
		cfg:          cfg,
		gocloak:      gck,
		adminTokenMu: &sync.Mutex{},
		store:        store,
	}, nil
}

// isRetryableKeycloakError checks if an error is retryable based on 409 conflicts, optimistic lock exceptions, and 500 internal server errors
func isRetryableKeycloakError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	// Check for 409 conflicts
	if strings.Contains(errStr, "409") && strings.Contains(errStr, "Conflict") {
		return true
	}

	// Check for optimistic lock exceptions
	if strings.Contains(errStr, "OptimisticLockException") {
		return true
	}

	// Check for 400 Bad Request with OptimisticLockException
	if strings.Contains(errStr, "400 Bad Request") && strings.Contains(errStr, "OptimisticLockException") {
		return true
	}

	// Check for 500 Internal Server Error (common during Keycloak startup/initialization)
	if strings.Contains(errStr, "500 Internal Server Error") {
		return true
	}

	return false
}

// retryWithExponentialBackoff performs exponential backoff with jitter for retryable errors
func retryWithExponentialBackoff(operation func() error, maxRetries int, baseDelay time.Duration) error {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Only retry on specific retryable errors
		if !isRetryableKeycloakError(err) {
			return err
		}

		// Don't sleep on the last attempt
		if attempt == maxRetries-1 {
			break
		}

		// Calculate exponential backoff with jitter
		delay := baseDelay * time.Duration(1<<uint(attempt))   // 2^attempt
		jitter := time.Duration(rand.Int63n(int64(delay / 2))) // Random jitter up to 50% of delay
		totalDelay := delay + jitter

		log.Info().
			Err(err).
			Int("attempt", attempt+1).
			Int("max_retries", maxRetries).
			Dur("delay", totalDelay).
			Msg("Retrying Keycloak operation after retryable error")

		time.Sleep(totalDelay)
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func ensureConfiguration(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	return retryWithExponentialBackoff(func() error {
		err := setRealmConfigurations(gck, token, cfg)
		if err != nil {
			return err
		}

		if cfg.ClientSecret == "" {
			err = setAPIClientConfigurations(gck, token, cfg)
			if err != nil {
				return err
			}
		}

		err = ensureAPIClientRedirectURIs(gck, token, cfg)
		if err != nil {
			return err
		}

		return nil
	}, 5, 1*time.Second) // At least 5 retries with 1 second base delay
}

func setAPIClientConfigurations(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("client secret not set, looking up client secret")

	idOfClient, err := getIDOfKeycloakClient(gck, token, cfg.Realm, cfg.APIClientID)
	if err != nil {
		return fmt.Errorf("setAPIClientConfigurations: error getting clients: %s", err.Error())
	}

	if idOfClient == "" {
		log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("No configurations found, creating client")
		idOfClient, err = gck.CreateClient(context.Background(),
			token, cfg.Realm,
			gocloak.Client{
				ClientID: &cfg.APIClientID,
			},
		)
		if err != nil {
			if !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("getKeycloakClient: no Keycloak client found, attempt to create client failed with: %s", err.Error())
			}
			// Client already exists, get it
			idOfClient, err = getIDOfKeycloakClient(gck, token, cfg.Realm, cfg.APIClientID)
			if err != nil {
				return fmt.Errorf("getKeycloakClient: error getting clients: %s", err.Error())
			}
		}
	}
	creds, err := gck.GetClientSecret(context.Background(), token, cfg.Realm, idOfClient)
	if err != nil {
		return fmt.Errorf("setAPIClientConfigurations: error updating client secret: %s", err.Error())
	}

	cfg.ClientSecret = *creds.Value
	return nil
}

func ensureAPIClientRedirectURIs(gck *gocloak.GoCloak, token string, cfg *config.Keycloak) error {
	log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Configuring API client")

	idOfClient, err := getIDOfKeycloakClient(gck, token, cfg.Realm, cfg.APIClientID)
	if err != nil {
		return fmt.Errorf("ensureAPIClientRedirectURIs: error getting clients: %s", err.Error())
	}

	if idOfClient == "" {
		return fmt.Errorf("ensureAPIClientRedirectURIs: no Keycloak client found")
	}

	client, err := gck.GetClient(context.Background(), token, cfg.Realm, idOfClient)
	if err != nil {
		return fmt.Errorf("ensureAPIClientRedirectURIs: error getting client: %s", err.Error())
	}

	if client.RedirectURIs == nil || len(*client.RedirectURIs) == 0 {
		log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("No redirect URIs found, updating")
		u, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error parsing server URL: %s", err.Error())
		}
		if strings.Contains(u.Host, "localhost") {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using * as redirect URI for localhost")
			*client.RedirectURIs = []string{"*"}
		} else {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using server URL as redirect URI")
			u.Path = "*"
			*client.RedirectURIs = []string{u.String()}
		}

		err = retryWithExponentialBackoff(func() error {
			return gck.UpdateClient(context.Background(), token, cfg.Realm, *client)
		}, 5, 500*time.Millisecond)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error updating client: %s", err.Error())
		}
	}

	if client.WebOrigins == nil || len(*client.WebOrigins) == 0 {
		log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("No web origins found, updating")
		u, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error parsing server URL: %s", err.Error())
		}
		if strings.Contains(u.Host, "localhost") {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using * as web origin for localhost")
			*client.WebOrigins = []string{"*"}
		} else {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using server URL as web origin")
			u.Path = "*"
			*client.WebOrigins = []string{u.String()}
		}
		err = retryWithExponentialBackoff(func() error {
			return gck.UpdateClient(context.Background(), token, cfg.Realm, *client)
		}, 5, 500*time.Millisecond)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error updating client: %s", err.Error())
		}
	}

	// Logout URLs doesn't have a dedicated field in the Client, so we we must use the Attributes
	if client.Attributes == nil {
		client.Attributes = &map[string]string{}
	}
	attributes := *client.Attributes
	if _, exists := attributes["post.logout.redirect.uris"]; !exists {
		log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("No logout URLs found, updating")
		u, err := url.Parse(cfg.ServerURL)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error parsing server URL: %s", err.Error())
		}
		if strings.Contains(u.Host, "localhost") {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using * as logout URL for localhost")
			attributes["post.logout.redirect.uris"] = "*"
		} else {
			log.Info().Str("client_id", cfg.APIClientID).Str("realm", cfg.Realm).Msg("Using server URL as logout URL")
			u.Path = "*"
			attributes["post.logout.redirect.uris"] = u.String()
		}
		*client.Attributes = attributes

		err = retryWithExponentialBackoff(func() error {
			return gck.UpdateClient(context.Background(), token, cfg.Realm, *client)
		}, 5, 500*time.Millisecond)
		if err != nil {
			return fmt.Errorf("ensureAPIClientRedirectURIs: error updating client: %s", err.Error())
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

		// Initialize attributes if not set
		if keycloakRealmConfig.Attributes == nil {
			keycloakRealmConfig.Attributes = &map[string]string{}
		}

		_, err = gck.CreateRealm(context.Background(), token, keycloakRealmConfig)
		if err != nil {
			if !strings.Contains(err.Error(), "409") {
				return fmt.Errorf("setRealmConfiguration: no Keycloak realm found, attempt to create realm failed with: %s", err.Error())
			}
			// Realm already exists, get it
		}
		// OK, get again
		realm, err = gck.GetRealm(context.Background(), token, cfg.Realm)
		if err != nil {
			return fmt.Errorf("setRealmConfiguration: failed to get Keycloak realm, attempt to update realm config failed with: %s", err.Error())
		}
	}

	// Initialize attributes if not set
	if realm.Attributes == nil {
		realm.Attributes = &map[string]string{}
	}

	attributes := *realm.Attributes
	attributes["frontendUrl"] = cfg.KeycloakFrontEndURL
	*realm.Attributes = attributes

	// Set login theme to "helix" for both new and existing deployments
	if realm.LoginTheme == nil || *realm.LoginTheme != "helix" {
		realm.LoginTheme = addr("helix")
		log.Info().Str("realm", cfg.Realm).Msg("Setting login theme to 'helix'")
	}

	err = retryWithExponentialBackoff(func() error {
		return gck.UpdateRealm(context.Background(), token, *realm)
	}, 5, 500*time.Millisecond)
	if err != nil {
		return fmt.Errorf("setRealmConfiguration: attempt to update realm config failed with: %s", err.Error())
	}

	log.Info().
		Str("realm", cfg.Realm).
		Str("frontend_url", cfg.KeycloakFrontEndURL).
		Str("login_theme", gocloak.PString(realm.LoginTheme)).
		Msg("Configured realm")

	return nil
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
				log.Warn().
					Str("username", cfg.Username).
					Str("realm", cfg.AdminRealm).
					Err(err).
					Msg("failed getting admin token, retrying in 5 seconds....")
				time.Sleep(5 * time.Second)
				continue
			}

			// OK
			return token, nil
		}
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

	return token, nil
}

func (k *KeycloakAuthenticator) GetUserByID(ctx context.Context, userID string) (*types.User, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	user, err := k.gocloak.GetUserByID(ctx, adminToken.AccessToken, k.cfg.Realm, userID)
	if err != nil {
		log.Warn().
			Str("user_id", userID).
			Str("realm", k.cfg.Realm).
			Err(err).
			Msg("failed to get user from Keycloak")
		return nil, err
	}

	storeUser := &types.User{
		ID:       gocloak.PString(user.ID),
		Username: gocloak.PString(user.Username),
		Email:    gocloak.PString(user.Email),
		FullName: fmt.Sprintf("%s %s", gocloak.PString(user.FirstName), gocloak.PString(user.LastName)),
	}

	err = k.ensureStoreUser(storeUser)
	if err != nil {
		return nil, err
	}

	return storeUser, nil
}

// ensureStoreUser syncs user with the database record
func (k *KeycloakAuthenticator) ensureStoreUser(user *types.User) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	existing, err := k.store.GetUser(ctx, &store.GetUserQuery{
		ID: user.ID,
	})
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("ensureStoreUser: error getting user: %w", err)
		}
	}

	if existing == nil {
		_, err = k.store.CreateUser(ctx, user)
		if err != nil {
			if strings.Contains(err.Error(), "duplicate key") {
				return nil
			}
			return fmt.Errorf("ensureStoreUser: error creating user: %w", err)
		}

		// OK
		return nil
	}

	user.Deactivated = existing.Deactivated
	user.SB = existing.SB

	// If email or name hasn't changed, don't update
	if existing.Email == user.Email && existing.FullName == user.FullName && existing.Username == user.Username {
		return nil
	}

	// Update user
	existing.Email = user.Email
	existing.FullName = user.FullName
	existing.Username = user.Username

	_, err = k.store.UpdateUser(ctx, existing)
	if err != nil {
		return fmt.Errorf("ensureStoreUser: error updating user: %w", err)
	}

	return nil
}

// CreateKeycloakUser creates a user in Keycloak, used in API integration tests
func (k *KeycloakAuthenticator) CreateKeycloakUser(ctx context.Context, user *types.User) (*types.User, error) {
	adminToken, err := k.getAdminToken(ctx)
	if err != nil {
		return nil, err
	}

	log.Info().
		Str("user_id", user.ID).
		Str("email", user.Email).
		Str("realm", k.cfg.Realm).
		Str("username", user.Username).Msg("creating user in Keycloak")

	userID, err := k.gocloak.CreateUser(ctx, adminToken.AccessToken, k.cfg.Realm, gocloak.User{
		ID:            &user.ID,
		Enabled:       addr(true),
		Email:         &user.Email,
		Username:      &user.Username,
		FirstName:     &user.FullName,
		EmailVerified: addr(true),
	})
	if err != nil {
		return nil, err
	}

	user.ID = userID

	return user, nil
}

func (k *KeycloakAuthenticator) ValidateUserToken(ctx context.Context, token string) (*jwt.Token, error) {
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

	return j, nil
}

func addr[T any](t T) *T { return &t }

// Compile-time interface check:
var _ Authenticator = (*KeycloakAuthenticator)(nil)
