package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/gorm"
)

func (s *PostgresStore) CreateModel(ctx context.Context, model *types.Model) (*types.Model, error) {
	err := validateModel(model)
	if err != nil {
		return nil, err
	}

	model.Created = time.Now()
	model.Updated = time.Now()

	err = s.gdb.WithContext(ctx).Create(model).Error
	if err != nil {
		return nil, err
	}
	return s.GetModel(ctx, model.ID)
}

func validateModel(model *types.Model) error {
	if model.Type == "" {
		return fmt.Errorf("type not specified")
	}

	if model.ID == "" {
		return fmt.Errorf("id not specified")
	}

	if model.Memory == 0 {
		return fmt.Errorf("memory not specified")
	}

	if model.Type == types.ModelTypeChat && model.ContextLength == 0 {
		return fmt.Errorf("context length not specified")
	}

	return nil
}

func (s *PostgresStore) UpdateModel(ctx context.Context, model *types.Model) (*types.Model, error) {
	if model.ID == "" {
		return nil, fmt.Errorf("id not specified")
	}

	model.Updated = time.Now()

	err := s.gdb.WithContext(ctx).Save(model).Error
	if err != nil {
		return nil, err
	}
	return s.GetModel(ctx, model.ID)
}

func (s *PostgresStore) GetModel(ctx context.Context, id string) (*types.Model, error) {
	if id == "" {
		return nil, fmt.Errorf("id not specified")
	}

	var model types.Model
	err := s.gdb.WithContext(ctx).Where("id = ?", id).First(&model).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &model, nil
}

type ListModelsQuery struct {
	Type    types.ModelType
	Name    string
	Runtime types.ModelRuntimeType
}

func (s *PostgresStore) ListModels(ctx context.Context, q *ListModelsQuery) ([]*types.Model, error) {
	var models []*types.Model

	query := s.gdb.WithContext(ctx)

	if q.Type != "" {
		query = query.Where("type = ?", q.Type)
	}

	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}

	if q.Runtime != "" {
		query = query.Where("runtime = ?", q.Runtime)
	}

	err := query.Find(&models).Error
	if err != nil {
		return nil, err
	}

	return models, nil
}

func (s *PostgresStore) DeleteModel(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("id not specified")
	}

	err := s.gdb.WithContext(ctx).Delete(&types.Model{
		ID: id,
	}).Error
	if err != nil {
		return err
	}

	return nil
}
