package oauth

import (
	"context"
	"crypto/tls"
	"encoding/json"
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

// ScopeError represents an error when the user doesn't have the required scopes
type ScopeError struct {
	Missing []string // Missing scopes that the user needs
	Has     []string // Scopes the user currently has
}

// Error returns a string representation of the ScopeError
func (e *ScopeError) Error() string {
	return fmt.Sprintf("missing required OAuth scopes: %v (has: %v)", e.Missing, e.Has)
}

// Manager handles OAuth provider registrations and connections
type Manager struct {
	store         store.Store
	providers     map[string]Provider
	mutex         sync.RWMutex
	tlsSkipVerify bool
}

// NewManager creates a new OAuth manager
func NewManager(store store.Store, tlsSkipVerify bool) *Manager {
	return &Manager{
		store:         store,
		providers:     make(map[string]Provider),
		tlsSkipVerify: tlsSkipVerify,
	}
}

// TODO: sync provider configuration periodically
// Start starts the OAuth manager. Which:
// Reloads configuration every 10 seconds
// Refetches tokens for all connections every 1 minute
func (m *Manager) Start(ctx context.Context) error {

	err := m.LoadProviders(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load OAuth providers")
	}

	err = m.RefreshExpiredTokens(ctx, 5*time.Minute)
	if err != nil {
		log.Error().Err(err).Msg("Failed to refresh expired tokens")
	}

	go func() {
		// Creating two tickers for each of the tasks, we don't
		// want to run them at the same time
		providerTicker := time.NewTicker(10 * time.Second)
		tokenTicker := time.NewTicker(1 * time.Minute)

		for {
			select {
			case <-ctx.Done():
				return
			case <-providerTicker.C:
				err := m.LoadProviders(ctx)
				if err != nil {
					log.Error().Err(err).Msg("Failed to load OAuth providers")
				}
			case <-tokenTicker.C:
				err := m.RefreshExpiredTokens(ctx, 5*time.Minute)
				if err != nil {
					log.Error().Err(err).Msg("Failed to refresh expired tokens")
				}
			}
		}
	}()

	return nil
}

// LoadProviders loads all enabled OAuth providers from the database.
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

	// Initialize providers
	for _, config := range providers {
		if err := m.InitProvider(ctx, config); err != nil {
			log.Error().Err(err).Str("provider_id", config.ID).Msg("Failed to initialize provider")
			// Continue with other providers
			continue
		}
	}

	return nil
}

// InitProvider initializes an OAuth provider
func (m *Manager) InitProvider(ctx context.Context, config *types.OAuthProvider) error {
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
	return nil
}

