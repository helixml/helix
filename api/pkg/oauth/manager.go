package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

var (
	ErrProviderNotFound         = errors.New("provider not found")
	ErrProviderAlreadyExists    = errors.New("provider already exists")
	ErrConnectionNotFound       = errors.New("connection not found")
	ErrNotImplemented           = errors.New("not implemented")
	ErrInvalidAuthorizationCode = errors.New("invalid authorization code")
	ErrAuthorizationFailed      = errors.New("authorization failed")
)

// Manager handles OAuth provider registrations and connections
type Manager struct {
	store     store.Store
	providers map[string]Provider
	mutex     sync.RWMutex
}

// NewManager creates a new OAuth manager
func NewManager(store store.Store) *Manager {
	return &Manager{
		store:     store,
		providers: make(map[string]Provider),
	}
}

// LoadProviders loads all enabled OAuth providers from the database
func (m *Manager) LoadProviders(ctx context.Context) error {
	log.Info().Msg("Loading OAuth providers")

	// Load all enabled providers
	providers, err := m.store.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{
		Enabled: true,
	})
	if err != nil {
		return fmt.Errorf("failed to list providers: %w", err)
	}

	// Initialize providers
	for _, config := range providers {
		if err := m.InitProvider(ctx, config); err != nil {
			log.Error().Err(err).Str("provider_id", config.ID).Msg("Failed to initialize provider")
			// Continue with other providers
			continue
		}
	}

	log.Info().Int("count", len(providers)).Msg("Loaded OAuth providers")
	return nil
}

// InitProvider initializes an OAuth provider
func (m *Manager) InitProvider(ctx context.Context, config *types.OAuthProvider) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	var provider Provider
	var err error

	// Create the appropriate provider based on type and version
	switch config.Type {
	case types.OAuthProviderTypeAtlassian:
		provider, err = NewAtlassianProvider(ctx, config, m.store)
	case types.OAuthProviderTypeGitHub:
		provider, err = NewGithubProvider(ctx, config, m.store)
	case types.OAuthProviderTypeGoogle:
		provider, err = NewGoogleProvider(ctx, config, m.store)
	case types.OAuthProviderTypeMicrosoft:
		provider, err = NewMicrosoftProvider(ctx, config, m.store)
	default:
		// Generic OAuth2 provider
		if config.Version == types.OAuthVersion2 {
			provider, err = NewOAuth2Provider(ctx, config, m.store)
		} else {
			return fmt.Errorf("unsupported provider type and version: %s %s", config.Type, config.Version)
		}
	}

	if err != nil {
		return err
	}

	m.providers[config.ID] = provider
	log.Info().Str("provider", config.Name).Str("id", config.ID).Msg("Initialized provider")
	return nil
}

// GetProvider returns a provider by ID
func (m *Manager) GetProvider(id string) (Provider, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	provider, found := m.providers[id]
	if !found {
		return nil, errors.New("provider not found")
	}
	return provider, nil
}

// GetProviderByType returns a provider by type
func (m *Manager) GetProviderByType(providerType types.OAuthProviderType) (Provider, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, provider := range m.providers {
		if provider.GetType() == providerType {
			return provider, nil
		}
	}
	return nil, errors.New("provider not found")
}

// RefreshExpiredTokens refreshes tokens that are about to expire
func (m *Manager) RefreshExpiredTokens(ctx context.Context, threshold time.Duration) error {
	log.Debug().Msg("Checking for expired tokens")

	// Get connections that are about to expire
	connections, err := m.store.GetOAuthConnectionsNearExpiry(ctx, time.Now().Add(threshold))
	if err != nil {
		return fmt.Errorf("failed to get connections near expiry: %w", err)
	}

	if len(connections) == 0 {
		return nil
	}

	log.Info().Int("count", len(connections)).Msg("Found connections with tokens to refresh")

	// Refresh each connection
	for _, connection := range connections {
		if err := m.RefreshConnection(ctx, connection); err != nil {
			log.Error().Err(err).Str("connection_id", connection.ID).Msg("Failed to refresh token")
			// Continue with other connections
			continue
		}
	}

	return nil
}

// RefreshConnection refreshes a specific connection
func (m *Manager) RefreshConnection(ctx context.Context, connection *types.OAuthConnection) error {
	provider, err := m.GetProvider(connection.ProviderID)
	if err != nil {
		return err
	}

	// Refresh the token if needed
	if err := provider.RefreshTokenIfNeeded(ctx, connection); err != nil {
		return err
	}

	// Update the connection in the database
	_, err = m.store.UpdateOAuthConnection(ctx, connection)
	return err
}

// GetConnection returns a user's connection to a provider
func (m *Manager) GetConnection(ctx context.Context, userID, providerID string) (*types.OAuthConnection, error) {
	connection, err := m.store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)
	if err != nil {
		return nil, err
	}

	// Check if token needs refreshing
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	// Refresh the token if needed
	if err := provider.RefreshTokenIfNeeded(ctx, connection); err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Update the connection if it was refreshed
	if connection.UpdatedAt.After(connection.CreatedAt) {
		var updatedConnection *types.OAuthConnection
		updatedConnection, err = m.store.UpdateOAuthConnection(ctx, connection)
		if err != nil {
			return nil, fmt.Errorf("failed to update connection: %w", err)
		}
		connection = updatedConnection
	}

	return connection, nil
}

// MakeRequest makes an API request using a user's connection
func (m *Manager) MakeRequest(ctx context.Context, userID, providerID, method, url string, body io.Reader) (*http.Response, error) {
	connection, err := m.GetConnection(ctx, userID, providerID)
	if err != nil {
		return nil, err
	}

	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	return provider.MakeAuthorizedRequest(ctx, connection, method, url, body)
}

