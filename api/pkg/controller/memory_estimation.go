package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// MemoryEstimationCache provides caching for memory estimation results
type MemoryEstimationCache struct {
	mu      sync.RWMutex
	entries map[string]*MemoryEstimationCacheEntry
	ttl     time.Duration
}

// MemoryEstimationCacheEntry represents a cached memory estimation result
type MemoryEstimationCacheEntry struct {
	Result    *memory.EstimationResult `json:"result"`
	Timestamp time.Time                `json:"timestamp"`
	Key       string                   `json:"key"`
}

// RunnerInfoProvider provides runner information for memory estimation
type RunnerInfoProvider interface {
	GetStatus(runnerID string) (*types.RunnerStatus, error)
}

// RunnerSender provides runner communication for memory estimation
type RunnerSender interface {
	Send(ctx context.Context, runnerID string, headers map[string]string, req *types.Request, timeout time.Duration) (*types.Response, error)
	RunnerIDs() []string
}

// ModelProvider provides model information for memory estimation
type ModelProvider interface {
	ListModels(ctx context.Context) ([]*types.Model, error)
}

// StoreModelProvider wraps a store to implement ModelProvider interface
type StoreModelProvider struct {
	store interface {
		ListModels(ctx context.Context, query *store.ListModelsQuery) ([]*types.Model, error)
	}
}

func NewStoreModelProvider(s interface {
	ListModels(ctx context.Context, query *store.ListModelsQuery) ([]*types.Model, error)
}) ModelProvider {
	return &StoreModelProvider{store: s}
}

func (smp *StoreModelProvider) ListModels(ctx context.Context) ([]*types.Model, error) {
	return smp.store.ListModels(ctx, &store.ListModelsQuery{})
}

// MemoryEstimationRequest represents a request for memory estimation
type MemoryEstimationRequest struct {
	ModelID string `json:"model_id"`
	// CRITICAL: GPUCount is the NUMBER OF GPUs in the hardware configuration (1, 2, 4, 8, etc.)
	// It is NOT the number of layers to offload to GPU (that's always auto-detect = -1)
	// This API parameter controls how many GPUs to simulate in the estimation
	GPUCount      int `json:"gpu_count,omitempty"`
	ContextLength int `json:"context_length,omitempty"`
	BatchSize     int `json:"batch_size,omitempty"`
}

// MemoryEstimationResponse represents a response with memory estimation
type MemoryEstimationResponse struct {
	ModelID   string                       `json:"model_id"`
	GPUConfig []types.GPUInfoForEstimation `json:"gpu_config"`
	Estimate  *memory.MemoryEstimate       `json:"estimate"`
	Cached    bool                         `json:"cached"`
	Error     string                       `json:"error,omitempty"`
}

// MemoryEstimationService provides memory estimation services for the controlplane
type MemoryEstimationService struct {
	cache           *MemoryEstimationCache
	runnerSender    RunnerSender  // Interface to send requests to runners via NATS
	modelProvider   ModelProvider // Interface to get list of models
	stopChan        chan struct{}
	refreshInterval time.Duration
}

// NewMemoryEstimationService creates a new memory estimation service
func NewMemoryEstimationService(runnerSender RunnerSender, modelProvider ModelProvider) *MemoryEstimationService {
	return &MemoryEstimationService{
		cache: &MemoryEstimationCache{
			entries: make(map[string]*MemoryEstimationCacheEntry),
			ttl:     24 * time.Hour, // Cache for 24 hours
		},
		runnerSender:    runnerSender,
		modelProvider:   modelProvider,
		stopChan:        make(chan struct{}),
		refreshInterval: 15 * time.Minute, // Refresh cache every 15 minutes
	}
}

