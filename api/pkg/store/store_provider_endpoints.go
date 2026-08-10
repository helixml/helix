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

func (s *PostgresStore) CreateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	if providerEndpoint.ID == "" {
		providerEndpoint.ID = system.GenerateProviderEndpointID()
	}

	if providerEndpoint.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if providerEndpoint.EndpointType == "" {
		return nil, fmt.Errorf("endpoint type not specified")
	}

	providerEndpoint.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(providerEndpoint).Error
	if err != nil {
		return nil, err
	}
	return s.GetProviderEndpoint(ctx, &GetProviderEndpointsQuery{ID: providerEndpoint.ID})
}

func (s *PostgresStore) UpdateProviderEndpoint(ctx context.Context, providerEndpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	if providerEndpoint.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if providerEndpoint.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if providerEndpoint.EndpointType == "" {
		return nil, fmt.Errorf("endpoint type not specified")
	}

	providerEndpoint.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(&providerEndpoint).Error
	if err != nil {
		return nil, err
	}
	return s.GetProviderEndpoint(ctx, &GetProviderEndpointsQuery{ID: providerEndpoint.ID})
}

func (s *PostgresStore) GetProviderEndpoint(ctx context.Context, q *GetProviderEndpointsQuery) (*types.ProviderEndpoint, error) {
	var providerEndpoint types.ProviderEndpoint
	query := s.gdb.WithContext(ctx)

	if q.ID != "" {
		query = query.Where("id = ?", q.ID)
	}

	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}

	if q.OwnerType != "" {
		query = query.Where("owner_type = ?", q.OwnerType)
	}

	err := query.First(&providerEndpoint).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &providerEndpoint, nil
}

func (s *PostgresStore) ListProviderEndpoints(ctx context.Context, q *ListProviderEndpointsQuery) ([]*types.ProviderEndpoint, error) {
	var providerEndpoints []*types.ProviderEndpoint
	query := s.gdb.WithContext(ctx)

	// If all is true, load all endpoints
	if q.All {
		err := query.Find(&providerEndpoints).Error
		if err != nil {
			return nil, err
		}
		return providerEndpoints, nil
	}

	// Resolve which endpoint_type corresponds to the requested owner type. This
	// must match q.OwnerType so that org-scoped (endpoint_type='org') and, in
	// the future, team-scoped endpoints are returned rather than always
	// filtering on user-owned ('user') rows.
	ownershipType := ownershipEndpointType(q.OwnerType)

	if q.WithGlobal {
		// Owner's endpoints (of the requested ownership type) OR global endpoints
		query = query.Where(
			"(owner = ? AND endpoint_type = ?) OR endpoint_type = ?",
			q.Owner, ownershipType, types.ProviderEndpointTypeGlobal,
		)
	} else {
		// Owner's endpoints only (of the requested ownership type)
		query = query.Where("owner = ? AND endpoint_type = ?", q.Owner, ownershipType)
	}

	err := query.Find(&providerEndpoints).Error
	if err != nil {
		return nil, err
	}
	return providerEndpoints, nil
}

// ownershipEndpointType maps an owner type to the provider endpoint_type used
// for endpoints it directly owns. It defaults to user endpoints so callers that
// don't set OwnerType keep their previous behavior.
func ownershipEndpointType(ownerType types.OwnerType) types.ProviderEndpointType {
	switch ownerType {
	case types.OwnerTypeOrg:
		return types.ProviderEndpointTypeOrg
	default:
		return types.ProviderEndpointTypeUser
	}
}

func (s *PostgresStore) DeleteProviderEndpoint(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.ProviderEndpoint{
		ID: id,
	}).Error
	if err != nil {
		return err
	}
	return nil
}
