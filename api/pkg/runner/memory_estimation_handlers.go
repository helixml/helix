package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ollama/ollama/api"
	"github.com/ollama/ollama/discover"
	"github.com/ollama/ollama/fs/ggml"
	"github.com/ollama/ollama/llm"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
)

// Mutex to protect global environment state during memory estimation
var memoryEstimationMutex sync.Mutex

// MemoryEstimationRequest represents a request for memory estimation using exact Ollama
// MemoryEstimationRequest is now defined in types package to ensure consistency
// between API and runner sides

// MemoryEstimationResponse is now defined in types package for consistency

// GPUConfigurationResult contains memory estimation for a specific GPU setup
// GPUConfigurationResult is now MemoryEstimationConfiguration in types package for consistency

// getMemoryEstimationHandler handles memory estimation requests using exact Ollama code
func (apiServer *HelixRunnerAPIServer) getMemoryEstimationHandler(w http.ResponseWriter, r *http.Request) {
	startTime := getCurrentTimeMillis()

	var req types.MemoryEstimationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("RUNNER_DEBUG: error decoding memory estimation request")
		apiServer.writeEstimationError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest, startTime)
		return
	}

	log.Info().
		Str("model_name", req.ModelName).
		Int("context_length", req.ContextLength).
		Int("batch_size", req.BatchSize).
		Int("num_parallel", req.NumParallel).
		Str("runner_id", apiServer.runnerOptions.ID).
		Msg("RUNNER_DEBUG: received memory estimation request with full parameters")

	// Validate request
	log.Debug().
		Str("model_name", req.ModelName).
		Msg("RUNNER_DEBUG: starting request validation")
	if err := apiServer.validateEstimationRequest(req); err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Int("context_length", req.ContextLength).
			Int("batch_size", req.BatchSize).
			Int("num_parallel", req.NumParallel).
			Msg("RUNNER_DEBUG: validation failed for memory estimation request")
		apiServer.writeEstimationError(w, err.Error(), http.StatusBadRequest, startTime)
		return
	}
	log.Debug().
		Str("model_name", req.ModelName).
		Msg("RUNNER_DEBUG: validation passed")

	// Find the model file using existing logic
	log.Debug().
		Str("model_name", req.ModelName).
		Msg("RUNNER_DEBUG: looking for model file")
	modelPath, err := apiServer.findModelFile(req.ModelName, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Msg("RUNNER_DEBUG: failed to find model file")
		apiServer.writeEstimationError(w, fmt.Sprintf("Model not found: %v", err), http.StatusNotFound, startTime)
		return
	}
	log.Debug().
		Str("model_name", req.ModelName).
		Str("model_path", modelPath).
		Msg("RUNNER_DEBUG: found model file")

	// Load model using Ollama's exact GGUF parser
	file, err := os.Open(modelPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_path", modelPath).
			Msg("failed to open model file")
		apiServer.writeEstimationError(w, fmt.Sprintf("Failed to open model: %v", err), http.StatusInternalServerError, startTime)
		return
	}
	defer file.Close()

	ggmlModel, err := ggml.Decode(file, 1024)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_path", modelPath).
			Msg("failed to decode GGUF using Ollama parser")
		apiServer.writeEstimationError(w, fmt.Sprintf("Failed to decode GGUF: %v", err), http.StatusInternalServerError, startTime)
		return
	}

	// Prepare response
	response := types.MemoryEstimationResponse{
		ModelName:    req.ModelName,
		ModelPath:    modelPath,
		Architecture: ggmlModel.KV().Architecture(),
		BlockCount:   int(ggmlModel.KV().BlockCount()),
		RunnerID:     apiServer.runnerOptions.ID,
	}

	log.Info().
		Str("model_name", req.ModelName).
		Str("architecture", response.Architecture).
		Int("block_count", response.BlockCount).
		Msg("model loaded successfully, calculating memory estimates")

	// Use mutex to protect global environment state from concurrent modifications
	memoryEstimationMutex.Lock()
	defer memoryEstimationMutex.Unlock()

	// Save original environment variables to restore after estimation
	origFlashAttn := os.Getenv("OLLAMA_FLASH_ATTENTION")
	origKVCacheType := os.Getenv("OLLAMA_KV_CACHE_TYPE")

	// Set environment variables to match actual runtime configuration
	os.Setenv("OLLAMA_FLASH_ATTENTION", "1")
	os.Setenv("OLLAMA_KV_CACHE_TYPE", memory.DefaultKVCacheType)

	// Restore original environment variables when function exits
	defer func() {
		if origFlashAttn == "" {
			os.Unsetenv("OLLAMA_FLASH_ATTENTION")
		} else {
			os.Setenv("OLLAMA_FLASH_ATTENTION", origFlashAttn)
		}
		if origKVCacheType == "" {
			os.Unsetenv("OLLAMA_KV_CACHE_TYPE")
		} else {
			os.Setenv("OLLAMA_KV_CACHE_TYPE", origKVCacheType)
		}
	}()

	// Use standard GPU configurations for theoretical memory estimation
	// This ensures consistent results independent of actual hardware state during startup
	log.Info().
		Str("MEMORY_ESTIMATION_DEBUG", "using_standard_gpus").
		Msg("🔧 MEMORY_DEBUG: Using standard GPU configs instead of actual GPU discovery for theoretical estimation")

	// Create standard GPU configurations with 80GB memory each
	allGPUs := make([]discover.GpuInfo, 8) // Support up to 8 GPUs
	for i := 0; i < 8; i++ {
		gpu := discover.GpuInfo{
			Library:       "cuda",
			MinimumMemory: 512 * 1024 * 1024, // 512MB minimum
			ID:            fmt.Sprintf("standard-gpu-%d", i),
			Name:          fmt.Sprintf("Standard GPU %d", i),
		}
		// Set memory info through embedded struct
		gpu.TotalMemory = 80 * 1024 * 1024 * 1024 // 80GB total
		gpu.FreeMemory = 80 * 1024 * 1024 * 1024  // 80GB free
		allGPUs[i] = gpu
	}

	// Test different GPU configurations
	configs := []struct {
		name     string
		gpuCount int
	}{
		{"single_gpu", 1},
		{"dual_gpu", 2},
		{"quad_gpu", 4},
		{"octo_gpu", 8},
	}

	for _, config := range configs {
		// Skip if we don't have enough GPUs
		if config.gpuCount > len(allGPUs) {
			continue
		}

		// Create GPU slice for this configuration
		gpusToUse := allGPUs[:config.gpuCount]

		// Create Ollama options
		// CRITICAL: Multiply context length by numParallel because Ollama allocates numParallel * contextLength total context
		adjustedContextLength := req.ContextLength * req.NumParallel
		opts := api.Options{
			Runner: api.Runner{
				NumCtx:   adjustedContextLength,
				NumBatch: req.BatchSize,
				NumGPU:   -1, // -1 = auto-detect max layers (NOT GPU count!)
			},
		}

		// DEBUG: Log exact parameters being passed to Ollama
		log.Info().
			Str("MEMORY_ESTIMATION_DEBUG", "ollama_params").
			Str("model_name", req.ModelName).
			Str("config", config.name).
			Int("gpu_count", config.gpuCount).
			Int("original_context_length", req.ContextLength).
			Int("adjusted_context_length", adjustedContextLength).
			Int("num_ctx", opts.Runner.NumCtx).
			Int("num_batch", opts.Runner.NumBatch).
			Int("num_gpu", opts.Runner.NumGPU).
			Int("num_parallel", req.NumParallel).
			Str("architecture", response.Architecture).
			Int("block_count", response.BlockCount).
			Msg("🔧 MEMORY_DEBUG: About to call Ollama's EstimateGPULayers with adjusted context length (original * numParallel)")

		// Set the same environment variables that are used when actually running Ollama
		// This ensures our memory estimation accounts for flash attention and other optimizations
		log.Info().
			Str("MEMORY_ESTIMATION_DEBUG", "env_vars_set").
			Str("flash_attention", "1").
			Str("kv_cache_type", memory.DefaultKVCacheType).
			Msg("🔧 MEMORY_DEBUG: Using runtime environment variables for estimation")

		// Use Ollama's exact EstimateGPULayers function
		estimate := llm.EstimateGPULayers(gpusToUse, ggmlModel, []string{}, opts, req.NumParallel)

		// Log raw Ollama estimation result
		log.Info().
			Str("MEMORY_DEBUG", "raw_ollama_estimation").
			Str("model_name", req.ModelName).
			Str("config", config.name).
			Int("gpu_count", config.gpuCount).
			Int("layers", estimate.Layers).
			Uint64("total_size_bytes", estimate.TotalSize).
			Uint64("total_size_gb", estimate.TotalSize/(1024*1024*1024)).
			Float64("total_size_gib", float64(estimate.TotalSize)/(1024*1024*1024)).
			Uint64("vram_size_bytes", estimate.VRAMSize).
			Uint64("vram_size_gb", estimate.VRAMSize/(1024*1024*1024)).
			Float64("vram_size_gib", float64(estimate.VRAMSize)/(1024*1024*1024)).
			Uint64("graph_bytes", estimate.Graph).
			Uint64("graph_gb", estimate.Graph/(1024*1024*1024)).
			Str("tensor_split", estimate.TensorSplit).
			Interface("gpu_sizes", estimate.GPUSizes).
			Msg("🔥 MEMORY_DEBUG: Raw Ollama estimation result")

		// Convert to our response format using shared struct
		result := types.MemoryEstimationConfiguration{
			Name:          config.name,
			GPUCount:      config.gpuCount,
			GPUSizes:      estimate.GPUSizes,
			TotalMemory:   estimate.TotalSize,
			VRAMRequired:  estimate.VRAMSize,
			WeightsMemory: estimateWeights(estimate),
			KVCache:       estimateKVCache(estimate),
			GraphMemory:   estimate.Graph,
			TensorSplit:   estimate.TensorSplit,
			LayersOnGPU:   estimate.Layers,
			TotalLayers:   response.BlockCount + 1, // +1 for output layer
			FullyLoaded:   estimate.Layers >= response.BlockCount+1,
		}

		// Log the converted result
		log.Info().
			Str("MEMORY_DEBUG", "converted_result").
			Str("model_name", req.ModelName).
			Str("config", config.name).
			Int("gpu_count", config.gpuCount).
			Uint64("result_total_memory_bytes", result.TotalMemory).
			Uint64("result_total_memory_gb", result.TotalMemory/(1024*1024*1024)).
			Float64("result_total_memory_gib", float64(result.TotalMemory)/(1024*1024*1024)).
			Uint64("result_vram_required_bytes", result.VRAMRequired).
			Uint64("result_vram_required_gb", result.VRAMRequired/(1024*1024*1024)).
			Float64("result_vram_required_gib", float64(result.VRAMRequired)/(1024*1024*1024)).
			Uint64("kv_cache_memory", result.KVCache).
			Uint64("weights_memory", result.WeightsMemory).
			Bool("fully_loaded", result.FullyLoaded).
			Msg("🔥 MEMORY_DEBUG: Converted MemoryEstimationConfiguration")

		response.Configurations = append(response.Configurations, result)

		log.Info().
			Str("config", config.name).
			Int("gpu_count", config.gpuCount).
			Int("layers_on_gpu", result.LayersOnGPU).
			Int("total_layers", result.TotalLayers).
			Uint64("vram_gb", result.VRAMRequired/(1024*1024*1024)).
			Uint64("total_gb", result.TotalMemory/(1024*1024*1024)).
			Bool("fully_loaded", result.FullyLoaded).
			Msg("memory estimation result")
	}

	// CPU-only estimation disabled - not properly supported and adds confusion
	// We only support GPU-based inference, so CPU estimates are misleading
	log.Debug().
		Str("model_name", req.ModelName).
		Msg("Skipping CPU-only estimation - not supported")
	response.Success = true
	response.ResponseTimeMs = getCurrentTimeMillis() - startTime

	// Log final response summary
	log.Info().
		Str("MEMORY_DEBUG", "final_response").
		Str("model_name", req.ModelName).
		Str("architecture", response.Architecture).
		Int("config_count", len(response.Configurations)).
		Int64("response_time_ms", response.ResponseTimeMs).
		Interface("all_configurations", response.Configurations).
		Msg("🔥 MEMORY_DEBUG: Final memory estimation response")

	log.Info().
		Str("model_name", req.ModelName).
		Str("architecture", response.Architecture).
		Int("config_count", len(response.Configurations)).
		Int64("response_time_ms", response.ResponseTimeMs).
		Msg("memory estimation completed successfully")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding memory estimation response")
	}
}

