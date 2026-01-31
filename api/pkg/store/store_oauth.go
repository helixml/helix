package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// ListOAuthProvidersQuery contains filters for listing OAuth providers
type ListOAuthProvidersQuery struct {
	CreatorID   string `json:"creator_id"`
	CreatorType string `json:"creator_type"`
	Type        string `json:"type"`
	Enabled     bool   `json:"enabled"`
}

// ListOAuthConnectionsQuery contains filters for listing OAuth connections
type ListOAuthConnectionsQuery struct {
	UserID     string `json:"user_id"`
	ProviderID string `json:"provider_id"`
}

// OAuth Provider methods

// CreateOAuthProvider creates a new OAuth provider
func (s *PostgresStore) CreateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	if provider.ID == "" {
		provider.ID = uuid.New().String()
	}

	if provider.CreatedAt.IsZero() {
		provider.CreatedAt = time.Now()
	}

	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(provider).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth provider: %w", err)
	}

	return provider, nil
}

// GetOAuthProvider gets an OAuth provider by ID
func (s *PostgresStore) GetOAuthProvider(ctx context.Context, id string) (*types.OAuthProvider, error) {
	var provider types.OAuthProvider

	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		First(&provider).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get OAuth provider: %w", err)
	}

	return &provider, nil
}

// UpdateOAuthProvider updates an existing OAuth provider
func (s *PostgresStore) UpdateOAuthProvider(ctx context.Context, provider *types.OAuthProvider) (*types.OAuthProvider, error) {
	provider.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(provider).Error
	if err != nil {
		return nil, fmt.Errorf("failed to update OAuth provider: %w", err)
	}

	return provider, nil
}

// DeleteOAuthProvider deletes an OAuth provider by ID
func (s *PostgresStore) DeleteOAuthProvider(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.OAuthProvider{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete OAuth provider: %w", err)
	}

	return nil
}

// ListOAuthProviders lists OAuth providers with optional filters
func (s *PostgresStore) ListOAuthProviders(ctx context.Context, query *ListOAuthProvidersQuery) ([]*types.OAuthProvider, error) {
	var providers []*types.OAuthProvider

	db := s.gdb.WithContext(ctx)

	if query != nil {
		if query.CreatorID != "" {
			db = db.Where("creator_id = ?", query.CreatorID)
		}

		if query.CreatorType != "" {
			db = db.Where("creator_type = ?", types.OwnerType(query.CreatorType))
		}

		if query.Type != "" {
			db = db.Where("type = ?", query.Type)
		}

		if query.Enabled {
			db = db.Where("enabled = ?", true)
		}
	}

	err := db.Find(&providers).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list OAuth providers: %w", err)
	}

	return providers, nil
}

// OAuth Connection methods

// CreateOAuthConnection creates a new OAuth connection
func (s *PostgresStore) CreateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	if connection.ID == "" {
		connection.ID = uuid.New().String()
	}

	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = time.Now()
	}

	if connection.UpdatedAt.IsZero() {
		connection.UpdatedAt = time.Now()
	}

	// Check if a connection with this user and provider already exists
	var existingConnection types.OAuthConnection
	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND provider_id = ?", connection.UserID, connection.ProviderID).
		First(&existingConnection).Error

	if err == nil {
		// Connection exists, update it
		connection.ID = existingConnection.ID
		connection.CreatedAt = existingConnection.CreatedAt
		connection.UpdatedAt = time.Now()

		err = s.gdb.WithContext(ctx).Save(connection).Error
		if err != nil {
			return nil, fmt.Errorf("failed to update existing OAuth connection: %w", err)
		}
	} else if err == gorm.ErrRecordNotFound {
		// No existing connection, create a new one
		err = s.gdb.WithContext(ctx).Create(connection).Error
		if err != nil {
			return nil, fmt.Errorf("failed to create OAuth connection: %w", err)
		}
	} else {
		// Some other error
		return nil, fmt.Errorf("failed to check for existing connection: %w", err)
	}

	return connection, nil
}

// GetOAuthConnection gets an OAuth connection by ID
func (s *PostgresStore) GetOAuthConnection(ctx context.Context, id string) (*types.OAuthConnection, error) {
	var connection types.OAuthConnection

	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		First(&connection).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get OAuth connection: %w", err)
	}

	return &connection, nil
}

