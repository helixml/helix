package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Nerzal/gocloak/v13"
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
	ctx    context.Context
	store  store.Store
	mockKC *mockKeycloakServer
	auth   *KeycloakAuthenticator
}

func (suite *KeycloakTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	ctrl := gomock.NewController(suite.T())
	suite.store = store.NewMockStore(ctrl)

	// Setup mock Keycloak server
	suite.mockKC = newMockKeycloakServer()
}

func (suite *KeycloakTestSuite) TearDownSuite() {
	if suite.mockKC != nil && suite.mockKC.server != nil {
		suite.mockKC.server.Close()
	}
}

// mockKeycloakServer simulates a Keycloak server for testing
type mockKeycloakServer struct {
	server        *httptest.Server
	validTokens   map[string]bool
	users         map[string]*gocloak.User
	failureMode   string
	responseDelay time.Duration
	mu            sync.RWMutex
}

func newMockKeycloakServer() *mockKeycloakServer {
	m := &mockKeycloakServer{
		validTokens: make(map[string]bool),
		users:       make(map[string]*gocloak.User),
	}
	m.server = httptest.NewServer(http.HandlerFunc(m.handler))
	return m
}

func (m *mockKeycloakServer) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Simulate configured delay
	time.Sleep(m.responseDelay)

	// Simulate configured failures
	if m.failureMode != "" {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": m.failureMode})
		return
	}

	fmt.Println("request", r.URL.Path)

	switch {
	case strings.Contains(r.URL.Path, "/admin/serverinfo"):
		// Server info endpoint
		json.NewEncoder(w).Encode(map[string]interface{}{
			"systemInfo": map[string]interface{}{
				"version": "1.0.0",
			},
		})
	case strings.Contains(r.URL.Path, "/realms/master/protocol/openid-connect/token"):
		// Admin login endpoint
		json.NewEncoder(w).Encode(gocloak.JWT{
			AccessToken: "mock_admin_token",
			ExpiresIn:   60,
		})
	case r.URL.Path == "/admin/realms" || strings.HasSuffix(r.URL.Path, "/admin/realms/"):
		// Create realm endpoint
		if r.Method == "POST" {
			var realm gocloak.RealmRepresentation
			if err := json.NewDecoder(r.Body).Decode(&realm); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Initialize attributes if not set
			if realm.Attributes == nil {
				realm.Attributes = &map[string]string{}
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(realm)
		}
	case r.URL.Path == "/admin/realms/helix":
		// Get/Update realm endpoint
		switch r.Method {
		case "GET":
			attrs := map[string]string{
				"frontendUrl": "http://localhost:8080",
			}
			json.NewEncoder(w).Encode(gocloak.RealmRepresentation{
				Realm:      gocloak.StringP("helix"),
				Enabled:    gocloak.BoolP(true),
				Attributes: &attrs,
			})
		case "PUT":
			var realm gocloak.RealmRepresentation
			if err := json.NewDecoder(r.Body).Decode(&realm); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			// Initialize attributes if not set
			if realm.Attributes == nil {
				realm.Attributes = &map[string]string{}
			}
			w.WriteHeader(http.StatusOK)
		}
	case strings.Contains(r.URL.Path, "/admin/realms/helix/clients"):
		// Client management endpoints
		switch r.Method {
		case "GET":
			// Return empty list if no query param (should never happen)
			clientID := r.URL.Query().Get("clientId")
			if clientID == "" {
				json.NewEncoder(w).Encode([]gocloak.Client{})
				return
			}
			// Return mock client for the queried client ID
			json.NewEncoder(w).Encode([]gocloak.Client{
				{
					ID:                        gocloak.StringP("test-client-id"),
					ClientID:                  &clientID,
					DirectAccessGrantsEnabled: gocloak.BoolP(true),
				},
			})
		case "POST":
			var client gocloak.Client
			if err := json.NewDecoder(r.Body).Decode(&client); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"id": "test-client-id"})
		}
	case strings.Contains(r.URL.Path, "/admin/realms/helix/users/"):
		// User lookup endpoint
		userID := strings.TrimPrefix(r.URL.Path, "/admin/realms/helix/users/")
		if user, exists := m.users[userID]; exists {
			json.NewEncoder(w).Encode(user)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	case strings.Contains(r.URL.Path, "/realms/helix/protocol/openid-connect/token/introspect"):
		// Token validation endpoint
		token := r.FormValue("token")
		if m.validTokens[token] {
			json.NewEncoder(w).Encode(map[string]interface{}{"active": true})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
		}
	default:
		fmt.Printf("No handler for path: %s\n", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (suite *KeycloakTestSuite) SetupTest() {
	// Reset mock server state before each test
	suite.mockKC.validTokens = make(map[string]bool)
	suite.mockKC.users = make(map[string]*gocloak.User)
	suite.mockKC.failureMode = ""
	suite.mockKC.responseDelay = 0

	// Create fresh authenticator for each test
	cfg := &config.Keycloak{
		KeycloakURL:         suite.mockKC.server.URL,
		Realm:               "helix",
		AdminRealm:          "master",
		Username:            "admin",
		Password:            "admin",
		APIClientID:         "test-client",
		ClientSecret:        "test-secret",
		FrontEndClientID:    "frontend-client",
		KeycloakFrontEndURL: "http://localhost:8080",
		ServerURL:           "http://localhost:3000",
	}

	auth, err := NewKeycloakAuthenticator(cfg, suite.store)
	suite.Require().NoError(err, "Failed to create authenticator")
	suite.Require().NotNil(auth, "Authenticator should not be nil")
	suite.auth = auth
}

func (suite *KeycloakTestSuite) TestValidateUserToken() {
	tests := []struct {
		name    string
		token   string
		setup   func()
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
			name:  "valid_token",
			token: "valid.test.token",
			setup: func() {
				suite.mockKC.mu.Lock()
				suite.mockKC.validTokens["valid.test.token"] = true
				suite.mockKC.mu.Unlock()
			},
			wantErr: false,
		},
		{
			name:  "expired_token",
			token: "expired.test.token",
			setup: func() {
				suite.mockKC.mu.Lock()
				suite.mockKC.validTokens["expired.test.token"] = false
				suite.mockKC.mu.Unlock()
			},
			wantErr: true,
			errMsg:  "invalid or expired token",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

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
		setup   func()
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
			name:   "valid_user",
			userID: "valid-user-id",
			setup: func() {
				suite.mockKC.mu.Lock()
				suite.mockKC.users["valid-user-id"] = &gocloak.User{
					ID:        gocloak.StringP("valid-user-id"),
					Username:  gocloak.StringP("testuser"),
					Email:     gocloak.StringP("test@example.com"),
					FirstName: gocloak.StringP("Test"),
					LastName:  gocloak.StringP("User"),
				}
				suite.mockKC.mu.Unlock()
			},
			wantErr: false,
		},
		{
			name:    "non_existent_user",
			userID:  "non-existent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			if tt.setup != nil {
				tt.setup()
			}

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

	// Setup valid test data
	suite.mockKC.mu.Lock()
	suite.mockKC.validTokens["valid.token"] = true
	suite.mockKC.users["test-user"] = &gocloak.User{
		ID:       gocloak.StringP("test-user"),
		Username: gocloak.StringP("testuser"),
	}
	suite.mockKC.mu.Unlock()

	suite.Run("concurrent_token_validations", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := suite.auth.ValidateUserToken(suite.ctx, "valid.token")
				if err != nil {
					errChan <- err
				}
			}()
		}

		go func() {
			wg.Wait()
			close(errChan)
		}()

		for err := range errChan {
			suite.Fail("Concurrent validation failed:", err)
		}
	})

	suite.Run("concurrent_user_lookups", func() {
		var wg sync.WaitGroup
		errChan := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				user, err := suite.auth.GetUserByID(suite.ctx, "test-user")
				if err != nil {
					errChan <- err
					return
				}
				if user == nil {
					errChan <- fmt.Errorf("user should not be nil")
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
		// Set a long delay in the mock server
		suite.mockKC.mu.Lock()
		suite.mockKC.responseDelay = 2 * time.Second
		suite.mockKC.mu.Unlock()

		ctx, cancel := context.WithTimeout(suite.ctx, 1*time.Second)
		defer cancel()

		_, err := suite.auth.GetUserByID(ctx, "test-user")
		suite.Error(err)
		suite.Contains(err.Error(), "context deadline exceeded")

		// Reset delay
		suite.mockKC.mu.Lock()
		suite.mockKC.responseDelay = 0
		suite.mockKC.mu.Unlock()
	})

	suite.Run("server_error_handling", func() {
		// Simulate server error
		suite.mockKC.mu.Lock()
		suite.mockKC.failureMode = "internal_error"
		suite.mockKC.mu.Unlock()

		_, err := suite.auth.GetUserByID(suite.ctx, "test-user")
		suite.Error(err)

		// Reset failure mode
		suite.mockKC.mu.Lock()
		suite.mockKC.failureMode = ""
		suite.mockKC.mu.Unlock()
	})
}
