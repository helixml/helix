package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/memory"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// MemoryEstimationRequest represents a request for memory estimation
type MemoryEstimationRequest struct {
	ModelName       string                    `json:"model_name"`
	GPUConfig       []types.GPUInfoForEstimation `json:"gpu_config"`
	Options         memory.EstimateOptions    `json:"options"`
	UseModelPath    string                    `json:"use_model_path,omitempty"` // Optional: use specific model path
}

// MemoryEstimationResponse represents the response from memory estimation
type MemoryEstimationResponse struct {
	Success         bool                    `json:"success"`
	Result          *memory.EstimationResult `json:"result,omitempty"`
	Error           string                  `json:"error,omitempty"`
	RunnerID        string                  `json:"runner_id"`
	EstimationTime  int64                   `json:"estimation_time_ms"`
}

// estimateModelMemoryHandler handles requests for model memory estimation
func (apiServer *HelixRunnerAPIServer) estimateModelMemoryHandler(w http.ResponseWriter, r *http.Request) {
	startTime := getCurrentTimeMillis()
	
	var req MemoryEstimationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("error decoding memory estimation request")
		apiServer.writeMemoryEstimationError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest, startTime)
		return
	}

	log.Info().
		Str("model_name", req.ModelName).
		Int("gpu_count", len(req.GPUConfig)).
		Str("runner_id", apiServer.runnerOptions.ID).
		Msg("received memory estimation request")

	// Find the model file
	modelPath, err := apiServer.findModelFile(req.ModelName, req.UseModelPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Msg("failed to find model file")
		apiServer.writeMemoryEstimationError(w, fmt.Sprintf("Model not found: %v", err), http.StatusNotFound, startTime)
		return
	}

	// Convert GPU config to memory estimation format
	gpuInfos := convertGPUConfigToMemoryFormat(req.GPUConfig)

	// Validate estimation options
	if err := memory.ValidateEstimateOptions(req.Options); err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Msg("invalid estimation options")
		apiServer.writeMemoryEstimationError(w, fmt.Sprintf("Invalid options: %v", err), http.StatusBadRequest, startTime)
		return
	}

	// Perform memory estimation
	result, err := memory.EstimateModelMemory(modelPath, gpuInfos, req.Options)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Str("model_path", modelPath).
			Msg("memory estimation failed")
		apiServer.writeMemoryEstimationError(w, fmt.Sprintf("Estimation failed: %v", err), http.StatusInternalServerError, startTime)
		return
	}

	// Add model name to result
	result.ModelName = req.ModelName
	result.ModelPath = modelPath

	response := MemoryEstimationResponse{
		Success:        true,
		Result:         result,
		RunnerID:       apiServer.runnerOptions.ID,
		EstimationTime: getCurrentTimeMillis() - startTime,
	}

	log.Info().
		Str("model_name", req.ModelName).
		Str("runner_id", apiServer.runnerOptions.ID).
		Str("recommendation", result.Recommendation).
		Int64("estimation_time_ms", response.EstimationTime).
		Msg("memory estimation completed successfully")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding memory estimation response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeMemoryEstimationError writes an error response for memory estimation
func (apiServer *HelixRunnerAPIServer) writeMemoryEstimationError(w http.ResponseWriter, errorMsg string, statusCode int, startTime int64) {
	response := MemoryEstimationResponse{
		Success:        false,
		Error:          errorMsg,
		RunnerID:       apiServer.runnerOptions.ID,
		EstimationTime: getCurrentTimeMillis() - startTime,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding memory estimation error response")
	}
}

// findModelFile finds the model file path for a given model name using Ollama's manifest system
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

