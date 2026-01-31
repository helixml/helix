package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/filestore"
	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/nats-io/nats.go"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

const (
	submitChatCompletionRequestTimeout = 300 * time.Second
	defaultRequestTimeout              = 5 * time.Second   // Reduced from 300s - we need fast failure for slot reconciliation
	evictionRequestTimeout             = 120 * time.Second // Longer timeout for eviction operations which can take time
	cacheUpdateInterval                = 1 * time.Second   // Reduced from 5s for more responsive dashboard updates
)

type RunnerController struct {
	runners     []string
	runnersMu   *sync.RWMutex // Dedicated mutex for runners list only
	callbackMu  *sync.RWMutex // Dedicated mutex for callback function only
	ps          pubsub.PubSub
	ctx         context.Context
	fs          filestore.FileStore
	slotsCache  *LockingRunnerMap[types.ListRunnerSlotsResponse]
	statusCache *LockingRunnerMap[types.RunnerStatus]
	store       store.Store

	onRunnerConnectedFn       func(runnerID string)                         // Callback for when a new runner connects
	getDetailedMemoryResultFn func(modelID string) *memory.EstimationResult // Callback to get detailed memory estimation
	getSchedulerSlotsFn       func() map[uuid.UUID]*Slot                    // Callback to get scheduler's desired state slots
	healthChecker             HealthChecker                                 // Interface for health checking, allows mocking in tests
	runnerClient              RunnerClient                                  // Interface for runner requests, allows mocking in tests
}

// HealthChecker interface allows for mocking health checks in tests
type HealthChecker interface {
	GetHealthz(runnerID string) error
	SetModels(runnerID string) error
}

// RunnerClient interface allows for mocking runner requests in tests
type RunnerClient interface {
	CreateSlot(runnerID string, slotID uuid.UUID, req *types.CreateRunnerSlotRequest) error
	DeleteSlot(runnerID string, slotID uuid.UUID) error
	FetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error)
	FetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error)
	FetchStatus(runnerID string) (types.RunnerStatus, error)
	SyncSystemSettings(runnerID string, settings *types.RunnerSystemConfigRequest) error
	SubmitChatCompletionRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error
	SubmitEmbeddingRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error
	SubmitImageGenerationRequest(slot *Slot, session *types.Session) error
}

// NATSHealthChecker implements HealthChecker using NATS communication
type NATSHealthChecker struct {
	controller *RunnerController
}

// NATSRunnerClient implements RunnerClient using NATS communication
type NATSRunnerClient struct {
	controller *RunnerController
}

func (r *NATSRunnerClient) CreateSlot(runnerID string, _ uuid.UUID, req *types.CreateRunnerSlotRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, &types.Request{
		Method: "POST",
		URL:    "/api/v1/slots",
		Body:   body,
	}, 30*time.Minute)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error creating slot: %s", resp.Body)
	}
	return nil
}

func (r *NATSRunnerClient) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, &types.Request{
		Method: "DELETE",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	}, evictionRequestTimeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("error deleting slot: %s", resp.Body)
	}
	return nil
}

func (r *NATSRunnerClient) FetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	}, defaultRequestTimeout)
	if err != nil {
		return types.RunnerSlot{}, fmt.Errorf("error getting slot: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return types.RunnerSlot{}, fmt.Errorf("error getting slot (status %d): %s", resp.StatusCode, resp.Body)
	}

	var slot types.RunnerSlot
	if err := json.Unmarshal(resp.Body, &slot); err != nil {
		return types.RunnerSlot{}, fmt.Errorf("error unmarshalling slot: %w", err)
	}
	return slot, nil
}

func (r *NATSRunnerClient) FetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/slots",
	}, defaultRequestTimeout)
	if err != nil {
		return types.ListRunnerSlotsResponse{}, err
	}
	var slots types.ListRunnerSlotsResponse
	if err := json.Unmarshal(resp.Body, &slots); err != nil {
		return types.ListRunnerSlotsResponse{}, err
	}
	return slots, nil
}

func (r *NATSRunnerClient) FetchStatus(runnerID string) (types.RunnerStatus, error) {
	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/status",
	}, defaultRequestTimeout)
	if err != nil {
		return types.RunnerStatus{}, err
	}

	var status types.RunnerStatus
	if err := json.Unmarshal(resp.Body, &status); err != nil {
		return types.RunnerStatus{}, err
	}
	return status, nil
}

func (r *NATSRunnerClient) SyncSystemSettings(runnerID string, settings *types.RunnerSystemConfigRequest) error {
	reqBody, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal system config request: %w", err)
	}

	req := &types.Request{
		Method: "PUT",
		URL:    "/api/v1/system/config",
		Body:   reqBody,
	}

	resp, err := r.controller.Send(r.controller.ctx, runnerID, nil, req, 30*time.Second)
	if err != nil {
		return fmt.Errorf("failed to send system config to runner %s: %w", runnerID, err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("runner %s returned non-200 status for system config: %d", runnerID, resp.StatusCode)
	}
	return nil
}

func (h *NATSHealthChecker) GetHealthz(runnerID string) error {
	resp, err := h.controller.Send(h.controller.ctx, runnerID, nil, &types.Request{
		Method: "GET",
		URL:    "/api/v1/healthz",
	}, defaultRequestTimeout)
	if err != nil {
		return fmt.Errorf("error getting healthz: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runner %s is not healthy", runnerID)
	}
	return nil
}

func (h *NATSHealthChecker) SetModels(runnerID string) error {
	// If no store is configured (e.g., in tests), skip model synchronization
	if h.controller.store == nil {
		log.Debug().Str("runner_id", runnerID).Msg("no store configured, skipping model synchronization")
		return nil
	}

	enabled := true
	// Fetch all enabled models
	models, err := h.controller.store.ListModels(context.Background(), &store.ListModelsQuery{
		Enabled: &enabled,
	})
	if err != nil {
		return fmt.Errorf("error listing models: %w", err)
	}

	bts, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("error marshalling models: %w", err)
	}

	resp, err := h.controller.Send(h.controller.ctx, runnerID, nil, &types.Request{
		Method: "POST",
		URL:    "/api/v1/helix-models",
		Body:   bts,
	}, defaultRequestTimeout)
	if err != nil {
		return fmt.Errorf("error setting models: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("runner %s is not healthy", runnerID)
	}
	return nil
}

type RunnerControllerConfig struct {
	PubSub pubsub.PubSub
	FS     filestore.FileStore
	Store  store.Store

	OnRunnerConnectedFn       func(runnerID string)                         // Optional callback for when a new runner connects
	GetDetailedMemoryResultFn func(modelID string) *memory.EstimationResult // Optional callback to get detailed memory estimation
	HealthChecker             HealthChecker                                 // Optional: for testing, defaults to NATS-based implementation
	RunnerClient              RunnerClient                                  // Optional: for testing, defaults to NATS-based implementation
}

