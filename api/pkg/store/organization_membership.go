package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

type ListOrganizationMembershipsQuery struct {
	OrganizationID string
	UserID         string
}

type GetOrganizationMembershipQuery struct {
	OrganizationID string
	UserID         string
}

// CreateOrganizationMembership creates a new organization membership
func (s *PostgresStore) CreateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	if membership.UserID == "" {
		return nil, fmt.Errorf("user_id not specified")
	}

	if membership.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	membership.CreatedAt = time.Now()
	membership.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Create(membership).Error
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			return nil, fmt.Errorf("user %s already a member of organization %s", membership.UserID, membership.OrganizationID)
		}
		return nil, err
	}
	return s.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{
		OrganizationID: membership.OrganizationID,
		UserID:         membership.UserID,
	})
}

// GetOrganizationMembership retrieves an organization membership by organization ID and user ID
func (s *PostgresStore) GetOrganizationMembership(ctx context.Context, q *GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
	if q.OrganizationID == "" || q.UserID == "" {
		return nil, fmt.Errorf("organization_id and user_id must be specified")
	}

	var membership types.OrganizationMembership
	err := s.gdb.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", q.OrganizationID, q.UserID).
		Preload("User").
		First(&membership).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &membership, nil
}

// ListOrganizationMemberships lists organization memberships based on query parameters
func (s *PostgresStore) ListOrganizationMemberships(ctx context.Context, q *ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
	query := s.gdb.WithContext(ctx)

	if q != nil {
		if q.OrganizationID != "" {
			query = query.Where("organization_id = ?", q.OrganizationID)
		}
		if q.UserID != "" {
			query = query.Where("user_id = ?", q.UserID)
		}
	}

	var memberships []*types.OrganizationMembership
	err := query.Preload("User").Find(&memberships).Error
	if err != nil {
		return nil, err
	}

	return memberships, nil
}

// UpdateOrganizationMembership updates an existing organization membership
func (s *PostgresStore) UpdateOrganizationMembership(ctx context.Context, membership *types.OrganizationMembership) (*types.OrganizationMembership, error) {
	if membership.UserID == "" {
		return nil, fmt.Errorf("user_id not specified")
	}

	if membership.OrganizationID == "" {
		return nil, fmt.Errorf("organization_id not specified")
	}

	membership.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(membership).Error
	if err != nil {
		return nil, err
	}
	return s.GetOrganizationMembership(ctx, &GetOrganizationMembershipQuery{
		OrganizationID: membership.OrganizationID,
		UserID:         membership.UserID,
	})
}

// DeleteOrganizationMembership deletes an organization membership
func (s *PostgresStore) DeleteOrganizationMembership(ctx context.Context, organizationID, userID string) error {
	if organizationID == "" || userID == "" {
		return fmt.Errorf("organization_id and user_id must be specified")
	}

	// We need to perform this in a transaction to ensure data consistency
	return s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// First delete all team memberships for this user in this organization
		if err := tx.Where("organization_id = ? AND user_id = ?", organizationID, userID).
			Delete(&types.TeamMembership{}).Error; err != nil {
			return fmt.Errorf("failed to delete team memberships: %w", err)
		}

		// Then delete the organization membership
		if err := tx.Where("organization_id = ? AND user_id = ?", organizationID, userID).
			Delete(&types.OrganizationMembership{}).Error; err != nil {
			return fmt.Errorf("failed to delete organization membership: %w", err)
		}

		return nil
	})
}