// validateEstimationRequest validates the memory estimation request
func (apiServer *HelixRunnerAPIServer) validateEstimationRequest(req types.MemoryEstimationRequest) error {
	if req.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}

	if req.ContextLength < 1 || req.ContextLength > 1000000 {
		log.Error().
			Str("model_name", req.ModelName).
			Int("context_length", req.ContextLength).
			Msg("RUNNER_DEBUG: context_length validation failed")
		return fmt.Errorf("context_length must be between 1 and 1,000,000, got %d", req.ContextLength)
	}

	if req.BatchSize < 1 || req.BatchSize > 10000 {
		log.Error().
			Str("model_name", req.ModelName).
			Int("batch_size", req.BatchSize).
			Msg("RUNNER_DEBUG: batch_size validation failed")
		return fmt.Errorf("batch_size must be between 1 and 10,000, got %d", req.BatchSize)
	}

	if req.NumParallel < 1 || req.NumParallel > 100 {
		log.Error().
			Str("model_name", req.ModelName).
			Int("num_parallel", req.NumParallel).
			Msg("RUNNER_DEBUG: num_parallel validation failed")
		return fmt.Errorf("num_parallel must be between 1 and 100, got %d", req.NumParallel)
	}

	return nil
}

// writeEstimationError writes an error response for memory estimation requests
func (apiServer *HelixRunnerAPIServer) writeEstimationError(w http.ResponseWriter, errorMsg string, statusCode int, startTime int64) {
	responseTime := getCurrentTimeMillis() - startTime
	response := types.MemoryEstimationResponse{
		Success:        false,
		Error:          errorMsg,
		RunnerID:       apiServer.runnerOptions.ID,
		ResponseTimeMs: responseTime,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding memory estimation error response")
	}
}

