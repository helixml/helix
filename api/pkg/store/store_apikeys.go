package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateAPIKey(ctx context.Context, apiKey *types.ApiKey) (*types.ApiKey, error) {
	if apiKey.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	if apiKey.Key == "" {
		return nil, fmt.Errorf("key not specified")
	}

	apiKey.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(apiKey).Error
	if err != nil {
		return nil, err
	}
	return s.GetAPIKey(ctx, &types.ApiKey{
		Key: apiKey.Key,
	})
}

func (s *PostgresStore) GetAPIKey(ctx context.Context, query *types.ApiKey) (*types.ApiKey, error) {
	if query.Key == "" && query.Owner == "" {
		return nil, fmt.Errorf("key or owner not specified")
	}

	var apiKey types.ApiKey
	err := s.gdb.WithContext(ctx).Where(query).First(&apiKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &apiKey, nil
}

func (s *PostgresStore) ListAPIKeys(ctx context.Context, q *ListAPIKeysQuery) ([]*types.ApiKey, error) {
	var apiKeys []*types.ApiKey
	queryAPIKey := &types.ApiKey{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
	}

	if q.Type != "" {
		queryAPIKey.Type = q.Type
	}

	if q.AppID != "" {
		queryAPIKey.AppID = &sql.NullString{String: q.AppID, Valid: true}
	}

	err := s.gdb.WithContext(ctx).Where(queryAPIKey).Find(&apiKeys).Error
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (s *PostgresStore) DeleteAPIKey(ctx context.Context, key string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.ApiKey{
		Key: key,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
