package auth

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHelixAuthenticator_GetOIDCClient(t *testing.T) {
	// Create a minimal config for the authenticator
	cfg := &config.ServerConfig{
		Auth: config.Auth{
			Regular: config.Regular{
				TokenValidity: 24 * time.Hour,
			},
		},
	}

	authenticator, err := NewHelixAuthenticator(cfg, nil, "test-secret", nil)
	require.NoError(t, err)

	// HelixAuthenticator should return nil for GetOIDCClient
	// since it doesn't use OIDC
	oidcClient := authenticator.GetOIDCClient()
	assert.Nil(t, oidcClient, "HelixAuthenticator.GetOIDCClient() should return nil")
}

func TestAuthenticator_Interface_GetOIDCClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a mock authenticator
	mockAuth := NewMockAuthenticator(ctrl)

	// Create a mock OIDC client
	mockOIDC := NewMockOIDC(ctrl)

	// Set up expectation - GetOIDCClient returns the mock OIDC client
	mockAuth.EXPECT().GetOIDCClient().Return(mockOIDC)

	// Call the method
	var auth Authenticator = mockAuth
	result := auth.GetOIDCClient()

	// Verify result
	assert.Equal(t, mockOIDC, result, "GetOIDCClient should return the expected OIDC client")
}

func TestAuthenticator_Interface_GetOIDCClient_Nil(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create a mock authenticator
	mockAuth := NewMockAuthenticator(ctrl)

	// Set up expectation - GetOIDCClient returns nil (like HelixAuthenticator)
	mockAuth.EXPECT().GetOIDCClient().Return(nil)

	// Call the method
	var auth Authenticator = mockAuth
	result := auth.GetOIDCClient()

	// Verify result
	assert.Nil(t, result, "GetOIDCClient should return nil when no OIDC client is available")
}
