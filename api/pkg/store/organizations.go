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

type ListOrganizationsQuery struct {
	Owner     string
	OwnerType types.OwnerType
}

// CreateOrganization creates a new organization
func (s *PostgresStore) CreateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	if org.ID == "" {
		org.ID = system.GenerateOrganizationID()
	}

	if org.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	org.CreatedAt = time.Now()
	org.UpdatedAt = time.Now()

	// Check if the organization name is unique
	existingOrg, err := s.GetOrganization(ctx, &GetOrganizationQuery{Name: org.Name})
	if err != nil && err != ErrNotFound {
		return nil, err
	}
	if existingOrg != nil {
		return nil, fmt.Errorf("organization with name %s already exists", org.Name)
	}

	err = s.gdb.WithContext(ctx).Create(org).Error
	if err != nil {
		return nil, err
	}
	return s.GetOrganization(ctx, &GetOrganizationQuery{ID: org.ID})
}

type GetOrganizationQuery struct {
	ID   string
	Name string
}

// GetOrganization retrieves an organization by ID
func (s *PostgresStore) GetOrganization(ctx context.Context, q *GetOrganizationQuery) (*types.Organization, error) {
	if q.ID == "" && q.Name == "" {
		return nil, fmt.Errorf("id or name not specified")
	}

	query := s.gdb.WithContext(ctx)

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}

	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}

	var org types.Organization
	err := query.First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &org, nil
}

// ListOrganizations lists organizations based on query parameters
func (s *PostgresStore) ListOrganizations(ctx context.Context, q *ListOrganizationsQuery) ([]*types.Organization, error) {
	query := s.gdb.WithContext(ctx)

	if q != nil {
		if q.Owner != "" {
			query = query.Where("owner = ?", q.Owner)
		}
		if q.OwnerType != "" {
			query = query.Where("owner_type = ?", q.OwnerType)
		}
	}

	var organizations []*types.Organization
	err := query.Find(&organizations).Error
	if err != nil {
		return nil, err
	}

	return organizations, nil
}

// UpdateOrganization updates an existing organization
func (s *PostgresStore) UpdateOrganization(ctx context.Context, org *types.Organization) (*types.Organization, error) {
	if org.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if org.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	org.UpdatedAt = time.Now()

	err := s.gdb.WithContext(ctx).Save(org).Error
	if err != nil {
		return nil, err
	}
	return s.GetOrganization(ctx, &GetOrganizationQuery{ID: org.ID})
}

// DeleteOrganization deletes an organization by ID
func (s *PostgresStore) DeleteOrganization(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete all memberships first
		if err := tx.Where("organization_id = ?", id).Delete(&types.OrganizationMembership{}).Error; err != nil {
			return err
		}

		// Delete all teams
		if err := tx.Where("organization_id = ?", id).Delete(&types.Team{}).Error; err != nil {
			return err
		}

		// Delete all roles
		if err := tx.Where("organization_id = ?", id).Delete(&types.Role{}).Error; err != nil {
			return err
		}

		// Finally delete the organization
		return tx.Delete(&types.Organization{ID: id}).Error
	})

	return err
}
