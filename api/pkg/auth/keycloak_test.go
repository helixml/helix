package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/Nerzal/gocloak/v13"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
)

func TestKeycloakSuite(t *testing.T) {
	suite.Run(t, new(KeycloakTestSuite))
}

type KeycloakTestSuite struct {
	suite.Suite
	ctx   context.Context
	store store.Store
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

	keycloakAuthenticator, err := NewKeycloakAuthenticator(&config.Keycloak{
		KeycloakURL:         keycloakCfg.KeycloakURL,
		KeycloakFrontEndURL: keycloakCfg.KeycloakFrontEndURL,
		ServerURL:           keycloakCfg.ServerURL,
		APIClientID:         keycloakCfg.APIClientID,
		FrontEndClientID:    keycloakCfg.FrontEndClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            keycloakCfg.Username,
		Password:            keycloakCfg.Password,
	}, suite.store)
	if err != nil {
		suite.T().Fatalf("failed to create keycloak authenticator: %v", err)
	}

	suite.auth = keycloakAuthenticator
}

func (suite *KeycloakTestSuite) TestValidateUserToken() {
	tests := []struct {
		name    string
		token   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty_token",
			token:   "",
			wantErr: true,
			errMsg:  "DecodeAccessToken: invalid or malformed token",
		},
		{
			name:    "malformed_token",
			token:   "not.a.jwt",
			wantErr: true,
			errMsg:  "DecodeAccessToken: invalid or malformed token",
		},
		{
			name:    "invalid_token",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c", // #gitleaks:allow
			wantErr: true,
			errMsg:  "cannot find a key to decode the token",
		},
		{
			name:    "tampered_token",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbiIsInJvbGUiOiJhZG1pbiJ9.tampered", // #gitleaks:allow
			wantErr: true,
			errMsg:  "cannot find a key to decode the token",
		},
		{
			name:    "token_with_invalid_signature",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.invalid_signature", // #gitleaks:allow
			wantErr: true,
			errMsg:  "cannot find a key to decode the token",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			_, err := suite.auth.ValidateUserToken(suite.ctx, tt.token)
			if tt.wantErr {
				suite.Error(err)
				if tt.errMsg != "" {
					suite.Contains(err.Error(), tt.errMsg)
				}
			} else {
				suite.NoError(err)
			}
		})
	}
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

func (suite *KeycloakTestSuite) TestConcurrentAccess() {
	// Ensure we have a valid authenticator
	suite.Require().NotNil(suite.auth, "Authenticator should be initialized in SetupTest")

	// Test concurrent user lookups with a non-existent user
	suite.Run("concurrent_user_lookups", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := suite.auth.GetUserByID(suite.ctx, "non-existent-user")
				if err == nil {
					errChan <- fmt.Errorf("expected error for non-existent user")
				}
			}()
		}

		go func() {
			wg.Wait()
			close(errChan)
		}()

		for err := range errChan {
			suite.Fail("Concurrent user lookup failed:", err)
		}
	})

	suite.Run("concurrent_token_validations", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, 10)
		invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.token"

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := suite.auth.ValidateUserToken(suite.ctx, invalidToken)
				if err == nil {
					errChan <- fmt.Errorf("expected error for invalid token")
				}
			}()
		}

		go func() {
			wg.Wait()
			close(errChan)
		}()

		for err := range errChan {
			suite.Fail("Concurrent token validation failed:", err)
		}
	})
}

func (suite *KeycloakTestSuite) TestErrorHandling() {
	suite.Run("timeout_handling", func() {
		ctx, cancel := context.WithTimeout(suite.ctx, 1*time.Millisecond)
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
			FrontEndClientID:    suite.auth.cfg.FrontEndClientID,
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
