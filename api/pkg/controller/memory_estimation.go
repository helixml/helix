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
	// Convert request to internal format - use large GPU memory so it doesn't limit the calculation
	// The actual memory requirement is what matters, not whether it fits in a specific GPU
	gpuMemory := uint64(80 * 1024 * 1024 * 1024) // 80GB - large enough for any model

	numGPUs := 1
	if req.NumGPU > 1 {
		numGPUs = req.NumGPU
	} else if req.NumGPU == -1 {
		numGPUs = 2 // Default to 2 for auto-detect
	}

	gpuConfig := make([]types.GPUInfoForEstimation, numGPUs)
	for i := 0; i < numGPUs; i++ {
		gpuConfig[i] = types.GPUInfoForEstimation{
			Index:         i,
			Library:       "cuda",
			FreeMemory:    gpuMemory,
			TotalMemory:   gpuMemory,
			MinimumMemory: 512 * 1024 * 1024,
		}
	}

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

	opts := memory.EstimateOptions{
		NumCtx:      contextLength,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      req.NumGPU,
		KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
	}

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
	// Generate cache key
	cacheKey := s.generateCacheKey(modelName, gpuConfig, opts)

	// Check cache first
	if result := s.cache.get(cacheKey); result != nil {
		log.Debug().
			Str("model_name", modelName).
			Str("cache_key", cacheKey).
			Msg("returning cached memory estimation result")
		return result, nil
	}

	// Find a runner that has this model
	runnerID, err := s.findRunnerWithModel(ctx, modelName)
	if err != nil {
		return nil, fmt.Errorf("failed to find runner with model %s: %w", modelName, err)
	}

	// Call runner API for estimation via NATS
	result, err := s.callRunnerForEstimation(ctx, runnerID, modelName, gpuConfig, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to get estimation from runner %s: %w", runnerID, err)
	}

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

// PrewarmCache eagerly caches memory estimates for common configurations
func (s *MemoryEstimationService) PrewarmCache(ctx context.Context, modelNames []string) error {
	log.Info().
		Int("model_count", len(modelNames)).
		Msg("starting memory estimation cache prewarming")

	// Common GPU configurations to cache
	commonConfigs := [][]types.GPUInfoForEstimation{
		// Single GPU configurations
		{{
			ID:            "0",
			Index:         0,
			Library:       "cuda",
			FreeMemory:    24 * 1024 * 1024 * 1024, // 24GB
			TotalMemory:   24 * 1024 * 1024 * 1024,
			MinimumMemory: 512 * 1024 * 1024,
			Name:          "NVIDIA RTX 4090",
		}},
		{{
			ID:            "0",
			Index:         0,
			Library:       "cuda",
			FreeMemory:    80 * 1024 * 1024 * 1024, // 80GB
			TotalMemory:   80 * 1024 * 1024 * 1024,
			MinimumMemory: 512 * 1024 * 1024,
			Name:          "NVIDIA H100",
		}},
		// Dual GPU configurations
		{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    24 * 1024 * 1024 * 1024,
				TotalMemory:   24 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA RTX 4090",
			},
			{
				ID:            "1",
				Index:         1,
				Library:       "cuda",
				FreeMemory:    24 * 1024 * 1024 * 1024,
				TotalMemory:   24 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA RTX 4090",
			},
		},
		{
			{
				ID:            "0",
				Index:         0,
				Library:       "cuda",
				FreeMemory:    80 * 1024 * 1024 * 1024,
				TotalMemory:   80 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA H100",
			},
			{
				ID:            "1",
				Index:         1,
				Library:       "cuda",
				FreeMemory:    80 * 1024 * 1024 * 1024,
				TotalMemory:   80 * 1024 * 1024 * 1024,
				MinimumMemory: 512 * 1024 * 1024,
				Name:          "NVIDIA H100",
			},
		},
	}

	// Default estimation options
	opts := memory.EstimateOptions{
		NumCtx:      4096,
		NumBatch:    512,
		NumParallel: 1,
		NumGPU:      -1, // Auto
		KVCacheType: "f16",
	}

	// Preload estimates for each model and GPU configuration
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 5) // Limit concurrent requests

	for _, modelName := range modelNames {
		for _, gpuConfig := range commonConfigs {
			wg.Add(1)
			go func(model string, config []types.GPUInfoForEstimation) {
				defer wg.Done()

				semaphore <- struct{}{}        // Acquire
				defer func() { <-semaphore }() // Release

				_, err := s.EstimateModelMemory(ctx, model, config, opts)
				if err != nil {
					log.Warn().
						Err(err).
						Str("model", model).
						Int("gpu_count", len(config)).
						Msg("failed to preload memory estimation")
				} else {
					log.Debug().
						Str("model", model).
						Int("gpu_count", len(config)).
						Msg("preloaded memory estimation")
				}
			}(modelName, gpuConfig)
		}
	}

	wg.Wait()

	log.Info().
		Int("model_count", len(modelNames)).
		Int("config_count", len(commonConfigs)).
		Msg("completed memory estimation cache prewarming")

	return nil
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