// Helper functions to extract memory components from Ollama's MemoryEstimate
func estimateKVCache(estimate llm.MemoryEstimate) uint64 {
	// Ollama's MemoryEstimate doesn't expose internal KV cache directly
	// Estimate as portion of VRAM minus graph memory
	if estimate.VRAMSize > estimate.Graph {
		// KV cache is typically 10-30% of total VRAM for large contexts
		return (estimate.VRAMSize - estimate.Graph) / 4
	}
	return 0
}

func estimateWeights(estimate llm.MemoryEstimate) uint64 {
	// Model weights are the bulk of the memory
	if estimate.TotalSize > estimate.Graph {
		kvEstimate := estimateKVCache(estimate)
		weights := estimate.TotalSize - estimate.Graph - kvEstimate
		if weights > 0 {
			return weights
		}
	}
	// Fallback: assume weights are 70% of total
	return estimate.TotalSize * 7 / 10
}

func getCurrentTimeMillis() int64 {
	return time.Now().UnixMilli()
}

// Helper functions for model file discovery
func (apiServer *HelixRunnerAPIServer) findModelFile(modelName, explicitPath string) (string, error) {
	// If explicit path is provided, use it
	if explicitPath != "" {
		return explicitPath, nil
	}

	// Check if this runner has the model
	if !apiServer.hasModel(modelName) {
		return "", fmt.Errorf("runner does not have model %s", modelName)
	}

	// Parse the model name to get registry/library/model format
	manifestPath := apiServer.getManifestPath(modelName)

	// Read the manifest file
	modelBlobDigest, err := apiServer.parseModelManifest(manifestPath)
	if err != nil {
		log.Debug().
			Err(err).
			Str("model_name", modelName).
			Str("manifest_path", manifestPath).
			Msg("failed to parse manifest, trying fallback method")
		return apiServer.findModelFileFallback(modelName)
	}

	// Construct the blob path
	blobPath := filepath.Join("/root/.cache/huggingface/blobs", modelBlobDigest)

	// Verify the blob exists and is a model file
	if info, err := os.Stat(blobPath); err == nil && info.Size() > 100*1024*1024 {
		log.Debug().
			Str("model_name", modelName).
			Str("blob_path", blobPath).
			Str("blob_digest", modelBlobDigest).
			Msg("found model blob via manifest")
		return blobPath, nil
	}

	// Fallback to searching for files
	return apiServer.findModelFileFallback(modelName)
}

