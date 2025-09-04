package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/rs/zerolog/log"
)

// startZedPlanningSession starts a new Zed planning session
func (apiServer *HelixAPIServer) startZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	var request services.ZedPlanningRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	// Validate required fields
	if request.SpecTaskID == "" {
		system.Error(w, http.StatusBadRequest, "SpecTask ID is required")
		return
	}
	if request.ProjectName == "" {
		system.Error(w, http.StatusBadRequest, "Project name is required")
		return
	}
	if request.OwnerID == "" {
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}
	if request.Requirements == "" {
		system.Error(w, http.StatusBadRequest, "Requirements are required")
		return
	}

	// Validate repository specification
	if request.RepositoryID == "" && request.RepositoryURL == "" && !request.CreateRepo {
		system.Error(w, http.StatusBadRequest, "Must specify repository_id, repository_url, or create_repo=true")
		return
	}

	// Start planning session
	result, err := apiServer.zedPlanningService.StartPlanningSession(r.Context(), &request)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to start planning session")
		system.Error(w, http.StatusInternalServerError, "Failed to start planning session: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusCreated, result)
}

// getZedPlanningSession retrieves planning session information by ID
func (apiServer *HelixAPIServer) getZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		system.Error(w, http.StatusBadRequest, "Planning session ID is required")
		return
	}

	session, err := apiServer.zedPlanningService.GetPlanningSession(r.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get planning session")
		system.Error(w, http.StatusNotFound, "Planning session not found: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusOK, session)
}

// listZedPlanningSessions lists all planning sessions for the authenticated user
func (apiServer *HelixAPIServer) listZedPlanningSessions(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	status := r.URL.Query().Get("status")

	// If no owner_id specified, try to get from authenticated user
	if ownerID == "" {
		// This would typically come from authentication middleware
		// For now, return error
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}

	sessions, err := apiServer.zedPlanningService.ListPlanningSessions(r.Context(), ownerID)
	if err != nil {
		log.Error().Err(err).Str("owner_id", ownerID).Msg("Failed to list planning sessions")
		system.Error(w, http.StatusInternalServerError, "Failed to list planning sessions: %s", err.Error())
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

	system.JsonResponse(w, http.StatusOK, sessions)
}

// completeZedPlanning completes a planning session (approve or reject)
func (apiServer *HelixAPIServer) completeZedPlanning(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		system.Error(w, http.StatusBadRequest, "Planning session ID is required")
		return
	}

	var request CompletePlanningRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	result, err := apiServer.zedPlanningService.CompletePlanning(r.Context(), sessionID, request.Approved)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to complete planning session")
		system.Error(w, http.StatusInternalServerError, "Failed to complete planning session: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusOK, result)
}

// cancelZedPlanningSession cancels an active planning session
func (apiServer *HelixAPIServer) cancelZedPlanningSession(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	sessionID := vars["id"]
	if sessionID == "" {
		system.Error(w, http.StatusBadRequest, "Planning session ID is required")
		return
	}

	err := apiServer.zedPlanningService.CancelPlanningSession(r.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to cancel planning session")
		system.Error(w, http.StatusInternalServerError, "Failed to cancel planning session: %s", err.Error())
		return
	}

	response := CancelPlanningResponse{
		SessionID: sessionID,
		Cancelled: true,
		Message:   "Planning session cancelled successfully",
	}

	system.JsonResponse(w, http.StatusOK, response)
}

// createZedPlanningFromSample creates a new planning session using a sample repository
func (apiServer *HelixAPIServer) createZedPlanningFromSample(w http.ResponseWriter, r *http.Request) {
	var request CreatePlanningFromSampleRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	// Validate required fields
	if request.SampleType == "" {
		system.Error(w, http.StatusBadRequest, "Sample type is required")
		return
	}
	if request.ProjectName == "" {
		system.Error(w, http.StatusBadRequest, "Project name is required")
		return
	}
	if request.OwnerID == "" {
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}
	if request.Requirements == "" {
		system.Error(w, http.StatusBadRequest, "Requirements are required")
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
		system.Error(w, http.StatusInternalServerError, "Failed to start planning from sample: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusCreated, result)
}

// getZedPlanningSampleTypes returns available sample repository types
func (apiServer *HelixAPIServer) getZedPlanningSampleTypes(w http.ResponseWriter, r *http.Request) {
	sampleTypes := []SampleType{
		{
			ID:          "nodejs-todo",
			Name:        "Node.js Todo App",
			Description: "A simple todo application built with Node.js and Express",
			TechStack:   []string{"javascript", "nodejs", "express", "mongodb"},
		},
		{
			ID:          "python-api",
			Name:        "Python API Service",
			Description: "A FastAPI microservice with PostgreSQL integration",
			TechStack:   []string{"python", "fastapi", "postgresql", "sqlalchemy"},
		},
		{
			ID:          "react-dashboard",
			Name:        "React Dashboard",
			Description: "A modern admin dashboard built with React and Material-UI",
			TechStack:   []string{"javascript", "react", "typescript", "material-ui"},
		},
	}

	response := SampleTypesResponse{
		SampleTypes: sampleTypes,
		Count:       len(sampleTypes),
	}

	system.JsonResponse(w, http.StatusOK, response)
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

// SampleType represents a sample repository type
type SampleType struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TechStack   []string `json:"tech_stack"`
}

// SampleTypesResponse represents the response for sample types
type SampleTypesResponse struct {
	SampleTypes []SampleType `json:"sample_types"`
	Count       int          `json:"count"`
}
