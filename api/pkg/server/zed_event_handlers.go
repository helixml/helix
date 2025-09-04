package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// handleZedInstanceEvent godoc
// @Summary Handle Zed instance events
// @Description Process events from Zed instances including status changes, thread updates, and coordination
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   request body types.ZedInstanceEvent true "Zed instance event"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/zed/events [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleZedInstanceEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// For Zed events, we might not have a regular user session
	// These events come from Zed runners, so we need different auth
	// For now, we'll validate based on headers or API key

	var event types.ZedInstanceEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if event.InstanceID == "" {
		http.Error(w, "instance_id is required", http.StatusBadRequest)
		return
	}
	if event.EventType == "" {
		http.Error(w, "event_type is required", http.StatusBadRequest)
		return
	}

	// Process event through ZedIntegrationService
	err := s.specDrivenTaskService.ZedIntegrationService.HandleZedInstanceEvent(ctx, &event)
	if err != nil {
		log.Error().Err(err).
			Str("instance_id", event.InstanceID).
			Str("event_type", event.EventType).
			Msg("Failed to process Zed instance event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":     true,
		"instance_id": event.InstanceID,
		"event_type":  event.EventType,
		"processed":   true,
	}

	writeResponse(w, response, http.StatusOK)
}

// handleZedThreadEvent godoc
// @Summary Handle Zed thread-specific events
// @Description Process thread-specific events from Zed instances
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   instanceId path string true "Zed Instance ID"
// @Param   threadId path string true "Zed Thread ID"
// @Param   request body map[string]interface{} true "Thread event data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/zed/instances/{instanceId}/threads/{threadId}/events [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleZedThreadEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	instanceID := vars["instanceId"]
	threadID := vars["threadId"]

	if instanceID == "" {
		http.Error(w, "instance ID is required", http.StatusBadRequest)
		return
	}
	if threadID == "" {
		http.Error(w, "thread ID is required", http.StatusBadRequest)
		return
	}

	var eventData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&eventData); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Create thread event
	event := &types.ZedInstanceEvent{
		InstanceID: instanceID,
		ThreadID:   threadID,
		EventType:  "thread_status_changed",
		Data:       eventData,
	}

	// If event type is specified in data, use it
	if eventType, ok := eventData["event_type"].(string); ok {
		event.EventType = eventType
	}

	// Try to find SpecTask ID from thread mapping
	if specTaskID, ok := eventData["spec_task_id"].(string); ok {
		event.SpecTaskID = specTaskID
	} else {
		// Try to find it from database
		zedThreads, err := s.Store.ListSpecTaskZedThreads(ctx, "")
		if err == nil {
			for _, thread := range zedThreads {
				if thread.ZedThreadID == threadID {
					event.SpecTaskID = thread.SpecTaskID
					break
				}
			}
		}
	}

	// Process event
	err := s.specDrivenTaskService.ZedIntegrationService.HandleZedInstanceEvent(ctx, event)
	if err != nil {
		log.Error().Err(err).
			Str("instance_id", instanceID).
			Str("thread_id", threadID).
			Str("event_type", event.EventType).
			Msg("Failed to process Zed thread event")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":     true,
		"instance_id": instanceID,
		"thread_id":   threadID,
		"event_type":  event.EventType,
		"processed":   true,
	}

	writeResponse(w, response, http.StatusOK)
}