// selectEstimateFromResult selects the appropriate estimate from EstimationResult based on recommendation
// For UI display purposes, prefer GPU estimates when available to show actual VRAM usage
func selectEstimateFromResult(result *memory.EstimationResult) *memory.MemoryEstimate {
	switch result.Recommendation {
	case "single_gpu":
		return result.SingleGPU
	case "tensor_parallel":
		return result.TensorParallel
	case "insufficient_memory":
		// For UI display, prefer to show GPU estimates to show actual VRAM requirements
		if result.SingleGPU != nil && result.SingleGPU.VRAMSize > 0 {
			return result.SingleGPU
		} else if result.TensorParallel != nil && result.TensorParallel.VRAMSize > 0 {
			return result.TensorParallel
		} else {
			return nil // No valid GPU estimate available
		}
	default:
		// Default to single GPU if available
		if result.SingleGPU != nil {
			return result.SingleGPU
		} else if result.TensorParallel != nil {
			return result.TensorParallel
		} else {
			return nil // No valid GPU estimate available
		}
	}
}

// EstimateModelMemoryFromRequest estimates memory requirements for a model from a request
func (s *MemoryEstimationService) EstimateModelMemoryFromRequest(ctx context.Context, req *MemoryEstimationRequest) (*MemoryEstimationResponse, error) {
	// CRITICAL: req.GPUCount is the NUMBER OF GPUs in hardware config, NOT layers to offload!
	// Determine number of GPUs for hardware estimation
	numGPUs := 1
	if req.GPUCount > 1 {
		numGPUs = req.GPUCount
	} else if req.GPUCount == -1 {
		numGPUs = 2 // Default to 2 for auto-detect
	}

	// Use standard GPU configuration with 80GB per GPU (large enough for any model)
	gpuConfig := types.CreateStandardGPUConfig(numGPUs, 80)

	// Get the model's configured context length from the store
	contextLength := 4096 // Default fallback
	if models, err := s.modelProvider.ListModels(ctx); err == nil {
		for _, model := range models {
			if model.ID == req.ModelID {
				contextLength = int(model.ContextLength)
				log.Debug().
					Str("model_id", req.ModelID).
					Int("context_length", contextLength).
					Msg("using model's configured context length")
				break
			}
		}
	}

	// CRITICAL: ALWAYS use -1 for layer offload (auto-detect max layers that fit)
	// The req.GPUCount parameter controls GPU hardware config, NOT layer offload!
	opts := types.CreateOllamaEstimateOptions(int64(contextLength), types.AutoDetectLayers)

	// Allow request to override the configured context length
	if req.ContextLength > 0 {
		opts.NumCtx = req.ContextLength
		log.Debug().
			Str("model_id", req.ModelID).
			Int("requested_context_length", req.ContextLength).
			Int("configured_context_length", contextLength).
			Msg("overriding model's configured context length with request parameter")
	}
	if req.BatchSize > 0 {
		opts.NumBatch = req.BatchSize
	}

	// Check cache first
	cacheKey := s.generateCacheKey(req.ModelID, gpuConfig, opts)
	if result := s.cache.get(cacheKey); result != nil {
		return &MemoryEstimationResponse{
			ModelID:   req.ModelID,
			GPUConfig: gpuConfig,
			Estimate:  selectEstimateFromResult(result),
			Cached:    true,
		}, nil
	}

	// Estimate memory
	result, err := s.EstimateModelMemory(ctx, req.ModelID, gpuConfig, opts)
	if err != nil {
		return &MemoryEstimationResponse{
			ModelID: req.ModelID,
			Error:   err.Error(),
		}, nil
	}

	return &MemoryEstimationResponse{
		ModelID:   req.ModelID,
		GPUConfig: gpuConfig,
		Estimate:  selectEstimateFromResult(result),
		Cached:    false,
	}, nil
}

