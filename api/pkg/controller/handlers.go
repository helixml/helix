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
					log.Debug().
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
		log.Trace().
			Str("model_id", model.ID).
			Str("model_type", string(model.Type)).
			Str("runtime", string(model.Runtime)).
			Uint64("memory_bytes", memory).
			Bool("enabled", model.Enabled).
			Msg("📋 Loaded model from store")
	}

	log.Trace().
		Int("total_models_in_store", len(allModels)).
		Msg("📊 Total models loaded from store for memory lookup")

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
		var modelsWithMemory []*types.RunnerModelStatus
		for _, model := range runnerStatus.Models {
			if memory, exists := modelMemoryMap[model.ModelID]; exists {
				// Copy the model and add GGUF memory estimate
				modelWithMemory := *model // Copy the struct
				modelWithMemory.Memory = memory
				modelsWithMemory = append(modelsWithMemory, &modelWithMemory)
				log.Debug().
					Str("runner_id", runnerStatus.ID).
					Str("model_id", model.ModelID).
					Uint64("gguf_memory_bytes", memory).
					Msg("✅ Using GGUF-based memory estimate for runner model")
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
						Str("runner_id", runnerStatus.ID).
						Str("slot_id", slot.ID.String()).
						Str("model", slot.Model).
						Uint64("gguf_memory_bytes", memory).
						Msg("✅ Added slot model with GGUF estimate to models list")
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

	return &types.DashboardData{
		Runners:             runners,
		Queue:               queue,
		SchedulingDecisions: schedulingDecisions,
	}, nil
}

// getGGUFBasedMemoryEstimateForDashboard attempts to get GGUF-based memory estimate for Ollama models for dashboard display
func (c *Controller) getGGUFBasedMemoryEstimateForDashboard(ctx context.Context, modelID string) (uint64, error) {
	// Access memory estimation service through scheduler
	memEstService := c.scheduler.GetMemoryEstimationService()
	if memEstService == nil {
		return 0, fmt.Errorf("memory estimation service not available")
	}
	// Use default GPU configuration for estimation (single GPU with large memory)
	gpuConfig := []types.GPUInfoForEstimation{
		{
			TotalMemory: 80 * 1024 * 1024 * 1024, // 80GB - large enough for any model
			Index:       0,
		},
	}

	// Use default estimation options similar to memory estimation service
	opts := memory.EstimateOptions{
		NumCtx:      4096,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      1,
		KVCacheType: "f16",
	}

	// Get memory estimation
	result, err := memEstService.EstimateModelMemory(ctx, modelID, gpuConfig, opts)
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
	case "cpu_only", "insufficient_memory":
		// For UI display, prefer GPU estimates to show actual VRAM requirements
		if result.SingleGPU != nil && result.SingleGPU.VRAMSize > 0 {
			estimate = result.SingleGPU
		} else if result.TensorParallel != nil && result.TensorParallel.VRAMSize > 0 {
			estimate = result.TensorParallel
		} else {
			return 0, fmt.Errorf("no GPU-based estimate available for model %s", modelID)
		}
	default:
		return 0, fmt.Errorf("unknown recommendation %s for model %s", result.Recommendation, modelID)
	}

	if estimate == nil || estimate.VRAMSize == 0 {
		return 0, fmt.Errorf("invalid memory estimate for model %s", modelID)
	}

	return estimate.VRAMSize, nil
}

func (c *Controller) GetSchedulerHeartbeats(_ context.Context) (interface{}, error) {
	return c.scheduler.GetGoroutineHeartbeats(), nil
}

// DeleteSlotFromScheduler removes a slot from the scheduler's desired state
// This allows the reconciler to clean up the slot from the runner
func (c *Controller) DeleteSlotFromScheduler(_ context.Context, slotID uuid.UUID) error {
	log.Info().Str("slot_id", slotID.String()).Msg("DEBUG: Controller.DeleteSlotFromScheduler called")
	err := c.scheduler.DeleteSlot(slotID)
	if err != nil {
		log.Error().Err(err).Str("slot_id", slotID.String()).Msg("DEBUG: scheduler.DeleteSlot failed")
		return err
	}
	log.Info().Str("slot_id", slotID.String()).Msg("DEBUG: scheduler.DeleteSlot completed successfully")
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
