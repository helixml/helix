// Spec-driven task handlers with audit logging integration
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/ptr"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// createTaskFromPrompt godoc
// @Summary Create spec-driven task from simple prompt
// @Description Create a new task from a simple description and start spec generation
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   request body types.CreateTaskRequest true "Task creation request"
// @Success 201 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/from-prompt [post]
func (s *HelixAPIServer) createTaskFromPrompt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("Failed to decode create task request")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Authorize user to create task in the project
	if err := s.authorizeUserToProjectByID(ctx, user, req.ProjectID, types.ActionCreate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Set user ID and email from context
	req.UserID = user.ID
	req.UserEmail = user.Email

	// Validate request
	if req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// Create task via spec-driven service
	task, err := s.specDrivenTaskService.CreateTaskFromPrompt(ctx, &req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create task from prompt")
		http.Error(w, fmt.Sprintf("failed to create task: %v", err), http.StatusInternalServerError)
		return
	}

	// Log audit event for task creation
	if s.auditLogService != nil {
		s.auditLogService.LogTaskCreated(ctx, task, user.ID, user.Email)
	}

	log.Info().
		Str("task_id", task.ID).
		Str("user_id", user.ID).
		Str("project_id", req.ProjectID).
		Msg("Task created from prompt")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// getTask godoc
// @Summary Get spec-driven task details
// @Description Get detailed information about a specific spec-driven task
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Task ID"
// @Success 200 {object} types.SpecTask
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId} [get]
func (s *HelixAPIServer) getTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get task")
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to get task in the project
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Compute PullRequestURL for tasks with PullRequestID (external repos like ADO)
	if task.PullRequestID != "" && task.PullRequestURL == "" {
		project, err := s.Store.GetProject(ctx, task.ProjectID)
		if err == nil && project.DefaultRepoID != "" {
			repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
			if err == nil && repo.ExternalURL != "" {
				task.PullRequestURL = fmt.Sprintf("%s/pullrequest/%s", repo.ExternalURL, task.PullRequestID)
			}
		}
	}

	// Populate agent activity data (work state for reconciliation UI)
	if task.PlanningSessionID != "" {
		// Get SessionUpdatedAt from session
		session, err := s.Store.GetSession(ctx, task.PlanningSessionID)
		if err == nil && session != nil {
			task.SessionUpdatedAt = &session.Updated
		}
		// Get AgentWorkState from activity record
		activity, err := s.Store.GetExternalAgentActivity(ctx, task.PlanningSessionID)
		if err == nil && activity != nil {
			task.AgentWorkState = activity.AgentWorkState
			task.LastPromptContent = activity.LastPromptContent
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// listTasks godoc
// @Summary List spec-driven tasks
// @Description List spec-driven tasks with optional filtering by project, status, or user
// @Tags    spec-driven-tasks
// @Produce json
// @Param   project_id query string false "Filter by project ID"
// @Param   status query string false "Filter by status"
// @Param   user_id query string false "Filter by user ID"
// @Param   include_archived query bool false "Include archived tasks" default(false)
// @Param   limit query int false "Limit number of results" default(50)
// @Param   offset query int false "Offset for pagination" default(0)
// @Success 200 {array} types.SpecTask
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks [get]
func (s *HelixAPIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	projectID := query.Get("project_id")

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to list tasks in the project
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionList); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	filters := &types.SpecTaskFilters{
		ProjectID:       projectID,
		Status:          types.SpecTaskStatus(query.Get("status")),
		UserID:          query.Get("user_id"),
		Limit:           parseIntQuery(query.Get("limit"), 50),
		Offset:          parseIntQuery(query.Get("offset"), 0),
		IncludeArchived: query.Get("include_archived") == "true",
		ArchivedOnly:    query.Get("archived_only") == "true",
	}

	tasks, err := s.Store.ListSpecTasks(ctx, filters)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list tasks")
		http.Error(w, fmt.Sprintf("failed to list tasks: %v", err), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null for empty results
	if tasks == nil {
		tasks = []*types.SpecTask{}
	}

	// Compute PullRequestURL for tasks with PullRequestID (external repos like ADO)
	if projectID != "" {
		project, err := s.Store.GetProject(ctx, projectID)
		if err == nil && project.DefaultRepoID != "" {
			repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
			if err == nil && repo.ExternalURL != "" {
				for _, task := range tasks {
					if task.PullRequestID != "" && task.PullRequestURL == "" {
						task.PullRequestURL = fmt.Sprintf("%s/pullrequest/%s", repo.ExternalURL, task.PullRequestID)
					}
				}
			}
		}
	}

	// Populate SessionUpdatedAt and AgentWorkState for agent activity detection
	// Collect all session IDs and batch query for efficiency
	sessionIDs := make([]string, 0)
	for _, task := range tasks {
		if task.PlanningSessionID != "" {
			sessionIDs = append(sessionIDs, task.PlanningSessionID)
		}
	}
	if len(sessionIDs) > 0 {
		sessions, err := s.Store.GetSessionsByIDs(ctx, sessionIDs)
		if err == nil {
			// Build a map for quick lookup
			sessionMap := make(map[string]*types.Session)
			for _, session := range sessions {
				sessionMap[session.ID] = session
			}
			// Populate SessionUpdatedAt on each task
			for _, task := range tasks {
				if session, ok := sessionMap[task.PlanningSessionID]; ok {
					task.SessionUpdatedAt = &session.Updated
				}
			}
		}

		// Also populate AgentWorkState and LastPromptContent from activity records
		for _, task := range tasks {
			if task.PlanningSessionID != "" {
				activity, err := s.Store.GetExternalAgentActivity(ctx, task.PlanningSessionID)
				if err == nil && activity != nil {
					task.AgentWorkState = activity.AgentWorkState
					task.LastPromptContent = activity.LastPromptContent
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// approveSpecs godoc
// @Summary Approve or reject generated specifications
// @Description Human approval/rejection of specs generated by AI agent
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "Task ID"
// @Param   request body types.SpecApprovalResponse true "Approval response"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/approve-specs [post]
func (s *HelixAPIServer) approveSpecs(w http.ResponseWriter, r *http.Request) {
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

	// Return updated task
	existingTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get updated task")
		http.Error(w, "failed to get updated task", http.StatusInternalServerError)
		return
	}

	// Authorize user to approve specs in the project
	if err := s.authorizeUserToProjectByID(ctx, user, existingTask.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.SpecApprovalResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("Failed to decode approval request")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	existingTask.SpecApprovedBy = user.ID
	existingTask.SpecApprovedAt = ptr.To(time.Now())
	existingTask.Status = types.TaskStatusSpecApproved
	existingTask.SpecApproval = &req

	err = s.Store.UpdateSpecTask(ctx, existingTask)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to update task")
		http.Error(w, fmt.Sprintf("failed to update task: %v", err), http.StatusInternalServerError)
		return
	}

	// Log audit event for spec approval
	if req.Approved && s.auditLogService != nil {
		s.auditLogService.LogTaskApproved(ctx, existingTask, user.ID, user.Email)
	}

	log.Info().
		Str("task_id", taskID).
		Str("user_id", user.ID).
		Bool("approved", req.Approved).
		Msg("Spec approval processed")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existingTask)
}

// getTaskSpecs godoc
// @Summary Get task specifications for review
// @Description Get the generated specifications for human review
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Task ID"
// @Success 200 {object} TaskSpecsResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/specs [get]
func (s *HelixAPIServer) getTaskSpecs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get task")
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to get specs in the project
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Only return specs if they've been generated
	if task.Status == types.TaskStatusBacklog || task.Status == types.TaskStatusSpecGeneration {
		http.Error(w, "specifications not yet generated", http.StatusNotFound)
		return
	}

	response := TaskSpecsResponse{
		TaskID:             task.ID,
		Status:             task.Status,
		OriginalPrompt:     task.OriginalPrompt,
		RequirementsSpec:   task.RequirementsSpec,
		TechnicalDesign:    task.TechnicalDesign,
		ImplementationPlan: task.ImplementationPlan,
		SpecApprovedBy:     task.SpecApprovedBy,
		SpecApprovedAt:     task.SpecApprovedAt,
		SpecRevisionCount:  task.SpecRevisionCount,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getTaskProgress godoc
// @Summary Get spec-driven task progress
// @Description Get detailed progress information for a spec-driven task including specification and implementation phases
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Task ID"
// @Success 200 {object} TaskProgressResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/progress [get]
func (s *HelixAPIServer) getTaskProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get task")
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to get progress in the project
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Build progress response
	progress := TaskProgressResponse{
		TaskID:    task.ID,
		Status:    getSpecificationStatus(task.Status),
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
		Specification: PhaseProgress{
			Status:        getSpecificationStatus(task.Status),
			Agent:         "", // No separate agent field needed
			SessionID:     task.PlanningSessionID,
			StartedAt:     &task.CreatedAt, // Spec generation starts when task is created
			CompletedAt:   task.SpecApprovedAt,
			RevisionCount: task.SpecRevisionCount,
		},
		Implementation: PhaseProgress{
			Status:    getImplementationStatus(task.Status),
			Agent:     "",                     // No separate agent - reuses planning agent
			SessionID: task.PlanningSessionID, // Same session continues into implementation
			StartedAt: task.SpecApprovedAt,
			// CompletedAt will be set when implementation is done
		},
	}

	// Try to get checklist progress from tasks.md in helix-specs branch
	checklistProgress := s.getChecklistProgress(ctx, task)
	if checklistProgress != nil {
		progress.Checklist = checklistProgress
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progress)
}

// Helper functions
func parseIntQuery(value string, defaultValue int) int {
	if value == "" {
		return defaultValue
	}
	// Simple parsing - you might want to use strconv.Atoi with error handling
	var result int
	if _, err := fmt.Sscanf(value, "%d", &result); err != nil {
		return defaultValue
	}
	return result
}

// startPlanning godoc
// @Summary Start planning for a SpecTask
// @Description Explicitly start spec generation (planning phase) for a backlog task. This transitions the task to planning status and starts a spec generation session.
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Param keyboard query string false "XKB keyboard layout code (e.g., 'us', 'fr', 'de') - for testing browser locale detection"
// @Param timezone query string false "IANA timezone (e.g., 'Europe/Paris') - for testing browser locale detection"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/start-planning [post]
// @Security BearerAuth
func (s *HelixAPIServer) startPlanning(w http.ResponseWriter, r *http.Request) {
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

	// Parse optional query parameters for browser locale settings
	// These allow testing keyboard layout detection via ?keyboard=fr&timezone=Europe/Paris
	opts := types.StartPlanningOptions{
		KeyboardLayout: r.URL.Query().Get("keyboard"),
		Timezone:       r.URL.Query().Get("timezone"),
	}

	// Get the task
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get SpecTask")
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return
	}

	// Authorize user to start planning in the project
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify task is in backlog status
	if task.Status != types.TaskStatusBacklog {
		http.Error(w, fmt.Sprintf("task is not in backlog status (current: %s)", task.Status), http.StatusBadRequest)
		return
	}

	task.PlanningOptions = opts
	task.UpdatedAt = time.Now()

	// Check if Just Do It mode is enabled - skip spec and go straight to implementation
	if task.JustDoItMode {
		task.Status = types.TaskStatusQueuedImplementation
	} else {
		// Normal mode: Start spec generation
		task.Status = types.TaskStatusQueuedSpecGeneration
	}

	// Save the task with queued status first (so response reflects immediate status)
	err = s.Store.UpdateSpecTask(ctx, task)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to update SpecTask to queued status")
		http.Error(w, fmt.Sprintf("failed to update SpecTask: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(task)
}

// updateSpecTask handles PUT /api/v1/spec-tasks/{taskId}
// @Summary Update SpecTask
// @Description Update SpecTask status, priority, or other fields
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "SpecTask ID"
// @Param request body types.SpecTaskUpdateRequest true "Update request"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateSpecTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	var updateReq types.SpecTaskUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&updateReq); err != nil {
		log.Error().Err(err).Msg("Failed to decode SpecTask update request")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing task
	task, err := s.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get SpecTask for update")
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to update task in the project
	if err := s.authorizeUserToProjectByID(r.Context(), user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Update fields if provided
	if updateReq.Status != "" {
		task.Status = updateReq.Status
	}
	if updateReq.Priority != "" {
		task.Priority = updateReq.Priority
	}
	if updateReq.Name != "" {
		task.Name = updateReq.Name
	}
	if updateReq.Description != "" {
		task.Description = updateReq.Description
	}
	if updateReq.JustDoItMode != nil {
		task.JustDoItMode = *updateReq.JustDoItMode
	}
	if updateReq.HelixAppID != "" {
		task.HelixAppID = updateReq.HelixAppID

		// Sync session's ParentApp so restart uses new agent's display settings
		if task.PlanningSessionID != "" {
			session, err := s.Store.GetSession(r.Context(), task.PlanningSessionID)
			if err == nil && session != nil && session.ParentApp != updateReq.HelixAppID {
				session.ParentApp = updateReq.HelixAppID
				if _, err := s.Store.UpdateSession(r.Context(), *session); err != nil {
					log.Warn().Err(err).
						Str("session_id", task.PlanningSessionID).
						Str("new_agent", updateReq.HelixAppID).
						Msg("Failed to update session ParentApp (continuing)")
				} else {
					log.Info().
						Str("session_id", task.PlanningSessionID).
						Str("new_agent", updateReq.HelixAppID).
						Msg("Updated session ParentApp to match spec task agent")
				}
			}
		}
	}
	// Update user short title (pointer allows clearing with empty string)
	if updateReq.UserShortTitle != nil {
		task.UserShortTitle = *updateReq.UserShortTitle
	}

	// Update in store
	err = s.Store.UpdateSpecTask(r.Context(), task)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to update SpecTask")
		http.Error(w, fmt.Sprintf("failed to update SpecTask: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// archiveSpecTask godoc
// @Summary Archive or unarchive a spec task
// @Description Archive a spec task to hide it from the main view, or unarchive to restore it
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "Task ID"
// @Param request body types.SpecTaskArchiveRequest true "Archive request"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/archive [patch]
// @Security BearerAuth
func (s *HelixAPIServer) archiveSpecTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	var req types.SpecTaskArchiveRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().Err(err).Msg("Failed to decode archive request")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get existing task
	task, err := s.Store.GetSpecTask(r.Context(), taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to get SpecTask for archiving")
		http.Error(w, "SpecTask not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to archive task in the project
	if err := s.authorizeUserToProjectByID(r.Context(), user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// When archiving, stop any running external agents
	if req.Archived {
		// Stop the external agent if it's running
		if task.PlanningSessionID != "" {
			session, sessionErr := s.Store.GetSession(r.Context(), task.PlanningSessionID)
			if sessionErr == nil && session.Metadata.AgentType == "zed_external" {
				stopErr := s.externalAgentExecutor.StopDesktop(r.Context(), task.PlanningSessionID)
				if stopErr != nil {
					log.Warn().
						Err(stopErr).
						Str("task_id", taskID).
						Str("session_id", task.PlanningSessionID).
						Msg("Failed to stop agent session when archiving (continuing anyway)")
				} else {
					log.Info().
						Str("task_id", taskID).
						Str("session_id", task.PlanningSessionID).
						Msg("Stopped agent session before archiving")
				}
			}
		}

		// Stop any implementation session agents
		// Get all sessions for this task's project and stop ones related to this task
		externalAgent, agentErr := s.Store.GetSpecTaskExternalAgent(r.Context(), taskID)
		if agentErr == nil && externalAgent != nil && externalAgent.Status == "running" {
			stopErr := s.externalAgentExecutor.StopDesktop(r.Context(), externalAgent.ID)
			if stopErr != nil {
				log.Warn().
					Err(stopErr).
					Str("task_id", taskID).
					Str("agent_id", externalAgent.ID).
					Msg("Failed to stop external agent when archiving (continuing anyway)")
			} else {
				// Update agent status
				externalAgent.Status = "stopped"
				_ = s.Store.UpdateSpecTaskExternalAgent(r.Context(), externalAgent)

				log.Info().
					Str("task_id", taskID).
					Str("agent_id", externalAgent.ID).
					Msg("Stopped external agent before archiving")
			}
		}
	}

	// Update archived status
	task.Archived = req.Archived

	// Update in store
	err = s.Store.UpdateSpecTask(r.Context(), task)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to archive SpecTask")
		http.Error(w, fmt.Sprintf("failed to archive SpecTask: %v", err), http.StatusInternalServerError)
		return
	}

	action := "archived"
	if !req.Archived {
		action = "unarchived"
	}
	log.Info().Str("task_id", taskID).Bool("archived", req.Archived).Msgf("SpecTask %s", action)

	// Log audit event for archive/unarchive
	if s.auditLogService != nil {
		if req.Archived {
			s.auditLogService.LogTaskArchived(r.Context(), task, user.ID, user.Email)
		} else {
			s.auditLogService.LogTaskUnarchived(r.Context(), task, user.ID, user.Email)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func getSpecificationStatus(taskStatus types.SpecTaskStatus) string {
	switch taskStatus {
	case types.TaskStatusBacklog:
		return "pending"
	case types.TaskStatusSpecGeneration:
		return "in_progress"
	case types.TaskStatusSpecReview:
		return "awaiting_review"
	case types.TaskStatusSpecRevision:
		return "revision_requested"
	case types.TaskStatusSpecApproved:
		return "completed"
	case types.TaskStatusSpecFailed:
		return "failed"
	default:
		if isImplementationStatus(taskStatus) {
			return "completed"
		}
		return "unknown"
	}
}

func getImplementationStatus(taskStatus types.SpecTaskStatus) string {
	switch taskStatus {
	case types.TaskStatusImplementationQueued:
		return "queued"
	case types.TaskStatusImplementation:
		return "in_progress"
	case types.TaskStatusImplementationReview:
		return "code_review"
	case types.TaskStatusDone:
		return "completed"
	case types.TaskStatusImplementationFailed:
		return "failed"
	default:
		return "pending"
	}
}

func isImplementationStatus(status types.SpecTaskStatus) bool {
	implementationStatuses := []types.SpecTaskStatus{
		types.TaskStatusImplementationQueued,
		types.TaskStatusImplementation,
		types.TaskStatusImplementationReview,
		types.TaskStatusDone,
		types.TaskStatusImplementationFailed,
	}
	for _, s := range implementationStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// Response types
type TaskSpecsResponse struct {
	TaskID             string               `json:"task_id"`
	Status             types.SpecTaskStatus `json:"status"`
	OriginalPrompt     string               `json:"original_prompt"`
	RequirementsSpec   string               `json:"requirements_spec"`
	TechnicalDesign    string               `json:"technical_design"`
	ImplementationPlan string               `json:"implementation_plan"`
	SpecApprovedBy     string               `json:"spec_approved_by,omitempty"`
	SpecApprovedAt     *time.Time           `json:"spec_approved_at,omitempty"`
	SpecRevisionCount  int                  `json:"spec_revision_count"`
}

type TaskProgressResponse struct {
	TaskID         string                   `json:"task_id"`
	Status         string                   `json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`
	Specification  PhaseProgress            `json:"specification"`
	Implementation PhaseProgress            `json:"implementation"`
	Checklist      *types.ChecklistProgress `json:"checklist,omitempty"` // Progress from tasks.md
}

type PhaseProgress struct {
	Status        string     `json:"status"`
	Agent         string     `json:"agent,omitempty"`
	SessionID     string     `json:"session_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	RevisionCount int        `json:"revision_count,omitempty"`
}

// getChecklistProgress fetches task checklist progress from helix-specs branch
func (s *HelixAPIServer) getChecklistProgress(ctx context.Context, task *types.SpecTask) *types.ChecklistProgress {
	// Get the project's default repository
	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		log.Debug().Err(err).Str("project_id", task.ProjectID).Msg("Could not get project for checklist progress")
		return nil
	}

	if project.DefaultRepoID == "" {
		return nil
	}

	// Get repository path
	repo, err := s.gitRepositoryService.GetRepository(ctx, project.DefaultRepoID)
	if err != nil {
		log.Debug().Err(err).Str("repo_id", project.DefaultRepoID).Msg("Could not get repository for checklist progress")
		return nil
	}

	// Parse task progress from tasks.md in helix-specs branch
	taskProgress, err := services.ParseTaskProgress(repo.LocalPath, task.ID, task.DesignDocPath)
	if err != nil {
		log.Debug().Err(err).Str("task_id", task.ID).Msg("Could not parse task progress from helix-specs")
		return nil
	}

	// Convert to response type
	checklist := &types.ChecklistProgress{
		Tasks:          make([]types.ChecklistItem, len(taskProgress.Tasks)),
		TotalTasks:     taskProgress.TotalTasks,
		CompletedTasks: taskProgress.CompletedTasks,
		ProgressPct:    taskProgress.ProgressPct,
	}

	for i, t := range taskProgress.Tasks {
		checklist.Tasks[i] = types.ChecklistItem{
			Index:       t.Index,
			Description: t.Description,
			Status:      string(t.Status),
		}
	}

	if taskProgress.InProgressTask != nil {
		checklist.InProgressTask = &types.ChecklistItem{
			Index:       taskProgress.InProgressTask.Index,
			Description: taskProgress.InProgressTask.Description,
			Status:      string(taskProgress.InProgressTask.Status),
		}
	}

	return checklist
}

// getBoardSettings godoc
// @Summary Get board settings for spec tasks
// @Description Get the Kanban board settings (WIP limits) for the default project
// @Tags spec-driven-tasks
// @Produce json
// @Success 200 {object} types.BoardSettings
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/board-settings [get]
// @Security BearerAuth
func (s *HelixAPIServer) getBoardSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get default project
	project, err := s.Store.GetProject(ctx, "default")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get default project")
		http.Error(w, "failed to get board settings", http.StatusInternalServerError)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to get board settings in the project
	if err := s.authorizeUserToProjectByID(ctx, user, project.ID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Return board settings or defaults
	var boardSettings types.BoardSettings
	if project.Metadata.BoardSettings != nil {
		boardSettings = *project.Metadata.BoardSettings
	} else {
		// Return default settings if not found
		boardSettings = types.BoardSettings{
			WIPLimits: types.WIPLimits{
				Planning:       3,
				Review:         2,
				Implementation: 5,
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(boardSettings)
}

// updateBoardSettings godoc
// @Summary Update board settings for spec tasks
// @Description Update the Kanban board settings (WIP limits) for the default project
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param request body types.BoardSettings true "Board settings"
// @Success 200 {object} types.BoardSettings
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/board-settings [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateBoardSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to update board settings in the project
	if err := s.authorizeUserToProjectByID(ctx, user, "default", types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var settings types.BoardSettings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		log.Error().Err(err).Msg("Failed to decode board settings")
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Get default project
	project, err := s.Store.GetProject(ctx, "default")
	if err != nil {
		log.Error().Err(err).Msg("Failed to get default project")
		http.Error(w, "failed to update board settings", http.StatusInternalServerError)
		return
	}

	// Update board settings in metadata
	project.Metadata.BoardSettings = &settings
	project.UpdatedAt = time.Now()

	err = s.Store.UpdateProject(ctx, project)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save project")
		http.Error(w, "failed to save board settings", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Interface("wip_limits", settings.WIPLimits).
		Msg("Updated board settings")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}
