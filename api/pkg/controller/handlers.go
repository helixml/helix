package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

func (c *Controller) GetStatus(ctx context.Context, user *types.User) (types.UserStatus, error) {
	usermeta, err := c.Options.Store.GetUserMeta(ctx, user.ID)

	if err != nil || usermeta == nil {
		usermeta = &types.UserMeta{
			ID:     user.ID,
			Config: types.UserConfig{},
		}
	}

	return types.UserStatus{
		Admin:  user.Admin,
		User:   user.ID,
		Slug:   usermeta.Slug,
		Config: usermeta.Config,
	}, nil
}

func (c *Controller) CreateAPIKey(ctx context.Context, user *types.User, apiKey *types.ApiKey) (*types.ApiKey, error) {
	key, err := system.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	apiKey.Key = key
	apiKey.Owner = user.ID
	apiKey.OwnerType = user.Type

	return c.Options.Store.CreateAPIKey(ctx, apiKey)
}

func (c *Controller) GetAPIKeys(ctx context.Context, user *types.User) ([]*types.ApiKey, error) {
	apiKeys, err := c.Options.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
		// filter by APIKeyType_API when deciding whether to auto-create user
		// API keys
		Type: types.APIkeytypeAPI,
	})
	if err != nil {
		return nil, err
	}
	if len(apiKeys) == 0 {
		_, err := c.CreateAPIKey(ctx, user, &types.ApiKey{
			Name: "default",
			Type: types.APIkeytypeAPI,
		})
		if err != nil {
			return nil, err
		}
		return c.GetAPIKeys(ctx, user)
	}
	// return all api key types
	apiKeys, err = c.Options.Store.ListAPIKeys(ctx, &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: user.Type,
	})
	if err != nil {
		return nil, err
	}
	return apiKeys, nil
}

