package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
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

	err = s.seedSampleProjects(ctx)
	if err != nil {
		log.Err(err).Msg("failed to seed sample projects")
	}

	return nil
}

func (s *PostgresStore) seedOllamaModels(ctx context.Context) error {
	ollamaModels, _ := model.GetDefaultOllamaModels()

	for i, model := range ollamaModels {
		// Skip hidden models - don't seed them at all
		if model.Hide {
			continue
		}

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
				if existingModel.Concurrency != model.Concurrency {
					updateData.Concurrency = model.Concurrency
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
			Concurrency:   model.Concurrency,
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
		// Skip hidden models - don't seed them at all
		if model.Hide {
			continue
		}

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
		// Skip hidden models - don't seed them at all
		if model.Hide {
			continue
		}

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

	// Allow 0 memory for Ollama models since they auto-detect memory requirements
	if model.Memory == 0 && model.Runtime != types.RuntimeOllama {
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

// seedSampleProjects seeds the database with sample project templates
func (s *PostgresStore) seedSampleProjects(ctx context.Context) error {
	sampleProjects := []types.SampleProject{
		{
			ID:          "sample-todo-app",
			Name:        "Todo App (React + FastAPI)",
			Description: "A full-stack todo application with React frontend and FastAPI backend. Perfect for learning full-stack development patterns.",
			Category:    "web",
			Difficulty:  "beginner",
			RepositoryURL: "https://github.com/helixml/sample-todo-app",
			ThumbnailURL: "",
			StartupScript: `#!/bin/bash
# Install frontend dependencies
cd frontend && npm install && npm run dev &

# Install backend dependencies
cd ../backend && pip install -r requirements.txt && uvicorn main:app --reload &

echo "✅ Todo app started! Frontend: http://localhost:3000, Backend: http://localhost:8000"
`,
			SampleTasks: mustMarshalJSON([]types.SampleProjectTask{
				{
					Title:       "Add user authentication",
					Description: "Implement user login and registration with JWT tokens",
					Priority:    "high",
					Type:        "feature",
				},
				{
					Title:       "Implement dark mode toggle",
					Description: "Add a dark mode theme toggle to the application settings",
					Priority:    "medium",
					Type:        "feature",
				},
				{
					Title:       "Add task categories",
					Description: "Allow users to organize todos into custom categories",
					Priority:    "medium",
					Type:        "feature",
				},
				{
					Title:       "Export tasks to CSV",
					Description: "Add functionality to export todo list as CSV file",
					Priority:    "low",
					Type:        "feature",
				},
			}),
		},
		{
			ID:          "sample-chat-app",
			Name:        "Real-time Chat (Node.js + Socket.io)",
			Description: "A real-time chat application using Node.js, Express, and Socket.io. Learn WebSocket programming and real-time communication.",
			Category:    "web",
			Difficulty:  "intermediate",
			RepositoryURL: "https://github.com/helixml/sample-chat-app",
			ThumbnailURL: "",
			StartupScript: `#!/bin/bash
# Install dependencies
npm install

# Start dev server
npm run dev &

echo "✅ Chat app started on http://localhost:3000"
`,
			SampleTasks: mustMarshalJSON([]types.SampleProjectTask{
				{
					Title:       "Add private messaging",
					Description: "Implement direct messaging between users",
					Priority:    "high",
					Type:        "feature",
				},
				{
					Title:       "Implement read receipts",
					Description: "Show when messages have been read by recipients",
					Priority:    "medium",
					Type:        "feature",
				},
				{
					Title:       "Add file upload support",
					Description: "Allow users to share files and images in chat",
					Priority:    "medium",
					Type:        "feature",
				},
				{
					Title:       "Create user presence indicators",
					Description: "Show online/offline status for users",
					Priority:    "low",
					Type:        "feature",
				},
			}),
		},
		{
			ID:          "sample-blog",
			Name:        "Blog Platform (Next.js + Markdown)",
			Description: "A modern blog platform built with Next.js and Markdown. Learn static site generation, file-based routing, and content management.",
			Category:    "web",
			Difficulty:  "beginner",
			RepositoryURL: "https://github.com/helixml/sample-blog",
			ThumbnailURL: "",
			StartupScript: `#!/bin/bash
# Install dependencies
npm install

# Start Next.js dev server
npm run dev &

echo "✅ Blog started on http://localhost:3000"
`,
			SampleTasks: mustMarshalJSON([]types.SampleProjectTask{
				{
					Title:       "Add tag filtering",
					Description: "Allow users to filter blog posts by tags",
					Priority:    "medium",
					Type:        "feature",
				},
				{
					Title:       "Implement search functionality",
					Description: "Add full-text search across all blog posts",
					Priority:    "high",
					Type:        "feature",
				},
				{
					Title:       "Add RSS feed",
					Description: "Generate RSS feed for blog subscribers",
					Priority:    "low",
					Type:        "feature",
				},
				{
					Title:       "Create reading time estimates",
					Description: "Show estimated reading time for each post",
					Priority:    "low",
					Type:        "feature",
				},
			}),
		},
	}

	for _, sample := range sampleProjects {
		// Check if sample project already exists
		existing, err := s.GetSampleProject(ctx, sample.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Err(err).Str("sample_id", sample.ID).Msg("Failed to check if sample project exists")
			continue
		}

		if existing != nil {
			log.Debug().Str("sample_id", sample.ID).Msg("Sample project already exists, skipping")
			continue
		}

		// Create sample project
		_, err = s.CreateSampleProject(ctx, &sample)
		if err != nil {
			log.Error().Err(err).Str("sample_id", sample.ID).Msg("Failed to seed sample project")
			continue
		}

		log.Info().Str("sample_id", sample.ID).Str("name", sample.Name).Msg("Seeded sample project")
	}

	return nil
}

// mustMarshalJSON marshals a value to JSON or panics (for seed data only)
func mustMarshalJSON(v interface{}) datatypes.JSON {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON for seed data: %v", err))
	}
	return datatypes.JSON(data)
}
