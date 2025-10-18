package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/rs/zerolog/log"
)

// startZedPlanningSession starts a new Zed planning session
func (apiServer *HelixAPIServer) startZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	var request services.ZedPlanningRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.SpecTaskID == "" {
		http.Error(w, "SpecTask ID is required", http.StatusBadRequest)
		return
	}
	if request.ProjectName == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}
	if request.OwnerID == "" {
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
		return
	}
	if request.Requirements == "" {
		http.Error(w, "Requirements are required", http.StatusBadRequest)
		return
	}

	// Validate repository specification
	if request.RepositoryID == "" && request.RepositoryURL == "" && !request.CreateRepo {
		http.Error(w, "Must specify repository_id, repository_url, or create_repo=true", http.StatusBadRequest)
		return
	}

	// Start planning session
	result, err := apiServer.zedPlanningService.StartPlanningSession(r.Context(), &request)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to start planning session")
		http.Error(w, fmt.Sprintf("Failed to start planning session: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// getZedPlanningSession retrieves planning session information by ID
func (apiServer *HelixAPIServer) getZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		http.Error(w, "Planning session ID is required", http.StatusBadRequest)
		return
	}

	session, err := apiServer.zedPlanningService.GetPlanningSession(r.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get planning session")
		http.Error(w, fmt.Sprintf("Planning session not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// listZedPlanningSessions lists all planning sessions for the authenticated user
func (apiServer *HelixAPIServer) listZedPlanningSessions(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	status := r.URL.Query().Get("status")

	// If no owner_id specified, try to get from authenticated user
	if ownerID == "" {
		// This would typically come from authentication middleware
		// For now, return error
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
		return
	}

	sessions, err := apiServer.zedPlanningService.ListPlanningSessions(r.Context(), ownerID)
	if err != nil {
		log.Error().Err(err).Str("owner_id", ownerID).Msg("Failed to list planning sessions")
		http.Error(w, fmt.Sprintf("Failed to list planning sessions: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Filter by status if specified
	if status != "" {
		filtered := make([]*services.ZedPlanningSession, 0)
		for _, session := range sessions {
			if string(session.Status) == status {
				filtered = append(filtered, session)
			}
		}
		sessions = filtered
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sessions)
}

// completeZedPlanning completes a planning session (approve or reject)
func (apiServer *HelixAPIServer) completeZedPlanning(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		http.Error(w, "Planning session ID is required", http.StatusBadRequest)
		return
	}

	var request CompletePlanningRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	result, err := apiServer.zedPlanningService.CompletePlanning(r.Context(), sessionID, request.Approved)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to complete planning session")
		http.Error(w, fmt.Sprintf("Failed to complete planning session: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// cancelZedPlanningSession cancels an active planning session
func (apiServer *HelixAPIServer) cancelZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		http.Error(w, "Planning session ID is required", http.StatusBadRequest)
		return
	}

	err := apiServer.zedPlanningService.CancelPlanningSession(r.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to cancel planning session")
		http.Error(w, fmt.Sprintf("Failed to cancel planning session: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := CancelPlanningResponse{
		SessionID: sessionID,
		Cancelled: true,
		Message:   "Planning session cancelled successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// createZedPlanningFromSample creates a new planning session using a sample repository
func (apiServer *HelixAPIServer) createZedPlanningFromSample(w http.ResponseWriter, r *http.Request) {
	var request CreatePlanningFromSampleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.SampleType == "" {
		http.Error(w, "Sample type is required", http.StatusBadRequest)
		return
	}
	if request.ProjectName == "" {
		http.Error(w, "Project name is required", http.StatusBadRequest)
		return
	}
	if request.OwnerID == "" {
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
		return
	}
	if request.Requirements == "" {
		http.Error(w, "Requirements are required", http.StatusBadRequest)
		return
	}

	// Convert to planning request
	planningRequest := &services.ZedPlanningRequest{
		SpecTaskID:     request.SpecTaskID,
		ProjectName:    request.ProjectName,
		Description:    request.Description,
		Requirements:   request.Requirements,
		OwnerID:        request.OwnerID,
		CreateRepo:     true,
		SampleType:     request.SampleType,
		Environment:    request.Environment,
		PlanningPrompt: request.PlanningPrompt,
	}

	// Start planning session
	result, err := apiServer.zedPlanningService.StartPlanningSession(r.Context(), planningRequest)
	if err != nil {
		log.Error().Err(err).Str("sample_type", request.SampleType).Msg("Failed to start planning from sample")
		http.Error(w, fmt.Sprintf("Failed to start planning from sample: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(result)
}

// Request/Response types for API documentation

// CompletePlanningRequest represents a request to complete planning
type CompletePlanningRequest struct {
	Approved bool   `json:"approved"`
	Comments string `json:"comments,omitempty"`
}

// CancelPlanningResponse represents the response for cancelling planning
type CancelPlanningResponse struct {
	SessionID string `json:"session_id"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message"`
}

// CreatePlanningFromSampleRequest represents a request to create planning from sample
type CreatePlanningFromSampleRequest struct {
	SpecTaskID     string            `json:"spec_task_id"`
	SampleType     string            `json:"sample_type"`
	ProjectName    string            `json:"project_name"`
	Description    string            `json:"description"`
	Requirements   string            `json:"requirements"`
	OwnerID        string            `json:"owner_id"`
	Environment    map[string]string `json:"environment,omitempty"`
	PlanningPrompt string            `json:"planning_prompt,omitempty"`
}
