package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

func (s *PostgresStore) seedModels(ctx context.Context) error {
	if !s.cfg.SeedModels {
		return nil
	}

	err := s.seedOllamaModels(ctx)
	if err != nil {
		log.Err(err).Msg("failed to seed ollama models")
	}

	err = s.seedDiffusersModels(ctx)
	if err != nil {
		log.Err(err).Msg("failed to seed diffusers models")
	}

	err = s.seedVLLMModels(ctx)
	if err != nil {
		log.Err(err).Msg("failed to seed vllm models")
	}

	return nil
}

func (s *PostgresStore) seedOllamaModels(ctx context.Context) error {
	ollamaModels, _ := model.GetDefaultOllamaModels()

	for _, model := range ollamaModels {
		// Check if model already exists
		existingModel, err := s.GetModel(ctx, model.ID)
		if err != nil && err != ErrNotFound {
			return err
		}

		if existingModel != nil {
			continue
		}

		// Create model
		m := &types.Model{
			ID:            model.ID,
			Name:          model.Name,
			Type:          types.ModelTypeChat,
			Runtime:       types.ModelRuntimeTypeOllama,
			ContextLength: model.ContextLength,
			Memory:        model.Memory,
			Description:   model.Description,
			Hide:          model.Hide,
			Enabled:       true,
		}

		_, err = s.CreateModel(ctx, m)
		if err != nil {
			return fmt.Errorf("failed to create model %s: %w", model.ID, err)
		}
	}

	return nil
}

func (s *PostgresStore) seedDiffusersModels(ctx context.Context) error {
	diffusersModels, _ := model.GetDefaultDiffusersModels()
}

func (s *PostgresStore) seedVLLMModels(ctx context.Context) error {
	vllmModels, _ := model.GetDefaultVLLMModels()
}

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