// RegisterProvider loads and registers a provider from the database
func (m *Manager) RegisterProvider(ctx context.Context, providerID string) (Provider, error) {
	// Check if already registered
	m.mutex.RLock()
	if provider, exists := m.providers[providerID]; exists {
		m.mutex.RUnlock()
		return provider, nil
	}
	m.mutex.RUnlock()

	// Load from database
	dbProvider, err := m.store.GetOAuthProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider from database: %w", err)
	}

	// Create provider instance based on type and version
	var provider Provider

	switch dbProvider.Type {
	case types.OAuthProviderTypeAtlassian:
		if dbProvider.Version == types.OAuthVersion1 {
			provider, err = NewAtlassianProvider(ctx, dbProvider, m.store)
		} else {
			return nil, fmt.Errorf("unsupported OAuth version %s for Atlassian", dbProvider.Version)
		}
	case types.OAuthProviderTypeGitHub:
		if dbProvider.Version == types.OAuthVersion2 {
			provider, err = NewGithubProvider(ctx, dbProvider, m.store)
		} else {
			return nil, fmt.Errorf("unsupported OAuth version %s for GitHub", dbProvider.Version)
		}
	case types.OAuthProviderTypeGoogle:
		if dbProvider.Version == types.OAuthVersion2 {
			provider, err = NewGoogleProvider(ctx, dbProvider, m.store)
		} else {
			return nil, fmt.Errorf("unsupported OAuth version %s for Google", dbProvider.Version)
		}
	case types.OAuthProviderTypeMicrosoft:
		if dbProvider.Version == types.OAuthVersion2 {
			provider, err = NewMicrosoftProvider(ctx, dbProvider, m.store)
		} else {
			return nil, fmt.Errorf("unsupported OAuth version %s for Microsoft", dbProvider.Version)
		}
	case types.OAuthProviderTypeCustom:
		if dbProvider.Version == types.OAuthVersion2 {
			provider, err = NewOAuth2Provider(ctx, dbProvider, m.store)
		} else {
			return nil, fmt.Errorf("unsupported OAuth version %s for custom provider", dbProvider.Version)
		}
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", dbProvider.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// Register the provider
	m.mutex.Lock()
	m.providers[providerID] = provider
	m.mutex.Unlock()

	return provider, nil
}

// StartOAuthFlow initiates the OAuth flow for a provider
func (m *Manager) StartOAuthFlow(ctx context.Context, userID, providerID, redirectURL string) (string, error) {
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return "", err
	}

	return provider.GetAuthorizationURL(ctx, userID, redirectURL)
}

// CompleteOAuthFlow completes the OAuth flow with the given code
func (m *Manager) CompleteOAuthFlow(ctx context.Context, userID, providerID, code string) (*types.OAuthConnection, error) {
	provider, err := m.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	connection, err := provider.CompleteAuthorization(ctx, userID, code)
	if err != nil {
		return nil, err
	}

	// Save the connection to the database
	savedConnection, err := m.store.CreateOAuthConnection(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("failed to store connection: %w", err)
	}

	return savedConnection, nil
}

// ListUserConnections returns all OAuth connections for a user
func (m *Manager) ListUserConnections(ctx context.Context, userID string) ([]*types.OAuthConnection, error) {
	return m.store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
		UserID: userID,
	})
}

// DeleteConnection removes a user's connection to a provider
func (m *Manager) DeleteConnection(ctx context.Context, userID, connectionID string) error {
	// Verify the connection belongs to the user
	connection, err := m.store.GetOAuthConnection(ctx, connectionID)
	if err != nil {
		return err
	}

	if connection.UserID != userID {
		return errors.New("connection does not belong to user")
	}

	return m.store.DeleteOAuthConnection(ctx, connectionID)
}

// CreateAtlassianProvider creates a new Atlassian provider configuration
func (m *Manager) CreateAtlassianProvider(ctx context.Context, config *types.OAuthProvider) (*types.OAuthProvider, error) {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

	config.Type = types.OAuthProviderTypeAtlassian
	config.Version = types.OAuthVersion1

	provider, err := m.store.CreateOAuthProvider(ctx, config)
	if err != nil {
		return nil, err
	}

	// Register the provider
	_, err = m.RegisterProvider(ctx, provider.ID)
	if err != nil {
		// If registration fails, delete the provider
		_ = m.store.DeleteOAuthProvider(ctx, provider.ID)
		return nil, err
	}

	return provider, nil
}

// CreateOAuth2Provider creates a new OAuth 2.0 provider configuration
func (m *Manager) CreateOAuth2Provider(ctx context.Context, config *types.OAuthProvider) (*types.OAuthProvider, error) {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

	// Make sure it's a supported OAuth 2.0 provider
	if config.Type != types.OAuthProviderTypeGoogle &&
		config.Type != types.OAuthProviderTypeMicrosoft &&
		config.Type != types.OAuthProviderTypeGitHub &&
		config.Type != types.OAuthProviderTypeCustom {
		return nil, fmt.Errorf("unsupported provider type: %s", config.Type)
	}

	config.Version = types.OAuthVersion2

	provider, err := m.store.CreateOAuthProvider(ctx, config)
	if err != nil {
		return nil, err
	}

	// Register the provider
	_, err = m.RegisterProvider(ctx, provider.ID)
	if err != nil {
		// If registration fails, delete the provider
		_ = m.store.DeleteOAuthProvider(ctx, provider.ID)
		return nil, err
	}

	return provider, nil
}
