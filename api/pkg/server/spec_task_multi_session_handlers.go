package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// createImplementationSessions godoc
// @Summary Create implementation sessions for a SpecTask
// @Description Create multiple work sessions from an approved SpecTask implementation plan
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   request body types.SpecTaskImplementationSessionsCreateRequest true "Implementation sessions configuration"
// @Success 200 {object} types.SpecTaskMultiSessionOverviewResponse
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/implementation-sessions [post]
// @Security BearerAuth
func (s *HelixAPIServer) createImplementationSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	var req types.SpecTaskImplementationSessionsCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set the spec task ID from URL
	req.SpecTaskID = taskID

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Create implementation sessions
	overview, err := s.specDrivenTaskService.MultiSessionManager.CreateImplementationSessions(ctx, taskID, &req)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to create implementation sessions")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, overview, http.StatusOK)
}

// spawnWorkSession godoc
// @Summary Spawn a new work session from an existing one
// @Description Create a new work session that is spawned from an existing active work session
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   sessionId path string true "Parent Work Session ID"
// @Param   request body types.SpecTaskWorkSessionSpawnRequest true "Spawn configuration"
// @Success 201 {object} types.SpecTaskWorkSessionDetailResponse
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/spawn [post]
// @Security BearerAuth
func (s *HelixAPIServer) spawnWorkSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	parentSessionID := vars["sessionId"]
	if parentSessionID == "" {
		http.Error(w, "session ID is required", http.StatusBadRequest)
		return
	}

	var req types.SpecTaskWorkSessionSpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set the parent session ID from URL
	req.ParentWorkSessionID = parentSessionID

	// Validate that user owns the parent work session
	parentWorkSession, err := s.Store.GetSpecTaskWorkSession(ctx, parentSessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", parentSessionID).Msg("Failed to get parent work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Check spec task ownership
	specTask, err := s.Store.GetSpecTask(ctx, parentWorkSession.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", parentWorkSession.SpecTaskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Spawn work session
	detail, err := s.specDrivenTaskService.MultiSessionManager.SpawnWorkSession(ctx, parentSessionID, &req)
	if err != nil {
		log.Error().Err(err).Str("parent_session_id", parentSessionID).Msg("Failed to spawn work session")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, detail, http.StatusCreated)
}

// getSpecTaskMultiSessionOverview godoc
// @Summary Get multi-session overview for a SpecTask
// @Description Get comprehensive overview of all work sessions and progress for a SpecTask
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskMultiSessionOverviewResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/multi-session-overview [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskMultiSessionOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Get multi-session overview
	overview, err := s.specDrivenTaskService.MultiSessionManager.GetMultiSessionOverview(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get multi-session overview")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, overview, http.StatusOK)
}

// getSpecTaskProgress godoc
// @Summary Get detailed progress for a SpecTask
// @Description Get detailed progress information including phase progress and implementation task status
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskProgressResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/progress [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecTaskProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Get progress information
	progress, err := s.specDrivenTaskService.MultiSessionManager.GetSpecTaskProgress(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task progress")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, progress, http.StatusOK)
}

// listSpecTaskWorkSessions godoc
// @Summary List work sessions for a SpecTask
// @Description Get all work sessions associated with a specific SpecTask
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   phase query string false "Filter by phase" Enums(planning, implementation, validation)
// @Success 200 {object} types.SpecTaskWorkSessionListResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/work-sessions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listSpecTaskWorkSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Parse phase filter
	var phaseFilter *types.SpecTaskPhase
	if phaseStr := r.URL.Query().Get("phase"); phaseStr != "" {
		phase := types.SpecTaskPhase(phaseStr)
		phaseFilter = &phase
	}

	// Get work sessions
	workSessions, err := s.Store.ListWorkSessionsBySpecTask(ctx, taskID, phaseFilter)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to list work sessions")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to non-pointer slice
	sessionSlice := make([]types.SpecTaskWorkSession, len(workSessions))
	for i, ws := range workSessions {
		sessionSlice[i] = *ws
	}

	response := types.SpecTaskWorkSessionListResponse{
		WorkSessions: sessionSlice,
		Total:        len(sessionSlice),
	}

	writeResponse(w, response, http.StatusOK)
}

// getWorkSessionDetail godoc
// @Summary Get detailed information about a work session
// @Description Get comprehensive details about a specific work session including related entities
// @Tags    spec-driven-tasks
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Success 200 {object} types.SpecTaskWorkSessionDetailResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getWorkSessionDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "session ID is required", http.StatusBadRequest)
		return
	}

	// Get work session
	workSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", workSession.SpecTaskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Return detailed information via service
	detail, err := s.specDrivenTaskService.MultiSessionManager.GetWorkSessionDetail(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session detail")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, detail, http.StatusOK)
}