// EstimateModelMemory estimates memory requirements for a model
func (s *MemoryEstimationService) EstimateModelMemory(ctx context.Context, modelName string, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	// Debug logging for context length tracing
	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "entry_point").
		Str("model_name", modelName).
		Int("num_ctx", opts.NumCtx).
		Int("num_batch", opts.NumBatch).
		Int("num_parallel", opts.NumParallel).
		Int("num_gpu", opts.NumGPU).
		Str("kv_cache_type", opts.KVCacheType).
		Int("gpu_config_count", len(gpuConfig)).
		Msg("🐠 SHARK EstimateModelMemory called with parameters")

	// Check if this is an Ollama model - only Ollama models support GGUF-based estimation
	models, err := s.modelProvider.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	var targetModel *types.Model
	for _, model := range models {
		if model.ID == modelName {
			targetModel = model
			break
		}
	}

	if targetModel == nil {
		return nil, fmt.Errorf("model %s not found", modelName)
	}

	// Only provide GGUF-based estimates for Ollama models
	if targetModel.Runtime != types.RuntimeOllama {
		return nil, fmt.Errorf("GGUF-based memory estimation is only available for Ollama models, model %s uses runtime %s", modelName, targetModel.Runtime)
	}

	// Generate cache key
	cacheKey := s.generateCacheKey(modelName, gpuConfig, opts)

	// Check cache first
	if result := s.cache.get(cacheKey); result != nil {
		log.Debug().
			Str("model_name", modelName).
			Str("cache_key", cacheKey).
			Int("num_ctx_used", opts.NumCtx).
			Str("kv_cache_type_used", opts.KVCacheType).
			Str("recommendation", result.Recommendation).
			Interface("single_gpu", result.SingleGPU).
			Interface("tensor_parallel", result.TensorParallel).
			Msg("🐠 SHARK returning cached memory estimation result - showing what params were used")
		return result, nil
	}

	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "cache_miss").
		Str("model_name", modelName).
		Str("cache_key", cacheKey).
		Int("num_ctx", opts.NumCtx).
		Str("kv_cache_type", opts.KVCacheType).
		Msg("🐠 SHARK cache miss - will calculate new estimation")

	// Find a runner that has this model
	log.Info().
		Str("model_name", modelName).
		Msg("PREWARM_DEBUG: About to find runner with model for memory estimation")

	runnerID, err := s.findRunnerWithModel(ctx, modelName)
	if err != nil {
		log.Error().
			Str("model_name", modelName).
			Err(err).
			Msg("PREWARM_DEBUG: CRITICAL - Failed to find runner with model - no available runners may have this model yet")
		return nil, fmt.Errorf("failed to find runner with model %s: %w", modelName, err)
	}

	// Get memory estimation from runner using exact Ollama algorithm
	estimationResp, err := s.getMemoryEstimationFromRunner(ctx, runnerID, modelName, opts)
	if err != nil {
		log.Error().
			Str("model_name", modelName).
			Str("runner_id", runnerID).
			Err(err).
			Msg("PREWARM_DEBUG: CRITICAL - Failed to get memory estimation from runner - timing issue or runner not ready")
		return nil, fmt.Errorf("failed to get memory estimation from runner %s: %w", runnerID, err)
	}

	// Log raw runner response
	log.Debug().
		Str("MEMORY_DEBUG", "raw_runner_response").
		Str("model_name", modelName).
		Str("runner_id", runnerID).
		Str("architecture", estimationResp.Architecture).
		Int("config_count", len(estimationResp.Configurations)).
		Interface("configurations", estimationResp.Configurations).
		Msg("🔥 MEMORY_DEBUG: Raw response from runner")

	// Debug logging for memory estimation results from runner
	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "runner_estimation_received").
		Str("architecture", estimationResp.Architecture).
		Uint64("block_count", estimationResp.BlockCount).
		Int("config_count", len(estimationResp.Configurations)).
		Int64("response_time_ms", estimationResp.ResponseTime).
		Msg("🐠 SHARK Memory estimation received from runner using exact Ollama")

	// Debug logging for GPU configuration being used
	for i, gpu := range gpuConfig {
		log.Debug().
			Str("MEMORY_ESTIMATION_DEBUG", "gpu_config").
			Int("gpu_index", i).
			Str("gpu_id", gpu.ID).
			Str("library", gpu.Library).
			Uint64("total_memory_bytes", gpu.TotalMemory).
			Uint64("total_memory_gb", gpu.TotalMemory/(1024*1024*1024)).
			Uint64("free_memory_bytes", gpu.FreeMemory).
			Uint64("free_memory_gb", gpu.FreeMemory/(1024*1024*1024)).
			Uint64("minimum_memory_bytes", gpu.MinimumMemory).
			Uint64("minimum_memory_gb", gpu.MinimumMemory/(1024*1024*1024)).
			Msg("🐠 SHARK GPU configuration for memory estimation")
	}

	// Convert runner's multi-config response to our EstimationResult format
	result := s.convertRunnerEstimationToResult(estimationResp, gpuConfig, opts)

	// Debug logging for memory estimation results
	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "result").
		Str("model_name", modelName).
		Str("runner_id", runnerID).
		Str("recommendation", result.Recommendation).
		Int("num_ctx_used", opts.NumCtx).
		Uint64("single_gpu_total_mb", func() uint64 {
			if result.SingleGPU != nil {
				return result.SingleGPU.TotalSize / (1024 * 1024)
			}
			return 0
		}()).
		Uint64("single_gpu_kv_cache_mb", func() uint64 {
			if result.SingleGPU != nil {
				return result.SingleGPU.KVCache / (1024 * 1024)
			}
			return 0
		}()).
		Msg("memory estimation result details")

	// Cache the result
	s.cache.set(cacheKey, result)

	log.Info().
		Str("model_name", modelName).
		Str("runner_id", runnerID).
		Str("recommendation", result.Recommendation).
		Msg("successfully obtained memory estimation")

	return result, nil
}

