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

	"github.com/helixml/helix/api/pkg/types"
)

// Mutex to protect global environment state during memory estimation
var memoryEstimationMutex sync.Mutex

// MemoryEstimationRequest represents a request for memory estimation using exact Ollama
type MemoryEstimationRequest struct {
	ModelName     string `json:"model_name"`
	ContextLength int    `json:"context_length"`
	BatchSize     int    `json:"batch_size"`
	NumParallel   int    `json:"num_parallel"`
}

// MemoryEstimationResponse contains memory estimates for different GPU configurations
type MemoryEstimationResponse struct {
	Success        bool                     `json:"success"`
	Error          string                   `json:"error,omitempty"`
	ModelName      string                   `json:"model_name"`
	ModelPath      string                   `json:"model_path"`
	Architecture   string                   `json:"architecture"`
	BlockCount     uint64                   `json:"block_count"`
	Configurations []GPUConfigurationResult `json:"configurations"`
	ResponseTime   int64                    `json:"response_time_ms"`
	RunnerID       string                   `json:"runner_id"`
}

// GPUConfigurationResult contains memory estimation for a specific GPU setup
type GPUConfigurationResult struct {
	Name          string   `json:"name"`           // "single_gpu", "dual_gpu", "cpu_only", etc.
	GPUCount      int      `json:"gpu_count"`      // Number of GPUs used
	LayersOnGPU   int      `json:"layers_on_gpu"`  // How many layers fit on GPU
	TotalLayers   int      `json:"total_layers"`   // Total layers in model
	VRAMRequired  uint64   `json:"vram_required"`  // VRAM needed in bytes
	TotalMemory   uint64   `json:"total_memory"`   // Total memory (VRAM + CPU) in bytes
	GraphMemory   uint64   `json:"graph_memory"`   // Graph computation memory in bytes
	KVCacheMemory uint64   `json:"kv_cache"`       // KV cache memory in bytes (estimated)
	WeightsMemory uint64   `json:"weights_memory"` // Model weights memory in bytes (estimated)
	FullyLoaded   bool     `json:"fully_loaded"`   // True if all layers fit on GPU
	GPUSizes      []uint64 `json:"gpu_sizes"`      // Memory per GPU in bytes
	TensorSplit   string   `json:"tensor_split"`   // Tensor split configuration
}

