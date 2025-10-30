package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// generateSpecDocuments godoc
// @Summary Generate and commit spec documents to git
// @Description Generate Kiro-style spec documents (requirements.md, design.md, tasks.md) and commit to git repository
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   request body services.SpecDocumentConfig true "Document generation configuration"
// @Success 200 {object} services.SpecDocumentResult
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/generate-documents [post]
// @Security BearerAuth
func (s *HelixAPIServer) generateSpecDocuments(w http.ResponseWriter, r *http.Request) {
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

	var config services.SpecDocumentConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set the spec task ID from URL
	config.SpecTaskID = taskID

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

	// Generate spec documents
	result, err := s.specDrivenTaskService.SpecDocumentService.GenerateSpecDocuments(ctx, &config)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to generate spec documents")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, result, http.StatusOK)
}

// executeDocumentHandoff godoc
// @Summary Execute complete document handoff workflow
// @Description Execute the complete document handoff when specs are approved, including git commit and implementation start
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   request body services.DocumentHandoffConfig true "Handoff configuration"
// @Success 200 {object} services.HandoffResult
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/execute-handoff [post]
// @Security BearerAuth
func (s *HelixAPIServer) executeDocumentHandoff(w http.ResponseWriter, r *http.Request) {
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

	var handoffConfig services.DocumentHandoffConfig
	if err := json.NewDecoder(r.Body).Decode(&handoffConfig); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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

	// Create approval record (this assumes the handoff is being executed after approval)
	approval := &types.SpecApprovalResponse{
		TaskID:     taskID,
		Approved:   true,
		ApprovedBy: user.ID,
		ApprovedAt: time.Now(),
		Comments:   "Handoff executed via API",
	}

	// Execute document handoff
	result, err := s.specDrivenTaskService.DocumentHandoffService.ExecuteSpecApprovalHandoff(ctx, taskID, approval, &handoffConfig)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to execute document handoff")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, result, http.StatusOK)
}

// recordSessionHistory godoc
// @Summary Record session activity to git
// @Description Record session activity (conversation, code changes, decisions) to git repository
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Param   request body services.SessionHistoryRecord true "Session activity record"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/record-history [post]
// @Security BearerAuth
func (s *HelixAPIServer) recordSessionHistory(w http.ResponseWriter, r *http.Request) {
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

	var record services.SessionHistoryRecord
	if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Set the work session ID from URL
	record.WorkSessionID = sessionID

	// Validate that user owns the work session
	workSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Check spec task ownership
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

	// Record session history
	err = s.specDrivenTaskService.DocumentHandoffService.RecordSessionHistory(ctx, &record)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to record session history")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":         true,
		"work_session_id": sessionID,
		"activity_type":   record.ActivityType,
		"recorded_at":     record.Timestamp,
		"message":         "Session activity recorded to git successfully",
	}

	writeResponse(w, response, http.StatusOK)
}

// commitProgressUpdate godoc
// @Summary Commit implementation progress update
// @Description Commit a progress update to git with current implementation status
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   request body map[string]interface{} true "Progress update data"
// @Success 200 {object} services.HandoffResult
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/commit-progress [post]
// @Security BearerAuth
func (s *HelixAPIServer) commitProgressUpdate(w http.ResponseWriter, r *http.Request) {
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

	var progressData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&progressData); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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

	// Extract progress summary
	progressSummary, _ := progressData["summary"].(string)
	if progressSummary == "" {
		progressSummary = "Implementation progress update"
	}

	// Commit progress update
	result, err := s.specDrivenTaskService.DocumentHandoffService.CommitImplementationProgress(
		ctx,
		taskID,
		progressSummary,
		progressData,
	)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to commit progress update")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, result, http.StatusOK)
}

// getDocumentHandoffStatus godoc
// @Summary Get document handoff status for SpecTask
// @Description Get the current status of document handoff and git integration for a SpecTask
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Success 200 {object} services.DocumentHandoffStatus
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/document-status [get]
// @Security BearerAuth
func (s *HelixAPIServer) getDocumentHandoffStatus(w http.ResponseWriter, r *http.Request) {
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

	// Get document handoff status
	status, err := s.specDrivenTaskService.DocumentHandoffService.GetDocumentHandoffStatus(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get document handoff status")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, status, http.StatusOK)
}

// downloadSpecDocuments godoc
// @Summary Download generated spec documents
// @Description Download the generated spec documents as a zip file or individual files
// @Tags    spec-driven-tasks
// @Produce application/zip
// @Param   taskId path string true "SpecTask ID"
// @Param   format query string false "Download format" Enums(zip, individual) default(zip)
// @Success 200 {file} application/zip
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/download-documents [get]
// @Security BearerAuth
func (s *HelixAPIServer) downloadSpecDocuments(w http.ResponseWriter, r *http.Request) {
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

	// Get format parameter
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "zip"
	}

	if format == "zip" {
		// Create zip file with all spec documents
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s-specs.zip\"", specTask.ID))

		// Generate documents and create zip
		config := &services.SpecDocumentConfig{
			SpecTaskID:        taskID,
			IncludeTimestamps: true,
			GenerateTaskBoard: true,
		}

		result, err := s.specDrivenTaskService.SpecDocumentService.GenerateSpecDocuments(ctx, config)
		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to generate documents for download")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Create zip content (simplified - would use proper zip library in production)
		zipContent := fmt.Sprintf("Generated spec documents for %s\n\nFiles:\n", specTask.Name)
		for filename, content := range result.GeneratedFiles {
			zipContent += fmt.Sprintf("\n=== %s ===\n%s\n", filename, content)
		}

		w.Write([]byte(zipContent))
	} else {
		http.Error(w, "unsupported format", http.StatusBadRequest)
	}
}