// GetCachedEstimation returns a cached estimation if available
func (s *MemoryEstimationService) GetCachedEstimation(modelName string, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) *memory.EstimationResult {
	cacheKey := s.generateCacheKey(modelName, gpuConfig, opts)
	return s.cache.get(cacheKey)
}

// findRunnerWithModel finds a runner that has the specified model
func (s *MemoryEstimationService) findRunnerWithModel(ctx context.Context, modelName string) (runnerID string, err error) {
	// Get list of connected runners
	runnerIDs := s.runnerSender.RunnerIDs()

	log.Info().
		Str("model_name", modelName).
		Int("total_runners", len(runnerIDs)).
		Strs("runner_ids", runnerIDs).
		Msg("PREWARM_DEBUG: Looking for runner with model - checking all connected runners")

	// Check each runner for the model by calling the status endpoint
	for _, runnerID := range runnerIDs {
		// Create request to get runner status
		req := &types.Request{
			Method: "GET",
			URL:    "/api/v1/status",
			Body:   []byte{},
		}

		resp, err := s.runnerSender.Send(ctx, runnerID, nil, req, 5*time.Second)
		if err != nil {
			log.Error().
				Err(err).
				Str("runner_id", runnerID).
				Str("model_name", modelName).
				Msg("PREWARM_DEBUG: Failed to get runner status - runner may not be ready yet")
			log.Debug().
				Err(err).
				Str("runner_id", runnerID).
				Msg("failed to get runner status")
			continue
		}

		if resp.StatusCode != 200 {
			log.Error().
				Int("status_code", resp.StatusCode).
				Str("runner_id", runnerID).
				Str("model_name", modelName).
				Msg("PREWARM_DEBUG: Runner status request returned non-200 - runner may not be ready")
			log.Debug().
				Int("status_code", resp.StatusCode).
				Str("runner_id", runnerID).
				Msg("runner status request failed")
			continue
		}

		// Parse runner status response
		var status types.RunnerStatus
		if err := json.Unmarshal(resp.Body, &status); err != nil {
			log.Error().
				Err(err).
				Str("runner_id", runnerID).
				Str("model_name", modelName).
				Msg("PREWARM_DEBUG: Failed to parse runner status response")
			log.Debug().
				Err(err).
				Str("runner_id", runnerID).
				Msg("failed to parse runner status")
			continue
		}

		// Log what models this runner currently has loaded
		modelNames := make([]string, len(status.Models))
		for i, model := range status.Models {
			modelNames[i] = model.ModelID
		}
		log.Info().
			Str("runner_id", runnerID).
			Str("target_model", modelName).
			Strs("loaded_models", modelNames).
			Msg("PREWARM_DEBUG: Checking if runner has target model loaded")

		// Check if this runner has the model
		for _, model := range status.Models {
			if model.ModelID == modelName {
				log.Info().
					Str("runner_id", runnerID).
					Str("model_name", modelName).
					Msg("PREWARM_DEBUG: Found runner with model already loaded!")
				return runnerID, nil
			}
		}
	}

	return "", fmt.Errorf("no runner found with model %s", modelName)
}