// getMemoryEstimationHandler handles memory estimation requests using exact Ollama code
func (apiServer *HelixRunnerAPIServer) getMemoryEstimationHandler(w http.ResponseWriter, r *http.Request) {
	startTime := getCurrentTimeMillis()

	var req MemoryEstimationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("error decoding memory estimation request")
		apiServer.writeEstimationError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest, startTime)
		return
	}

	log.Info().
		Str("model_name", req.ModelName).
		Int("context_length", req.ContextLength).
		Int("batch_size", req.BatchSize).
		Int("num_parallel", req.NumParallel).
		Str("runner_id", apiServer.runnerOptions.ID).
		Msg("received memory estimation request")

	// Validate request
	if err := apiServer.validateEstimationRequest(req); err != nil {
		apiServer.writeEstimationError(w, err.Error(), http.StatusBadRequest, startTime)
		return
	}

	// Find the model file using existing logic
	modelPath, err := apiServer.findModelFile(req.ModelName, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Msg("failed to find model file")
		apiServer.writeEstimationError(w, fmt.Sprintf("Model not found: %v", err), http.StatusNotFound, startTime)
		return
	}

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
	response := MemoryEstimationResponse{
		ModelName:    req.ModelName,
		ModelPath:    modelPath,
		Architecture: ggmlModel.KV().Architecture(),
		BlockCount:   ggmlModel.KV().BlockCount(),
		RunnerID:     apiServer.runnerOptions.ID,
	}

	log.Info().
		Str("model_name", req.ModelName).
		Str("architecture", response.Architecture).
		Uint64("block_count", response.BlockCount).
		Msg("model loaded successfully, calculating memory estimates")

	// Use mutex to protect global environment state from concurrent modifications
	memoryEstimationMutex.Lock()
	defer memoryEstimationMutex.Unlock()

	// Save original environment variables to restore after estimation
	origFlashAttn := os.Getenv("OLLAMA_FLASH_ATTENTION")
	origKVCacheType := os.Getenv("OLLAMA_KV_CACHE_TYPE")

	// Set environment variables to match actual runtime configuration
	os.Setenv("OLLAMA_FLASH_ATTENTION", "1")
	os.Setenv("OLLAMA_KV_CACHE_TYPE", types.DefaultKVCacheType)

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

	// Get available GPUs using Ollama's discovery
	allGPUs := discover.GetGPUInfo()

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
		opts := api.Options{
			Runner: api.Runner{
				NumCtx:   req.ContextLength,
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
			Int("num_ctx", opts.Runner.NumCtx).
			Int("num_batch", opts.Runner.NumBatch).
			Int("num_gpu", opts.Runner.NumGPU).
			Int("num_parallel", req.NumParallel).
			Str("architecture", response.Architecture).
			Uint64("block_count", response.BlockCount).
			Msg("🔧 MEMORY_DEBUG: About to call Ollama's EstimateGPULayers with these exact parameters")

		// Set the same environment variables that are used when actually running Ollama
		// This ensures our memory estimation accounts for flash attention and other optimizations
		log.Info().
			Str("MEMORY_ESTIMATION_DEBUG", "env_vars_set").
			Str("flash_attention", "1").
			Str("kv_cache_type", types.DefaultKVCacheType).
			Msg("🔧 MEMORY_DEBUG: Using runtime environment variables for estimation")

		// Use Ollama's exact EstimateGPULayers function
		estimate := llm.EstimateGPULayers(gpusToUse, ggmlModel, []string{}, opts, req.NumParallel)

		// DEBUG: Log what Ollama returned
		log.Info().
			Str("MEMORY_ESTIMATION_DEBUG", "ollama_response").
			Str("model_name", req.ModelName).
			Str("config", config.name).
			Int("layers", estimate.Layers).
			Uint64("vram_size", estimate.VRAMSize).
			Uint64("total_size", estimate.TotalSize).
			Uint64("graph", estimate.Graph).
			Interface("gpu_sizes", estimate.GPUSizes).
			Str("tensor_split", estimate.TensorSplit).
			Msg("🔧 MEMORY_DEBUG: Ollama's EstimateGPULayers returned these values")

		// Convert to our response format
		result := GPUConfigurationResult{
			Name:          config.name,
			GPUCount:      config.gpuCount,
			LayersOnGPU:   estimate.Layers,
			TotalLayers:   int(response.BlockCount) + 1, // +1 for output layer
			VRAMRequired:  estimate.VRAMSize,
			TotalMemory:   estimate.TotalSize,
			GraphMemory:   estimate.Graph,
			KVCacheMemory: estimateKVCache(estimate),
			WeightsMemory: estimateWeights(estimate),
			FullyLoaded:   estimate.Layers >= int(response.BlockCount)+1,
			GPUSizes:      estimate.GPUSizes,
			TensorSplit:   estimate.TensorSplit,
		}

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

	// Add CPU-only configuration
	cpuGPU := discover.GpuInfo{
		Library: "cpu",
	}
	cpuGPU.FreeMemory = 1024 * 1024 * 1024 * 1024 // 1TB fake memory
	cpuGPU.TotalMemory = 1024 * 1024 * 1024 * 1024
	cpuGPUs := []discover.GpuInfo{cpuGPU}

	cpuOpts := api.Options{
		Runner: api.Runner{
			NumCtx:   req.ContextLength,
			NumBatch: req.BatchSize,
			NumGPU:   0, // 0 = CPU only
		},
	}

	cpuEstimate := llm.EstimateGPULayers(cpuGPUs, ggmlModel, []string{}, cpuOpts, req.NumParallel)

	cpuResult := GPUConfigurationResult{
		Name:          "cpu_only",
		GPUCount:      0,
		LayersOnGPU:   0,
		TotalLayers:   int(response.BlockCount) + 1,
		VRAMRequired:  0,
		TotalMemory:   cpuEstimate.TotalSize,
		GraphMemory:   0,
		KVCacheMemory: 0,
		WeightsMemory: cpuEstimate.TotalSize,
		FullyLoaded:   true, // CPU can handle any size (just slowly)
		GPUSizes:      []uint64{},
		TensorSplit:   "",
	}

	response.Configurations = append(response.Configurations, cpuResult)
	response.Success = true
	response.ResponseTime = getCurrentTimeMillis() - startTime

	log.Info().
		Str("model_name", req.ModelName).
		Str("architecture", response.Architecture).
		Int("config_count", len(response.Configurations)).
		Int64("response_time_ms", response.ResponseTime).
		Msg("memory estimation completed successfully")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding memory estimation response")
	}
}

// validateEstimationRequest validates the memory estimation request
func (apiServer *HelixRunnerAPIServer) validateEstimationRequest(req MemoryEstimationRequest) error {
	if req.ModelName == "" {
		return fmt.Errorf("model_name is required")
	}

	if req.ContextLength < 1 || req.ContextLength > 1000000 {
		return fmt.Errorf("context_length must be between 1 and 1,000,000, got %d", req.ContextLength)
	}

	if req.BatchSize < 1 || req.BatchSize > 10000 {
		return fmt.Errorf("batch_size must be between 1 and 10,000, got %d", req.BatchSize)
	}

	if req.NumParallel < 1 || req.NumParallel > 100 {
		return fmt.Errorf("num_parallel must be between 1 and 100, got %d", req.NumParallel)
	}

	return nil
}

// writeEstimationError writes an error response for memory estimation requests
func (apiServer *HelixRunnerAPIServer) writeEstimationError(w http.ResponseWriter, errorMsg string, statusCode int, startTime int64) {
	responseTime := getCurrentTimeMillis() - startTime
	response := MemoryEstimationResponse{
		Success:      false,
		Error:        errorMsg,
		RunnerID:     apiServer.runnerOptions.ID,
		ResponseTime: responseTime,
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