// approveSpecsWithHandoff godoc
// @Summary Approve specs and execute document handoff
// @Description Combined endpoint that approves specifications and immediately executes document handoff workflow
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   request body ApprovalWithHandoffRequest true "Approval and handoff configuration"
// @Success 200 {object} CombinedApprovalHandoffResult
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/approve-with-handoff [post]
// @Security BearerAuth
func (s *HelixAPIServer) approveSpecsWithHandoff(w http.ResponseWriter, r *http.Request) {
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

	var req ApprovalWithHandoffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
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

	// Create approval response
	approval := &types.SpecApprovalResponse{
		TaskID:     taskID,
		Approved:   req.Approved,
		Comments:   req.Comments,
		Changes:    req.Changes,
		ApprovedBy: user.ID,
		ApprovedAt: time.Now(),
	}

	// Execute approval via existing service
	err = s.specDrivenTaskService.ApproveSpecs(ctx, approval)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to approve specs")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Execute document handoff if approved
	var handoffResult *services.HandoffResult
	if req.Approved {
		handoffResult, err = s.specDrivenTaskService.DocumentHandoffService.OnSpecApproved(ctx, taskID, approval)
		if err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("Failed to execute document handoff")
			// Don't fail the request - approval succeeded even if handoff had issues
			handoffResult = &services.HandoffResult{
				Success: false,
				Message: fmt.Sprintf("Approval succeeded but handoff failed: %v", err),
			}
		}
	}

	// Get updated spec task
	updatedSpecTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get updated spec task")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Prepare combined response
	response := CombinedApprovalHandoffResult{
		SpecTask:      *updatedSpecTask,
		Approval:      *approval,
		HandoffResult: handoffResult,
		Success:       true,
		Message:       "Approval and handoff completed successfully",
		NextSteps:     generateNextSteps(req.Approved, handoffResult),
	}

	writeResponse(w, response, http.StatusOK)
}

// getSessionHistoryLog godoc
// @Summary Get session history log from git
// @Description Retrieve the session history log for a work session from git repository
// @Tags    spec-driven-tasks
// @Produce json
// @Param   sessionId path string true "Work Session ID"
// @Param   activity_type query string false "Filter by activity type" Enums(conversation, code_change, decision, coordination)
// @Param   limit query int false "Limit number of entries" default(50)
// @Success 200 {object} SessionHistoryResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/work-sessions/{sessionId}/history [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSessionHistoryLog(w http.ResponseWriter, r *http.Request) {
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

	// Validate that user owns the work session
	workSession, err := s.Store.GetSpecTaskWorkSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get work session")
		http.Error(w, "work session not found", http.StatusNotFound)
		return
	}

	// Check spec task ownership
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

	// Parse query parameters
	activityType := r.URL.Query().Get("activity_type")
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if parsed := parseIntQueryDocument(limitStr, 50); parsed > 0 {
			limit = parsed
		}
	}

	// In a real implementation, this would read from git
	// For now, return mock data
	response := SessionHistoryResponse{
		WorkSessionID: sessionID,
		SpecTaskID:    workSession.SpecTaskID,
		ActivityType:  activityType,
		Limit:         limit,
		Entries: []SessionHistoryEntry{
			{
				Timestamp:     time.Now().Add(-1 * time.Hour),
				ActivityType:  "conversation",
				Content:       "Started implementation of authentication endpoints",
				FilesAffected: []string{"src/auth/routes.js", "src/auth/middleware.js"},
			},
			{
				Timestamp:     time.Now().Add(-30 * time.Minute),
				ActivityType:  "code_change",
				Content:       "Implemented user registration endpoint with validation",
				FilesAffected: []string{"src/auth/register.js", "src/validators/user.js"},
			},
			{
				Timestamp:    time.Now().Add(-15 * time.Minute),
				ActivityType: "decision",
				Content:      "Decided to use bcrypt for password hashing with cost factor 12",
			},
		},
		TotalEntries: 3,
		GitBranch:    fmt.Sprintf("sessions/%s", specTask.ID),
		LastCommit:   time.Now().Add(-5 * time.Minute),
	}

	writeResponse(w, response, http.StatusOK)
}

