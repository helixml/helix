package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"

	"gorm.io/gorm"
)

type ListAccessGrantsQuery struct {
	OrganizationID string
	ResourceType   types.Resource
	ResourceID     string
	UserID         string
	TeamIDs        []string
}

// CreateAccessGrant creates a new resource access binding
func (s *PostgresStore) CreateAccessGrant(ctx context.Context, resourceAccess *types.AccessGrant, roles []*types.Role) (*types.AccessGrant, error) {
	if resourceAccess.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	if resourceAccess.ResourceID == "" {
		return nil, fmt.Errorf("resource_id not specified")
	}

	if resourceAccess.ResourceType == "" {
		return nil, fmt.Errorf("resource not specified")
	}

	if resourceAccess.UserID == "" && resourceAccess.TeamID == "" {
		return nil, fmt.Errorf("either user_id or team_id must be specified")
	}

	// If both are specified, return an error
	if resourceAccess.UserID != "" && resourceAccess.TeamID != "" {
		return nil, fmt.Errorf("either user_id or team_id must be specified, not both")
	}

	resourceAccess.ID = system.GenerateAccessGrantID()
	resourceAccess.CreatedAt = time.Now()
	resourceAccess.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Grant access to the resource
		err := tx.Create(resourceAccess).Error
		if err != nil {
			return err
		}

		// Create role bindings for the resource access binding
		for _, role := range roles {
			roleBinding := &types.AccessGrantRoleBinding{
				AccessGrantID:  resourceAccess.ID,
				RoleID:         role.ID,
				OrganizationID: resourceAccess.OrganizationID,
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
			}

			err := tx.Create(roleBinding).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return resourceAccess, nil
}

// ListAccessGrants retrieves access grants by resource type, resource ID and either user ID or team IDs
func (s *PostgresStore) ListAccessGrants(ctx context.Context, q *ListAccessGrantsQuery) ([]*types.AccessGrant, error) {
	if q.ResourceType == "" {
		return nil, fmt.Errorf("resource type must be specified")
	}

	if q.ResourceID == "" {
		return nil, fmt.Errorf("resource_id must be specified")
	}

	if q.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id must be specified")
	}

	if q.UserID == "" && len(q.TeamIDs) == 0 {
		return nil, fmt.Errorf("either user_id or team_ids must be specified")
	}

	query := s.gdb.WithContext(ctx).
		Where(&types.AccessGrant{
			OrganizationID: q.OrganizationID,
			ResourceType:   q.ResourceType,
			ResourceID:     q.ResourceID,
		})

	if q.UserID != "" {
		query = query.Where("user_id = ?", q.UserID)
	}

	if len(q.TeamIDs) > 0 {
		query = query.Where("team_id IN (?)", q.TeamIDs)
	}

	var grants []*types.AccessGrant
	err := query.Find(&grants).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Load associated roles for each	 binding
	for _, grant := range grants {
		var roleBindings []types.AccessGrantRoleBinding
		err := s.gdb.WithContext(ctx).
			Where("access_grant_id = ?", grant.ID).
			Find(&roleBindings).Error
		if err != nil {
			return nil, err
		}

		// Get roles for each role binding
		for _, rb := range roleBindings {
			var role types.Role
			err := s.gdb.WithContext(ctx).
				Where("id = ?", rb.RoleID).
				First(&role).Error
			if err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, err
				}
				continue
			}
			grant.Roles = append(grant.Roles, role)
		}
	}

	return grants, nil
}

// DeleteAccessGrant deletes a resource access binding
func (s *PostgresStore) DeleteAccessGrant(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id must be specified")
	}

	// First delete associated role bindings
	err := s.gdb.WithContext(ctx).
		Where("access_grant_id = ?", id).
		Delete(&types.AccessGrantRoleBinding{}).Error
	if err != nil {
		return err
	}

	// Then delete the binding itself
	return s.gdb.WithContext(ctx).
		Where("id = ?", id).
		Delete(&types.AccessGrant{}).Error
}