// RunnerMemoryEstimationRequest represents a request for memory estimation from runner
type RunnerMemoryEstimationRequest struct {
	ModelName     string `json:"model_name"`
	ContextLength int    `json:"context_length"`
	BatchSize     int    `json:"batch_size"`
	NumParallel   int    `json:"num_parallel"`
}

// RunnerMemoryEstimationResponse represents the response with memory estimates from runner
type RunnerMemoryEstimationResponse struct {
	Success        bool                     `json:"success"`
	Error          string                   `json:"error,omitempty"`
	ModelName      string                   `json:"model_name"`
	Architecture   string                   `json:"architecture"`
	BlockCount     uint64                   `json:"block_count"`
	Configurations []GPUConfigurationResult `json:"configurations"`
	ResponseTime   int64                    `json:"response_time_ms"`
	RunnerID       string                   `json:"runner_id"`
}

// GPUConfigurationResult contains memory estimation for a specific GPU setup
type GPUConfigurationResult struct {
	Name          string   `json:"name"`           // "single_gpu", "dual_gpu", etc.
	GPUCount      int      `json:"gpu_count"`      // Number of GPUs used
	LayersOnGPU   int      `json:"layers_on_gpu"`  // How many layers fit on GPU
	TotalLayers   int      `json:"total_layers"`   // Total layers in model
	VRAMRequired  uint64   `json:"vram_required"`  // VRAM needed in bytes
	TotalMemory   uint64   `json:"total_memory"`   // Total memory in bytes
	GraphMemory   uint64   `json:"graph_memory"`   // Graph computation memory in bytes
	KVCacheMemory uint64   `json:"kv_cache"`       // KV cache memory in bytes
	WeightsMemory uint64   `json:"weights_memory"` // Model weights memory in bytes
	FullyLoaded   bool     `json:"fully_loaded"`   // True if all layers fit on GPU
	GPUSizes      []uint64 `json:"gpu_sizes"`      // Memory per GPU in bytes
	TensorSplit   string   `json:"tensor_split"`   // Tensor split configuration
}

// getMemoryEstimationFromRunner gets memory estimates from a runner via NATS using exact Ollama
func (s *MemoryEstimationService) getMemoryEstimationFromRunner(ctx context.Context, runnerID, modelName string, opts memory.EstimateOptions) (*RunnerMemoryEstimationResponse, error) {
	// Prepare memory estimation request
	request := RunnerMemoryEstimationRequest{
		ModelName:     modelName,
		ContextLength: opts.NumCtx,
		BatchSize:     opts.NumBatch,
		NumParallel:   opts.NumParallel,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal memory estimation request: %w", err)
	}

	// Create NATS request
	req := &types.Request{
		Method: "POST",
		URL:    "/api/v1/memory-estimate",
		Body:   requestBody,
	}

	// Send request to runner via NATS
	log.Info().
		Str("runner_id", runnerID).
		Str("model_name", modelName).
		Msg("PREWARM_DEBUG: About to send memory estimation request to runner via NATS")

	resp, err := s.runnerSender.Send(ctx, runnerID, map[string]string{
		"Content-Type": "application/json",
	}, req, 30*time.Second)
	if err != nil {
		log.Error().
			Str("runner_id", runnerID).
			Str("model_name", modelName).
			Err(err).
			Msg("PREWARM_DEBUG: CRITICAL - Failed to send memory estimation request to runner via NATS - runner may not be ready!")
		return nil, fmt.Errorf("failed to send memory estimation request to runner: %w", err)
	}

	log.Info().
		Str("runner_id", runnerID).
		Str("model_name", modelName).
		Int("status_code", resp.StatusCode).
		Msg("PREWARM_DEBUG: Got response from runner via NATS")

	if resp.StatusCode != 200 {
		log.Error().
			Str("runner_id", runnerID).
			Str("model_name", modelName).
			Int("status_code", resp.StatusCode).
			Msg("PREWARM_DEBUG: CRITICAL - Runner returned non-200 status for memory estimation")
		return nil, fmt.Errorf("runner returned status %d for memory estimation request", resp.StatusCode)
	}

	// Parse response
	var response RunnerMemoryEstimationResponse
	if err := json.Unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to decode memory estimation response: %w", err)
	}

	if !response.Success {
		log.Error().
			Str("runner_id", runnerID).
			Str("model_name", modelName).
			Str("error", response.Error).
			Msg("PREWARM_DEBUG: CRITICAL - Runner returned success=false for memory estimation")
		return nil, fmt.Errorf("memory estimation failed: %s", response.Error)
	}

	return &response, nil
}

