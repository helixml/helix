package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

type GetAccessGrantRoleBindingsQuery struct {
	AccessGrantID  string
	RoleID         string
	OrganizationID string
}

// CreateAccessGrantRoleBinding creates a new role binding for an access grant
func (s *PostgresStore) CreateAccessGrantRoleBinding(ctx context.Context, binding *types.AccessGrantRoleBinding) (*types.AccessGrantRoleBinding, error) {
	if binding.AccessGrantID == "" {
		return nil, fmt.Errorf("access_grant_id not specified")
	}

	if binding.RoleID == "" {
		return nil, fmt.Errorf("role_id not specified")
	}

	if binding.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	binding.CreatedAt = time.Now()
	binding.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(binding).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create access grant role binding (access_grant_id: %s, role_id: %s, organization_id: %s): %w", binding.AccessGrantID, binding.RoleID, binding.OrganizationID, err)
	}

	return binding, nil
}

// DeleteAccessGrantRoleBinding deletes a role binding for an access grant
func (s *PostgresStore) DeleteAccessGrantRoleBinding(ctx context.Context, accessGrantID, roleID string) error {
	if accessGrantID == "" {
		return fmt.Errorf("access_grant_id must be specified")
	}

	if roleID == "" {
		return fmt.Errorf("role_id must be specified")
	}

	return s.gdb.WithContext(ctx).
		Where("access_grant_id = ? AND role_id = ?", accessGrantID, roleID).
		Delete(&types.AccessGrantRoleBinding{}).Error
}

// GetAccessGrantRoleBindings retrieves role bindings based on the provided query parameters
func (s *PostgresStore) GetAccessGrantRoleBindings(ctx context.Context, q *GetAccessGrantRoleBindingsQuery) ([]*types.AccessGrantRoleBinding, error) {
	query := s.gdb.WithContext(ctx)

	if q.AccessGrantID == "" {
		return nil, fmt.Errorf("access_grant_id must be specified")
	}

	query = query.Where("access_grant_id = ?", q.AccessGrantID)

	if q.RoleID != "" {
		query = query.Where("role_id = ?", q.RoleID)
	}

	if q.OrganizationID != "" {
		query = query.Where("organization_id = ?", q.OrganizationID)
	}

	var bindings []*types.AccessGrantRoleBinding
	err := query.Find(&bindings).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return bindings, nil
}