func NewRunnerController(ctx context.Context, cfg *RunnerControllerConfig) (*RunnerController, error) {
	controller := &RunnerController{
		ctx:   ctx,
		ps:    cfg.PubSub,
		fs:    cfg.FS,
		store: cfg.Store,

		runners:                   []string{},
		runnersMu:                 &sync.RWMutex{}, // Dedicated mutex for runners list
		callbackMu:                &sync.RWMutex{}, // Dedicated mutex for callback
		slotsCache:                NewLockingRunnerMap[types.ListRunnerSlotsResponse](),
		statusCache:               NewLockingRunnerMap[types.RunnerStatus](),
		onRunnerConnectedFn:       cfg.OnRunnerConnectedFn,
		getDetailedMemoryResultFn: cfg.GetDetailedMemoryResultFn,
	}

	// Set up health checker - use provided one or default to NATS-based
	if cfg.HealthChecker != nil {
		controller.healthChecker = cfg.HealthChecker
	} else {
		controller.healthChecker = &NATSHealthChecker{controller: controller}
	}

	// Set up runner client - use provided one or default to NATS-based
	if cfg.RunnerClient != nil {
		controller.runnerClient = cfg.RunnerClient
	} else {
		controller.runnerClient = &NATSRunnerClient{controller: controller}
	}

	sub, err := cfg.PubSub.SubscribeWithCtx(controller.ctx, pubsub.GetRunnerConnectedQueue("*"), func(_ context.Context, msg *nats.Msg) error {
		log.Debug().Str("subject", msg.Subject).Str("data", string(msg.Data)).Msg("runner ping")
		runnerID, err := pubsub.ParseRunnerID(msg.Subject)
		if err != nil {
			log.Error().Err(err).Str("subject", msg.Subject).Msg("error parsing runner ID")
			return err
		}

		// Only trigger prewarming on actual connection, not periodic pings
		if string(msg.Data) == "connected" {
			log.Info().Str("runner_id", runnerID).Msg("new runner connected")
			controller.OnConnectedHandler(runnerID)
		} else {
			// For ping messages, register runner if not already registered (handles API restart case)
			controller.runnersMu.RLock()
			isRegistered := slices.Contains(controller.runners, runnerID)
			controller.runnersMu.RUnlock()

			if !isRegistered {
				log.Info().Str("runner_id", runnerID).Msg("registering runner from ping (API restart recovery)")
				controller.OnConnectedHandler(runnerID)
			} else {
				log.Trace().Str("runner_id", runnerID).Str("data", string(msg.Data)).Msg("runner ping received")
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error subscribing to runner.connected.*: %w", err)
	}
	go func() {
		<-ctx.Done()
		err := sub.Unsubscribe()
		if err != nil {
			log.Error().Err(err).Msg("error unsubscribing from runner.connected.*")
		}
	}()

	go controller.reconcileCaches(ctx)

	return controller, nil
}

// RunnerClient returns the runner client interface
func (c *RunnerController) RunnerClient() RunnerClient {
	return c.runnerClient
}

// SetOnRunnerConnectedCallback sets the callback function for when a new runner connects
func (c *RunnerController) SetOnRunnerConnectedCallback(fn func(runnerID string)) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.onRunnerConnectedFn = fn
}

// setDetailedMemoryResultCallback sets the callback function to get detailed memory estimation results from scheduler
func (c *RunnerController) setDetailedMemoryResultCallback(fn func(modelID string) *memory.EstimationResult) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.getDetailedMemoryResultFn = fn
}

// setSchedulerSlotsCallback sets the callback function to get scheduler's desired state slots
func (c *RunnerController) setSchedulerSlotsCallback(fn func() map[uuid.UUID]*Slot) {
	c.callbackMu.Lock()
	defer c.callbackMu.Unlock()
	c.getSchedulerSlotsFn = fn
}

func (c *RunnerController) reconcileCaches(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			for _, runnerID := range c.statusCache.Keys() {
				if !slices.Contains(c.runners, runnerID) {
					c.statusCache.DeleteCache(runnerID)
				}
			}
			for _, runnerID := range c.slotsCache.Keys() {
				if !slices.Contains(c.runners, runnerID) {
					c.slotsCache.DeleteCache(runnerID)
				}
			}
		}
	}
}

func (c *RunnerController) Send(ctx context.Context, runnerID string, headers map[string]string, req *types.Request, timeout time.Duration) (*types.Response, error) {
	// PRE-CHECK: Fail fast if runner is not connected (TESTING DEADLOCK)
	if !slices.Contains(c.RunnerIDs(), runnerID) {
		return nil, fmt.Errorf("runner %s is not connected", runnerID)
	}

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	// Create a context that will be cancelled if the runner disconnects during the request
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// For long operations (> 10 seconds), start background monitoring to cancel if runner disappears
	if timeout > 10*time.Second {
		go func() {
			ticker := time.NewTicker(1 * time.Second) // Check every second
			defer ticker.Stop()

			for {
				select {
				case <-requestCtx.Done():
					return // Context already cancelled
				case <-ticker.C:
					// Check if runner is still connected using a separate read to avoid nested locking
					c.runnersMu.RLock()
					isConnected := slices.Contains(c.runners, runnerID)
					c.runnersMu.RUnlock()

					if !isConnected {
						log.Debug().
							Str("runner_id", runnerID).
							Str("method", req.Method).
							Str("url", req.URL).
							Msg("Runner disconnected during NATS request, cancelling")
						cancel() // Cancel the context immediately
						return
					}
				}
			}
		}()
	}

	// Publish the task to the "tasks" subject
	start := time.Now()
	response, err := c.ps.Request(requestCtx, pubsub.GetRunnerQueue(runnerID), headers, data, timeout)
	if err != nil {
		return nil, fmt.Errorf("error sending request to runner: %w", err)
	}
	duration := time.Since(start)

	var resp types.Response
	if err := json.Unmarshal(response, &resp); err != nil {
		return nil, fmt.Errorf("error unmarshalling response: %w", err)
	}
	log.Trace().
		Str("subject", pubsub.GetRunnerQueue(runnerID)).
		Str("runner_id", runnerID).Str("method", req.Method).
		Str("url", req.URL).
		Str("duration", duration.String()).
		Int("status_code", resp.StatusCode).
		Msg("request sent to runner")

	return &resp, nil
}

