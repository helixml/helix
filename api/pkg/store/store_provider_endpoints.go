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
	query := s.gdb.Debug().WithContext(ctx)

	// If all is true, load all endpoints
	if q.All {
		err := query.Find(&providerEndpoints).Error
		if err != nil {
			return nil, err
		}
		return providerEndpoints, nil
	}

	if q.Owner != "" {
		query = query.Where("owner = ?", q.Owner)
	}
	// Org not specified, loading user endpoints only
	query = query.Where("owner = ? AND endpoint_type = ?", q.Owner, types.ProviderEndpointTypeUser)

	if q.WithGlobal {
		query = query.Or("endpoint_type = ?", types.ProviderEndpointTypeGlobal)
	}

	err := query.Find(&providerEndpoints).Error
	if err != nil {
		return nil, err
	}
	return providerEndpoints, nil
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
