package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		log.Error().Err(err).Msg("Failed to list OAuth providers from database")
		return fmt.Errorf("failed to list providers: %w", err)
	}

	log.Debug().Int("total_providers", len(providers)).Msg("Found enabled providers in database")

	// Log detailed provider information
	for _, config := range providers {
		log.Debug().Str("provider_id", config.ID).Str("provider_name", config.Name).
			Str("provider_type", string(config.Type)).Bool("provider_enabled", config.Enabled).
			Msg("Found enabled provider in database")
	}

	// Initialize providers
	for _, config := range providers {
		if err := m.InitProvider(ctx, config); err != nil {
			log.Error().Err(err).Str("provider_id", config.ID).Msg("Failed to initialize provider")
			// Continue with other providers
			continue
		}
	}

	// Get all provider IDs for logging
	var providerIDs []string
	for id := range m.providers {
		providerIDs = append(providerIDs, id)
	}
	log.Info().Int("count", len(providers)).Strs("provider_ids", providerIDs).Msg("Loaded OAuth providers")
	return nil
}

// InitProvider initializes an OAuth provider
func (m *Manager) InitProvider(ctx context.Context, config *types.OAuthProvider) error {
	log.Debug().Str("provider_id", config.ID).Str("provider_name", config.Name).
		Str("provider_type", string(config.Type)).Bool("provider_enabled", config.Enabled).
		Msg("Initializing OAuth provider")

	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Create a generic OAuth2 provider for all provider types
	provider, err := NewOAuth2Provider(ctx, config, m.store)
	if err != nil {
		log.Error().Err(err).Str("provider_id", config.ID).Msg("Failed to create OAuth2 provider")
		return err
	}

	// Store with exact case from database
	m.providers[config.ID] = provider
	log.Info().Str("provider_name", config.Name).Str("provider_id", config.ID).
		Str("provider_type", string(config.Type)).Bool("provider_enabled", config.Enabled).
		Msg("Initialized OAuth provider")
	return nil
}

// GetProvider returns a provider with the given id.
func (m *Manager) GetProvider(id string) (Provider, error) {
	log.Debug().Str("provider_id", id).Msg("Looking up OAuth provider by ID")

	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Log all available provider IDs for debugging
	var availableIDs []string
	for providerID := range m.providers {
		availableIDs = append(availableIDs, providerID)
	}
	log.Debug().Strs("available_providers", availableIDs).Int("count", len(m.providers)).Msg("Available providers in memory")

	provider, found := m.providers[id]
	if !found {
		log.Error().Str("provider_id", id).Msg("Provider not found in memory cache")

		// Case-insensitive search for debugging purposes
		for providerID, _ := range m.providers {
			if strings.EqualFold(providerID, id) {
				log.Warn().Str("requested_id", id).Str("found_id", providerID).Msg("Found provider with case-insensitive match")
				// Try returning the case-insensitive match as a temporary fix
				return m.providers[providerID], nil
			}
		}

		// Try to load the provider directly from the database as a fallback
		log.Info().Str("provider_id", id).Msg("Attempting to load provider directly from database")
		ctx := context.Background()
		config, err := m.store.GetOAuthProvider(ctx, id)
		if err != nil {
			log.Error().Err(err).Str("provider_id", id).Msg("Failed to load provider from database")
			return nil, ErrProviderNotFound
		}

		if !config.Enabled {
			log.Warn().Str("provider_id", id).Msg("Provider found in database but is disabled")
			return nil, errors.New("provider is disabled")
		}

		// Initialize and add the provider
		log.Info().Str("provider_id", id).Str("provider_type", string(config.Type)).Msg("Initializing provider from database")
		err = m.InitProvider(ctx, config)
		if err != nil {
			log.Error().Err(err).Str("provider_id", id).Msg("Failed to initialize provider")
			return nil, fmt.Errorf("failed to initialize provider: %w", err)
		}

		// Add to the in-memory cache - not needed, already done in InitProvider
		log.Info().Str("provider_id", id).Msg("Successfully loaded provider from database")

		// Now get the provider from the cache
		provider, found = m.providers[id]
		if !found {
			log.Error().Str("provider_id", id).Msg("Provider was initialized but not found in cache")
			return nil, ErrProviderNotFound
		}

		return provider, nil
	}

	log.Debug().Str("provider_id", id).Str("provider_name", provider.GetName()).Msg("Found OAuth provider")
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

	// Create a generic OAuth2 provider for all provider types
	provider, err := NewOAuth2Provider(ctx, dbProvider, m.store)
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
	log.Debug().Str("provider_id", providerID).Str("user_id", userID).Msg("Initiating OAuth flow")

	provider, err := m.GetProvider(providerID)
	if err != nil {
		log.Error().Err(err).Str("provider_id", providerID).Str("user_id", userID).Msg("Failed to get provider for OAuth flow")
		return "", err
	}

	log.Debug().Str("provider_id", providerID).Str("provider_name", provider.GetName()).Str("user_id", userID).Msg("Found provider, getting authorization URL")

	authURL, err := provider.GetAuthorizationURL(ctx, userID, redirectURL)
	if err != nil {
		log.Error().Err(err).Str("provider_id", providerID).Str("user_id", userID).Msg("Failed to generate authorization URL")
		return "", err
	}

	log.Debug().Str("provider_id", providerID).Str("user_id", userID).Str("auth_url", authURL).Msg("Generated authorization URL")
	return authURL, nil
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

// CreateOAuth2Provider creates a new OAuth 2.0 provider configuration
func (m *Manager) CreateOAuth2Provider(ctx context.Context, config *types.OAuthProvider) (*types.OAuthProvider, error) {
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

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

// MarkProviderAsReachable marks a provider as reachable
func (m *Manager) MarkProviderAsReachable(ctx context.Context, providerID string) error {
	// Get the provider
	dbProvider, err := m.store.GetOAuthProvider(ctx, providerID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Update the provider as reachable (enabled)
	dbProvider.Enabled = true

	// Update the provider
	_, err = m.store.UpdateOAuthProvider(ctx, dbProvider)
	if err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}

	return nil
}

// MarkProviderAsUnreachable marks a provider as unreachable
func (m *Manager) MarkProviderAsUnreachable(ctx context.Context, providerID string) error {
	// Get the provider
	dbProvider, err := m.store.GetOAuthProvider(ctx, providerID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Update the provider as unreachable (disabled)
	dbProvider.Enabled = false

	// Update the provider
	_, err = m.store.UpdateOAuthProvider(ctx, dbProvider)
	if err != nil {
		return fmt.Errorf("failed to update provider: %w", err)
	}

	return nil
}

// GetOrCreateProviderInstance gets or creates a provider instance
func (m *Manager) GetOrCreateProviderInstance(ctx context.Context, providerID string) (Provider, error) {
	// Check if we already have the provider in memory
	m.mutex.RLock()
	provider, found := m.providers[providerID]
	m.mutex.RUnlock()
	if found {
		return provider, nil
	}

	// If not, get the provider config from the database
	dbProvider, err := m.store.GetOAuthProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider from database: %w", err)
	}

	// Create a generic OAuth2 provider for all provider types
	provider, err = NewOAuth2Provider(ctx, dbProvider, m.store)
	if err != nil {
		return nil, err
	}

	// Store the provider in memory
	m.mutex.Lock()
	m.providers[providerID] = provider
	m.mutex.Unlock()

	return provider, nil
}
