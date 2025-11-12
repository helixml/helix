package auth

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/Nerzal/gocloak/v13"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestKeycloakSuite(t *testing.T) {
	suite.Run(t, new(KeycloakTestSuite))
}

type KeycloakTestSuite struct {
	suite.Suite
	ctx   context.Context
	store *store.MockStore
	auth  *KeycloakAuthenticator
}

func (suite *KeycloakTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)

	var keycloakCfg config.Keycloak

	err := envconfig.Process("", &keycloakCfg)
	suite.NoError(err)

	// CI=true means we're running in a CI environment
	if os.Getenv("CI") == "true" {
		suite.T().Logf("CI=true means we're running in a CI environment, using the value (%s) for Keycloak URL", keycloakCfg.KeycloakURL)
	} else {
		keycloakCfg.KeycloakURL = "http://localhost:30080/auth"
		suite.T().Logf("Not running in a CI environment, assuming we're running docker-compose.dev.yaml and using the value (%s) for Keycloak URL", keycloakCfg.KeycloakURL)
	}

	if keycloakCfg.Password == "" {
		keycloakCfg.Password = "REPLACE_ME"
		suite.T().Logf("Keycloak password is not set, using the default docker-compose.dev.yaml value of (%s)", keycloakCfg.Password)
	}

	fmt.Printf("keycloakCfg: %+v\n", keycloakCfg)

	cfg := &config.ServerConfig{}
	cfg.Auth.Keycloak = config.Keycloak{
		KeycloakURL:         keycloakCfg.KeycloakURL,
		KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
		ServerURL:           keycloakCfg.ServerURL,
		APIClientID:         keycloakCfg.APIClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            keycloakCfg.Username,
		Password:            keycloakCfg.Password,
	}

	keycloakAuthenticator, err := NewKeycloakAuthenticator(cfg, suite.store)
	if err != nil {
		suite.T().Fatalf("failed to create keycloak authenticator: %v", err)
	}

	suite.auth = keycloakAuthenticator
}

func (suite *KeycloakTestSuite) TestCreateUser() {
	userID := uuid.New().String()
	userEmail := fmt.Sprintf("test-create-user-%s@test.com", userID)
	user, err := suite.auth.CreateUser(suite.ctx, &types.User{
		ID:       userID,
		Email:    userEmail,
		Username: "username",
	})
	suite.NoError(err)
	suite.NotNil(user)
	suite.NotEmpty(user.ID)
	suite.Equal(userEmail, user.Email)

	// Setup get user mock
	suite.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(user, nil)
	suite.store.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Return(user, nil)

	// Get the user from Keycloak
	user, err = suite.auth.GetUserByID(suite.ctx, user.ID)
	suite.NoError(err)
	suite.NotNil(user)
	suite.Equal(userEmail, user.Email)
}

func (suite *KeycloakTestSuite) TestGetUserByID() {
	tests := []struct {
		name    string
		userID  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "sql_injection_attempt",
			userID:  "1; DROP TABLE users;--",
			wantErr: true,
		},
		{
			name:    "very_long_userid",
			userID:  strings.Repeat("a", 1000),
			wantErr: true,
		},
		{
			name:    "null_bytes_in_userid",
			userID:  "user\x00id",
			wantErr: true,
		},
		{
			name:    "non_existent_user",
			userID:  "non-existent",
			wantErr: true,
		},
		{
			name:    "xss_attempt",
			userID:  "<script>alert('xss')</script>",
			wantErr: true,
		},
		{
			name:    "path_traversal_attempt",
			userID:  "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "special_chars",
			userID:  "user@#$%^&*()",
			wantErr: true,
		},
		{
			name:    "unicode_injection",
			userID:  "user\u0000\u0009\u000B\u000C\u000D\u0020",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			user, err := suite.auth.GetUserByID(suite.ctx, tt.userID)
			if tt.wantErr {
				suite.Error(err)
				if tt.errMsg != "" {
					suite.Contains(err.Error(), tt.errMsg)
				}
			} else {
				suite.NoError(err)
				suite.NotNil(user)
				if user != nil {
					suite.Equal(tt.userID, user.ID)
				}
			}
		})
	}
}

func (suite *KeycloakTestSuite) TestErrorHandling() {
	suite.Run("timeout_handling", func() {
		ctx, cancel := context.WithTimeout(suite.ctx, -1*time.Second)
		defer cancel()

		_, err := suite.auth.GetUserByID(ctx, "test-user")
		suite.Error(err)
		suite.Contains(err.Error(), "context deadline exceeded")
	})

	suite.Run("invalid_realm", func() {
		// Use a context with timeout to prevent infinite retries
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		invalidCfg := &config.Keycloak{
			KeycloakURL:         suite.auth.cfg.KeycloakURL,
			KeycloakFrontEndURL: suite.auth.cfg.KeycloakFrontEndURL,
			ServerURL:           suite.auth.cfg.ServerURL,
			APIClientID:         suite.auth.cfg.APIClientID,
			AdminRealm:          "invalid-realm",
			Realm:               "invalid-realm",
			Username:            suite.auth.cfg.Username,
			Password:            suite.auth.cfg.Password,
		}

		// Create a new gocloak client with the invalid realm
		gck := gocloak.NewClient(invalidCfg.KeycloakURL)

		// Try to login directly - this should fail immediately
		_, err := gck.LoginAdmin(ctx, invalidCfg.Username, invalidCfg.Password, invalidCfg.AdminRealm)
		suite.Error(err)
		suite.Contains(err.Error(), "404 Not Found: Realm does not exist")
	})
}

