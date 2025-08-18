package server

import (
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// estimateModelMemory godoc
// @Summary Estimate model memory requirements
// @Description Estimate memory requirements for a model on different GPU configurations
// @Tags models
// @Param model_id path string true "Model ID"
// @Param num_gpu query int false "Number of GPUs (default: auto-detect)"
// @Param context_length query int false "Context length (default: model default)"
// @Param batch_size query int false "Batch size (default: 512)"
// @Success 200 {object} controller.MemoryEstimationResponse
// @Failure 400 {string} string "Invalid request parameters"
// @Failure 404 {string} string "Model not found"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/helix-models/{model_id}/memory-estimate [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) estimateModelMemory(rw http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	modelID := vars["model_id"]

	if modelID == "" {
		http.Error(rw, "model_id is required", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	numGPU := -1 // Auto-detect by default
	if numGPUStr := r.URL.Query().Get("num_gpu"); numGPUStr != "" {
		var err error
		numGPU, err = strconv.Atoi(numGPUStr)
		if err != nil {
			http.Error(rw, "invalid num_gpu parameter", http.StatusBadRequest)
			return
		}
	}

	contextLength := 0 // Use model default
	if contextLengthStr := r.URL.Query().Get("context_length"); contextLengthStr != "" {
		var err error
		contextLength, err = strconv.Atoi(contextLengthStr)
		if err != nil {
			http.Error(rw, "invalid context_length parameter", http.StatusBadRequest)
			return
		}
	}

	batchSize := 512 // Default batch size
	if batchSizeStr := r.URL.Query().Get("batch_size"); batchSizeStr != "" {
		var err error
		batchSize, err = strconv.Atoi(batchSizeStr)
		if err != nil {
			http.Error(rw, "invalid batch_size parameter", http.StatusBadRequest)
			return
		}
	}

	// Get memory estimation service
	memoryService := apiServer.Controller.GetMemoryEstimationService()
	if memoryService == nil {
		log.Error().Msg("memory estimation service not available")
		http.Error(rw, "memory estimation service not available", http.StatusServiceUnavailable)
		return
	}

	// Create estimation request
	req := &controller.MemoryEstimationRequest{
		ModelID:       modelID,
		NumGPU:        numGPU,
		ContextLength: contextLength,
		BatchSize:     batchSize,
	}

	// Estimate memory requirements
	resp, err := memoryService.EstimateModelMemoryFromRequest(r.Context(), req)
	if err != nil {
		log.Error().
			Err(err).
			Str("model_id", modelID).
			Msg("failed to estimate model memory")
		http.Error(rw, "failed to estimate model memory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(rw, resp, http.StatusOK)
}

// listModelMemoryEstimates godoc
// @Summary List memory estimates for multiple models
// @Description Get memory estimates for multiple models with different GPU configurations
// @Tags models
// @Param model_ids query string false "Comma-separated list of model IDs"
// @Param num_gpu query int false "Number of GPUs (default: auto-detect)"
// @Success 200 {array} controller.MemoryEstimationResponse
// @Failure 400 {string} string "Invalid request parameters"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/helix-models/memory-estimates [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listModelMemoryEstimates(rw http.ResponseWriter, r *http.Request) {
	modelIDsStr := r.URL.Query().Get("model_ids")
	if modelIDsStr == "" {
		// If no specific models requested, get estimates for all available Ollama models only
		models, err := apiServer.Store.ListModels(r.Context(), &store.ListModelsQuery{
			Runtime: types.RuntimeOllama,
		})
		if err != nil {
			log.Error().Err(err).Msg("failed to list models")
			http.Error(rw, "failed to list models: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var modelIDs []string
		for _, model := range models {
			modelIDs = append(modelIDs, model.ID)
		}
		modelIDsStr = ""
		for i, id := range modelIDs {
			if i > 0 {
				modelIDsStr += ","
			}
			modelIDsStr += id
		}
	}

	// Parse query parameters
	numGPU := -1 // Auto-detect by default
	if numGPUStr := r.URL.Query().Get("num_gpu"); numGPUStr != "" {
		var err error
		numGPU, err = strconv.Atoi(numGPUStr)
		if err != nil {
			http.Error(rw, "invalid num_gpu parameter", http.StatusBadRequest)
			return
		}
	}

	// Get memory estimation service
	memoryService := apiServer.Controller.GetMemoryEstimationService()
	if memoryService == nil {
		log.Error().Msg("memory estimation service not available")
		http.Error(rw, "memory estimation service not available", http.StatusServiceUnavailable)
		return
	}

	// Split model IDs and create requests
	modelIDs := []string{}
	if modelIDsStr != "" {
		for _, id := range splitAndTrim(modelIDsStr, ",") {
			if id != "" {
				modelIDs = append(modelIDs, id)
			}
		}
	}

	responses := make([]*controller.MemoryEstimationResponse, 0, len(modelIDs))

	// Estimate memory for each model
	for _, modelID := range modelIDs {
		req := &controller.MemoryEstimationRequest{
			ModelID: modelID,
			NumGPU:  numGPU,
		}

		resp, err := memoryService.EstimateModelMemoryFromRequest(r.Context(), req)
		if err != nil {
			log.Warn().
				Err(err).
				Str("model_id", modelID).
				Msg("failed to estimate memory for model, skipping")
			continue
		}

		responses = append(responses, resp)
	}

	writeResponse(rw, responses, http.StatusOK)
}

// Helper function to split and trim strings
func splitAndTrim(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := make([]string, 0)
	for _, part := range splitString(s, sep) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}
	parts := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
