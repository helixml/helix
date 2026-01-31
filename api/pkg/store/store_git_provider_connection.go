package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// Git Provider Connection methods (PAT-based connections)
// These are separate from OAuth connections - they store user's personal access tokens
// for browsing and cloning repositories from GitHub, GitLab, Azure DevOps, etc.

// CreateGitProviderConnection creates a new PAT-based git provider connection
func (s *PostgresStore) CreateGitProviderConnection(ctx context.Context, connection *types.GitProviderConnection) error {
	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = time.Now()
	}
	if connection.UpdatedAt.IsZero() {
		connection.UpdatedAt = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(connection).Error
	if err != nil {
		return fmt.Errorf("failed to create git provider connection: %w", err)
	}

	return nil
}

// GetGitProviderConnection gets a PAT-based git provider connection by ID
func (s *PostgresStore) GetGitProviderConnection(ctx context.Context, id string) (*types.GitProviderConnection, error) {
	var connection types.GitProviderConnection

	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		First(&connection).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get git provider connection: %w", err)
	}

	return &connection, nil
}

// ListGitProviderConnections lists all PAT-based git provider connections for a user
func (s *PostgresStore) ListGitProviderConnections(ctx context.Context, userID string) ([]*types.GitProviderConnection, error) {
	var connections []*types.GitProviderConnection

	err := s.gdb.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list git provider connections: %w", err)
	}

	return connections, nil
}

// DeleteGitProviderConnection deletes a PAT-based git provider connection by ID
func (s *PostgresStore) DeleteGitProviderConnection(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.GitProviderConnection{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete git provider connection: %w", err)
	}

	return nil
}
