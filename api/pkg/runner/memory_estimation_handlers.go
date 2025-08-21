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
	"github.com/rs/zerolog/log"
)

// ModelMetadataRequest represents a request for model GGUF metadata
type ModelMetadataRequest struct {
	ModelName    string `json:"model_name"`
	UseModelPath string `json:"use_model_path,omitempty"` // Optional: use specific model path
}

// ModelMetadataResponse represents the response with GGUF metadata
type ModelMetadataResponse struct {
	Success      bool                  `json:"success"`
	Metadata     *memory.ModelMetadata `json:"metadata,omitempty"`
	Error        string                `json:"error,omitempty"`
	RunnerID     string                `json:"runner_id"`
	ModelPath    string                `json:"model_path"`
	ResponseTime int64                 `json:"response_time_ms"`
}

// getModelMetadataHandler handles requests for model GGUF metadata only
func (apiServer *HelixRunnerAPIServer) getModelMetadataHandler(w http.ResponseWriter, r *http.Request) {
	startTime := getCurrentTimeMillis()

	var req ModelMetadataRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("error decoding model metadata request")
		apiServer.writeMetadataError(w, "Invalid request body: "+err.Error(), http.StatusBadRequest, startTime)
		return
	}

	log.Info().
		Str("model_name", req.ModelName).
		Str("runner_id", apiServer.runnerOptions.ID).
		Msg("received model metadata request")

	// Find the model file
	modelPath, err := apiServer.findModelFile(req.ModelName, req.UseModelPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Msg("failed to find model file")
		apiServer.writeMetadataError(w, fmt.Sprintf("Model not found: %v", err), http.StatusNotFound, startTime)
		return
	}

	// Load GGUF metadata only (no calculation)
	metadata, err := memory.LoadModelMetadata(modelPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_name", req.ModelName).
			Str("model_path", modelPath).
			Msg("failed to load model metadata")
		apiServer.writeMetadataError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError, startTime)
		return
	}

	responseTime := getCurrentTimeMillis() - startTime
	response := ModelMetadataResponse{
		Success:      true,
		Metadata:     metadata,
		RunnerID:     apiServer.runnerOptions.ID,
		ModelPath:    modelPath,
		ResponseTime: responseTime,
	}

	log.Info().
		Str("model_name", req.ModelName).
		Str("model_path", modelPath).
		Str("architecture", metadata.Architecture).
		Uint64("block_count", metadata.BlockCount).
		Int64("response_time_ms", responseTime).
		Msg("successfully extracted model metadata")

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding model metadata response")
	}
}

// writeMetadataError writes an error response for model metadata requests
func (apiServer *HelixRunnerAPIServer) writeMetadataError(w http.ResponseWriter, errorMsg string, statusCode int, startTime int64) {
	responseTime := getCurrentTimeMillis() - startTime
	response := ModelMetadataResponse{
		Success:      false,
		Error:        errorMsg,
		RunnerID:     apiServer.runnerOptions.ID,
		ResponseTime: responseTime,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error().Err(err).Msg("error encoding metadata error response")
	}
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

func getCurrentTimeMillis() int64 {
	return time.Now().UnixMilli()
}
