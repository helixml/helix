package auth

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

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

	fmt.Printf("keycloakCfg: %+v\n", keycloakCfg)

	keycloakAuthenticator, err := NewKeycloakAuthenticator(&config.Keycloak{
		KeycloakURL:         "http://localhost:30080/auth",
		KeycloakFrontEndURL: "http://localhost:30080/auth",
		ServerURL:           keycloakCfg.KeycloakFrontEndURL,
		APIClientID:         keycloakCfg.APIClientID,
		FrontEndClientID:    keycloakCfg.FrontEndClientID,
		AdminRealm:          keycloakCfg.AdminRealm,
		Realm:               keycloakCfg.Realm,
		Username:            "admin",
		Password:            "REPLACE_ME",
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
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
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
}

func (suite *KeycloakTestSuite) TestErrorHandling() {
	suite.Run("timeout_handling", func() {
		ctx, cancel := context.WithTimeout(suite.ctx, 1*time.Millisecond)
		defer cancel()

		_, err := suite.auth.GetUserByID(ctx, "test-user")
		suite.Error(err)
		suite.Contains(err.Error(), "context deadline exceeded")
	})
}
