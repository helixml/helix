package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateAPIKey(ctx context.Context, apiKey *types.APIKey) (*types.APIKey, error) {
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
	return s.GetAPIKey(ctx, apiKey.Key)
}

func (s *PostgresStore) GetAPIKey(ctx context.Context, key string) (*types.APIKey, error) {
	var apiKey types.APIKey
	err := s.gdb.WithContext(ctx).Where("key = ?", key).First(&apiKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &apiKey, nil
}

func (s *PostgresStore) ListAPIKeys(ctx context.Context, q *ListApiKeysQuery) ([]*types.APIKey, error) {
	var apiKeys []*types.APIKey
	queryAPIKey := &types.APIKey{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
	}

	if q.Type != "" {
		queryAPIKey.Type = q.Type
	}

	if q.AppID != "" {
		queryAPIKey.AppID = q.AppID
	}

	err := s.gdb.WithContext(ctx).Where(queryAPIKey).Find(&apiKeys).Error
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (s *PostgresStore) DeleteAPIKey(ctx context.Context, key string) error {
	err := s.gdb.WithContext(ctx).Delete(&types.APIKey{
		Key: key,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
