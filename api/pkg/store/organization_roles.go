package store

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// autoMigrateRoleConfig syncs all role configs in the database
func (s *PostgresStore) autoMigrateRoleConfig(ctx context.Context) error {
	for _, role := range types.Roles {
		err := s.gdb.WithContext(ctx).Model(&types.Role{}).Where("name = ?", role.Name).UpdateColumn("config", role.Config).Error
		if err != nil {
			return fmt.Errorf("failed to migrate role config for %s: %w", role.Name, err)
		}
	}
	return nil
}

func (s *PostgresStore) CreateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	if role.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	if role.Name == "" {
		return nil, fmt.Errorf("name not specified")
	}

	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(role).Error
	if err != nil {
		return nil, err
	}

	return role, nil
}

// GetRole retrieves a role by ID
func (s *PostgresStore) GetRole(ctx context.Context, id string) (*types.Role, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var role types.Role
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&role).Error
	if err != nil {
		return nil, err
	}

	return &role, nil
}

// ListRoles lists roles based on organization ID
func (s *PostgresStore) ListRoles(ctx context.Context, organizationID string) ([]*types.Role, error) {
	if organizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	var roles []*types.Role
	err := s.gdb.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Find(&roles).Error
	if err != nil {
		return nil, err
	}

	return roles, nil
}

// UpdateRole updates an existing role
func (s *PostgresStore) UpdateRole(ctx context.Context, role *types.Role) (*types.Role, error) {
	if role.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if role.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	role.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(role).Error
	if err != nil {
		return nil, err
	}

	return role, nil
}

// DeleteRole deletes a role by ID
func (s *PostgresStore) DeleteRole(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	return s.gdb.WithContext(ctx).Delete(&types.Role{}, "id = ?", id).Error
}