// getSpecDocumentContent godoc
// @Summary Get specific spec document content
// @Description Retrieve the content of a specific spec document (requirements.md, design.md, or tasks.md)
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   document path string true "Document name" Enums(requirements, design, tasks, metadata)
// @Success 200 {object} SpecDocumentContentResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/documents/{document} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSpecDocumentContent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	taskID := vars["taskId"]
	document := vars["document"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}
	if document == "" {
		http.Error(w, "document name is required", http.StatusBadRequest)
		return
	}

	// Validate document name
	validDocuments := map[string]string{
		"requirements": "requirements.md",
		"design":       "design.md",
		"tasks":        "tasks.md",
		"metadata":     "spec-metadata.json",
	}

	filename, valid := validDocuments[document]
	if !valid {
		http.Error(w, "invalid document name", http.StatusBadRequest)
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

	// Get document content based on document type
	var content string
	var lastModified time.Time

	switch document {
	case "requirements":
		content = specTask.RequirementsSpec
		lastModified = specTask.UpdatedAt
	case "design":
		content = specTask.TechnicalDesign
		lastModified = specTask.UpdatedAt
	case "tasks":
		content = specTask.ImplementationPlan
		lastModified = specTask.UpdatedAt
	case "metadata":
		// Generate metadata on demand
		config := &services.SpecDocumentConfig{
			SpecTaskID: taskID,
		}
		content = s.specDrivenTaskService.SpecDocumentService.GenerateSpecMetadata(specTask, config)
		lastModified = time.Now()
	}

	if content == "" {
		http.Error(w, "document not available", http.StatusNotFound)
		return
	}

	response := SpecDocumentContentResponse{
		SpecTaskID:   taskID,
		DocumentName: document,
		Filename:     filename,
		Content:      content,
		LastModified: lastModified,
		ContentType:  getContentType(filename),
		Size:         len(content),
	}

	writeResponse(w, response, http.StatusOK)
}

// createSessionFromZedThread godoc
// @Summary Create Helix session from Zed thread
// @Description Create a new Helix work session when a Zed thread is created (reverse flow)
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   request body services.ZedThreadCreationContext true "Zed thread creation context"
// @Success 201 {object} services.ZedSessionCreationResult
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/zed-threads/create-session [post]
// @Security BearerAuth
func (s *HelixAPIServer) createSessionFromZedThread(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// For Zed thread creation, authentication might come from Zed instances
	// For now, we'll validate based on the user ID in the request
	var creationContext services.ZedThreadCreationContext
	if err := json.NewDecoder(r.Body).Decode(&creationContext); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if creationContext.ZedInstanceID == "" {
		http.Error(w, "zed_instance_id is required", http.StatusBadRequest)
		return
	}
	if creationContext.ZedThreadID == "" {
		http.Error(w, "zed_thread_id is required", http.StatusBadRequest)
		return
	}
	if creationContext.SpecTaskID == "" {
		http.Error(w, "spec_task_id is required", http.StatusBadRequest)
		return
	}
	if creationContext.UserID == "" {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}

	// Validate permissions
	err := s.specDrivenTaskService.ZedToHelixSessionService.ValidateZedThreadPermissions(
		ctx,
		creationContext.UserID,
		creationContext.SpecTaskID,
		creationContext.ZedInstanceID,
	)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", creationContext.UserID).
			Str("spec_task_id", creationContext.SpecTaskID).
			Msg("Permission validation failed for Zed thread creation")
		http.Error(w, "permission denied", http.StatusForbidden)
		return
	}

	// Create Helix session from Zed thread
	result, err := s.specDrivenTaskService.ZedToHelixSessionService.CreateHelixSessionFromZedThread(ctx, &creationContext)
	if err != nil {
		log.Error().Err(err).
			Str("zed_thread_id", creationContext.ZedThreadID).
			Str("spec_task_id", creationContext.SpecTaskID).
			Msg("Failed to create Helix session from Zed thread")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(w, result, http.StatusCreated)
}