// GetOAuthConnectionByUserAndProvider gets a connection by user ID and provider ID
func (s *PostgresStore) GetOAuthConnectionByUserAndProvider(ctx context.Context, userID, providerID string) (*types.OAuthConnection, error) {
	var connection types.OAuthConnection

	err := s.gdb.WithContext(ctx).
		Preload("Provider").
		Where("user_id = ? AND provider_id = ? AND deleted_at IS NULL", userID, providerID).
		First(&connection).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get OAuth connection: %w", err)
	}

	return &connection, nil
}

// UpdateOAuthConnection updates an existing OAuth connection
func (s *PostgresStore) UpdateOAuthConnection(ctx context.Context, connection *types.OAuthConnection) (*types.OAuthConnection, error) {
	connection.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(connection).Error
	if err != nil {
		return nil, fmt.Errorf("failed to update OAuth connection: %w", err)
	}

	return connection, nil
}

// DeleteOAuthConnection deletes an OAuth connection by ID
func (s *PostgresStore) DeleteOAuthConnection(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.OAuthConnection{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete OAuth connection: %w", err)
	}

	return nil
}

// ListOAuthConnections lists OAuth connections with optional filters
func (s *PostgresStore) ListOAuthConnections(ctx context.Context, query *ListOAuthConnectionsQuery) ([]*types.OAuthConnection, error) {
	var connections []*types.OAuthConnection

	db := s.gdb.WithContext(ctx)

	// Always exclude deleted records
	db = db.Where("deleted_at IS NULL")

	if query != nil {
		if query.UserID != "" {
			db = db.Where("user_id = ?", query.UserID)
		}

		if query.ProviderID != "" {
			db = db.Where("provider_id = ?", query.ProviderID)
		}
	}

	err := db.Preload("Provider").Find(&connections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list OAuth connections: %w", err)
	}

	return connections, nil
}

// GetOAuthConnectionsNearExpiry gets connections that are about to expire
func (s *PostgresStore) GetOAuthConnectionsNearExpiry(ctx context.Context, expiresBefore time.Time) ([]*types.OAuthConnection, error) {
	var connections []*types.OAuthConnection

	err := s.gdb.WithContext(ctx).
		Where("expires_at <= ?", expiresBefore).
		Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth connections near expiry: %w", err)
	}

	return connections, nil
}

// OAuth Request Token methods

// CreateOAuthRequestToken creates a new OAuth request token
func (s *PostgresStore) CreateOAuthRequestToken(ctx context.Context, token *types.OAuthRequestToken) (*types.OAuthRequestToken, error) {
	if token.ID == "" {
		token.ID = uuid.New().String()
	}

	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(token).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create OAuth request token: %w", err)
	}

	return token, nil
}

// GetOAuthRequestToken gets request tokens for a user and provider
func (s *PostgresStore) GetOAuthRequestToken(ctx context.Context, userID, providerID string) ([]*types.OAuthRequestToken, error) {
	var tokens []*types.OAuthRequestToken

	err := s.gdb.WithContext(ctx).
		Where("user_id = ? AND provider_id = ?", userID, providerID).
		Order("created_at DESC").
		Find(&tokens).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth request tokens: %w", err)
	}

	return tokens, nil
}

// GetOAuthRequestTokenByState gets request tokens by state
func (s *PostgresStore) GetOAuthRequestTokenByState(ctx context.Context, state string) ([]*types.OAuthRequestToken, error) {
	var tokens []*types.OAuthRequestToken

	err := s.gdb.WithContext(ctx).
		Where("state = ?", state).
		Order("created_at DESC").
		Find(&tokens).Error

	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth request tokens by state: %w", err)
	}

	return tokens, nil
}

// DeleteOAuthRequestToken deletes an OAuth request token by ID
func (s *PostgresStore) DeleteOAuthRequestToken(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.OAuthRequestToken{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete OAuth request token: %w", err)
	}

	return nil
}

// GenerateRandomState generates a random state string for OAuth flow
func (s *PostgresStore) GenerateRandomState(_ context.Context) (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(b), nil
}