func (c *Controller) DeleteAPIKey(ctx context.Context, user *types.User, apiKey string) error {
	fetchedAPIKey, err := c.Options.Store.GetAPIKey(ctx, apiKey)
	if err != nil {
		return err
	}
	if fetchedAPIKey == nil {
		return errors.New("no such key")
	}
	// only the owner of an api key can delete it
	if fetchedAPIKey.Owner != user.ID || fetchedAPIKey.OwnerType != user.Type {
		return errors.New("unauthorized")
	}
	err = c.Options.Store.DeleteAPIKey(ctx, fetchedAPIKey.Key)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) CheckAPIKey(ctx context.Context, apiKey string) (*types.ApiKey, error) {
	key, err := c.Options.Store.GetAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (c *Controller) GetDashboardData(ctx context.Context) (*types.DashboardData, error) {
	runnerStatuses, err := c.scheduler.RunnerStatus()
	if err != nil {
		return nil, err
	}

	// Get all models from the store to lookup memory requirements
	allModels, err := c.Options.Store.ListModels(ctx, &store.ListModelsQuery{})
	if err != nil {
		log.Warn().Err(err).Msg("error getting models for memory lookup, proceeding without memory info")
		allModels = []*types.Model{} // Continue with empty list
	}

	// Create a map for fast model memory lookups with GGUF-based estimates for Ollama models
	modelMemoryMap := make(map[string]uint64)
	for _, model := range allModels {
		memory := model.Memory

		// For Ollama models, only use GGUF-based memory estimate - skip if not available
		if model.Runtime == types.RuntimeOllama {
			if c.scheduler == nil {
				log.Debug().
					Str("model_id", model.ID).
					Msg("📋 scheduler not available for GGUF estimation, skipping model")
				continue // Skip this model entirely
			} else {
				ggufMemory, err := c.getGGUFBasedMemoryEstimateForDashboard(ctx, model.ID)
				if err != nil {
					log.Trace().
						Str("model_id", model.ID).
						Err(err).
						Msg("📋 GGUF estimation failed, skipping model")
					continue // Skip this model entirely
				} else {
					memory = ggufMemory
					log.Debug().
						Str("model_id", model.ID).
						Uint64("gguf_memory_bytes", ggufMemory).
						Msg("📋 Using GGUF-based memory estimate for dashboard")
				}
			}
		}

		modelMemoryMap[model.ID] = memory
	}

	runners := make([]*types.DashboardRunner, 0, len(runnerStatuses))
	for _, runnerStatus := range runnerStatuses {
		var runnerSlots []*types.RunnerSlot
		runnerSlots, err = c.scheduler.RunnerSlots(runnerStatus.ID)
		if err != nil {
			log.Warn().Err(err).Str("runner_id", runnerStatus.ID).Msg("error getting runner slots, this shouldn't happen, please investigate this runner")
			runnerSlots = []*types.RunnerSlot{}
		}

		// Log what models the runner is reporting
		log.Debug().
			Str("runner_id", runnerStatus.ID).
			Int("model_count", len(runnerStatus.Models)).
			Msg("🏃 Runner reporting models")

		for _, model := range runnerStatus.Models {
			log.Debug().
				Str("runner_id", runnerStatus.ID).
				Str("model_id", model.ModelID).
				Str("runtime", string(model.Runtime)).
				Bool("download_in_progress", model.DownloadInProgress).
				Msg("🏃 Runner model")
		}

		// Only include models that have GGUF estimates (no fallback to store values)
		modelsWithMemory := make([]*types.RunnerModelStatus, 0)
		for _, model := range runnerStatus.Models {
			if memory, exists := modelMemoryMap[model.ModelID]; exists {
				// Copy the model and add GGUF memory estimate
				modelWithMemory := *model // Copy the struct
				modelWithMemory.Memory = memory
				modelsWithMemory = append(modelsWithMemory, &modelWithMemory)
				log.Debug().
					Str("MEMORY_DEBUG", "adding_to_modelsWithMemory").
					Str("runner_id", runnerStatus.ID).
					Str("model_id", model.ModelID).
					Uint64("gguf_memory_bytes", memory).
					Uint64("gguf_memory_gb", memory/(1024*1024*1024)).
					Float64("gguf_memory_gib", float64(memory)/(1024*1024*1024)).
					Msg("🔥 MEMORY_DEBUG: Adding model with memory to runner models list")
			} else {
				// Skip models without GGUF estimates entirely (for Ollama models)
				log.Debug().
					Str("runner_id", runnerStatus.ID).
					Str("model_id", model.ModelID).
					Msg("⏭️ Skipping model without GGUF estimate from dashboard")
			}
		}

		// Also create model entries for any running slots that aren't in the runner's model list
		// This is important because VLLM models may be running as slots but not reported in Models
		slotModelMap := make(map[string]bool)
		for _, model := range modelsWithMemory {
			slotModelMap[model.ModelID] = true
		}

		// Add models from running slots that aren't already in the models list
		for _, slot := range runnerSlots {
			if !slotModelMap[slot.Model] {
				// Only add slot models that have GGUF estimates (no fallback to store values)
				if memory, exists := modelMemoryMap[slot.Model]; exists {
					slotModel := &types.RunnerModelStatus{
						ModelID: slot.Model,
						Runtime: slot.Runtime,
						Memory:  memory,
						// These fields are unknown for slot-derived models
						DownloadInProgress: false,
						DownloadPercent:    0,
						Error:              "",
					}
					modelsWithMemory = append(modelsWithMemory, slotModel)
					slotModelMap[slot.Model] = true
					log.Debug().
						Str("MEMORY_DEBUG", "adding_slot_model_to_modelsWithMemory").
						Str("runner_id", runnerStatus.ID).
						Str("slot_id", slot.ID.String()).
						Str("model", slot.Model).
						Uint64("gguf_memory_bytes", memory).
						Uint64("gguf_memory_gb", memory/(1024*1024*1024)).
						Float64("gguf_memory_gib", float64(memory)/(1024*1024*1024)).
						Msg("🔥 MEMORY_DEBUG: Adding slot model with memory to models list")
				} else {
					// Skip slot models without GGUF estimates entirely (for Ollama models)
					log.Debug().
						Str("runner_id", runnerStatus.ID).
						Str("slot_id", slot.ID.String()).
						Str("model", slot.Model).
						Msg("⏭️ Skipping slot model without GGUF estimate from dashboard")
				}
			}
		}

		runners = append(runners, &types.DashboardRunner{
			ID:              runnerStatus.ID,
			Created:         runnerStatus.Created,
			Updated:         runnerStatus.Updated,
			Version:         runnerStatus.Version,
			TotalMemory:     runnerStatus.TotalMemory,
			FreeMemory:      runnerStatus.FreeMemory,
			UsedMemory:      runnerStatus.UsedMemory,
			AllocatedMemory: runnerStatus.AllocatedMemory,
			GPUCount:        runnerStatus.GPUCount,
			GPUs:            runnerStatus.GPUs,
			Labels:          runnerStatus.Labels,
			Slots:           runnerSlots,
			Models:          modelsWithMemory, // Use models with memory info (now includes slot models)
			ProcessStats:    runnerStatus.ProcessStats,
			GPUMemoryStats:  runnerStatus.GPUMemoryStats,
		})
	}
	queue, err := c.scheduler.Queue()
	if err != nil {
		return nil, err
	}

	// Get recent scheduling decisions (last 50)
	schedulingDecisions := c.scheduler.GetSchedulingDecisions(50)

	// Get recent global allocation decisions (last 25)
	globalAllocationDecisions := c.scheduler.GetGlobalAllocationDecisions(25)

	return &types.DashboardData{
		Runners:                   runners,
		Queue:                     queue,
		SchedulingDecisions:       schedulingDecisions,
		GlobalAllocationDecisions: globalAllocationDecisions,
	}, nil
}

// getGGUFBasedMemoryEstimateForDashboard attempts to get GGUF-based memory estimate for Ollama models for dashboard display
func (c *Controller) getGGUFBasedMemoryEstimateForDashboard(ctx context.Context, modelID string) (uint64, error) {
	// Access memory estimation service through scheduler
	memEstService := c.scheduler.GetMemoryEstimationService()
	if memEstService == nil {
		return 0, fmt.Errorf("memory estimation service not available")
	}

	// Get model from store first
	models, err := c.Options.Store.ListModels(ctx, &store.ListModelsQuery{})
	if err != nil {
		return 0, fmt.Errorf("failed to list models from store: %w", err)
	}

	var targetModel *types.Model
	for _, model := range models {
		if model.ID == modelID {
			targetModel = model
			break
		}
	}

	if targetModel == nil {
		return 0, fmt.Errorf("model %s not found in store", modelID)
	}

	if targetModel.Runtime != types.RuntimeOllama {
		return 0, fmt.Errorf("GGUF-based estimation only available for Ollama models, got %s", targetModel.Runtime)
	}

	if targetModel.ContextLength == 0 {
		log.Error().
			Str("model_id", modelID).
			Msg("CRITICAL: model has no context length configured - cannot estimate memory for dashboard")
		return 0, fmt.Errorf("model %s has no context length configured", modelID)
	}

	// Use model's actual context length and correct KV cache type
	opts := memory.CreateAutoEstimateOptions(targetModel.ContextLength)

	// CRITICAL: Use same concurrency setting as scheduler to ensure consistent cache keys and estimates
	// This fixes the 14.848 GiB vs 47.75 GB discrepancy
	if targetModel.Concurrency > 0 {
		opts.NumParallel = targetModel.Concurrency
	} else if targetModel.Runtime == types.RuntimeOllama {
		opts.NumParallel = memory.DefaultOllamaParallelSequences
	}

	// Get memory estimation
	result, err := memEstService.EstimateModelMemory(ctx, modelID, opts)
	if err != nil {
		return 0, fmt.Errorf("failed to estimate model memory: %w", err)
	}

	// Select the appropriate estimate based on recommendation
	var estimate *memory.MemoryEstimate
	switch result.Recommendation {
	case "single_gpu":
		estimate = result.SingleGPU
	case "tensor_parallel":
		estimate = result.TensorParallel
	case "insufficient_memory":
		// For UI display, prefer GPU estimates to show actual VRAM requirements

		if result.SingleGPU != nil && result.SingleGPU.TotalSize > 0 {
			estimate = result.SingleGPU
		} else if result.TensorParallel != nil && result.TensorParallel.TotalSize > 0 {
			estimate = result.TensorParallel
		} else {
			log.Debug().
				Str("model_id", modelID).
				Bool("single_gpu_nil", result.SingleGPU == nil).
				Uint64("single_gpu_vram", func() uint64 {
					if result.SingleGPU != nil {
						return result.SingleGPU.VRAMSize
					} else {
						return 0
					}
				}()).
				Uint64("single_gpu_total", func() uint64 {
					if result.SingleGPU != nil {
						return result.SingleGPU.TotalSize
					} else {
						return 0
					}
				}()).
				Bool("tensor_parallel_nil", result.TensorParallel == nil).
				Uint64("tensor_parallel_vram", func() uint64 {
					if result.TensorParallel != nil {
						return result.TensorParallel.VRAMSize
					} else {
						return 0
					}
				}()).
				Uint64("tensor_parallel_total", func() uint64 {
					if result.TensorParallel != nil {
						return result.TensorParallel.TotalSize
					} else {
						return 0
					}
				}()).
				Msg("📋 SALMON No valid GPU estimates found for display")
			return 0, fmt.Errorf("no GPU-based estimate available for model %s", modelID)
		}
	default:
		return 0, fmt.Errorf("unknown recommendation %s for model %s", result.Recommendation, modelID)
	}

	if estimate == nil {
		return 0, fmt.Errorf("invalid memory estimate for model %s", modelID)
	}

	// Always use TotalSize for consistent memory estimation
	if estimate.TotalSize > 0 {
		return estimate.TotalSize, nil
	} else {
		return 0, fmt.Errorf("invalid memory estimate for model %s", modelID)
	}
}

func (c *Controller) GetSchedulerHeartbeats(_ context.Context) (interface{}, error) {
	return c.scheduler.GetGoroutineHeartbeats(), nil
}

// DeleteSlotFromScheduler removes a slot from the scheduler's desired state
// This allows the reconciler to clean up the slot from the runner
func (c *Controller) DeleteSlotFromScheduler(_ context.Context, slotID uuid.UUID) error {
	err := c.scheduler.DeleteSlot(slotID)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) updateSubscriptionUser(userID string, stripeCustomerID string, stripeSubscriptionID string, active bool) error {
	existingUser, err := c.Options.Store.GetUserMeta(context.Background(), userID)
	if err != nil || existingUser != nil {
		existingUser = &types.UserMeta{
			ID: userID,
			Config: types.UserConfig{
				StripeCustomerID:     stripeCustomerID,
				StripeSubscriptionID: stripeSubscriptionID,
			},
		}
	}
	existingUser.Config.StripeSubscriptionActive = active
	_, err = c.Options.Store.EnsureUserMeta(context.Background(), *existingUser)
	return err
}

func (c *Controller) HandleSubscriptionEvent(eventType types.SubscriptionEventType, user types.StripeUser) error {
	isSubscriptionActive := true
	if eventType == types.SubscriptionEventTypeDeleted {
		isSubscriptionActive = false
	}
	err := c.updateSubscriptionUser(user.UserID, user.StripeCustomerID, user.SubscriptionID, isSubscriptionActive)
	if err != nil {
		return err
	}
	return c.Options.Janitor.WriteSubscriptionEvent(eventType, user)
}
