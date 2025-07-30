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
	"github.com/helixml/helix/api/pkg/data"
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
	defaultRequestTimeout              = 300 * time.Second
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

	// Publish the task to the "tasks" subject
	start := time.Now()
	response, err := c.ps.Request(ctx, pubsub.GetRunnerQueue(runnerID), headers, data, timeout)
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

func (c *RunnerController) OnConnectedHandler(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Add the runner to the cluster if it is not already in the cluster.
	if !slices.Contains(c.runners, id) {
		c.runners = append(c.runners, id)

		// Reconcile runner models immediately
		err := c.SetModels(id)
		if err != nil {
			log.Error().Err(err).Str("runner_id", id).Msg("error setting models on runner")
		}

		// Call the prewarming callback if it's set
		if c.onRunnerConnectedFn != nil {
			go func() {
				// Run in a goroutine to avoid blocking runner registration
				c.onRunnerConnectedFn(id)
			}()
		}
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
	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("no last interaction found: %w", err)
	}

	userInteractions := data.FilterUserInteractions(session.Interactions)
	if len(userInteractions) == 0 {
		return fmt.Errorf("no user interaction found")
	}

	// Note that there's no system prompt in the openai api. So I'm just going to merge the previous
	// user interactions into a single prompt. Fingers crossed there's no limits.
	// Merge the user interactions into a single prompt
	prompt := strings.Builder{}
	for _, interaction := range userInteractions[:len(userInteractions)-1] {
		prompt.WriteString(interaction.Message)
		prompt.WriteString("\n")
	}
	prompt.WriteString(userInteractions[len(userInteractions)-1].Message)

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

// calculateVLLMMemoryUtilizationRatio calculates the optimal GPU memory utilization ratio
// for VLLM based on the model's memory requirements and the runner's total GPU memory.
// This ensures VLLM uses an appropriate amount of GPU memory without causing OOM errors.
//
// HACK: VLLM's --gpu-memory-utilization flag doesn't work as documented. According to a GitHub issue,
// when multiple VLLM instances are running on the same GPU, each subsequent instance needs to add
// the previous instances' memory utilization to its own utilization ratio. This is a workaround
// until VLLM fixes the calculation to work as described in the documentation.
//
// We extend this hack to all existing model instances, not just VLLM, since any GPU memory usage
// affects how much memory VLLM can actually use.
func (c *RunnerController) calculateVLLMMemoryUtilizationRatio(runnerID string, modelMemoryRequirement uint64) float64 {
	// Get the runner's total memory
	totalMemory := c.TotalMemory(runnerID)
	if totalMemory == 0 {
		log.Warn().
			Str("runner_id", runnerID).
			Msg("No GPU memory information available for runner, using default ratio")
		return 0.8 // Default fallback ratio
	}

	// Calculate this instance's base ratio
	finalRatio := float64(modelMemoryRequirement) / float64(totalMemory)

	// Ensure the ratio is within reasonable bounds (35% to 95%)
	// Lower bound of 35% allows for small models while avoiding potential VLLM issues
	// Upper bound of 95% prevents potential OOM from other GPU processes
	if finalRatio < 0.35 {
		finalRatio = 0.35
	} else if finalRatio > 0.95 {
		finalRatio = 0.95
	}

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("model_memory_bytes", modelMemoryRequirement).
		Uint64("total_gpu_memory_bytes", totalMemory).
		Float64("final_cumulative_ratio", finalRatio).
		Msg("Calculated cumulative VLLM memory utilization ratio (HACK: workaround for VLLM cumulative behavior)")

	return finalRatio
}

// getExistingMemoryUtilization calculates the cumulative memory utilization ratio
// of all existing model instances on the specified runner.
// This is part of the VLLM memory allocation hack - we need to account for ALL existing
// GPU memory usage, not just VLLM instances.
func (c *RunnerController) getExistingMemoryUtilization(runnerID string) float64 {
	totalMemory := c.TotalMemory(runnerID)
	if totalMemory == 0 {
		return 0.0
	}

	// Get the runner status to find allocated memory
	status, err := c.GetStatus(runnerID)
	if err != nil {
		log.Warn().
			Str("runner_id", runnerID).
			Err(err).
			Msg("Failed to get runner status for memory utilization calculation")
		return 0.0
	}

	// Use the allocated memory from the runner status
	// This represents the total memory currently allocated to all slots/models
	existingUtilizationRatio := float64(status.AllocatedMemory) / float64(totalMemory)

	log.Debug().
		Str("runner_id", runnerID).
		Uint64("allocated_memory_bytes", status.AllocatedMemory).
		Uint64("total_memory_bytes", totalMemory).
		Float64("existing_utilization_ratio", existingUtilizationRatio).
		Msg("Calculated existing memory utilization for cumulative VLLM hack (all models)")

	return existingUtilizationRatio
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

		// If this is a VLLM runtime, get the model-specific args
		if slot.InitialWork().Runtime() == types.RuntimeVLLM {
			// Get model-specific args for vLLM
			modelArgs, argsErr := model.GetVLLMArgsForModel(modelName)
			if argsErr == nil && len(modelArgs) > 0 {
				log.Debug().
					Str("model", modelName).
					Strs("vllm_args", modelArgs).
					Msg("Found model-specific vLLM args in scheduler")

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
					Msg("Created runtime_args map with substituted values in scheduler")
			}
		}
	} else {
		log.Warn().Str("model", modelName).Err(err).Msg("Could not get model, using default context length")
	}

	req := &types.CreateRunnerSlotRequest{
		ID: slot.ID,
		Attributes: types.CreateRunnerSlotAttributes{
			Runtime:                slot.InitialWork().Runtime(),
			Model:                  slot.InitialWork().ModelName().String(),
			ModelMemoryRequirement: slot.Memory(),
			ContextLength:          contextLength,
			RuntimeArgs:            runtimeArgs,
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

func (c *RunnerController) SubmitFinetuningRequest(slot *Slot, session *types.Session) error {
	log.Info().Str("session_id", session.ID).Msg("processing fine-tuning interaction")

	lastInteraction, err := data.GetLastInteraction(session)
	if err != nil {
		return fmt.Errorf("error getting last interaction: %w", err)
	}
	headers := map[string]string{}
	headers[pubsub.HelixNatsReplyHeader] = pubsub.GetRunnerResponsesQueue(session.Owner, lastInteraction.ID)

	// TODO(Phil): the old code had some complicated logic around merging multiple jsonl files.
	// I'll just use the jsonl files from the last interaction for now.

	// and append them to one large JSONL file
	userInteractions := data.FilterUserInteractions(session.Interactions)
	finetuneInteractions := data.FilterFinetuneInteractions(userInteractions)
	if len(finetuneInteractions) == 0 {
		return fmt.Errorf("no finetune interactions found")
	}
	lastFinetuneInteraction := finetuneInteractions[len(finetuneInteractions)-1]
	if len(lastFinetuneInteraction.Files) != 1 {
		return fmt.Errorf("last interaction should have exactly one file")
	}
	combinedFile := lastFinetuneInteraction.Files[0]

	// Check that combined file size is not zero
	fi, err := c.fs.Get(c.ctx, combinedFile)
	if err != nil {
		return fmt.Errorf("error checking jsonl file: %w", err)
	}
	if fi.Size <= 1 {
		// Check for 1 byte to account for just a newline character
		return fmt.Errorf("training data file is empty")
	}
	log.Debug().Str("session_id", session.ID).Int64("file_size", fi.Size).Msgf("combined file size")

	req := openai.FineTuningJobRequest{
		Model:          session.ModelName,
		TrainingFile:   combinedFile,
		ValidationFile: "",
		Hyperparameters: &openai.Hyperparameters{
			Epochs:                 1,
			LearningRateMultiplier: 0.0002,
			BatchSize:              6,
		},
		Suffix: session.ID, // Use the suffix to identify the session and the final directory for the LORA
	}

	requestBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	natsReq := &types.RunnerNatsReplyRequest{
		RequestID:     lastInteraction.ID, // Use the last interaction ID as the request ID for sessions, it's important that this kept in sync with the receiver code
		CreatedAt:     time.Now(),
		OwnerID:       session.Owner,
		SessionID:     session.ID,
		InteractionID: lastInteraction.ID,
		Request:       requestBytes,
	}
	body, err := json.Marshal(natsReq)
	if err != nil {
		return err
	}
	resp, err := c.Send(c.ctx, slot.RunnerID, headers, &types.Request{
		Method: "POST",
		URL:    fmt.Sprintf("/api/v1/slots/%s/v1/helix/fine_tuning/jobs", slot.ID),
		Body:   body,
	}, 10*time.Minute)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error submitting finetuning request: %s", resp.Body)
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
