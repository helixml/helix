package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SeedModelFromEnv represents a model definition from environment variables
type SeedModelFromEnv struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Runtime       string                 `json:"runtime"`
	Memory        interface{}            `json:"memory,omitempty"` // Can be number (bytes) or string ("8GB")
	ContextLength int64                  `json:"context_length,omitempty"`
	Concurrency   int                    `json:"concurrency,omitempty"`
	Description   string                 `json:"description,omitempty"`
	Enabled       *bool                  `json:"enabled,omitempty"` // Pointer to distinguish nil from false
	Hide          *bool                  `json:"hide,omitempty"`
	AutoPull      *bool                  `json:"auto_pull,omitempty"`
	Prewarm       *bool                  `json:"prewarm,omitempty"`
	UserModified  *bool                  `json:"user_modified,omitempty"` // Should usually be false for seeded models
	RuntimeArgs   map[string]interface{} `json:"runtime_args,omitempty"`
}

// SeedModelsFromEnvironment seeds models from environment variables
func (s *PostgresStore) SeedModelsFromEnvironment(ctx context.Context) error {
	if !s.shouldSeedModels() {
		log.Debug().Msg("model seeding disabled")
		return nil
	}

	seedModels, err := s.loadSeedModelsFromEnvironment()
	if err != nil {
		log.Error().Err(err).Msg("failed to load seed models from environment")
		return fmt.Errorf("failed to load seed models: %w", err)
	}

	if len(seedModels) == 0 {
		log.Debug().Msg("no seed models found in environment")
		return nil
	}

	log.Info().Int("count", len(seedModels)).Msg("seeding models from environment variables")

	for _, seedModel := range seedModels {
		if err := s.seedModel(ctx, seedModel); err != nil {
			log.Error().
				Err(err).
				Str("model_id", seedModel.ID).
				Msg("failed to seed model")
			// Continue with other models rather than failing completely
		}
	}

	log.Info().Msg("completed model seeding from environment variables")
	return nil
}

// shouldSeedModels checks if model seeding is enabled
func (s *PostgresStore) shouldSeedModels() bool {
	enabled := os.Getenv("HELIX_SEED_MODELS_ENABLED")
	if enabled == "" {
		return true // Default to enabled
	}
	return strings.ToLower(enabled) == "true"
}

// loadSeedModelsFromEnvironment loads seed models from environment variables
func (s *PostgresStore) loadSeedModelsFromEnvironment() ([]*SeedModelFromEnv, error) {
	var seedModels []*SeedModelFromEnv

	// Get prefix for individual model environment variables
	prefix := os.Getenv("HELIX_SEED_MODELS_PREFIX")
	if prefix == "" {
		prefix = "HELIX_SEED_MODEL_"
	}

	// Find all environment variables with the prefix
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, prefix) {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) != 2 {
				continue
			}

			envName := parts[0]
			envValue := parts[1]

			if envValue == "" {
				continue
			}

			log.Debug().Str("env_var", envName).Msg("found seed model environment variable")

			var seedModel SeedModelFromEnv
			if err := json.Unmarshal([]byte(envValue), &seedModel); err != nil {
				log.Error().
					Err(err).
					Str("env_var", envName).
					Str("value", envValue).
					Msg("failed to parse seed model JSON")
				continue
			}

			// Validate required fields
			if seedModel.ID == "" || seedModel.Name == "" || seedModel.Type == "" || seedModel.Runtime == "" {
				log.Error().
					Str("env_var", envName).
					Str("model_id", seedModel.ID).
					Msg("seed model missing required fields (id, name, type, runtime)")
				continue
			}

			seedModels = append(seedModels, &seedModel)
		}
	}

	log.Info().Int("count", len(seedModels)).Msg("loaded seed models from environment")
	return seedModels, nil
}

// seedModel creates or updates a single model from seed data
func (s *PostgresStore) seedModel(ctx context.Context, seedModel *SeedModelFromEnv) error {
	// Check if model already exists
	existingModel, err := s.GetModel(ctx, seedModel.ID)
	if err != nil && err != ErrNotFound {
		return fmt.Errorf("failed to check existing model: %w", err)
	}

	// Convert seed model to types.Model
	model, err := s.convertSeedModelToTypesModel(seedModel)
	if err != nil {
		return fmt.Errorf("failed to convert seed model: %w", err)
	}

	if existingModel != nil {
		// Model exists - check if we should update it
		if existingModel.UserModified {
			log.Debug().
				Str("model_id", seedModel.ID).
				Msg("skipping update of user-modified model")
			return nil
		}

		updateExisting := os.Getenv("HELIX_SEED_MODELS_UPDATE_EXISTING")
		if strings.ToLower(updateExisting) != "true" {
			log.Debug().
				Str("model_id", seedModel.ID).
				Msg("skipping update of existing model (HELIX_SEED_MODELS_UPDATE_EXISTING=false)")
			return nil
		}

		// Update existing model
		model.ID = existingModel.ID // Ensure we keep the same ID
		model.Created = existingModel.Created
		model.Updated = time.Now()

		if err := s.gdb.WithContext(ctx).Save(model).Error; err != nil {
			return fmt.Errorf("failed to update model: %w", err)
		}

		log.Info().
			Str("model_id", model.ID).
			Str("name", model.Name).
			Msg("updated model from environment seed")
	} else {
		// Create new model
		model.Created = time.Now()
		model.Updated = time.Now()

		if err := s.gdb.WithContext(ctx).Create(model).Error; err != nil {
			return fmt.Errorf("failed to create model: %w", err)
		}

		log.Info().
			Str("model_id", model.ID).
			Str("name", model.Name).
			Msg("created model from environment seed")
	}

	return nil
}

