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
	defaultRequestTimeout              = 5 * time.Second // Reduced from 300s - we need fast failure for slot reconciliation
	cacheUpdateInterval                = 1 * time.Second // Reduced from 5s for more responsive dashboard updates
)

type RunnerController struct {
	runners             []string
	mu                  *sync.RWMutex
	ps                  pubsub.PubSub
	ctx                 context.Context
	fs                  filestore.FileStore
	slotsCache          *LockingRunnerMap[types.ListRunnerSlotsResponse]
	statusCache         *LockingRunnerMap[types.RunnerStatus]
	store               store.Store
	onRunnerConnectedFn func(runnerID string) // Callback for when a new runner connects
}

type RunnerControllerConfig struct {
	PubSub              pubsub.PubSub
	FS                  filestore.FileStore
	Store               store.Store
	OnRunnerConnectedFn func(runnerID string) // Optional callback for when a new runner connects
}

func NewRunnerController(ctx context.Context, cfg *RunnerControllerConfig) (*RunnerController, error) {
	controller := &RunnerController{
		ctx:                 ctx,
		ps:                  cfg.PubSub,
		fs:                  cfg.FS,
		store:               cfg.Store,
		runners:             []string{},
		mu:                  &sync.RWMutex{},
		slotsCache:          NewLockingRunnerMap[types.ListRunnerSlotsResponse](),
		statusCache:         NewLockingRunnerMap[types.RunnerStatus](),
		onRunnerConnectedFn: cfg.OnRunnerConnectedFn,
	}

	sub, err := cfg.PubSub.SubscribeWithCtx(controller.ctx, pubsub.GetRunnerConnectedQueue("*"), func(_ context.Context, msg *nats.Msg) error {
		log.Debug().Str("subject", msg.Subject).Str("data", string(msg.Data)).Msg("runner ping")
		runnerID, err := pubsub.ParseRunnerID(msg.Subject)
		if err != nil {
			log.Error().Err(err).Str("subject", msg.Subject).Msg("error parsing runner ID")
			return err
		}
		controller.OnConnectedHandler(runnerID)
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

// SetOnRunnerConnectedCallback sets the callback function for when a new runner connects
func (c *RunnerController) SetOnRunnerConnectedCallback(fn func(runnerID string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onRunnerConnectedFn = fn
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
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("error marshalling request: %w", err)
	}

	// Check if runner is still connected before making NATS call
	if !slices.Contains(c.RunnerIDs(), runnerID) {
		return nil, fmt.Errorf("runner %s is not connected", runnerID)
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
					// Check if runner is still connected
					if !slices.Contains(c.RunnerIDs(), runnerID) {
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

	reqBody, err := json.Marshal(configReq)
	if err != nil {
		return fmt.Errorf("failed to marshal system config request: %w", err)
	}

	// Send system config to runner
	req := &types.Request{
		Method: "PUT",
		URL:    "/api/v1/system/config",
		Body:   reqBody,
	}

	resp, err := c.Send(ctx, runnerID, nil, req, 30*time.Second)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Msg("failed to sync system settings to runner")
		return fmt.Errorf("failed to send system config to runner %s: %w", runnerID, err)
	}

	if resp.StatusCode != 200 {
		log.Error().
			Int("status_code", resp.StatusCode).
			Str("runner_id", runnerID).
			Str("response_body", string(resp.Body)).
			Msg("runner rejected system settings update")
		return fmt.Errorf("runner %s rejected system settings with status %d", runnerID, resp.StatusCode)
	}

	log.Info().
		Str("runner_id", runnerID).
		Bool("hf_token_sent", hfToken != nil && *hfToken != "").
		Msg("successfully synced system settings to runner")

	return nil
}

// SyncSystemSettingsToAllRunners pushes system settings to all connected runners
func (c *RunnerController) SyncSystemSettingsToAllRunners(ctx context.Context) {
	c.mu.RLock()
	runners := make([]string, len(c.runners))
	copy(runners, c.runners)
	c.mu.RUnlock()

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
	c.mu.Lock()
	defer c.mu.Unlock()

	isNewRunner := !slices.Contains(c.runners, id)

	// Add the runner to the cluster if it is not already in the cluster.
	if isNewRunner {
		c.runners = append(c.runners, id)
		log.Info().Str("runner_id", id).Msg("new runner connected")
	} else {
		log.Info().Str("runner_id", id).Msg("existing runner reconnected")
	}

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
	if c.onRunnerConnectedFn != nil {
		go func() {
			// Run in a goroutine to avoid blocking runner registration
			log.Info().
				Str("runner_id", id).
				Bool("is_new_runner", isNewRunner).
				Msg("triggering prewarming for connected runner")
			c.onRunnerConnectedFn(id)
		}()
	}
}

func (c *RunnerController) deleteRunner(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var newRunners []string
	for _, runner := range c.runners {
		if runner != id {
			newRunners = append(newRunners, runner)
		}
	}
	c.runners = newRunners
}

func (c *RunnerController) RunnerIDs() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

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

// CanFitModelOnAnyGPU checks if any individual GPU has enough free memory for the model
// This is critical for single-GPU VLLM models to avoid memory fragmentation issues
func (c *RunnerController) CanFitModelOnAnyGPU(runnerID string, modelMemoryRequirement uint64) bool {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status for GPU check")
		return false
	}

	// Check each individual GPU
	for _, gpu := range status.GPUs {
		if gpu.FreeMemory >= modelMemoryRequirement {
			log.Debug().
				Str("runner_id", runnerID).
				Int("gpu_index", gpu.Index).
				Uint64("gpu_free_memory", gpu.FreeMemory).
				Uint64("model_memory_requirement", modelMemoryRequirement).
				Msg("Found GPU with sufficient memory for model")
			return true
		}
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Int("gpu_count", len(status.GPUs)).
		Msg("No individual GPU has sufficient memory for model")

	return false
}

// GetGPUMemoryInfo returns detailed per-GPU memory information for debugging
func (c *RunnerController) GetGPUMemoryInfo(runnerID string) ([]*types.GPUStatus, error) {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		return nil, err
	}
	return status.GPUs, nil
}

// CanFitModelOnMultipleGPUs checks if a model can be distributed across multiple GPUs
// Returns the optimal GPU indices and whether allocation is possible
func (c *RunnerController) CanFitModelOnMultipleGPUs(runnerID string, modelMemoryRequirement uint64, minGPUs int) ([]int, bool) {
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

	// Add 10% overhead for tensor parallelism communication and other overhead
	memoryPerGPU = uint64(float64(memoryPerGPU) * 1.1)

	var selectedGPUs []int
	for _, gpu := range status.GPUs {
		if gpu.FreeMemory >= memoryPerGPU {
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
			Msg("Not enough GPUs with sufficient memory for multi-GPU model")
		return nil, false
	}

	log.Debug().
		Str("runner_id", runnerID).
		Ints("selected_gpu_indices", selectedGPUs).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Uint64("memory_per_gpu", memoryPerGPU).
		Msg("Found suitable GPUs for multi-GPU model")

	return selectedGPUs, true
}

// GetOptimalGPUAllocation determines the best GPU allocation strategy for a model
// Returns single GPU index for single-GPU models, or multiple GPU indices for multi-GPU models
func (c *RunnerController) GetOptimalGPUAllocation(runnerID string, modelMemoryRequirement uint64) (singleGPU *int, multiGPUs []int, tensorParallelSize int) {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status for GPU allocation")
		return nil, nil, 0
	}

	// First, try to fit the model on a single GPU
	if c.CanFitModelOnAnyGPU(runnerID, modelMemoryRequirement) {
		// Find the best single GPU
		var bestGPU *int
		var maxFreeMemory uint64

		for _, gpu := range status.GPUs {
			if gpu.FreeMemory >= modelMemoryRequirement && gpu.FreeMemory > maxFreeMemory {
				maxFreeMemory = gpu.FreeMemory
				idx := gpu.Index
				bestGPU = &idx
			}
		}

		if bestGPU != nil {
			log.Debug().
				Str("runner_id", runnerID).
				Int("selected_gpu", *bestGPU).
				Uint64("model_memory_requirement", modelMemoryRequirement).
				Msg("Selected single GPU for model")
			return bestGPU, nil, 1
		}
	}

	// If single GPU doesn't work, try multi-GPU allocation
	// Start with 2 GPUs and scale up as needed
	for numGPUs := 2; numGPUs <= len(status.GPUs); numGPUs++ {
		if gpuIndices, canFit := c.CanFitModelOnMultipleGPUs(runnerID, modelMemoryRequirement, numGPUs); canFit {
			log.Debug().
				Str("runner_id", runnerID).
				Ints("selected_gpus", gpuIndices).
				Int("tensor_parallel_size", numGPUs).
				Uint64("model_memory_requirement", modelMemoryRequirement).
				Msg("Selected multi-GPU allocation for model")
			return nil, gpuIndices, numGPUs
		}
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("model_memory_requirement", modelMemoryRequirement).
		Int("available_gpus", len(status.GPUs)).
		Msg("Could not find suitable GPU allocation for model")

	return nil, nil, 0
}

func (c *RunnerController) Version(runnerID string) string {
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Error().Err(err).Msg("error getting runner status")
		return ""
	}
	return status.Version
}

func (c *RunnerController) GetSlots(runnerID string) ([]*types.RunnerSlot, error) {
	// Get the slots from the runner.
	slots, err := c.getSlots(runnerID)
	if err != nil {
		return nil, err
	}
	return slots.Slots, nil
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
	if len(args) == 0 {
		return args
	}

	// Calculate the dynamic memory utilization ratio
	memoryUtilizationRatio := c.calculateVLLMMemoryUtilizationRatio(runnerID, modelMemoryRequirement)
	ratioStr := fmt.Sprintf("%.2f", memoryUtilizationRatio)

	// Create a new slice to avoid modifying the original
	substitutedArgs := make([]string, len(args))
	copy(substitutedArgs, args)

	// Replace the placeholder with the calculated value
	for i, arg := range substitutedArgs {
		if arg == "{{.DynamicMemoryUtilizationRatio}}" {
			substitutedArgs[i] = ratioStr
			log.Debug().
				Str("runner_id", runnerID).
				Str("original_value", arg).
				Str("substituted_value", ratioStr).
				Msg("Substituted VLLM memory utilization placeholder")
		}
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

					log.Debug().
						Str("model", modelName).
						Strs("original_args", argStrings).
						Strs("substituted_args", substitutedArgs).
						Interface("runtime_args", runtimeArgs).
						Msg("Substituted placeholders in stored RuntimeArgs")
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

					log.Debug().
						Str("model", modelName).
						Strs("original_args", modelArgs).
						Strs("substituted_args", substitutedArgs).
						Interface("runtime_args", runtimeArgs).
						Msg("Created runtime_args map with substituted values from hardcoded fallback")
				}
			}
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

	req := &types.CreateRunnerSlotRequest{
		ID: slot.ID,
		Attributes: types.CreateRunnerSlotAttributes{
			Runtime:                slot.InitialWork().Runtime(),
			Model:                  slot.InitialWork().ModelName().String(),
			ModelMemoryRequirement: slot.Memory(),
			ContextLength:          contextLength,
			RuntimeArgs:            runtimeArgs,
			GPUIndex:               gpuIndex,
			GPUIndices:             gpuIndices,
			TensorParallelSize:     tensorParallelSize,
		},
	}

	// Log the full request being sent to the runner
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("slot_id", slot.ID.String()).
		Str("model", slot.InitialWork().ModelName().String()).
		Interface("runtime_args", runtimeArgs).
		Msg("Sending CreateRunnerSlotRequest to runner with RuntimeArgs")

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// Log the serialized JSON for debugging
	log.Debug().
		Str("runner_id", slot.RunnerID).
		Str("json_body", string(body)).
		Msg("JSON body being sent to runner")

		resp, err := c.Send(c.ctx, slot.RunnerID, nil, &types.Request{
		Method: "POST",
		URL:    "/api/v1/slots",
		Body:   body,
	}, 30*time.Minute) // Increased from 1 minute to allow enough time for model downloads
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("error creating slot: %s", resp.Body)
	}
	return nil
}

func (c *RunnerController) DeleteSlot(runnerID string, slotID uuid.UUID) error {
	log.Debug().
		Str("runner_id", runnerID).
		Str("slot_id", slotID.String()).
		Msg("deleting slot")
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
		Method: "DELETE",
		URL:    fmt.Sprintf("/api/v1/slots/%s", slotID.String()),
	}, defaultRequestTimeout)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("error deleting slot: %s", resp.Body)
	}
	return nil
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
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
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

func (c *RunnerController) SetModels(runnerID string) error {
	enabled := true
	// Fetch all enabled models
	models, err := c.store.ListModels(context.Background(), &store.ListModelsQuery{
		Enabled: &enabled,
	})
	if err != nil {
		return fmt.Errorf("error listing models: %w", err)
	}

	bts, err := json.Marshal(models)
	if err != nil {
		return fmt.Errorf("error marshalling models: %w", err)
	}

	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
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

func (c *RunnerController) fetchSlot(runnerID string, slotID uuid.UUID) (types.RunnerSlot, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
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

func (c *RunnerController) fetchStatus(runnerID string) (types.RunnerStatus, error) {
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
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
	resp, err := c.Send(c.ctx, runnerID, nil, &types.Request{
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