// convertRunnerEstimationToResult converts runner's multi-config estimation to our EstimationResult format
func (s *MemoryEstimationService) convertRunnerEstimationToResult(runnerResp *RunnerMemoryEstimationResponse, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) *memory.EstimationResult {
	log.Debug().
		Str("MEMORY_DEBUG", "converting_runner_response").
		Str("model_name", runnerResp.ModelName).
		Int("config_count", len(runnerResp.Configurations)).
		Msg("🔥 MEMORY_DEBUG: Starting conversion from runner response")

	// Create fake metadata from runner response for backward compatibility
	metadata := &memory.ModelMetadata{
		Architecture: runnerResp.Architecture,
		BlockCount:   runnerResp.BlockCount,
		Layers:       make(map[string]memory.LayerInfo),
	}

	result := &memory.EstimationResult{
		ModelName:   runnerResp.ModelName,
		Metadata:    metadata,
		EstimatedAt: time.Now(),
	}

	// Find the appropriate configurations from runner response
	var singleGPUConfig, tensorParallelConfig *GPUConfigurationResult

	for i := range runnerResp.Configurations {
		config := &runnerResp.Configurations[i]
		switch config.Name {
		case "single_gpu":
			singleGPUConfig = config
		case "dual_gpu", "quad_gpu", "octo_gpu":
			if tensorParallelConfig == nil || config.GPUCount > tensorParallelConfig.GPUCount {
				tensorParallelConfig = config
			}
			// Skip cpu_only configurations - not supported
		}
	}

	// Convert to our MemoryEstimate format
	if singleGPUConfig != nil {
		log.Debug().
			Str("MEMORY_DEBUG", "converting_single_gpu_config").
			Str("config_name", singleGPUConfig.Name).
			Uint64("total_memory_bytes", singleGPUConfig.TotalMemory).
			Uint64("total_memory_gb", singleGPUConfig.TotalMemory/(1024*1024*1024)).
			Float64("total_memory_gib", float64(singleGPUConfig.TotalMemory)/(1024*1024*1024)).
			Uint64("vram_required_bytes", singleGPUConfig.VRAMRequired).
			Msg("🔥 MEMORY_DEBUG: Converting single GPU config")
		result.SingleGPU = s.convertConfigToMemoryEstimate(singleGPUConfig, metadata, opts, []types.GPUInfoForEstimation{gpuConfig[0]})
	}

	if tensorParallelConfig != nil && len(gpuConfig) >= tensorParallelConfig.GPUCount {
		result.TensorParallel = s.convertConfigToMemoryEstimate(tensorParallelConfig, metadata, opts, gpuConfig[:tensorParallelConfig.GPUCount])
	}

	// CPU-only estimation disabled - not properly supported

	// Determine recommendation based on what's available and works
	if result.SingleGPU != nil && result.SingleGPU.FullyLoaded && len(gpuConfig) >= 1 {
		result.Recommendation = "single_gpu"
	} else if result.TensorParallel != nil && result.TensorParallel.FullyLoaded {
		result.Recommendation = "tensor_parallel"
	} else if result.SingleGPU != nil && result.SingleGPU.Layers > 0 {
		result.Recommendation = "single_gpu"
	} else {
		result.Recommendation = "insufficient_memory"
	}

	return result
}