// convertSeedModelToTypesModel converts a SeedModelFromEnv to types.Model
func (s *PostgresStore) convertSeedModelToTypesModel(seedModel *SeedModelFromEnv) (*types.Model, error) {
	model := &types.Model{
		ID:          seedModel.ID,
		Name:        seedModel.Name,
		Description: seedModel.Description,
	}

	// Convert type
	switch strings.ToLower(seedModel.Type) {
	case "chat":
		model.Type = types.ModelTypeChat
	case "image":
		model.Type = types.ModelTypeImage
	case "embed":
		model.Type = types.ModelTypeEmbed
	default:
		return nil, fmt.Errorf("unsupported model type: %s", seedModel.Type)
	}

	// Convert runtime
	switch strings.ToLower(seedModel.Runtime) {
	case "ollama":
		model.Runtime = types.RuntimeOllama
	case "vllm":
		model.Runtime = types.RuntimeVLLM
	case "diffusers":
		model.Runtime = types.RuntimeDiffusers
	case "axolotl":
		model.Runtime = types.RuntimeAxolotl
	default:
		return nil, fmt.Errorf("unsupported runtime: %s", seedModel.Runtime)
	}

	// Convert memory (handle both string and number formats)
	if seedModel.Memory != nil {
		memory, err := s.parseMemory(seedModel.Memory)
		if err != nil {
			return nil, fmt.Errorf("invalid memory format: %w", err)
		}
		model.Memory = memory
	}

	// Set optional fields with defaults
	model.ContextLength = seedModel.ContextLength
	model.Concurrency = seedModel.Concurrency
	model.Enabled = getBoolWithDefault(seedModel.Enabled, true)
	model.Hide = getBoolWithDefault(seedModel.Hide, false)
	model.AutoPull = getBoolWithDefault(seedModel.AutoPull, false)
	model.Prewarm = getBoolWithDefault(seedModel.Prewarm, false)
	model.UserModified = getBoolWithDefault(seedModel.UserModified, false) // Seeded models are system-managed

	// Copy runtime args
	if seedModel.RuntimeArgs != nil {
		model.RuntimeArgs = seedModel.RuntimeArgs
	}

	return model, nil
}

// parseMemory parses memory from various formats (bytes as number, or strings like "8GB")
func (s *PostgresStore) parseMemory(memory interface{}) (uint64, error) {
	switch v := memory.(type) {
	case float64:
		return uint64(v), nil
	case int:
		return uint64(v), nil
	case int64:
		return uint64(v), nil
	case string:
		return s.parseMemoryString(v)
	default:
		return 0, fmt.Errorf("unsupported memory type: %T", memory)
	}
}

// parseMemoryString parses memory strings like "8GB", "4096MB", "1073741824"
func (s *PostgresStore) parseMemoryString(memStr string) (uint64, error) {
	memStr = strings.TrimSpace(strings.ToUpper(memStr))

	// Try parsing as plain number first
	if val, err := strconv.ParseUint(memStr, 10, 64); err == nil {
		return val, nil
	}

	// Parse with units (support both binary and decimal units)
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(B|KB|MB|GB|TB|KIB|MIB|GIB|TIB)?$`)
	matches := re.FindStringSubmatch(memStr)
	if len(matches) != 3 {
		return 0, fmt.Errorf("invalid memory format: %s", memStr)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value: %s", matches[1])
	}

	unit := matches[2]
	if unit == "" {
		unit = "B" // Default to bytes
	}

	var multiplier uint64
	switch unit {
	case "B":
		multiplier = 1
	// Binary units (powers of 1024)
	case "KIB":
		multiplier = 1024
	case "MIB":
		multiplier = 1024 * 1024
	case "GIB":
		multiplier = 1024 * 1024 * 1024
	case "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	// Decimal units (powers of 1000)
	case "KB":
		multiplier = 1000
	case "MB":
		multiplier = 1000 * 1000
	case "GB":
		multiplier = 1000 * 1000 * 1000
	case "TB":
		multiplier = 1000 * 1000 * 1000 * 1000
	default:
		return 0, fmt.Errorf("unsupported memory unit: %s", unit)
	}

	return uint64(value * float64(multiplier)), nil
}

// getBoolWithDefault returns the bool value or default if nil
func getBoolWithDefault(ptr *bool, defaultVal bool) bool {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}