// getManifestPath constructs the manifest path for a model
func (apiServer *HelixRunnerAPIServer) getManifestPath(modelName string) string {
	// Handle different model name formats
	// Examples: "qwen3:8b", "library/qwen3:8b", "registry.ollama.ai/library/qwen3:8b"
	
	parts := strings.Split(modelName, ":")
	if len(parts) != 2 {
		// Default tag if not specified
		parts = []string{modelName, "latest"}
	}
	
	nameWithTag := parts[0]
	tag := parts[1]
	
	// Handle registry/library prefix
	if !strings.Contains(nameWithTag, "/") {
		nameWithTag = "library/" + nameWithTag
	}
	if !strings.HasPrefix(nameWithTag, "registry.ollama.ai/") {
		nameWithTag = "registry.ollama.ai/" + nameWithTag
	}
	
	return filepath.Join("/root/.cache/huggingface/manifests", nameWithTag, tag)
}

// parseModelManifest parses an Ollama manifest file to extract the model blob digest
func (apiServer *HelixRunnerAPIServer) parseModelManifest(manifestPath string) (string, error) {
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return "", fmt.Errorf("failed to read manifest: %w", err)
	}
	
	// Parse the manifest JSON
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
	
	// Find the model layer (largest layer, typically the GGUF file)
	var modelDigest string
	var maxSize int64
	
	for _, layer := range manifest.Layers {
		if layer.MediaType == "application/vnd.ollama.image.model" && layer.Size > maxSize {
			maxSize = layer.Size
			// Extract the hash from the digest (format: "sha256:hash")
			if parts := strings.Split(layer.Digest, ":"); len(parts) == 2 {
				modelDigest = "sha256-" + parts[1] // Ollama uses sha256-hash format for blob names
			}
		}
	}
	
	if modelDigest == "" {
		return "", fmt.Errorf("no model layer found in manifest")
	}
	
	return modelDigest, nil
}

// findModelFileFallback tries to find model files without using manifests
func (apiServer *HelixRunnerAPIServer) findModelFileFallback(modelName string) (string, error) {
	// Ollama model storage paths in Helix runner
	possibleBasePaths := []string{
		"/root/.cache/huggingface/blobs", // Primary blob storage
		"/root/.ollama/models/blobs",     // Fallback for standard Ollama
		"/tmp/ollama/models/blobs",       // For development
	}

	// Try to find the model file
	for _, basePath := range possibleBasePaths {
		// List files in the blob directory and look for GGUF files
		if files, err := filepath.Glob(filepath.Join(basePath, "sha256-*")); err == nil {
			for _, file := range files {
				if isLikelyModelFile(file) {
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

// hasModel checks if this runner has the specified model
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

// isLikelyModelFile checks if a file is likely to be a model file
func isLikelyModelFile(path string) bool {
	// Check file size - model files are typically large
	if info, err := os.Stat(path); err == nil {
		// Model files are typically > 100MB
		if info.Size() > 100*1024*1024 {
			return true
		}
	}
	return false
}

// convertGPUConfigToMemoryFormat converts GPU config from types to memory format
func convertGPUConfigToMemoryFormat(gpuConfig []types.GPUInfoForEstimation) []memory.GPUInfo {
	var result []memory.GPUInfo
	for _, gpu := range gpuConfig {
		memoryGPU := memory.GPUInfo{
			ID:            gpu.ID,
			Index:         gpu.Index,
			Library:       gpu.Library,
			FreeMemory:    gpu.FreeMemory,
			TotalMemory:   gpu.TotalMemory,
			MinimumMemory: gpu.MinimumMemory,
			Name:          gpu.Name,
		}
		result = append(result, memoryGPU)
	}
	return result
}

// getCurrentTimeMillis returns current time in milliseconds
func getCurrentTimeMillis() int64 {
	return time.Now().UnixMilli()
}

// getAvailableModelsForEstimationHandler returns models available for estimation on this runner
func (apiServer *HelixRunnerAPIServer) getAvailableModelsForEstimationHandler(w http.ResponseWriter, r *http.Request) {
	apiServer.modelsMu.Lock()
	models := make([]*types.Model, len(apiServer.models))
	copy(models, apiServer.models)
	apiServer.modelsMu.Unlock()

	response := struct {
		Success  bool            `json:"success"`
		Models   []*types.Model  `json:"models"`
		RunnerID string          `json:"runner_id"`
	}{
		Success:  true,
		Models:   models,
		RunnerID: apiServer.runnerOptions.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding available models response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