// updateWorkSessionStatus godoc
// @Summary Update work session status
// @Description Update the status of a work session and handle state transitions
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Param   request body types.SpecTaskWorkSessionUpdateRequest true "Status update"
// @Success 200 {object} types.SpecTaskWorkSession
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/status [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateWorkSessionStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "session ID is required", http.StatusBadRequest)
		return
	}

	var req types.SpecTaskWorkSessionUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get work session
	workSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", workSession.SpecTaskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Update status via service if provided
	if req.Status != "" {
		err = s.specDrivenTaskService.MultiSessionManager.UpdateWorkSessionStatus(ctx, sessionID, req.Status)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to update work session status")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Update other fields if provided
	if req.Name != "" {
		workSession.Name = req.Name
	}
	if req.Description != "" {
		workSession.Description = req.Description
	}

	if req.Name != "" || req.Description != "" {
		err = s.Store.UpdateSpecTaskWorkSession(ctx, workSession)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to update work session")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Return updated work session
	updatedWorkSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get updated work session")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, updatedWorkSession, http.StatusOK)
}

// updateZedThreadStatus godoc
// @Summary Update Zed thread status
// @Description Update the status of a Zed thread associated with a work session
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Param   request body types.SpecTaskZedThreadUpdateRequest true "Zed thread status update"
// @Success 200 {object} types.SpecTaskZedThread
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/zed-thread [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateZedThreadStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	sessionID := vars["sessionId"]
	if sessionID == "" {
		http.Error(w, "session ID is required", http.StatusBadRequest)
		return
	}

	var req types.SpecTaskZedThreadUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get work session
	workSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", workSession.SpecTaskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Update Zed thread status via service if provided
	if req.Status != "" {
		err = s.specDrivenTaskService.MultiSessionManager.UpdateZedThreadStatus(ctx, sessionID, req.Status)
		if err != nil {
			log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to update zed thread status")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Get updated Zed thread
	zedThread, err := s.Store.GetSpecTaskZedThreadByWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get zed thread")
		http.Error(w, "zed thread not found", http.StatusNotFound)
		return
	}

	writeResponse(w, zedThread, http.StatusOK)
}

// listImplementationTasks godoc
// @Summary List implementation tasks for a SpecTask
// @Description Get all parsed implementation tasks from a SpecTask's implementation plan
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskImplementationTaskListResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/implementation-tasks [get]
// @Security BearerAuth
func (s *HelixAPIServer) listImplementationTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	// Validate that user owns the spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get spec task")
		http.Error(w, "spec task not found", http.StatusNotFound)
		return
	}

	if specTask.CreatedBy != user.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	// Get implementation tasks
	implTasks, err := s.Store.ListSpecTaskImplementationTasks(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to list implementation tasks")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to non-pointer slice
	taskSlice := make([]types.SpecTaskImplementationTask, len(implTasks))
	for i, task := range implTasks {
		taskSlice[i] = *task
	}

	response := types.SpecTaskImplementationTaskListResponse{
		ImplementationTasks: taskSlice,
		Total:               len(taskSlice),
	}

	writeResponse(w, response, http.StatusOK)
}