// convertConfigToMemoryEstimate converts a GPU configuration result to our MemoryEstimate format
func (s *MemoryEstimationService) convertConfigToMemoryEstimate(config *GPUConfigurationResult, metadata *memory.ModelMetadata, opts memory.EstimateOptions, gpus []types.GPUInfoForEstimation) *memory.MemoryEstimate {
	log.Debug().
		Str("MEMORY_DEBUG", "convertConfigToMemoryEstimate_input").
		Str("config_name", config.Name).
		Uint64("input_total_memory_bytes", config.TotalMemory).
		Uint64("input_total_memory_gb", config.TotalMemory/(1024*1024*1024)).
		Float64("input_total_memory_gib", float64(config.TotalMemory)/(1024*1024*1024)).
		Uint64("input_vram_required_bytes", config.VRAMRequired).
		Msg("🔥 MEMORY_DEBUG: Input values to convertConfigToMemoryEstimate")

	// Convert GPU info
	gpuInfos := make([]memory.GPUInfo, len(gpus))
	for i, gpu := range gpus {
		gpuInfos[i] = memory.GPUInfo{
			ID:            gpu.ID,
			Index:         gpu.Index,
			Library:       gpu.Library,
			FreeMemory:    gpu.FreeMemory,
			TotalMemory:   gpu.TotalMemory,
			MinimumMemory: gpu.MinimumMemory,
		}
	}

	// Parse tensor split
	var tensorSplit []int
	if config.TensorSplit != "" {
		parts := strings.Split(config.TensorSplit, ",")
		tensorSplit = make([]int, len(parts))
		for i, part := range parts {
			if val, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				tensorSplit[i] = val
			}
		}
	}

	estimate := &memory.MemoryEstimate{
		Architecture:     metadata.Architecture,
		Layers:           config.LayersOnGPU,
		VRAMSize:         config.VRAMRequired,
		TotalSize:        config.TotalMemory,
		Graph:            config.GraphMemory,
		KVCache:          config.KVCacheMemory,
		Weights:          config.WeightsMemory,
		Projectors:       0,
		FullyLoaded:      config.FullyLoaded,
		RequiresFallback: config.LayersOnGPU == 0,
		EstimatedAt:      time.Now(),
		Options:          opts,
		GPUs:             gpuInfos,
		GPUSizes:         config.GPUSizes,
		TensorSplit:      tensorSplit,
	}

	log.Debug().
		Str("MEMORY_DEBUG", "convertConfigToMemoryEstimate_output").
		Str("config_name", config.Name).
		Uint64("output_total_size_bytes", estimate.TotalSize).
		Uint64("output_total_size_gb", estimate.TotalSize/(1024*1024*1024)).
		Float64("output_total_size_gib", float64(estimate.TotalSize)/(1024*1024*1024)).
		Uint64("output_vram_size_bytes", estimate.VRAMSize).
		Msg("🔥 MEMORY_DEBUG: Output values from convertConfigToMemoryEstimate")

	return estimate
}

// generateCacheKey generates a cache key for the given parameters
func (s *MemoryEstimationService) generateCacheKey(modelName string, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) string {
	// Create a simple hash of the parameters
	key := fmt.Sprintf("%s_%d_%d_%d_%d_%d_%s",
		modelName,
		len(gpuConfig),
		opts.NumCtx,
		opts.NumBatch,
		opts.NumParallel,
		opts.NumGPU,
		opts.KVCacheType)

	// Add GPU memory info to key
	for _, gpu := range gpuConfig {
		key += fmt.Sprintf("_%s_%d", gpu.Library, gpu.TotalMemory/(1024*1024*1024)) // GB
	}

	return key
}

// Cache methods
func (c *MemoryEstimationCache) get(key string) *memory.EstimationResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Since(entry.Timestamp) > c.ttl {
		// Don't delete here to avoid write lock, let cleanup handle it
		return nil
	}

	return entry.Result
}

func (c *MemoryEstimationCache) set(key string, result *memory.EstimationResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &MemoryEstimationCacheEntry{
		Result:    result,
		Timestamp: time.Now(),
		Key:       key,
	}
}

// CleanupExpired removes expired entries from the cache
func (c *MemoryEstimationCache) CleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.Timestamp) > c.ttl {
			delete(c.entries, key)
		}
	}
}

// GetCacheStats returns cache statistics
func (c *MemoryEstimationCache) GetCacheStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"entry_count": len(c.entries),
		"ttl_hours":   c.ttl.Hours(),
	}
}

// StartCacheCleanup starts a background goroutine to clean up expired cache entries
func (s *MemoryEstimationService) StartCacheCleanup(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour) // Cleanup every hour
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cache.CleanupExpired()
				log.Debug().Msg("cleaned up expired memory estimation cache entries")
			}
		}
	}()
}

