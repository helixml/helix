package controller

import (
	"context"
	"encoding/json"
	"fmt"
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
	ModelID       string `json:"model_id"`
	NumGPU        int    `json:"num_gpu,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	BatchSize     int    `json:"batch_size,omitempty"`
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
	case "cpu_only", "insufficient_memory":
		// For UI display, prefer to show GPU estimates even if CPU-only is recommended
		// This allows users to see actual VRAM requirements
		if result.SingleGPU != nil && result.SingleGPU.VRAMSize > 0 {
			return result.SingleGPU
		} else if result.TensorParallel != nil && result.TensorParallel.VRAMSize > 0 {
			return result.TensorParallel
		} else {
			return result.CPUOnly
		}
	default:
		// Fallback to single GPU if available, otherwise tensor parallel
		if result.SingleGPU != nil {
			return result.SingleGPU
		} else if result.TensorParallel != nil {
			return result.TensorParallel
		} else {
			return result.CPUOnly
		}
	}
}

// EstimateModelMemoryFromRequest estimates memory requirements for a model from a request
func (s *MemoryEstimationService) EstimateModelMemoryFromRequest(ctx context.Context, req *MemoryEstimationRequest) (*MemoryEstimationResponse, error) {
	// Determine number of GPUs for estimation
	numGPUs := 1
	if req.NumGPU > 1 {
		numGPUs = req.NumGPU
	} else if req.NumGPU == -1 {
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

	opts := types.CreateOllamaEstimateOptions(int64(contextLength), req.NumGPU)

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
			Interface("cpu_only", result.CPUOnly).
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
	runnerID, err := s.findRunnerWithModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to find runner with model %s: %w", modelName, err)
	}

	// Get model metadata from runner via NATS
	metadata, err := s.getModelMetadataFromRunner(ctx, runnerID, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to get model metadata from runner %s: %w", runnerID, err)
	}

	// Debug logging for GGUF metadata that was parsed
	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "gguf_metadata_parsed").
		Str("architecture", metadata.Architecture).
		Str("file_type", metadata.FileType).
		Uint64("block_count", metadata.BlockCount).
		Uint64("embedding_length", metadata.EmbeddingLength).
		Uint64("context_length", metadata.ContextLength).
		Uint64("head_count", metadata.HeadCount).
		Uint64("head_count_kv", metadata.HeadCountKV).
		Uint64("key_length", metadata.KeyLength).
		Uint64("value_length", metadata.ValueLength).
		Uint64("ff_length", metadata.FFLength).
		Uint64("vocab_size", metadata.VocabSize).
		Int("total_layer_count", len(metadata.Layers)).
		Msg("🐠 SHARK GGUF metadata parsed from runner")

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

	// Do memory calculation in API using Ollama's algorithm
	result := s.calculateMemoryEstimateLocally(metadata, gpuConfig, opts)

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
			log.Debug().
				Err(err).
				Str("runner_id", runnerID).
				Msg("failed to get runner status")
			continue
		}

		if resp.StatusCode != 200 {
			log.Debug().
				Int("status_code", resp.StatusCode).
				Str("runner_id", runnerID).
				Msg("runner status request failed")
			continue
		}

		// Parse runner status response
		var status types.RunnerStatus
		if err := json.Unmarshal(resp.Body, &status); err != nil {
			log.Debug().
				Err(err).
				Str("runner_id", runnerID).
				Msg("failed to parse runner status")
			continue
		}

		// Check if this runner has the model
		for _, model := range status.Models {
			if model.ModelID == modelName {
				return runnerID, nil
			}
		}
	}

	return "", fmt.Errorf("no runner found with model %s", modelName)
}

// RunnerModelMetadataRequest represents a request for model metadata from runner
type RunnerModelMetadataRequest struct {
	ModelName    string `json:"model_name"`
	UseModelPath string `json:"use_model_path,omitempty"` // Optional: use specific model path
}

// RunnerModelMetadataResponse represents the response with GGUF metadata from runner
type RunnerModelMetadataResponse struct {
	Success      bool                  `json:"success"`
	Metadata     *memory.ModelMetadata `json:"metadata,omitempty"`
	Error        string                `json:"error,omitempty"`
	RunnerID     string                `json:"runner_id"`
	ModelPath    string                `json:"model_path"`
	ResponseTime int64                 `json:"response_time_ms"`
}

// getModelMetadataFromRunner gets GGUF metadata from a runner via NATS
func (s *MemoryEstimationService) getModelMetadataFromRunner(ctx context.Context, runnerID, modelName string) (*memory.ModelMetadata, error) {
	// Prepare metadata request
	request := RunnerModelMetadataRequest{
		ModelName: modelName,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metadata request: %w", err)
	}

	// Create NATS request
	req := &types.Request{
		Method: "POST",
		URL:    "/api/v1/model-metadata",
		Body:   requestBody,
	}

	// Send request to runner via NATS
	resp, err := s.runnerSender.Send(ctx, runnerID, map[string]string{
		"Content-Type": "application/json",
	}, req, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to send metadata request to runner: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("runner returned status %d for metadata request", resp.StatusCode)
	}

	// Parse response
	var response RunnerModelMetadataResponse
	if err := json.Unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to decode metadata response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("metadata extraction failed: %s", response.Error)
	}

	// Log the detailed metadata returned by the runner
	log.Debug().
		Str("RUNNER_METADATA_DEBUG", "detailed_response").
		Str("runner_id", runnerID).
		Str("model_name", modelName).
		Str("architecture", response.Metadata.Architecture).
		Str("file_type", response.Metadata.FileType).
		Uint64("block_count", response.Metadata.BlockCount).
		Uint64("embedding_length", response.Metadata.EmbeddingLength).
		Uint64("context_length", response.Metadata.ContextLength).
		Uint64("head_count", response.Metadata.HeadCount).
		Uint64("head_count_kv", response.Metadata.HeadCountKV).
		Uint64("key_length", response.Metadata.KeyLength).
		Uint64("value_length", response.Metadata.ValueLength).
		Uint64("ff_length", response.Metadata.FFLength).
		Uint64("vocab_size", response.Metadata.VocabSize).
		Int("layer_count", len(response.Metadata.Layers)).
		Msg("🔍 RUNNER returned GGUF metadata - these are the exact values we'll use in estimation")

	return response.Metadata, nil
}

// calculateMemoryEstimateLocally performs memory calculation in the API using metadata from runner
func (s *MemoryEstimationService) calculateMemoryEstimateLocally(metadata *memory.ModelMetadata, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) *memory.EstimationResult {
	// Debug logging for local calculation entry
	log.Debug().
		Str("MEMORY_ESTIMATION_DEBUG", "local_calculation").
		Str("architecture", metadata.Architecture).
		Uint64("block_count", metadata.BlockCount).
		Int("num_ctx", opts.NumCtx).
		Str("kv_cache_type", opts.KVCacheType).
		Int("gpu_count", len(gpuConfig)).
		Msg("calculateMemoryEstimateLocally called")

	// Convert types for our Ollama-based calculation
	gpuInfos := make([]memory.GPUInfo, len(gpuConfig))
	for i, gpu := range gpuConfig {
		gpuInfos[i] = memory.GPUInfo{
			ID:            gpu.ID,
			Index:         gpu.Index,
			Library:       gpu.Library,
			FreeMemory:    gpu.FreeMemory,
			TotalMemory:   gpu.TotalMemory,
			MinimumMemory: gpu.MinimumMemory,
		}

		// Debug GPU info
		log.Debug().
			Str("MEMORY_ESTIMATION_DEBUG", "gpu_info").
			Int("gpu_index", i).
			Str("gpu_id", gpu.ID).
			Uint64("free_memory_mb", gpu.FreeMemory/(1024*1024)).
			Uint64("total_memory_mb", gpu.TotalMemory/(1024*1024)).
			Msg("GPU configuration for estimation")
	}

	// Use our Ollama-based memory estimation logic
	estimate := memory.EstimateGPULayers(gpuInfos, metadata, opts)

	// Convert to EstimationResult format
	result := &memory.EstimationResult{
		ModelName:      metadata.Architecture, // Use architecture as model name
		Metadata:       metadata,
		SingleGPU:      estimate,
		Recommendation: "single_gpu",
		EstimatedAt:    time.Now(),
	}

	// If multiple GPUs, also calculate tensor parallel
	if len(gpuConfig) > 1 {
		result.TensorParallel = estimate
		result.Recommendation = "tensor_parallel"
	}

	// Determine final recommendation
	if estimate.RequiresFallback {
		result.Recommendation = "cpu_only"
		result.CPUOnly = estimate
		// Keep GPU estimates available for display purposes even when CPU-only is recommended
		// This allows dashboard to show actual VRAM requirements
	}

	return result
}

// generateCacheKey generates a cache key for the given parameters
func (s *MemoryEstimationService) generateCacheKey(modelName string, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) string {
	// Create a simple hash of the parameters
	key := fmt.Sprintf("%s_%d_%d_%d_%d_%s",
		modelName,
		len(gpuConfig),
		opts.NumCtx,
		opts.NumBatch,
		opts.NumParallel,
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