// SyncSystemSettings pushes system settings to a specific runner
func (c *RunnerController) SyncSystemSettings(ctx context.Context, runnerID string) error {
	// If no store is configured (e.g., in tests), skip system settings synchronization
	if c.store == nil {
		log.Debug().Str("runner_id", runnerID).Msg("no store configured, skipping system settings synchronization")
		return nil
	}

	// Get effective system settings (includes environment variable fallback)
	settings, err := c.store.GetEffectiveSystemSettings(ctx)
	if err != nil {
		log.Error().Err(err).Str("runner_id", runnerID).Msg("failed to get effective system settings for runner sync")
		return fmt.Errorf("failed to get effective system settings: %w", err)
	}

	// Prepare system config request
	var hfToken *string
	if settings.HuggingFaceToken != "" {
		hfToken = &settings.HuggingFaceToken
	}

	configReq := types.RunnerSystemConfigRequest{
		HuggingFaceToken: hfToken,
	}

	// Use RunnerClient interface for system settings sync
	err = c.runnerClient.SyncSystemSettings(runnerID, &configReq)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("failed to sync system settings to runner")
		return err
	}

	log.Info().
		Str("runner_id", runnerID).
		Bool("hf_token_sent", hfToken != nil && *hfToken != "").
		Msg("successfully synced system settings to runner")

	return nil
}

// SyncSystemSettingsToAllRunners pushes system settings to all connected runners
func (c *RunnerController) SyncSystemSettingsToAllRunners(ctx context.Context) {
	c.runnersMu.RLock()
	runners := make([]string, len(c.runners))
	copy(runners, c.runners)
	c.runnersMu.RUnlock()

	for _, runnerID := range runners {
		go func(id string) {
			if err := c.SyncSystemSettings(ctx, id); err != nil {
				log.Error().
					Err(err).
					Str("runner_id", id).
					Msg("failed to sync system settings to runner")
			}
		}(runnerID)
	}
}

func (c *RunnerController) OnConnectedHandler(id string) {
	// Check if runner is new and add to list if needed - minimal lock scope with panic safety
	isNewRunner := func() bool {
		c.runnersMu.Lock()
		defer c.runnersMu.Unlock()

		isNew := !slices.Contains(c.runners, id)
		if isNew {
			c.runners = append(c.runners, id)
			log.Info().Str("runner_id", id).Msg("new runner connected")
		} else {
			log.Info().Str("runner_id", id).Msg("existing runner reconnected")
		}
		return isNew
	}()

	// CRITICAL FIX: Invalidate caches when runner connects/reconnects
	// This ensures that status and slots information is fresh and available immediately
	// for scheduling decisions, rather than waiting up to 1 second for cache refresh
	c.statusCache.DeleteCache(id)
	c.slotsCache.DeleteCache(id)
	log.Debug().Str("runner_id", id).Msg("invalidated runner caches on connection")

	// Always reconcile runner models (for both new and reconnected runners)
	err := c.SetModels(id)
	if err != nil {
		log.Error().Err(err).Str("runner_id", id).Msg("error setting models on runner")
	}

	// Always sync system settings (for both new and reconnected runners)
	go func() {
		// Run in a goroutine to avoid blocking runner registration
		if err := c.SyncSystemSettings(c.ctx, id); err != nil {
			log.Error().
				Err(err).
				Str("runner_id", id).
				Msg("failed to sync system settings to connected runner")
		}
	}()

	// Always call the prewarming callback (for both new and reconnected runners)
	// This ensures that models get prewarmed when runners restart
	c.callbackMu.RLock()
	callback := c.onRunnerConnectedFn
	c.callbackMu.RUnlock()

	if callback != nil {
		go func() {
			// Run in a goroutine to avoid blocking runner registration
			defer func() {
				if r := recover(); r != nil {
					log.Error().
						Str("runner_id", id).
						Interface("panic", r).
						Msg("panic recovered in prewarming callback - this indicates a bug in the prewarming logic")
				}
			}()

			log.Info().
				Str("runner_id", id).
				Bool("is_new_runner", isNewRunner).
				Msg("triggering prewarming for connected runner")
			callback(id)
		}()
	}
}

func (c *RunnerController) deleteRunner(id string) {
	c.runnersMu.Lock()
	defer c.runnersMu.Unlock()
	var newRunners []string
	for _, runner := range c.runners {
		if runner != id {
			newRunners = append(newRunners, runner)
		}
	}
	c.runners = newRunners
}

func (c *RunnerController) RunnerIDs() []string {
	c.runnersMu.RLock()
	defer c.runnersMu.RUnlock()

	return c.runners
}

func (c *RunnerController) TotalMemory(runnerID string) uint64 {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return status.TotalMemory
}

func (c *RunnerController) FreeMemory(runnerID string) uint64 {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return status.FreeMemory
}

func (c *RunnerController) GPUCount(runnerID string) int {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}
	return status.GPUCount
}

// PerGPUMemory calculates the memory per individual GPU for single-GPU VLLM models
// This is what should be used for VLLM memory calculations instead of total aggregated memory
func (c *RunnerController) PerGPUMemory(runnerID string) uint64 {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return 0
	}

	if status.GPUCount == 0 {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("Runner has no GPU count information, falling back to total memory")
		return status.TotalMemory
	}

	perGPUMemory := status.TotalMemory / uint64(status.GPUCount)

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("total_memory", status.TotalMemory).
		Int("gpu_count", status.GPUCount).
		Uint64("per_gpu_memory", perGPUMemory).
		Msg("Calculated per-GPU memory for VLLM")

	return perGPUMemory
}

// GetGPUMemoryInfo returns detailed per-GPU memory information for debugging
func (c *RunnerController) GetGPUMemoryInfo(runnerID string) ([]*types.GPUStatus, error) {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		return nil, err
	}
	return status.GPUs, nil
}

func (c *RunnerController) Version(runnerID string) string {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return ""
	}
	return status.Version
}