// getZedInstanceStatus godoc
// @Summary Get Zed instance status for a SpecTask
// @Description Get the current status and information about a Zed instance associated with a SpecTask
// @Tags    zed-integration
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} types.ZedInstanceStatus
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/zed-instance [get]
// @Security BearerAuth
func (s *HelixAPIServer) getZedInstanceStatus(w http.ResponseWriter, r *http.Request) {
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

	// Get Zed instance status
	status, err := s.specDrivenTaskService.ZedIntegrationService.GetZedInstanceStatus(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get Zed instance status")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, status, http.StatusOK)
}

// shutdownZedInstance godoc
// @Summary Shutdown Zed instance for a SpecTask
// @Description Manually shutdown a Zed instance and all its threads for a SpecTask
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/zed-instance [delete]
// @Security BearerAuth
func (s *HelixAPIServer) shutdownZedInstance(w http.ResponseWriter, r *http.Request) {
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

	// Cleanup Zed instance
	err = s.specDrivenTaskService.ZedIntegrationService.CleanupZedInstance(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to cleanup Zed instance")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":      true,
		"spec_task_id": taskID,
		"zed_instance": specTask.ZedInstanceID,
		"shutdown":     true,
		"message":      "Zed instance shutdown initiated",
	}

	writeResponse(w, response, http.StatusOK)
}

// listZedThreads godoc
// @Summary List Zed threads for a SpecTask
// @Description Get all Zed threads associated with a SpecTask
// @Tags    zed-integration
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskZedThreadListResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/zed-threads [get]
// @Security BearerAuth
func (s *HelixAPIServer) listZedThreads(w http.ResponseWriter, r *http.Request) {
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

	// List Zed threads
	zedThreads, err := s.Store.ListSpecTaskZedThreads(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to list Zed threads")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to non-pointer slice
	threadSlice := make([]types.SpecTaskZedThread, len(zedThreads))
	for i, thread := range zedThreads {
		threadSlice[i] = *thread
	}

	response := types.SpecTaskZedThreadListResponse{
		ZedThreads: threadSlice,
		Total:      len(threadSlice),
	}

	writeResponse(w, response, http.StatusOK)
}

// updateZedThreadActivity godoc
// @Summary Update Zed thread activity
// @Description Update activity timestamp and status for a Zed thread
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   threadId path string true "Zed Thread ID"
// @Param   request body map[string]interface{} true "Activity data"
// @Success 200 {object} types.SpecTaskZedThread
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/zed/threads/{threadId}/activity [post]
// @Security BearerAuth
func (s *HelixAPIServer) updateZedThreadActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	threadID := vars["threadId"]
	if threadID == "" {
		http.Error(w, "thread ID is required", http.StatusBadRequest)
		return
	}

	var activityData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&activityData); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get the Zed thread
	zedThread, err := s.Store.GetSpecTaskZedThread(ctx, threadID)
	if err != nil {
		log.Error().Err(err).Str("thread_id", threadID).Msg("Failed to get Zed thread")
		http.Error(w, "Zed thread not found", http.StatusNotFound)
		return
	}

	// Determine new status from activity data
	status := zedThread.Status
	if newStatus, ok := activityData["status"].(string); ok {
		status = types.SpecTaskZedStatus(newStatus)
	}

	// Update via ZedIntegrationService
	err = s.specDrivenTaskService.ZedIntegrationService.UpdateZedThreadStatus(ctx, threadID, status, activityData)
	if err != nil {
		log.Error().Err(err).Str("thread_id", threadID).Msg("Failed to update Zed thread activity")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return updated thread
	updatedThread, err := s.Store.GetSpecTaskZedThread(ctx, threadID)
	if err != nil {
		log.Error().Err(err).Str("thread_id", threadID).Msg("Failed to get updated Zed thread")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, updatedThread, http.StatusOK)
}

// handleZedConnectionHeartbeat godoc
// @Summary Handle Zed connection heartbeat
// @Description Process heartbeat signals from Zed instances to maintain connection status
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   instanceId path string true "Zed Instance ID"
// @Param   request body map[string]interface{} true "Heartbeat data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/zed/instances/{instanceId}/heartbeat [post]
// @Security BearerAuth
func (s *HelixAPIServer) handleZedConnectionHeartbeat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)

	instanceID := vars["instanceId"]
	if instanceID == "" {
		http.Error(w, "instance ID is required", http.StatusBadRequest)
		return
	}

	var heartbeatData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&heartbeatData); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Create heartbeat event
	event := &types.ZedInstanceEvent{
		InstanceID: instanceID,
		EventType:  "heartbeat",
		Data:       heartbeatData,
	}

	// Try to get SpecTask ID from heartbeat data or lookup
	if specTaskID, ok := heartbeatData["spec_task_id"].(string); ok {
		event.SpecTaskID = specTaskID
	}

	// Process heartbeat
	err := s.specDrivenTaskService.ZedIntegrationService.HandleZedInstanceEvent(ctx, event)
	if err != nil {
		log.Error().Err(err).
			Str("instance_id", instanceID).
			Msg("Failed to process Zed heartbeat")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":      true,
		"instance_id":  instanceID,
		"heartbeat":    true,
		"acknowledged": true,
	}

	writeResponse(w, response, http.StatusOK)
}

// createZedThreadForWorkSession godoc
// @Summary Create Zed thread for work session
// @Description Manually create a Zed thread for a specific work session
// @Tags    zed-integration
// @Accept  json
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Param   request body types.SpecTaskZedThreadCreateRequest true "Thread creation request"
// @Success 201 {object} types.SpecTaskZedThread
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/zed-thread [post]
// @Security BearerAuth
func (s *HelixAPIServer) createZedThreadForWorkSession(w http.ResponseWriter, r *http.Request) {
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

	var req types.SpecTaskZedThreadCreateRequest
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

	// Validate ownership
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

	// Ensure SpecTask has Zed instance
	if specTask.ZedInstanceID == "" {
		// Create Zed instance for the SpecTask
		instanceID, err := s.specDrivenTaskService.ZedIntegrationService.CreateZedInstanceForSpecTask(ctx, specTask, nil)
		if err != nil {
			log.Error().Err(err).Str("spec_task_id", specTask.ID).Msg("Failed to create Zed instance")
			http.Error(w, "failed to create Zed instance", http.StatusInternalServerError)
			return
		}
		specTask.ZedInstanceID = instanceID
	}

	// Create Zed thread
	zedThread, err := s.specDrivenTaskService.ZedIntegrationService.CreateZedThreadForWorkSession(
		ctx,
		workSession,
		specTask.ZedInstanceID,
	)
	if err != nil {
		log.Error().Err(err).Str("work_session_id", sessionID).Msg("Failed to create Zed thread")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, zedThread, http.StatusCreated)
}
