package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

// Service Connection methods (GitHub Apps, ADO Service Principals, etc.)
// These are admin-configured organization-level connections for service-to-service auth

// CreateServiceConnection creates a new service connection
func (s *PostgresStore) CreateServiceConnection(ctx context.Context, connection *types.ServiceConnection) error {
	if connection.CreatedAt.IsZero() {
		connection.CreatedAt = time.Now()
	}
	if connection.UpdatedAt.IsZero() {
		connection.UpdatedAt = time.Now()
	}

	err := s.gdb.WithContext(ctx).Create(connection).Error
	if err != nil {
		return fmt.Errorf("failed to create service connection: %w", err)
	}

	return nil
}

// GetServiceConnection gets a service connection by ID
func (s *PostgresStore) GetServiceConnection(ctx context.Context, id string) (*types.ServiceConnection, error) {
	var connection types.ServiceConnection

	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		First(&connection).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service connection: %w", err)
	}

	return &connection, nil
}

// ListServiceConnections lists all service connections for an organization
func (s *PostgresStore) ListServiceConnections(ctx context.Context, organizationID string) ([]*types.ServiceConnection, error) {
	var connections []*types.ServiceConnection

	query := s.gdb.WithContext(ctx)

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}

	err := query.Order("created_at DESC").Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list service connections: %w", err)
	}

	return connections, nil
}

// ListServiceConnectionsByType lists service connections filtered by type
func (s *PostgresStore) ListServiceConnectionsByType(ctx context.Context, organizationID string, connType types.ServiceConnectionType) ([]*types.ServiceConnection, error) {
	var connections []*types.ServiceConnection

	query := s.gdb.WithContext(ctx).Where("type = ?", connType)

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}

	err := query.Order("created_at DESC").Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list service connections by type: %w", err)
	}

	return connections, nil
}

// ListServiceConnectionsByProvider lists service connections filtered by provider type
func (s *PostgresStore) ListServiceConnectionsByProvider(ctx context.Context, organizationID string, providerType types.ExternalRepositoryType) ([]*types.ServiceConnection, error) {
	var connections []*types.ServiceConnection

	query := s.gdb.WithContext(ctx).Where("provider_type = ?", providerType)

	if organizationID != "" {
		query = query.Where("organization_id = ?", organizationID)
	}

	err := query.Order("created_at DESC").Find(&connections).Error

	if err != nil {
		return nil, fmt.Errorf("failed to list service connections by provider: %w", err)
	}

	return connections, nil
}

// UpdateServiceConnection updates a service connection
func (s *PostgresStore) UpdateServiceConnection(ctx context.Context, connection *types.ServiceConnection) error {
	connection.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(connection).Error
	if err != nil {
		return fmt.Errorf("failed to update service connection: %w", err)
	}

	return nil
}

// DeleteServiceConnection deletes a service connection by ID
func (s *PostgresStore) DeleteServiceConnection(ctx context.Context, id string) error {
	err := s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.ServiceConnection{}).Error

	if err != nil {
		return fmt.Errorf("failed to delete service connection: %w", err)
	}

	return nil
}
