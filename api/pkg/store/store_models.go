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

	for i, model := range ollamaModels {
		// Check if model already exists
		existingModel, err := s.GetModel(ctx, model.ID)
		if err != nil && err != ErrNotFound {
			return err
		}

		// Determine sort order - llama3.1:8b-instruct-q8_0 gets priority
		sortOrder := i + 10 // Default sort order based on position in list + offset
		if model.ID == "llama3.1:8b-instruct-q8_0" {
			sortOrder = 1 // Top priority
		}

		if existingModel != nil {
			// Always update system-managed fields if user hasn't modified the model
			shouldUpdate := false
			updateData := *existingModel

			// Update sort order if not set
			if existingModel.SortOrder == 0 {
				updateData.SortOrder = sortOrder
				shouldUpdate = true
			}

			// If user hasn't modified the model, keep it in sync with code definitions
			log.Debug().
				Str("model_id", model.ID).
				Bool("user_modified", existingModel.UserModified).
				Bool("existing_prewarm", existingModel.Prewarm).
				Bool("code_prewarm", model.Prewarm).
				Msg("checking if model needs seeding updates")
			if !existingModel.UserModified {
				// Update all system-managed fields from code definitions
				if existingModel.Memory != model.Memory {
					updateData.Memory = model.Memory
					shouldUpdate = true
				}
				if existingModel.Name != model.Name {
					updateData.Name = model.Name
					shouldUpdate = true
				}
				if existingModel.Description != model.Description {
					updateData.Description = model.Description
					shouldUpdate = true
				}
				if existingModel.Hide != model.Hide {
					updateData.Hide = model.Hide
					shouldUpdate = true
				}
				if existingModel.Prewarm != model.Prewarm {
					log.Warn().
						Str("model_id", model.ID).
						Bool("existing_prewarm", existingModel.Prewarm).
						Bool("new_prewarm", model.Prewarm).
						Bool("user_modified", existingModel.UserModified).
						Msg("OVERRIDING prewarm setting during model seeding - this may override user dashboard changes!")
					updateData.Prewarm = model.Prewarm
					shouldUpdate = true
				}
				if existingModel.ContextLength != model.ContextLength {
					updateData.ContextLength = model.ContextLength
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				_, err = s.UpdateModel(ctx, &updateData)
				if err != nil {
					log.Err(err).Str("model_id", model.ID).Msg("failed to update existing ollama model")
				} else {
					log.Warn().
						Str("model_id", model.ID).
						Bool("user_modified", existingModel.UserModified).
						Bool("prewarm_updated", updateData.Prewarm != existingModel.Prewarm).
						Bool("final_prewarm", updateData.Prewarm).
						Msg("SEEDING OVERRODE model settings - check if this conflicts with user dashboard changes")
				}
			}
			continue
		}

		// Create new model
		m := &types.Model{
			ID:            model.ID,
			Name:          model.Name,
			Type:          types.ModelTypeChat,
			Runtime:       types.RuntimeOllama,
			ContextLength: model.ContextLength,
			Memory:        model.Memory,
			Description:   model.Description,
			Hide:          model.Hide,
			Enabled:       true,
			SortOrder:     sortOrder,
			Prewarm:       model.Prewarm,
			UserModified:  false, // New models are system-managed
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

	for i, model := range diffusersModels {
		// Check if model already exists
		existingModel, err := s.GetModel(ctx, model.ID)
		if err != nil && err != ErrNotFound {
			return err
		}

		if existingModel != nil {
			// Always update system-managed fields if user hasn't modified the model
			shouldUpdate := false
			updateData := *existingModel

			// Update sort order if not set
			if existingModel.SortOrder == 0 {
				updateData.SortOrder = i + 200 // Diffusers models get 200+ range
				shouldUpdate = true
			}

			// If user hasn't modified the model, keep it in sync with code definitions
			if !existingModel.UserModified {
				// Update all system-managed fields from code definitions
				if existingModel.Memory != model.Memory {
					updateData.Memory = model.Memory
					shouldUpdate = true
				}
				if existingModel.Name != model.Name {
					updateData.Name = model.Name
					shouldUpdate = true
				}
				if existingModel.Description != model.Description {
					updateData.Description = model.Description
					shouldUpdate = true
				}
				if existingModel.Hide != model.Hide {
					updateData.Hide = model.Hide
					shouldUpdate = true
				}
				if existingModel.Prewarm != model.Prewarm {
					updateData.Prewarm = model.Prewarm
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				_, err = s.UpdateModel(ctx, &updateData)
				if err != nil {
					log.Err(err).Str("model_id", model.ID).Msg("failed to update existing diffusers model")
				} else {
					log.Info().
						Str("model_id", model.ID).
						Bool("user_modified", existingModel.UserModified).
						Msg("updated existing diffusers model with latest system defaults")
				}
			}
			continue
		}

		// Create new model
		m := &types.Model{
			ID:            model.ID,
			Name:          model.Name,
			Type:          types.ModelTypeImage,
			Runtime:       types.RuntimeDiffusers,
			Memory:        model.Memory,
			Description:   model.Description,
			Hide:          model.Hide,
			Enabled:       true,
			ContextLength: 0,       // Image models don't have context length
			SortOrder:     i + 200, // Diffusers models get 200+ range
			Prewarm:       model.Prewarm,
			UserModified:  false, // New models are system-managed
		}

		_, err = s.CreateModel(ctx, m)
		if err != nil {
			return fmt.Errorf("failed to create diffusers model %s: %w", model.ID, err)
		}
	}

	return nil
}

func (s *PostgresStore) seedVLLMModels(ctx context.Context) error {
	vllmModels, _ := model.GetDefaultVLLMModels()

	for i, model := range vllmModels {
		// Check if model already exists
		existingModel, err := s.GetModel(ctx, model.ID)
		if err != nil && err != ErrNotFound {
			return err
		}

		// Determine model type based on the args
		modelType := types.ModelTypeChat // Default to chat
		for _, arg := range model.Args {
			if arg == "embed" {
				modelType = types.ModelTypeEmbed
				break
			}
		}

		// Determine sort order - embedding models get higher numbers (lower priority)
		sortOrder := i + 100 // Start VLLM models at 100+ to come after Ollama models
		if modelType == types.ModelTypeEmbed {
			sortOrder = i + 1000 // Embedding models get even higher numbers
		}

		if existingModel != nil {
			// Always update system-managed fields if user hasn't modified the model
			shouldUpdate := false
			updateData := *existingModel

			// Update sort order if not set
			if existingModel.SortOrder == 0 {
				updateData.SortOrder = sortOrder
				shouldUpdate = true
			}
			// Update model type if incorrect
			if existingModel.Type != modelType {
				log.Info().
					Str("model_id", model.ID).
					Str("old_type", string(existingModel.Type)).
					Str("new_type", string(modelType)).
					Msg("Updating VLLM model type")
				updateData.Type = modelType
				shouldUpdate = true
			}

			// If user hasn't modified the model, keep it in sync with code definitions
			log.Debug().
				Str("model_id", model.ID).
				Bool("user_modified", existingModel.UserModified).
				Bool("existing_prewarm", existingModel.Prewarm).
				Bool("code_prewarm", model.Prewarm).
				Msg("checking if VLLM model needs seeding updates")
			if !existingModel.UserModified {
				// Update all system-managed fields from code definitions
				if existingModel.Memory != model.Memory {
					updateData.Memory = model.Memory
					shouldUpdate = true
				}
				if existingModel.Name != model.Name {
					updateData.Name = model.Name
					shouldUpdate = true
				}
				if existingModel.Description != model.Description {
					updateData.Description = model.Description
					shouldUpdate = true
				}
				if existingModel.Hide != model.Hide {
					updateData.Hide = model.Hide
					shouldUpdate = true
				}
				if existingModel.Prewarm != model.Prewarm {
					log.Warn().
						Str("model_id", model.ID).
						Bool("existing_prewarm", existingModel.Prewarm).
						Bool("new_prewarm", model.Prewarm).
						Bool("user_modified", existingModel.UserModified).
						Msg("OVERRIDING VLLM prewarm setting during model seeding - this may override user dashboard changes!")
					updateData.Prewarm = model.Prewarm
					shouldUpdate = true
				}
				if existingModel.ContextLength != model.ContextLength {
					updateData.ContextLength = model.ContextLength
					shouldUpdate = true
				}
			}

			if shouldUpdate {
				_, err = s.UpdateModel(ctx, &updateData)
				if err != nil {
					log.Err(err).Str("model_id", model.ID).Msg("failed to update existing vllm model")
				} else {
					log.Warn().
						Str("model_id", model.ID).
						Bool("user_modified", existingModel.UserModified).
						Bool("prewarm_updated", updateData.Prewarm != existingModel.Prewarm).
						Bool("final_prewarm", updateData.Prewarm).
						Msg("SEEDING OVERRODE VLLM model settings - check if this conflicts with user dashboard changes")
				}
			}
			continue
		}

		// Create RuntimeArgs from model.Args
		runtimeArgs := map[string]interface{}{}
		if len(model.Args) > 0 {
			runtimeArgs["args"] = model.Args
		}

		// Create new model
		m := &types.Model{
			ID:            model.ID,
			Name:          model.Name,
			Type:          modelType,
			Runtime:       types.RuntimeVLLM,
			ContextLength: model.ContextLength,
			Memory:        model.Memory,
			Description:   model.Description,
			Hide:          model.Hide,
			Enabled:       true,
			SortOrder:     sortOrder,
			Prewarm:       model.Prewarm,
			RuntimeArgs:   runtimeArgs,
			UserModified:  false, // New models are system-managed
		}

		log.Info().
			Str("model_id", model.ID).
			Str("model_name", model.Name).
			Str("model_type", string(modelType)).
			Str("runtime", string(types.RuntimeVLLM)).
			Int("sort_order", sortOrder).
			Msg("Creating new VLLM model in database")

		_, err = s.CreateModel(ctx, m)
		if err != nil {
			return fmt.Errorf("failed to create vllm model %s: %w", model.ID, err)
		}
	}

	return nil
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

	// Check if model exists
	existingModel, err := s.GetModel(ctx, model.ID)
	if err != nil {
		return nil, err
	}

	if existingModel == nil {
		return nil, fmt.Errorf("model not found")
	}

	model.Updated = time.Now()

	err = s.gdb.WithContext(ctx).Save(model).Error
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
	Runtime types.Runtime
	Enabled *bool
}

func (s *PostgresStore) ListModels(ctx context.Context, q *ListModelsQuery) ([]*types.Model, error) {
	var models []*types.Model

	query := s.gdb.WithContext(ctx)

	// Handle nil query gracefully
	if q != nil {
		if q.Type != "" {
			query = query.Where("type = ?", q.Type)
		}

		if q.Name != "" {
			query = query.Where("name = ?", q.Name)
		}

		if q.Runtime != "" {
			query = query.Where("runtime = ?", q.Runtime)
		}

		if q.Enabled != nil {
			query = query.Where("enabled = ?", *q.Enabled)
		}
	}

	err := query.Order("sort_order ASC, created DESC").Find(&models).Error
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
