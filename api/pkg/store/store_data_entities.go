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

func (s *PostgresStore) CreateDataEntity(ctx context.Context, entity *types.DataEntity) (*types.DataEntity, error) {
	if entity.ID == "" {
		entity.ID = system.GenerateDataEntityID()
	}

	if entity.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	entity.Created = time.Now()

	err := s.gdb.WithContext(ctx).Create(entity).Error
	if err != nil {
		return nil, err
	}
	return s.GetDataEntity(ctx, entity.ID)
}

func (s *PostgresStore) UpdateDataEntity(ctx context.Context, entity *types.DataEntity) (*types.DataEntity, error) {
	if entity.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	if entity.Owner == "" {
		return nil, fmt.Errorf("owner not specified")
	}

	entity.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(&entity).Error
	if err != nil {
		return nil, err
	}
	return s.GetDataEntity(ctx, entity.ID)
}

func (s *PostgresStore) GetDataEntity(ctx context.Context, id string) (*types.DataEntity, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var entity types.DataEntity
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&entity).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &entity, nil
}

func (s *PostgresStore) ListDataEntities(ctx context.Context, q *ListDataEntitiesQuery) ([]*types.DataEntity, error) {
	var entities []*types.DataEntity
	err := s.gdb.WithContext(ctx).Where(&types.DataEntity{
		Owner:     q.Owner,
		OwnerType: q.OwnerType,
	}).Find(&entities).Error
	if err != nil {
		return nil, err
	}

	return entities, nil
}

func (s *PostgresStore) DeleteDataEntity(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.DataEntity{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
