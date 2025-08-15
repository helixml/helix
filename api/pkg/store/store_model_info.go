package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error) {
	err := validateDynamicModelInfo(modelInfo)
	if err != nil {
		return nil, err
	}

	modelInfo.Created = time.Now()
	modelInfo.Updated = time.Now()

	err = s.gdb.WithContext(ctx).Create(modelInfo).Error
	if err != nil {
		return nil, err
	}
	return s.GetDynamicModelInfo(ctx, modelInfo.ID)
}

func validateDynamicModelInfo(modelInfo *types.DynamicModelInfo) error {
	if modelInfo.ID == "" {
		return fmt.Errorf("id not specified (should be 'provider/model' slug)")
	}

	if modelInfo.Provider == "" {
		return fmt.Errorf("provider not specified")
	}

	if modelInfo.Name == "" {
		return fmt.Errorf("name not specified")
	}

	return nil
}

func (s *PostgresStore) UpdateDynamicModelInfo(ctx context.Context, modelInfo *types.DynamicModelInfo) (*types.DynamicModelInfo, error) {
	if modelInfo.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	// Check if model info exists
	existingModelInfo, err := s.GetDynamicModelInfo(ctx, modelInfo.ID)
	if err != nil {
		return nil, err
	}

	if existingModelInfo == nil {
		return nil, fmt.Errorf("dynamic model info not found")
	}

	modelInfo.Updated = time.Now()

	err = s.gdb.WithContext(ctx).Save(modelInfo).Error
	if err != nil {
		return nil, err
	}
	return s.GetDynamicModelInfo(ctx, modelInfo.ID)
}

func (s *PostgresStore) GetDynamicModelInfo(ctx context.Context, id string) (*types.DynamicModelInfo, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var modelInfo types.DynamicModelInfo
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&modelInfo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &modelInfo, nil
}

func (s *PostgresStore) ListDynamicModelInfos(ctx context.Context, q *types.ListDynamicModelInfosQuery) ([]*types.DynamicModelInfo, error) {
	var modelInfos []*types.DynamicModelInfo

	query := s.gdb.WithContext(ctx)

	if q.Provider != "" {
		query = query.Where("provider = ?", q.Provider)
	}

	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}

	err := query.Order("created DESC").Find(&modelInfos).Error
	if err != nil {
		return nil, err
	}

	return modelInfos, nil
}

func (s *PostgresStore) DeleteDynamicModelInfo(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.DynamicModelInfo{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