// GetProvider returns a provider with the given id.
func (m *Manager) GetProvider(id string) (Provider, error) {
	log.Debug().Str("provider_id", id).Msg("Looking up OAuth provider by ID")

	// Try with read lock first
	m.mutex.RLock()

	// Log all available provider IDs for debugging
	var availableIDs []string
	for providerID := range m.providers {
		availableIDs = append(availableIDs, providerID)
	}
	log.Debug().Strs("available_providers", availableIDs).Int("count", len(m.providers)).Msg("Available providers in memory")

	provider, found := m.providers[id]

	// Check for case-insensitive match while still holding read lock
	if !found {
		for providerID, p := range m.providers {
			if strings.EqualFold(providerID, id) {
				log.Warn().Str("requested_id", id).Str("found_id", providerID).Msg("Found provider with case-insensitive match")
				m.mutex.RUnlock()
				return p, nil
			}
		}
	}

	// Release read lock before attempting database operations
	m.mutex.RUnlock()

	// Return early if we found the provider
	if found {
		log.Debug().Str("provider_id", id).Str("provider_name", provider.GetName()).Msg("Found OAuth provider")
		return provider, nil
	}

	log.Error().Str("provider_id", id).Msg("Provider not found in memory cache")

	// Try to load the provider directly from the database as a fallback
	log.Info().Str("provider_id", id).Msg("Attempting to load provider directly from database")

	// Use a timeout context for database operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

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

	// Acquire write lock for initialization
	m.mutex.Lock()

	// Double-check if the provider was added by another thread while we were loading from DB
	if p, exists := m.providers[id]; exists {
		m.mutex.Unlock()
		log.Info().Str("provider_id", id).Msg("Provider was added by another thread while loading")
		return p, nil
	}

	// Initialize the provider (this must be done with the lock held to prevent race conditions)
	provider, err = NewOAuth2Provider(ctx, config, m.store)
	if err != nil {
		m.mutex.Unlock()
		log.Error().Err(err).Str("provider_id", id).Msg("Failed to initialize provider")
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	// Store the provider and release the lock
	m.providers[id] = provider
	m.mutex.Unlock()

	log.Info().Str("provider_id", id).Msg("Successfully loaded provider from database")
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

	// Try to create a new connection first
	log.Info().Str("user_id", userID).Str("provider_id", providerID).Msg("Attempting to create new OAuth connection")
	savedConnection, err := m.store.CreateOAuthConnection(ctx, connection)

	if err != nil {
		// Check if the error is due to a duplicate connection
		if strings.Contains(err.Error(), "duplicate key") || strings.Contains(err.Error(), "unique constraint") {
			log.Info().Str("user_id", userID).Str("provider_id", providerID).Msg("Connection already exists, retrieving existing connection")

			// Get the existing connection
			existingConnection, findErr := m.store.GetOAuthConnectionByUserAndProvider(ctx, userID, providerID)
			if findErr != nil {
				log.Error().Err(findErr).Str("user_id", userID).Str("provider_id", providerID).Msg("Failed to find existing connection after duplicate key error")
				return nil, fmt.Errorf("failed to handle duplicate connection: %w", findErr)
			}

			// Update the existing connection with new token info
			log.Info().Str("connection_id", existingConnection.ID).Msg("Updating existing OAuth connection")

			// Preserve the ID and created date, but update everything else
			connection.ID = existingConnection.ID
			connection.CreatedAt = existingConnection.CreatedAt

			// Update the connection
			updatedConnection, updateErr := m.store.UpdateOAuthConnection(ctx, connection)
			if updateErr != nil {
				log.Error().Err(updateErr).Str("connection_id", existingConnection.ID).Msg("Failed to update existing connection")
				return nil, fmt.Errorf("failed to update existing connection: %w", updateErr)
			}

			return updatedConnection, nil
		}

		// If it's not a duplicate key error, return the original error
		log.Error().Err(err).Msg("Failed to create OAuth connection")
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

// TestGitHubConnection tests a GitHub connection by listing repositories
func (m *Manager) TestGitHubConnection(ctx context.Context, connection *types.OAuthConnection) (map[string]interface{}, error) {
	// Check if this is a GitHub connection
	if connection.Provider.Type != types.OAuthProviderTypeGitHub {
		return nil, fmt.Errorf("not a GitHub connection")
	}

	// Make a request to the GitHub API to list the user's repositories
	// Create HTTP client with optional TLS skip verify for enterprise environments
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	if m.tlsSkipVerify {
		// Clone the default transport to preserve all default settings
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		client.Transport = transport
	}

	// Create the request
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		"https://api.github.com/user/repos?sort=updated&per_page=10",
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add the authorization header
	req.Header.Add("Authorization", "Bearer "+connection.AccessToken)
	req.Header.Add("Accept", "application/vnd.github.v3+json")
	req.Header.Add("User-Agent", "Helix-OAuth-Test")

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check the status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response
	var repos []map[string]interface{}
	if err := json.Unmarshal(body, &repos); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Return success with repo count
	return map[string]any{
		"success":     true,
		"repos_count": len(repos),
		"repos":       repos,
	}, nil
}

// GetTokenForApp retrieves an OAuth token for a specific user's connection to a provider
// This is used during app execution to inject OAuth tokens
func (m *Manager) GetTokenForApp(ctx context.Context, userID string, providerName string) (string, error) {
	provider, err := m.GetProviderByName(ctx, providerName)
	if err != nil {
		return "", fmt.Errorf("failed to get provider %s: %w", providerName, err)
	}

	connections, err := m.store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
		UserID:     userID,
		ProviderID: provider.ID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list user connections: %w", err)
	}

	for _, connection := range connections {
		// Refresh the token if needed
		if err := m.RefreshConnection(ctx, connection); err != nil {
			log.Warn().Err(err).Str("connection_id", connection.ID).Msg("Failed to refresh token, trying next connection")
			continue
		}

		// Found a valid connection with a refreshed token
		return connection.AccessToken, nil
	}

	return "", fmt.Errorf("no active connection found for provider %s", providerName)
}

// GetTokenForTool retrieves an OAuth token for a tool's OAuth provider and required scopes
// This is used during tool execution to inject OAuth tokens
func (m *Manager) GetTokenForTool(ctx context.Context, userID string, providerName string, requiredScopes []string) (string, error) {
	log.Info().Str("user_id", userID).Str("provider_name", providerName).Strs("required_scopes", requiredScopes).Msg("getting token for tool")

	provider, err := m.GetProviderByName(ctx, providerName)
	if err != nil {
		log.Error().Err(err).Str("provider_name", providerName).Msg("Failed to get provider")
		return "", fmt.Errorf("failed to get provider %s: %w", providerName, err)
	}

	connections, err := m.store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
		UserID:     userID,
		ProviderID: provider.ID,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Str("provider_id", provider.ID).Msg("Failed to list user connections")
		return "", fmt.Errorf("failed to list user connections: %w", err)
	}

	if len(connections) == 0 {
		log.Warn().
			Str("user_id", userID).
			Str("provider_name", providerName).
			Str("provider_id", provider.ID).
			Msg("No connections found for user and provider")
		return "", fmt.Errorf("no active connection found for provider %s", providerName)
	}

	for _, connection := range connections {
		// Check if token is expired
		if !connection.ExpiresAt.IsZero() && connection.ExpiresAt.Before(time.Now()) {
			log.Warn().
				Str("user_id", userID).
				Str("provider_name", providerName).
				Str("connection_id", connection.ID).
				Time("expires_at", connection.ExpiresAt).
				Msg("Token expired, attempting to refresh")

			// Try to refresh the token
			err := m.RefreshConnection(ctx, connection)
			if err != nil {
				log.Warn().
					Err(err).
					Str("connection_id", connection.ID).
					Str("provider_name", providerName).
					Msg("Failed to refresh token, trying next connection")
				continue
			}

			log.Info().
				Str("connection_id", connection.ID).
				Str("provider_name", providerName).
				Time("new_expiry", connection.ExpiresAt).
				Msg("Successfully refreshed OAuth token")
		}

		missingScopes := getMissingScopes(connection.Scopes, requiredScopes)
		if len(missingScopes) > 0 {
			log.Warn().
				Str("user_id", userID).
				Str("provider_name", providerName).
				Strs("missing_scopes", missingScopes).
				Strs("connection_scopes", connection.Scopes).
				Strs("required_scopes", requiredScopes).
				Msg("Missing required scopes for connection")
			continue
		}

		log.Info().
			Str("user_id", userID).
			Str("provider_name", providerName).
			Str("connection_id", connection.ID).
			Bool("has_access_token", connection.AccessToken != "").
			Str("token_prefix", connection.AccessToken[:10]+"...").
			Msg("Found valid token for tool")

		return connection.AccessToken, nil
	}

	log.Warn().
		Str("user_id", userID).
		Str("provider_name", providerName).
		Int("connection_count", len(connections)).
		Msg("No valid connection found for provider - all had expired tokens or missing scopes")

	return "", fmt.Errorf("no active connection found for provider %s", providerName)
}

// Helper function to check missing scopes
func getMissingScopes(existingScopes, requiredScopes []string) []string {
	var missingScopes []string
	for _, required := range requiredScopes {
		found := false
		for _, existing := range existingScopes {
			if existing == required {
				found = true
				break
			}
		}
		if !found {
			missingScopes = append(missingScopes, required)
		}
	}
	return missingScopes
}

// Add GetProviderByName method
func (m *Manager) GetProviderByName(ctx context.Context, name string) (*types.OAuthProvider, error) {
	providers, err := m.store.ListOAuthProviders(ctx, &store.ListOAuthProvidersQuery{
		Enabled: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	log.Info().
		Str("requested_provider", name).
		Int("available_providers", len(providers)).
		Msg("Looking for provider by name")

	// First try exact match
	for _, provider := range providers {
		if provider.Name == name {
			log.Info().
				Str("requested_provider", name).
				Str("found_provider", provider.Name).
				Str("provider_id", provider.ID).
				Msg("Found provider with exact name match")
			return provider, nil
		}
	}

	// Then try case-insensitive match
	for _, provider := range providers {
		if strings.EqualFold(provider.Name, name) {
			log.Info().
				Str("requested_provider", name).
				Str("found_provider", provider.Name).
				Str("provider_id", provider.ID).
				Msg("Found provider with case-insensitive name match")
			return provider, nil
		}
	}

	// Log all available provider names for debugging
	var availableNames []string
	for _, provider := range providers {
		availableNames = append(availableNames, provider.Name)
	}

	log.Warn().
		Str("requested_provider", name).
		Strs("available_providers", availableNames).
		Msg("No provider found with matching name")

	return nil, fmt.Errorf("no provider found with name %s", name)
}