// calculateAllocatedMemoryPerGPU calculates how much memory is allocated to each GPU based on scheduler's desired state
// ARCHITECTURAL CHANGE: This now uses the scheduler's desired state instead of actual runner state
// This prevents overscheduling by making decisions based on what we intend to create, not what currently exists
func (c *RunnerController) calculateAllocatedMemoryPerGPU(runnerID string) (map[int]uint64, error) {
	allocatedMemoryPerGPU := make(map[int]uint64)

	// CRITICAL: Use scheduler's desired state slots instead of actual runner slots
	// This prevents race conditions and cache invalidation issues in rapid scheduling scenarios
	if c.getSchedulerSlotsFn == nil {
		err := fmt.Errorf("no scheduler slots callback available for runner %s - this is a programming error", runnerID)
		log.Error().Str("runner_id", runnerID).Err(err).Msg("CRITICAL: No scheduler slots callback available")
		return allocatedMemoryPerGPU, err
	}

	// Get scheduler's desired state slots
	schedulerSlots := c.getSchedulerSlotsFn()

	log.Trace().
		Str("runner_id", runnerID).
		Int("total_scheduler_slots", len(schedulerSlots)).
		Msg("Using scheduler's desired state for memory calculation")

	for slotID, slot := range schedulerSlots {
		// Only consider slots for this specific runner
		if slot.RunnerID != runnerID {
			continue
		}

		// NEW ARCHITECTURE: Require configured models - no fallbacks
		if slot.InitialWork().model == nil || !slot.InitialWork().model.IsAllocationConfigured() {
			err := fmt.Errorf("slot %s has unconfigured model %s - all models must use NewModelForGPUAllocation",
				slotID.String(), slot.InitialWork().ModelName().String())
			log.Error().
				Str("runner_id", runnerID).
				Str("slot_id", slotID.String()).
				Str("model", slot.InitialWork().ModelName().String()).
				Err(err).
				Msg("CRITICAL: Unconfigured model in calculateAllocatedMemoryPerGPU")
			return allocatedMemoryPerGPU, err
		}

		// Use configured model memory (authoritative, never fails)
		modelMemory := slot.InitialWork().model.GetMemoryForAllocation()
		log.Debug().
			Str("runner_id", runnerID).
			Str("slot_id", slotID.String()).
			Str("model", slot.InitialWork().ModelName().String()).
			Uint64("configured_memory_gb", modelMemory/(1024*1024*1024)).
			Int("configured_gpu_count", slot.InitialWork().model.GetGPUCount()).
			Msg("Using configured model memory (no fallbacks)")

		// Verify we got valid memory value
		if modelMemory == 0 {
			err := fmt.Errorf("model memory callback returned zero for slot %s with model %s on runner %s",
				slotID.String(), slot.InitialWork().ModelName().String(), runnerID)
			log.Error().
				Str("runner_id", runnerID).
				Str("slot_id", slotID.String()).
				Str("model", slot.InitialWork().ModelName().String()).
				Err(err).
				Msg("CRITICAL: Model memory callback returned zero - this could lead to over-scheduling")
			return allocatedMemoryPerGPU, err
		}

		log.Trace().
			Str("runner_id", runnerID).
			Str("slot_id", slotID.String()).
			Str("model", slot.InitialWork().ModelName().String()).
			Uint64("model_memory_gb", modelMemory/(1024*1024*1024)).
			Msg("Using authoritative model memory from scheduler (desired state)")

		// Allocate memory to the appropriate GPU(s) based on slot's GPU allocation
		if slot.GPUAllocation != nil && len(slot.GPUAllocation.MultiGPUs) > 1 {
			// Multi-GPU model: distribute memory across GPUs
			memoryPerGPU := modelMemory / uint64(len(slot.GPUAllocation.MultiGPUs))
			for _, gpuIndex := range slot.GPUAllocation.MultiGPUs {
				allocatedMemoryPerGPU[gpuIndex] += memoryPerGPU
			}
		} else if slot.GPUAllocation != nil && slot.GPUAllocation.SingleGPU != nil {
			// Single GPU model: allocate full memory to this GPU
			allocatedMemoryPerGPU[*slot.GPUAllocation.SingleGPU] += modelMemory
		}
		// CPU-only slots (no GPU allocation) don't count toward GPU memory
	}

	log.Trace().
		Str("runner_id", runnerID).
		Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
		Msg("Calculated allocated memory per GPU (skipped slots with unknown memory requirements)")

	return allocatedMemoryPerGPU, nil
}

// CanFitModelOnAnyGPUAllocated checks if any individual GPU has enough free memory for the model based on allocated memory
func (c *RunnerController) CanFitModelOnAnyGPUAllocated(runnerID string, modelMemoryRequirement uint64, allocatedMemoryPerGPU map[int]uint64) bool {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status for GPU check")
		return false
	}

	// Check each individual GPU
	for _, gpu := range status.GPUs {
		allocatedMemory := allocatedMemoryPerGPU[gpu.Index]
		freeMemory := gpu.TotalMemory - allocatedMemory

		if freeMemory >= modelMemoryRequirement {
			log.Trace().
				Str("runner_id", runnerID).
				Int("gpu_index", gpu.Index).
				Uint64("gpu_total_memory", gpu.TotalMemory).
				Uint64("gpu_allocated_memory", allocatedMemory).
				Uint64("gpu_free_memory", freeMemory).
				Uint64("model_memory_requirement", modelMemoryRequirement).
				Msg("Found GPU with sufficient free memory for model (allocated-based)")
			return true
		}
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Int("gpu_count", len(status.GPUs)).
		Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
		Msg("No individual GPU has sufficient free memory for model (allocated-based)")

	return false
}

// CanFitModelOnMultipleGPUsAllocated checks if a model can be distributed across multiple GPUs
// based on allocated memory (not real-time memory) to prevent race conditions
func (c *RunnerController) CanFitModelOnMultipleGPUsAllocated(runnerID string, modelMemoryRequirement uint64, minGPUs int, allocatedMemoryPerGPU map[int]uint64) ([]int, bool) {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status for multi-GPU check")
		return nil, false
	}

	if len(status.GPUs) < minGPUs {
		log.Debug().
			Str("runner_id", runnerID).
			Int("available_gpus", len(status.GPUs)).
			Int("min_required_gpus", minGPUs).
			Msg("Not enough GPUs available for multi-GPU model")
		return nil, false
	}

	// For multi-GPU models, VLLM distributes the model roughly equally across GPUs
	// Each GPU needs approximately modelMemoryRequirement / numGPUs
	memoryPerGPU := modelMemoryRequirement / uint64(minGPUs)

	// Removed 10% overhead buffer for simplicity - keeping system predictable
	// until we're confident the core allocation logic is solid

	var selectedGPUs []int
	for _, gpu := range status.GPUs {
		allocatedMemory := allocatedMemoryPerGPU[gpu.Index]
		freeMemory := gpu.TotalMemory - allocatedMemory

		if freeMemory >= memoryPerGPU {
			selectedGPUs = append(selectedGPUs, gpu.Index)
			if len(selectedGPUs) >= minGPUs {
				break
			}
		}
	}

	if len(selectedGPUs) < minGPUs {
		log.Debug().
			Str("runner_id", runnerID).
			Uint64("memory_per_gpu_required", memoryPerGPU).
			Int("found_suitable_gpus", len(selectedGPUs)).
			Int("min_required_gpus", minGPUs).
			Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
			Msg("Not enough GPUs with sufficient free memory for multi-GPU model (allocated-based)")
		return nil, false
	}

	log.Debug().
		Str("runner_id", runnerID).
		Ints("selected_gpu_indices", selectedGPUs).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Uint64("memory_per_gpu", memoryPerGPU).
		Interface("allocated_memory_per_gpu", allocatedMemoryPerGPU).
		Msg("Found suitable GPUs for multi-GPU model (allocated-based)")

	return selectedGPUs, true
}