// StartBackgroundCacheRefresh starts a background goroutine that periodically refreshes the memory estimation cache
func (s *MemoryEstimationService) StartBackgroundCacheRefresh(ctx context.Context) {
	log.Info().
		Dur("refresh_interval", s.refreshInterval).
		Msg("starting background memory estimation cache refresh")

	ticker := time.NewTicker(s.refreshInterval)
	go func() {
		defer ticker.Stop()

		// Do an initial refresh after a short delay
		time.Sleep(30 * time.Second)
		s.refreshCacheForAllModels(ctx)

		for {
			select {
			case <-ctx.Done():
				log.Info().Msg("stopping background memory estimation cache refresh")
				return
			case <-s.stopChan:
				log.Info().Msg("stopping background memory estimation cache refresh via stop channel")
				return
			case <-ticker.C:
				s.refreshCacheForAllModels(ctx)
			}
		}
	}()
}

// StopBackgroundCacheRefresh stops the background cache refresh
func (s *MemoryEstimationService) StopBackgroundCacheRefresh() {
	select {
	case s.stopChan <- struct{}{}:
	default:
		// Channel might be closed or full, that's OK
	}
}

// refreshCacheForAllModels refreshes memory estimates for all models with common GPU configurations
func (s *MemoryEstimationService) refreshCacheForAllModels(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().
				Interface("panic", r).
				Msg("recovered from panic during memory estimation cache refresh")
		}
	}()

	log.Debug().Msg("starting background cache refresh for memory estimates")

	// Get all models from the store
	models, err := s.modelProvider.ListModels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to get models for cache refresh")
		return
	}

	log.Debug().Int("model_count", len(models)).Msg("refreshing memory estimation cache for models")

	// Common GPU configurations to cache
	gpuConfigs := []struct {
		name   string
		config []types.GPUInfoForEstimation
	}{
		{
			name:   "1GPU",
			config: types.CreateStandardGPUConfig(1, 80),
		},
		{
			name:   "2GPU",
			config: types.CreateStandardGPUConfig(2, 80),
		},
		{
			name:   "4GPU",
			config: types.CreateStandardGPUConfig(4, 80),
		},
	}

	// For each model, estimate memory for each GPU configuration
	for _, model := range models {
		// Skip non-Ollama models as they don't support GGUF-based estimation
		if model.Runtime != types.RuntimeOllama {
			continue
		}

		// Get model's actual context length - no fallbacks
		if model.ContextLength == 0 {
			log.Debug().
				Str("model_id", model.ID).
				Msg("skipping model with no context length configured")
			continue
		}

		for _, gpuConfig := range gpuConfigs {
			// Create EstimateOptions with model's actual context length
			opts := types.CreateEstimateOptionsForGPUArray(model.ContextLength)

			// Check if we already have a fresh cache entry
			cacheKey := s.generateCacheKey(model.ID, gpuConfig.config, opts)
			if entry := s.cache.get(cacheKey); entry != nil {
				// Entry exists and is still fresh, skip
				continue
			}

			// Estimate memory in background (don't block on individual model failures)
			go func(modelID string, config []types.GPUInfoForEstimation, opts memory.EstimateOptions, configName string) {
				log.Debug().
					Str("CACHE_REFRESH_DEBUG", "starting_estimation").
					Str("model_id", modelID).
					Str("config_name", configName).
					Int("num_ctx", opts.NumCtx).
					Str("kv_cache_type", opts.KVCacheType).
					Msg("Starting background memory estimation for cache")

				result, err := s.EstimateModelMemory(ctx, modelID, config, opts)
				if err != nil {
					log.Debug().
						Err(err).
						Str("model_id", modelID).
						Str("config_name", configName).
						Msg("failed to estimate memory for model in background refresh")
					return
				}

				log.Debug().
					Str("model_id", modelID).
					Str("config_name", configName).
					Str("recommendation", result.Recommendation).
					Msg("successfully cached memory estimate in background")
			}(model.ID, gpuConfig.config, opts, gpuConfig.name)
		}
	}

	log.Info().
		Int("model_count", len(models)).
		Msg("completed background cache refresh for memory estimates")
}