// RunnerMemoryEstimationRequest represents a request for memory estimation sent to runner (duplicated to avoid import cycle)
type RunnerMemoryEstimationRequest struct {
	ModelName    string                       `json:"model_name"`
	GPUConfig    []types.GPUInfoForEstimation `json:"gpu_config"`
	Options      memory.EstimateOptions       `json:"options"`
	UseModelPath string                       `json:"use_model_path,omitempty"` // Optional: use specific model path
}

// RunnerMemoryEstimationResponse represents the response from memory estimation (duplicated to avoid import cycle)
type RunnerMemoryEstimationResponse struct {
	Success        bool                     `json:"success"`
	Result         *memory.EstimationResult `json:"result,omitempty"`
	Error          string                   `json:"error,omitempty"`
	RunnerID       string                   `json:"runner_id"`
	EstimationTime int64                    `json:"estimation_time_ms"`
}

// callRunnerForEstimation calls a runner's memory estimation API via NATS
func (s *MemoryEstimationService) callRunnerForEstimation(ctx context.Context, runnerID, modelName string, gpuConfig []types.GPUInfoForEstimation, opts memory.EstimateOptions) (*memory.EstimationResult, error) {
	// Prepare request
	request := RunnerMemoryEstimationRequest{
		ModelName: modelName,
		GPUConfig: gpuConfig,
		Options:   opts,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create NATS request
	req := &types.Request{
		Method: "POST",
		URL:    "/api/v1/memory-estimation",
		Body:   requestBody,
	}

	// Send request to runner via NATS
	resp, err := s.runnerSender.Send(ctx, runnerID, map[string]string{
		"Content-Type": "application/json",
	}, req, 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to runner: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("runner returned status %d", resp.StatusCode)
	}

	// Parse response
	var response RunnerMemoryEstimationResponse
	if err := json.Unmarshal(resp.Body, &response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("estimation failed: %s", response.Error)
	}

	return response.Result, nil
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

	// Get all models
	models, err := s.modelProvider.ListModels(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list models for cache refresh")
		return
	}

	// Common GPU configurations to pre-cache - focus on number of GPUs, not memory size
	gpuConfigs := []struct {
		name   string
		config []types.GPUInfoForEstimation
		opts   memory.EstimateOptions
	}{
		{
			name: "single_gpu",
			config: []types.GPUInfoForEstimation{
				{
					Index:         0,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024, // Large enough for any model
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
			},
			opts: memory.EstimateOptions{
				NumCtx:      4096,
				NumBatch:    512,
				NumParallel: 1,
				NumGPU:      1,
				KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
			},
		},
		{
			name: "dual_gpu",
			config: []types.GPUInfoForEstimation{
				{
					Index:         0,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024, // Large enough for any model
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
				{
					Index:         1,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024,
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
			},
			opts: memory.EstimateOptions{
				NumCtx:      4096,
				NumBatch:    512,
				NumParallel: 1,
				NumGPU:      2,
				KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
			},
		},
		{
			name: "quad_gpu",
			config: []types.GPUInfoForEstimation{
				{
					Index:         0,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024,
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
				{
					Index:         1,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024,
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
				{
					Index:         2,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024,
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
				{
					Index:         3,
					Library:       "cuda",
					FreeMemory:    80 * 1024 * 1024 * 1024,
					TotalMemory:   80 * 1024 * 1024 * 1024,
					MinimumMemory: 512 * 1024 * 1024,
				},
			},
			opts: memory.EstimateOptions{
				NumCtx:      4096,
				NumBatch:    512,
				NumParallel: 1,
				NumGPU:      4,
				KVCacheType: "q8_0", // Match Ollama's actual KV cache configuration
			},
		},
	}

	refreshCount := 0
	for _, model := range models {
		// Only refresh cache for Ollama models for now
		if model.Runtime != types.RuntimeOllama {
			continue
		}

		for _, gpuConfig := range gpuConfigs {
			// Check if we already have a fresh cache entry
			cacheKey := s.generateCacheKey(model.ID, gpuConfig.config, gpuConfig.opts)
			if entry := s.cache.get(cacheKey); entry != nil {
				// Entry exists and is still fresh, skip
				continue
			}

			// Estimate memory in background (don't block on individual model failures)
			go func(modelID string, config []types.GPUInfoForEstimation, opts memory.EstimateOptions, configName string) {
				_, err := s.EstimateModelMemory(ctx, modelID, config, opts)
				if err != nil {
					log.Debug().
						Err(err).
						Str("model_id", modelID).
						Str("config", configName).
						Msg("failed to refresh memory estimate for model")
				} else {
					log.Debug().
						Str("model_id", modelID).
						Str("config", configName).
						Msg("refreshed memory estimate for model")
				}
			}(model.ID, gpuConfig.config, gpuConfig.opts, gpuConfig.name)

			refreshCount++
		}
	}

	log.Info().
		Int("model_count", len(models)).
		Int("refresh_count", refreshCount).
		Msg("completed background cache refresh for memory estimates")
}