func (c *RunnerController) GetSlots(runnerID string) ([]*types.RunnerSlot, error) {
	// Get the slots from the runner.
	slots, err := c.getSlots(runnerID)
	if err != nil {
		return nil, err
	}
	return slots.Slots, nil
}

func (r *NATSRunnerClient) SubmitChatCompletionRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	return r.controller.SubmitChatCompletionRequest(slot, req)
}

func (c *RunnerController) SubmitChatCompletionRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID)

	chatRequestBytes, err := json.Marshal(req.Request)
	if err != nil {
		return err
	}
	natsReq := types.RunnerNatsReplyRequest{
		RequestID:     req.RequestID,
		CreatedAt:     time.Now(),
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Request:       chatRequestBytes,
	}

	body, err := json.Marshal(natsReq)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/chat/completions", slot.ID),
		Body:   body,
	}, submitChatCompletionRequestTimeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting chat completion request: %s", resp.Body)
	}

	return nil
}

// SubmitEmbeddingRequest submits an embedding request to the runner
func (r *NATSRunnerClient) SubmitEmbeddingRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	return r.controller.SubmitEmbeddingRequest(slot, req)
}

func (c *RunnerController) SubmitEmbeddingRequest(slot *Slot, req *types.RunnerLLMInferenceRequest) error {
	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID)

	var embeddingRequestBytes []byte
	var err error
	var embeddingType string
	var inputType string
	var inputSize int

	// Check for flexible embedding request
	if req.FlexibleEmbeddingRequest != nil {
		embeddingType = "flexible"
		embeddingRequestBytes, err = json.Marshal(req.FlexibleEmbeddingRequest)

		if err == nil {
			// Determine input type and size for flexible requests
			if req.FlexibleEmbeddingRequest.Input != nil {
				switch v := req.FlexibleEmbeddingRequest.Input.(type) {
				case string:
					inputType = "string"
					inputSize = len(v)
				case []string:
					inputType = "[]string"
					inputSize = len(v)
				case [][]int:
					inputType = "[][]int"
					inputSize = len(v)
				}
			} else if len(req.FlexibleEmbeddingRequest.Messages) > 0 {
				inputType = "messages"
				inputSize = len(req.FlexibleEmbeddingRequest.Messages)
			}
		}
	} else {
		embeddingType = "standard"
		embeddingRequestBytes, err = json.Marshal(req.EmbeddingRequest)

		if err == nil {
			// Determine input type and size for standard requests
			switch v := req.EmbeddingRequest.Input.(type) {
			case string:
				inputType = "string"
				inputSize = len(v)
			case []string:
				inputType = "[]string"
				inputSize = len(v)
			case [][]int:
				inputType = "[][]int"
				inputSize = len(v)
			}
		}
	}

	if err != nil {
		log.Error().
			Str("component", "scheduler").
			Str("operation", "embedding").
			Str("request_id", req.RequestID).
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Str("embedding_type", embeddingType).
			Err(err).
			Msg("❌ Failed to marshal embedding request to JSON")
		return err
	}

	// Create a pretty-printed version for logging
	var prettyRequest bytes.Buffer
	if err := json.Indent(&prettyRequest, embeddingRequestBytes, "", "  "); err != nil {
		// If pretty printing fails, we'll just use the compact version
		prettyRequest.Write(embeddingRequestBytes)
	}

	// Log the embedding request details with embedding type
	requestModel := ""
	encodingFormat := ""
	dimensions := 0

	if req.FlexibleEmbeddingRequest != nil {
		requestModel = req.FlexibleEmbeddingRequest.Model
		encodingFormat = req.FlexibleEmbeddingRequest.EncodingFormat
		dimensions = req.FlexibleEmbeddingRequest.Dimensions
	} else {
		requestModel = string(req.EmbeddingRequest.Model)
		encodingFormat = string(req.EmbeddingRequest.EncodingFormat)
		dimensions = req.EmbeddingRequest.Dimensions
	}

	// Enhanced logging with detailed request info
	log.Info().
		Str("component", "scheduler").
		Str("operation", "embedding").
		Str("embedding_type", embeddingType).
		Str("request_id", req.RequestID).
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", requestModel).
		Str("encoding_format", encodingFormat).
		Str("input_type", inputType).
		Int("input_size", inputSize).
		Int("dimensions", dimensions).
		Str("request_json", prettyRequest.String()).
		Str("endpoint", fmt.Sprintf("/api/v1/slots/%s/v1/embedding", slot.ID)).
		Msg("🔢 Embedding request submitted to runner")

	natsReq := types.RunnerNatsReplyRequest{
		RequestID:     req.RequestID,
		CreatedAt:     time.Now(),
		OwnerID:       req.OwnerID,
		SessionID:     req.SessionID,
		InteractionID: req.InteractionID,
		Request:       embeddingRequestBytes,
	}

	body, err := json.Marshal(natsReq)
	if err != nil {
		log.Error().
			Str("component", "scheduler").
			Str("operation", "embedding").
			Str("request_id", req.RequestID).
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Err(err).
			Msg("❌ Failed to marshal embedding NATS request")
		return err
	}

	startTime := time.Now()
	endpoint := fmt.Sprintf("/api/v1/slots/%s/v1/embedding", slot.ID)
	log.Debug().
		Str("component", "scheduler").
		Str("operation", "embedding").
		Str("request_id", req.RequestID).
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("endpoint", endpoint).
		Str("method", "POST").
		Msg("📤 Sending embedding request to runner")

	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    endpoint,
		Body:   body,
	}, submitChatCompletionRequestTimeout) // Using the same timeout as chat completion

	durationMs := time.Since(startTime).Milliseconds()

	if err != nil {
		log.Error().
			Str("component", "scheduler").
			Str("operation", "embedding").
			Str("request_id", req.RequestID).
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Str("endpoint", endpoint).
			Int64("duration_ms", durationMs).
			Err(err).
			Msg("❌ Failed to submit embedding request to runner")
		return err
	}
	if resp.StatusCode != http.StatusOK {
		// Log detailed response information on error
		errorDetails := struct {
			StatusCode    int    `json:"status_code"`
			ResponseBody  string `json:"response_body"`
			RequestID     string `json:"request_id"`
			RunnerID      string `json:"runner_id"`
			SlotID        string `json:"slot_id"`
			Endpoint      string `json:"endpoint"`
			DurationMs    int64  `json:"duration_ms"`
			RequestMethod string `json:"request_method"`
		}{
			StatusCode:    resp.StatusCode,
			ResponseBody:  string(resp.Body),
			RequestID:     req.RequestID,
			RunnerID:      slot.RunnerID,
			SlotID:        slot.ID.String(),
			Endpoint:      endpoint,
			DurationMs:    durationMs,
			RequestMethod: "POST",
		}

		errorDetailsBytes, _ := json.Marshal(errorDetails)
		errMsg := fmt.Sprintf("error submitting embedding request: %s", resp.Body)
		log.Error().
			Str("component", "scheduler").
			Str("operation", "embedding").
			Str("request_id", req.RequestID).
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Int64("duration_ms", durationMs).
			Int("status_code", resp.StatusCode).
			Str("error_message", errMsg).
			Str("error_details", string(errorDetailsBytes)).
			Str("response_body", string(resp.Body)).
			Str("endpoint", endpoint).
			Msg("❌ Embedding request failed with non-OK status code")
		return fmt.Errorf("%s", errMsg)
	}

	// Log successful response
	log.Info().
		Str("component", "scheduler").
		Str("operation", "embedding").
		Str("request_id", req.RequestID).
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("endpoint", endpoint).
		Int64("duration_ms", durationMs).
		Int("status_code", resp.StatusCode).
		Msg("✅ Embedding request successfully submitted to runner")

	return nil
}