func (apiServer *HelixRunnerAPIServer) getManifestPath(modelName string) string {
	parts := strings.Split(modelName, ":")
	if len(parts) != 2 {
		parts = []string{modelName, "latest"}
	}

	nameWithTag := parts[0]
	tag := parts[1]

	if !strings.Contains(nameWithTag, "/") {
		nameWithTag = "library/" + nameWithTag
	}
	if !strings.HasPrefix(nameWithTag, "registry.ollama.ai/") {
		nameWithTag = "registry.ollama.ai/" + nameWithTag
	}

	return filepath.Join("/root/.cache/huggingface/manifests", nameWithTag, tag)
}

func (apiServer *HelixRunnerAPIServer) parseModelManifest(manifestPath string) (string, error) {
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest struct {
		Layers []struct {
			MediaType string `json:"mediaType"`
			Digest    string `json:"digest"`
			Size      int64  `json:"size"`
		} `json:"layers"`
	}

	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return "", fmt.Errorf("failed to parse manifest JSON: %w", err)
	}

	var modelDigest string
	var maxSize int64

	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.ollama.image.model" && layer.Size > maxSize {
			maxSize = layer.Size
			if parts := strings.Split(layer.Digest, ":"); len(parts) == 2 {
				modelDigest = "sha256-" + parts[1]
			}
		}
	}

	if modelDigest == "" {
		return "", fmt.Errorf("no model layer found in manifest")
	}

	return modelDigest, nil
}

func (apiServer *HelixRunnerAPIServer) findModelFileFallback(modelName string) (string, error) {
	possibleBasePaths := []string{
		"/root/.cache/huggingface/blobs",
		"/root/.ollama/models/blobs",
		"/tmp/ollama/models/blobs",
	}

	for _, basePath := range possibleBasePaths {
		if files, err := filepath.Glob(filepath.Join(basePath, "sha256-*")); err == nil {
			for _, file := range files {
				if apiServer.isLikelyModelFile(file) {
					log.Debug().
						Str("model_name", modelName).
						Str("found_path", file).
						Msg("found potential model file via fallback")
					return file, nil
				}
			}
		}
	}

	return "", fmt.Errorf("model file not found for %s", modelName)
}

func (apiServer *HelixRunnerAPIServer) hasModel(modelName string) bool {
	apiServer.modelsMu.Lock()
	defer apiServer.modelsMu.Unlock()

	for _, model := range apiServer.models {
		if model.ID == modelName {
			return true
		}
	}
	return false
}

func (apiServer *HelixRunnerAPIServer) isLikelyModelFile(path string) bool {
	if info, err := os.Stat(path); err == nil {
		return info.Size() > 100*1024*1024
	}
	return false
}