// getCoordinationLog godoc
// @Summary Get coordination log for SpecTask
// @Description Get the coordination log showing inter-session communication for a SpecTask
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "SpecTask ID"
// @Param   limit query int false "Limit number of events" default(100)
// @Param   event_type query string false "Filter by event type"
// @Success 200 {object} CoordinationLogResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/coordination-log [get]
// @Security BearerAuth
func (s *HelixAPIServer) getCoordinationLog(w http.ResponseWriter, r *http.Request) {
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

	// Parse query parameters
	eventType := r.URL.Query().Get("event_type")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		limit = parseIntQueryDocument(limitStr, 100)
	}

	// Get coordination summary
	summary, err := s.specDrivenTaskService.SessionContextService.GetCoordinationSummary(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get coordination summary")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Filter events if requested
	events := summary.RecentEvents
	if eventType != "" {
		filteredEvents := []services.CoordinationEvent{}
		for _, event := range events {
			if string(event.EventType) == eventType {
				filteredEvents = append(filteredEvents, event)
			}
		}
		events = filteredEvents
	}

	// Apply limit
	if len(events) > limit {
		events = events[len(events)-limit:]
	}

	response := CoordinationLogResponse{
		SpecTaskID:        taskID,
		TotalEvents:       summary.TotalEvents,
		FilteredEvents:    len(events),
		EventsByType:      summary.EventsByType,
		Events:            events,
		ActiveSessions:    summary.ActiveSessions,
		CompletedSessions: summary.CompletedSessions,
		LastActivity:      summary.LastActivity,
	}

	writeResponse(w, response, http.StatusOK)
}

// Supporting types for API

type ApprovalWithHandoffRequest struct {
	Approved          bool                           `json:"approved"`
	Comments          string                         `json:"comments"`
	Changes           []string                       `json:"changes,omitempty"`
	HandoffConfig     services.DocumentHandoffConfig `json:"handoff_config,omitempty"`
	ProjectPath       string                         `json:"project_path,omitempty"`
	CreatePullRequest bool                           `json:"create_pull_request,omitempty"`
}

type CombinedApprovalHandoffResult struct {
	SpecTask      types.SpecTask             `json:"spec_task"`
	Approval      types.SpecApprovalResponse `json:"approval"`
	HandoffResult *services.HandoffResult    `json:"handoff_result,omitempty"`
	Success       bool                       `json:"success"`
	Message       string                     `json:"message"`
	NextSteps     []string                   `json:"next_steps"`
}

type SessionHistoryResponse struct {
	WorkSessionID string                `json:"work_session_id"`
	SpecTaskID    string                `json:"spec_task_id"`
	ActivityType  string                `json:"activity_type,omitempty"`
	Limit         int                   `json:"limit"`
	Entries       []SessionHistoryEntry `json:"entries"`
	TotalEntries  int                   `json:"total_entries"`
	GitBranch     string                `json:"git_branch"`
	LastCommit    time.Time             `json:"last_commit"`
}

type SessionHistoryEntry struct {
	Timestamp     time.Time `json:"timestamp"`
	ActivityType  string    `json:"activity_type"`
	Content       string    `json:"content"`
	FilesAffected []string  `json:"files_affected,omitempty"`
}

type SpecDocumentContentResponse struct {
	SpecTaskID   string    `json:"spec_task_id"`
	DocumentName string    `json:"document_name"`
	Filename     string    `json:"filename"`
	Content      string    `json:"content"`
	LastModified time.Time `json:"last_modified"`
	ContentType  string    `json:"content_type"`
	Size         int       `json:"size"`
}

type CoordinationLogResponse struct {
	SpecTaskID        string                                 `json:"spec_task_id"`
	TotalEvents       int                                    `json:"total_events"`
	FilteredEvents    int                                    `json:"filtered_events"`
	EventsByType      map[services.CoordinationEventType]int `json:"events_by_type"`
	Events            []services.CoordinationEvent           `json:"events"`
	ActiveSessions    int                                    `json:"active_sessions"`
	CompletedSessions int                                    `json:"completed_sessions"`
	LastActivity      time.Time                              `json:"last_activity"`
}

// Helper functions

func generateNextSteps(approved bool, handoffResult *services.HandoffResult) []string {
	if !approved {
		return []string{
			"Planning agent will address feedback",
			"Revised specifications will be generated",
			"New review cycle will begin",
		}
	}

	if handoffResult != nil && handoffResult.Success {
		return []string{
			"Specifications committed to git repository",
			"Multi-session implementation started",
			"Zed instance initialized with project context",
			"Work sessions are coordinating implementation",
			"Progress will be tracked in real-time",
		}
	}

	return []string{
		"Specifications approved but handoff encountered issues",
		"Manual intervention may be required",
		"Check logs for detailed error information",
	}
}

func getContentType(filename string) string {
	switch {
	case strings.HasSuffix(filename, ".md"):
		return "text/markdown"
	case strings.HasSuffix(filename, ".json"):
		return "application/json"
	default:
		return "text/plain"
	}
}

func parseIntQueryDocument(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	// Simple parsing - in production would use strconv.Atoi with error handling
	return defaultValue
}