func (suite *KeycloakTestSuite) TestEnsureAPIClientRedirectURIs() {
	tests := []struct {
		name               string
		serverURL          string
		initialRedirectURI string
		initialWebOrigin   string
		initialLogoutURL   string
		wantErr            bool
		errMsg             string
	}{
		{
			name:      "localhost_server",
			serverURL: "http://localhost:3000",
			wantErr:   false,
		},
		{
			name:      "production_server",
			serverURL: "https://example.com",
			wantErr:   false,
		},
		{
			name:               "existing_uris_preserved",
			serverURL:          "https://example.com",
			initialRedirectURI: "https://custom-redirect.com/*",
			initialWebOrigin:   "https://custom-origin.com/*",
			initialLogoutURL:   "https://custom-logout.com/*",
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// Get admin token for the test
			adminToken, err := suite.auth.getAdminToken(suite.ctx)
			suite.Require().NoError(err, "Failed to get admin token")

			// Create a test config with the test server URL
			testCfg := &config.Keycloak{
				KeycloakURL:         suite.auth.cfg.KeycloakURL,
				KeycloakFrontEndURL: suite.auth.cfg.KeycloakFrontEndURL,
				ServerURL:           tt.serverURL,
				APIClientID:         "test-client-" + time.Now().Format("20060102150405"),
				Realm:               suite.auth.cfg.Realm,
			}

			// Create initial client configuration
			clientConfig := gocloak.Client{
				ClientID: &testCfg.APIClientID,
			}

			// Set initial URIs if provided
			if tt.initialRedirectURI != "" {
				redirectURIs := []string{tt.initialRedirectURI}
				clientConfig.RedirectURIs = &redirectURIs
			}
			if tt.initialWebOrigin != "" {
				webOrigins := []string{tt.initialWebOrigin}
				clientConfig.WebOrigins = &webOrigins
			}
			if tt.initialLogoutURL != "" {
				attributes := make(map[string]string)
				attributes["post.logout.redirect.uris"] = tt.initialLogoutURL
				clientConfig.Attributes = &attributes
			}

			// Create a test client
			gck := suite.auth.gocloak
			idOfClient, err := gck.CreateClient(
				suite.ctx,
				adminToken.AccessToken,
				testCfg.Realm,
				clientConfig,
			)
			suite.Require().NoError(err, "Failed to create test client")

			// Clean up the test client after the test
			defer func() {
				err := gck.DeleteClient(suite.ctx, adminToken.AccessToken, testCfg.Realm, idOfClient)
				suite.NoError(err, "Failed to delete test client")
			}()

			// Test ensureAPIClientRedirectURIs
			err = ensureAPIClientRedirectURIs(gck, adminToken.AccessToken, testCfg)
			if tt.wantErr {
				suite.Error(err)
				if tt.errMsg != "" {
					suite.Contains(err.Error(), tt.errMsg)
				}
			} else {
				suite.NoError(err)

				// Verify the client configuration
				client, err := gck.GetClient(suite.ctx, adminToken.AccessToken, testCfg.Realm, idOfClient)
				suite.NoError(err)

				// Check RedirectURIs
				suite.NotNil(client.RedirectURIs)
				suite.Len(*client.RedirectURIs, 1)
				if tt.initialRedirectURI != "" {
					suite.Equal(tt.initialRedirectURI, (*client.RedirectURIs)[0])
				} else if strings.Contains(tt.serverURL, "localhost") {
					suite.Equal("*", (*client.RedirectURIs)[0])
				} else {
					u, _ := url.Parse(tt.serverURL)
					u.Path = "*"
					suite.Equal(u.String(), (*client.RedirectURIs)[0])
				}

				// Check WebOrigins
				suite.NotNil(client.WebOrigins)
				suite.Len(*client.WebOrigins, 1)
				if tt.initialWebOrigin != "" {
					suite.Equal(tt.initialWebOrigin, (*client.WebOrigins)[0])
				} else if strings.Contains(tt.serverURL, "localhost") {
					suite.Equal("*", (*client.WebOrigins)[0])
				} else {
					u, _ := url.Parse(tt.serverURL)
					u.Path = "*"
					suite.Equal(u.String(), (*client.WebOrigins)[0])
				}

				// Check Logout URLs in Attributes
				suite.NotNil(client.Attributes)
				attributes := *client.Attributes
				logoutURL, exists := attributes["post.logout.redirect.uris"]
				suite.True(exists)
				if tt.initialLogoutURL != "" {
					suite.Equal(tt.initialLogoutURL, logoutURL)
				} else if strings.Contains(tt.serverURL, "localhost") {
					suite.Equal("*", logoutURL)
				} else {
					u, _ := url.Parse(tt.serverURL)
					u.Path = "*"
					suite.Equal(u.String(), logoutURL)
				}
			}
		})
	}
}
