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

// ListGlobalServiceConnections lists only deployment-global,
// admin-owned connections — those with no organization_id (the GitHub
// App, ADO Service Principal, and global Slack app). Org-scoped installs
// such as slack_workspace are excluded: they belong to an org and are
// managed in that org's own settings, not the global admin panel.
func (s *PostgresStore) ListGlobalServiceConnections(ctx context.Context) ([]*types.ServiceConnection, error) {
	var connections []*types.ServiceConnection

	err := s.gdb.WithContext(ctx).
		Where("organization_id = ? OR organization_id IS NULL", "").
		Order("created_at DESC").
		Find(&connections).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list global service connections: %w", err)
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

// GetServiceConnectionBySlackTeamID returns the slack_workspace
// connection whose Slack team id matches. This is the inbound routing
// hot path: every inbound Slack delivery carries a team_id and must
// resolve to the org that installed the app into that workspace. The
// slack_team_id column is indexed. Returns ErrNotFound when no
// workspace install matches.
func (s *PostgresStore) GetServiceConnectionBySlackTeamID(ctx context.Context, teamID string) (*types.ServiceConnection, error) {
	if teamID == "" {
		return nil, fmt.Errorf("team id is required")
	}

	var connection types.ServiceConnection
	err := s.gdb.WithContext(ctx).
		Where("type = ? AND slack_team_id = ?", types.ServiceConnectionTypeSlackWorkspace, teamID).
		First(&connection).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service connection by slack team id: %w", err)
	}

	return &connection, nil
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