func (r *NATSRunnerClient) SubmitImageGenerationRequest(slot *Slot, session *types.Session) error {
	return r.controller.SubmitImageGenerationRequest(slot, session)
}

func (c *RunnerController) SubmitImageGenerationRequest(slot *Slot, session *types.Session) error {
	lastInteraction := session.Interactions[len(session.Interactions)-1]

	// userInteractions := data.FilterUserInteractions(session.Interactions)
	// if len(userInteractions) == 0 {
	// 	return fmt.Errorf("no user interaction found")
	// }

	// Note that there's no system prompt in the openai api. So I'm just going to merge the previous
	// user interactions into a single prompt. Fingers crossed there's no limits.
	// Merge the user interactions into a single prompt
	prompt := strings.Builder{}
	for _, interaction := range session.Interactions[:len(session.Interactions)-1] {
		prompt.WriteString(interaction.PromptMessage)
		prompt.WriteString("\n")
	}
	prompt.WriteString(lastInteraction.PromptMessage)

	// Convert the session to a valid image generation request
	imageRequest := openai.ImageRequest{
		Prompt: prompt.String(),
		Model:  session.ModelName,
		N:      1,
		User:   session.Owner,
	}
	requestBytes, err := json.Marshal(imageRequest)
	if err != nil {
		return err
	}
	req := &types.RunnerNatsReplyRequest{
		RequestID:     lastInteraction.ID, // Use the last interaction ID as the request ID for sessions, it's important that this kept in sync with the receiver code
		CreatedAt:     time.Now(),
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: lastInteraction.ID,
		Request:       requestBytes,
	}

	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(req.OwnerID, req.RequestID)

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/helix/images/generations", slot.ID),
		Body:   body,
	}, 30*time.Second)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting image generation request: %s", resp.Body)
	}

	return nil
}

// calculateVLLMMemoryUtilizationRatio calculates the GPU memory utilization ratio
// for VLLM based on the model's memory requirements and the runner's per-GPU memory.
// This ensures VLLM uses an appropriate amount of GPU memory without causing OOM errors.
//
// IMPORTANT: For single-GPU VLLM models, this now uses per-GPU memory instead of
// total aggregated memory across all GPUs to fix multi-GPU memory calculation issues.
//
// Note: Previously contained complex workarounds for VLLM memory profiling bugs,
// but these were fixed in VLLM PR #18974, allowing for this simplified calculation.
func (c *RunnerController) calculateVLLMMemoryUtilizationRatio(runnerID string, modelMemoryRequirement uint64) float64 {
	// Get the runner's per-GPU memory (not total memory across all GPUs)
	// This is critical for multi-GPU systems where VLLM's --gpu-memory-utilization
	// parameter is interpreted as a per-GPU ratio, not a total memory ratio
	perGPUMemory := c.PerGPUMemory(runnerID)
	if perGPUMemory == 0 {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("No GPU memory information available for runner, using default ratio")
		return 0.8 // Default fallback ratio
	}

	// Calculate ratio based on per-GPU memory for single-GPU VLLM models
	finalRatio := float64(modelMemoryRequirement) / float64(perGPUMemory)

	// Ensure the ratio is within reasonable bounds (1% to 99%)
	// Lower bound of 1% allows for small models while avoiding potential VLLM issues
	// Upper bound of 99% prevents potential OOM from other GPU processes
	if finalRatio < 0.01 {
		finalRatio = 0.01
	} else if finalRatio > 0.99 {
		finalRatio = 0.99
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("model_memory_bytes", modelMemoryRequirement).
		Uint64("per_gpu_memory_bytes", perGPUMemory).
		Float64("final_per_gpu_ratio", finalRatio).
		Msg("Calculated VLLM memory utilization ratio (per-GPU)")

	return finalRatio
}

// substituteVLLMArgsPlaceholders replaces template placeholders in VLLM args with actual values
func (c *RunnerController) substituteVLLMArgsPlaceholders(args []string, runnerID string, modelMemoryRequirement uint64) []string {
	// Calculate the dynamic memory utilization ratio
	memoryUtilizationRatio := c.calculateVLLMMemoryUtilizationRatio(runnerID, modelMemoryRequirement)
	ratioStr := fmt.Sprintf("%.2f", memoryUtilizationRatio)

	// Create a new slice to avoid modifying the original
	var substitutedArgs []string
	if len(args) > 0 {
		substitutedArgs = make([]string, len(args))
		copy(substitutedArgs, args)
	}

	// Check if --gpu-memory-utilization is already present in the args
	hasGPUMemoryUtilization := false
	for i, arg := range substitutedArgs {
		if arg == "{{.DynamicMemoryUtilizationRatio}}" {
			substitutedArgs[i] = ratioStr
			log.Debug().
				Str("runner_id", runnerID).
				Str("original_value", arg).
				Str("substituted_value", ratioStr).
				Msg("Substituted VLLM memory utilization placeholder")
		} else if arg == "--gpu-memory-utilization" {
			hasGPUMemoryUtilization = true
		}
	}

	// If --gpu-memory-utilization is not present, automatically add it
	if !hasGPUMemoryUtilization {
		substitutedArgs = append(substitutedArgs, "--gpu-memory-utilization", ratioStr)
		log.Debug().
			Str("runner_id", runnerID).
			Str("memory_utilization_ratio", ratioStr).
			Msg("Automatically added --gpu-memory-utilization argument")
	}

	return substitutedArgs
}

func (c *RunnerController) CreateSlot(slot *Slot) error {
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", slot.InitialWork().ModelName().String()).
		Str("runtime", string(slot.InitialWork().Runtime())).
		Msg("creating slot")

	// Get the context length from the model
	modelName := slot.InitialWork().ModelName().String()

	// Get the model's context length
	var contextLength int64
	var runtimeArgs map[string]interface{}

	modelObj, err := c.store.GetModel(context.Background(), modelName)
	if err == nil {
		contextLength = modelObj.ContextLength
		if contextLength > 0 {
			log.Debug().Str("model", modelName).Int64("context_length", contextLength).Msg("Using context length from model")
		}

		// If this is a VLLM runtime, get the runtime args
		if slot.InitialWork().Runtime() == types.RuntimeVLLM {
			// First check if the model has stored RuntimeArgs
			if len(modelObj.RuntimeArgs) > 0 {
				log.Debug().
					Str("model", modelName).
					Interface("stored_runtime_args", modelObj.RuntimeArgs).
					Msg("Using stored RuntimeArgs from database")

				// Use the stored runtime args directly
				runtimeArgs = modelObj.RuntimeArgs

				// If there are args in the runtime args, substitute placeholders
				if args, ok := modelObj.RuntimeArgs["args"].([]interface{}); ok {
					// Convert []interface{} to []string for placeholder substitution
					argStrings := make([]string, len(args))
					for i, v := range args {
						argStrings[i] = fmt.Sprintf("%v", v)
					}

					// Substitute placeholders with actual values
					substitutedArgs := c.substituteVLLMArgsPlaceholders(argStrings, slot.RunnerID, modelObj.Memory)

					// Update the runtime args with substituted values
					runtimeArgs = map[string]interface{}{
						"args": substitutedArgs,
					}

					log.Info().
						Str("model", modelName).
						Strs("original_args", argStrings).
						Strs("substituted_args", substitutedArgs).
						Interface("runtime_args", runtimeArgs).
						Msg("VLLM Args: Substituted placeholders in stored RuntimeArgs")
				}
			} else {
				// Fall back to hardcoded args for backward compatibility
				modelArgs, argsErr := model.GetVLLMArgsForModel(modelName)
				if argsErr == nil && len(modelArgs) > 0 {
					log.Debug().
						Str("model", modelName).
						Strs("vllm_args", modelArgs).
						Msg("Falling back to hardcoded vLLM args (no stored RuntimeArgs)")

					// Substitute placeholders with actual values
					substitutedArgs := c.substituteVLLMArgsPlaceholders(modelArgs, slot.RunnerID, modelObj.Memory)

					// Add the substituted args to the runtime args
					runtimeArgs = map[string]interface{}{
						"args": substitutedArgs,
					}

					log.Info().
						Str("model", modelName).
						Strs("original_args", modelArgs).
						Strs("substituted_args", substitutedArgs).
						Interface("runtime_args", runtimeArgs).
						Msg("VLLM Args: Created runtime_args map with substituted values from hardcoded fallback")
				}
			}

			// Add VLLM concurrency configuration
			var numParallel int
			if modelObj.Concurrency > 0 {
				numParallel = modelObj.Concurrency
				log.Debug().
					Str("model", modelName).
					Int("concurrency", numParallel).
					Msg("Using per-model concurrency setting for VLLM")
			} else {
				// Use VLLM's natural default
				numParallel = types.DefaultVLLMParallelSequences
				log.Debug().
					Str("model", modelName).
					Int("concurrency", numParallel).
					Msg("Using VLLM natural default concurrency")
			}

			// Ensure runtimeArgs exists
			if runtimeArgs == nil {
				runtimeArgs = make(map[string]interface{})
			}

			// Get existing args or create new ones
			var existingArgs []string
			if args, ok := runtimeArgs["args"].([]string); ok {
				existingArgs = args
			} else if argsIface, ok := runtimeArgs["args"].([]interface{}); ok {
				// Convert []interface{} to []string
				existingArgs = make([]string, len(argsIface))
				for i, v := range argsIface {
					existingArgs[i] = fmt.Sprintf("%v", v)
				}
			}

			// Check if --max-num-seqs is already present
			hasMaxNumSeqs := false
			for i, arg := range existingArgs {
				if arg == "--max-num-seqs" {
					// Replace the value if it exists
					if i+1 < len(existingArgs) {
						existingArgs[i+1] = fmt.Sprintf("%d", numParallel)
						hasMaxNumSeqs = true
						break
					}
				}
			}

			// Add --max-num-seqs if not present
			if !hasMaxNumSeqs {
				existingArgs = append(existingArgs, "--max-num-seqs", fmt.Sprintf("%d", numParallel))
			}

			// Update runtime args
			runtimeArgs["args"] = existingArgs

			log.Info().
				Str("model", modelName).
				Int("max_num_seqs", numParallel).
				Strs("final_args", existingArgs).
				Msg("VLLM: Added concurrency configuration to runtime args")
		}
		// If this is an Ollama runtime, add concurrency configuration
		if slot.InitialWork().Runtime() == types.RuntimeOllama {
			// Get concurrency setting from model or use scheduler default
			var numParallel int
			if modelObj.Concurrency > 0 {
				numParallel = modelObj.Concurrency
				log.Debug().
					Str("model", modelName).
					Int("concurrency", numParallel).
					Msg("Using per-model concurrency setting for Ollama")
			} else {
				// Use reasonable default for Ollama
				numParallel = memory.DefaultOllamaParallelSequences
				log.Debug().
					Str("model", modelName).
					Int("concurrency", numParallel).
					Msg("Using Ollama default concurrency")
			}

			// Add concurrency to runtime args
			if runtimeArgs == nil {
				runtimeArgs = make(map[string]interface{})
			}
			runtimeArgs["num_parallel"] = numParallel

			log.Info().
				Str("model", modelName).
				Int("num_parallel", numParallel).
				Interface("runtime_args", runtimeArgs).
				Msg("Ollama: Added concurrency configuration to runtime args")
		}
	} else {
		log.Warn().Str("model", modelName).Err(err).Msg("Could not get model, using default context length")
	}

	// Extract GPU allocation from slot
	var gpuIndex *int
	var gpuIndices []int
	var tensorParallelSize int

	if slot.GPUAllocation != nil {
		gpuIndex = slot.GPUAllocation.SingleGPU
		gpuIndices = slot.GPUAllocation.MultiGPUs
		tensorParallelSize = slot.GPUAllocation.TensorParallelSize

		log.Debug().
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Interface("single_gpu", gpuIndex).
			Ints("multi_gpus", gpuIndices).
			Int("tensor_parallel_size", tensorParallelSize).
			Msg("Using GPU allocation from scheduler")
	}

	// NEW ARCHITECTURE: Require configured models - no fallbacks
	if slot.InitialWork().model == nil || !slot.InitialWork().model.IsAllocationConfigured() {
		return fmt.Errorf("slot %s has unconfigured model %s - all models must use NewModelForGPUAllocation",
			slot.ID.String(), slot.InitialWork().ModelName().String())
	}

	// Use configured model memory (authoritative, never fails)
	modelMemoryRequirement := slot.InitialWork().model.GetMemoryForAllocation()
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", slot.InitialWork().ModelName().String()).
		Uint64("configured_memory_bytes", modelMemoryRequirement).
		Uint64("configured_memory_gb", modelMemoryRequirement/(1024*1024*1024)).
		Int("configured_gpu_count", slot.InitialWork().model.GetGPUCount()).
		Msg("Using configured model memory (no fallbacks)")

	// CRITICAL SAFETY CHECK: Never allow slots with zero memory requirement
	// This prevents overscheduling bugs where the scheduler thinks no memory is allocated
	if modelMemoryRequirement == 0 {
		err := fmt.Errorf("CRITICAL: Cannot create slot with zero memory requirement for model %s on runner %s - this would cause overscheduling",
			slot.InitialWork().ModelName().String(), slot.RunnerID)
		log.Error().
			Str("runner_id", slot.RunnerID).
			Str("slot_id", slot.ID.String()).
			Str("model", slot.InitialWork().ModelName().String()).
			Err(err).
			Msg("CRITICAL: Refusing to create slot with zero memory requirement - fix model memory configuration")
		return err
	}

	// For Ollama models, collect memory estimation metadata for tooltip display
	var memoryEstimationMeta map[string]any
	if slot.InitialWork().Runtime() == types.RuntimeOllama {
		// Get the model to extract context length and other parameters
		models, err := c.store.ListModels(context.Background(), &store.ListModelsQuery{})
		if err == nil {
			for _, model := range models {
				if model.ID == slot.InitialWork().ModelName().String() {
					// Get detailed memory estimation breakdown for debugging
					var detailedBreakdown map[string]any
					if c.getDetailedMemoryResultFn != nil {
						if result := c.getDetailedMemoryResultFn(model.ID); result != nil {
							var estimate *memory.MemoryEstimate
							if tensorParallelSize > 1 && result.TensorParallel != nil {
								estimate = result.TensorParallel
							} else if result.SingleGPU != nil {
								estimate = result.SingleGPU
							}

							if estimate != nil {
								// Convert bytes to human-readable format for debugging
								formatMemory := func(bytes uint64) map[string]any {
									return map[string]any{
										"bytes": bytes,
										"mb":    bytes / (1024 * 1024),
										"gb":    float64(bytes) / (1024 * 1024 * 1024),
									}
								}

								detailedBreakdown = map[string]any{
									"total_memory":    formatMemory(estimate.TotalSize),
									"weights_memory":  formatMemory(estimate.Weights),
									"kv_cache_memory": formatMemory(estimate.KVCache),
									"graph_memory":    formatMemory(estimate.Graph),
									"vram_used":       formatMemory(estimate.VRAMSize),
									"layers_on_gpu":   estimate.Layers,
									"fully_loaded":    estimate.FullyLoaded,
									"architecture":    estimate.Architecture,
								}
							}
						}
					}

					memoryEstimationMeta = map[string]any{
						"kv_cache_type":  memory.DefaultKVCacheType, // KV cache type used in estimation
						"context_length": int(model.ContextLength),
						"batch_size":     512,
						"parallel_sequences": func() int {
							if runtimeArgs != nil {
								if numParallelVal, ok := runtimeArgs["num_parallel"].(int); ok {
									return numParallelVal
								}
							}
							return 1
						}(),
						"estimation_source": "gguf_analysis",
						"gpu_allocation_type": func() string {
							if tensorParallelSize > 1 {
								return "tensor_parallel"
							} else if gpuIndex != nil {
								return "single_gpu"
							} else {
								return "multi_gpu"
							}
						}(),
					}

					// Add detailed breakdown if available
					if detailedBreakdown != nil {
						memoryEstimationMeta["detailed_breakdown"] = detailedBreakdown
					}
					break
				}
			}
		}
	}

	req := &types.CreateRunnerSlotRequest{
		ID: slot.ID,
		Attributes: types.CreateRunnerSlotAttributes{
			Runtime:                slot.InitialWork().Runtime(),
			Model:                  slot.InitialWork().ModelName().String(),
			ModelMemoryRequirement: modelMemoryRequirement,
			ContextLength:          contextLength,
			RuntimeArgs:            runtimeArgs,
			GPUIndex:               gpuIndex,
			GPUIndices:             gpuIndices,
			TensorParallelSize:     tensorParallelSize,
			MemoryEstimationMeta:   memoryEstimationMeta,
		},
	}

	// Log the full request being sent to the runner
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", slot.InitialWork().ModelName().String()).
		Interface("runtime_args", runtimeArgs).
		Msg("Sending CreateRunnerSlotRequest to runner with RuntimeArgs")

	// Log the request for debugging
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Interface("request", req).
		Msg("Sending CreateRunnerSlotRequest to runner")

	return c.runnerClient.CreateSlot(slot.RunnerID, slot.ID, req)
}

func (c *RunnerController) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	log.Debug().
		Str("runner_id", runnerID).
		Str("slot_id", slotID.String()).
		Msg("deleting slot")
	return c.runnerClient.DeleteSlot(runnerID, slotID)
}

func (c *RunnerController) GetStatus(runnerID string) (*types.RunnerStatus, error) {
	cache := c.statusCache.GetOrCreateCache(c.ctx, runnerID, func() (types.RunnerStatus, error) {
		return c.fetchStatus(runnerID)
	}, CacheConfig{
		updateInterval: cacheUpdateInterval,
	})

	status, err := cache.Get()
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func (c *RunnerController) GetHealthz(runnerID string) error {
	return c.healthChecker.GetHealthz(runnerID)
}

func (c *RunnerController) SetModels(runnerID string) error {
	return c.healthChecker.SetModels(runnerID)
}

func (c *RunnerController) fetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	return c.runnerClient.FetchSlot(runnerID, slotID)
}

func (c *RunnerController) fetchStatus(runnerID string) (types.RunnerStatus, error) {
	return c.runnerClient.FetchStatus(runnerID)
}

func (c *RunnerController) getSlots(runnerID string) (*types.ListRunnerSlotsResponse, error) {
	cache := c.slotsCache.GetOrCreateCache(c.ctx, runnerID, func() (types.ListRunnerSlotsResponse, error) {
		return c.fetchSlots(runnerID)
	}, CacheConfig{
		updateInterval: cacheUpdateInterval,
	})

	slots, err := cache.Get()
	if err != nil {
		return nil, err
	}
	return &slots, nil
}

func (c *RunnerController) fetchSlots(runnerID string) (types.ListRunnerSlotsResponse, error) {
	return c.runnerClient.FetchSlots(runnerID)
}
