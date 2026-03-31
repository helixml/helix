// Spec-driven task handlers with audit logging integration
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
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

	// Compute PR URLs for RepoPullRequests array
	for i, repoPR := range task.RepoPullRequests {
		if repoPR.PRURL == "" && repoPR.PRID != "" {
			repo, err := s.Store.GetGitRepository(ctx, repoPR.RepositoryID)
			if err == nil && repo.ExternalURL != "" {
				task.RepoPullRequests[i].PRURL = services.GetPullRequestURL(repo, repoPR.PRID)
			}
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
// @Param   project_id query string true "Project ID"
// @Param   status query string false "Filter by status"
// @Param   user_id query string false "Filter by user ID"
// @Param   include_archived query bool false "Include archived tasks" default(false)
// @Param   with_depends_on query bool false "Include depends on tasks" default(false)
// @Param   labels query string false "Filter by labels (comma-separated, AND semantics)"
// @Param   limit query int false "Limit number of results" default(50)
// @Param   offset query int false "Offset for pagination" default(0)
// @Success 200 {array} types.SpecTask
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks [get]
func (s *HelixAPIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	projectID := query.Get("project_id")

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

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

	var labelFilter []string
	if labelsParam := query.Get("labels"); labelsParam != "" {
		for _, l := range strings.Split(labelsParam, ",") {
			if trimmed := strings.TrimSpace(l); trimmed != "" {
				labelFilter = append(labelFilter, trimmed)
			}
		}
	}

	filters := &types.SpecTaskFilters{
		ProjectID:       projectID,
		Status:          types.SpecTaskStatus(query.Get("status")),
		UserID:          query.Get("user_id"),
		WithDependsOn:   query.Get("with_depends_on") == "true",
		Limit:           parseIntQuery(query.Get("limit"), 0), // 0 = no limit, return all tasks
		Offset:          parseIntQuery(query.Get("offset"), 0),
		IncludeArchived: query.Get("include_archived") == "true",
		ArchivedOnly:    query.Get("archived_only") == "true",
		Labels:          labelFilter,
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

	// Compute PR URLs for RepoPullRequests array
	if projectID != "" {
		// Batch load repos for RepoPullRequests URL computation
		// Collect all unique repo IDs from all tasks
		repoIDsMap := make(map[string]bool)
		for _, task := range tasks {
			for _, repoPR := range task.RepoPullRequests {
				if repoPR.PRURL == "" && repoPR.PRID != "" {
					repoIDsMap[repoPR.RepositoryID] = true
				}
			}
		}

		// Load repos and build lookup map
		repoMap := make(map[string]*types.GitRepository)
		for repoID := range repoIDsMap {
			repo, err := s.Store.GetGitRepository(ctx, repoID)
			if err == nil {
				repoMap[repoID] = repo
			}
		}

		// Compute URLs for RepoPullRequests
		for _, task := range tasks {
			for i, repoPR := range task.RepoPullRequests {
				if repoPR.PRURL == "" && repoPR.PRID != "" {
					if repo, ok := repoMap[repoPR.RepositoryID]; ok && repo.ExternalURL != "" {
						task.RepoPullRequests[i].PRURL = services.GetPullRequestURL(repo, repoPR.PRID)
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
			// Populate SessionUpdatedAt and SandboxState on each task.
			// Live-check the executor to avoid stale DB metadata after sandbox restarts
			// (same pattern as getSession in session_handlers.go).
			for _, task := range tasks {
				if session, ok := sessionMap[task.PlanningSessionID]; ok {
					task.SessionUpdatedAt = &session.Updated

					// Live-check container status against the executor, overriding stale DB values.
					cfg := session.Metadata
					if cfg.ContainerName != "" && s.externalAgentExecutor != nil {
						_, err := s.externalAgentExecutor.GetSession(session.ID)
						if err != nil {
							cfg.ExternalAgentStatus = "stopped"
						} else {
							cfg.ExternalAgentStatus = "running"
						}
					} else if cfg.ContainerName != "" {
						cfg.ExternalAgentStatus = "stopped"
					}

					status := cfg.ExternalAgentStatus
					hasContainer := cfg.ContainerName != ""
					switch {
					case status == "stopped":
						task.SandboxState = "absent"
					case status == "running":
						task.SandboxState = "running"
					case hasContainer:
						task.SandboxState = "running"
					case status == "starting":
						task.SandboxState = "starting"
					default:
						task.SandboxState = "absent"
					}
					task.SandboxStatusMessage = cfg.StatusMessage
				}
			}
		}
	}

	// ETag support: hash the response to avoid sending unchanged data
	jsonBytes, err := json.Marshal(tasks)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	h := fnv.New64a()
	h.Write(jsonBytes)
	etag := fmt.Sprintf(`"%x"`, h.Sum64())

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes) //nolint:errcheck
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

	now := time.Now()
	existingTask.SpecApproval = &req
	existingTask.StatusUpdatedAt = &now
	if req.Approved {
		existingTask.SpecApprovedBy = user.ID
		existingTask.SpecApprovedAt = &now
		existingTask.Status = types.TaskStatusSpecApproved
	} else {
		// Rejection — don't set approval tracking fields, go straight to revision
		existingTask.Status = types.TaskStatusSpecRevision
	}

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

	// Process approval immediately in goroutine (don't wait for orchestrator polling)
	// This sends the implementation instruction to the agent right away
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if err := s.specDrivenTaskService.ApproveSpecs(context.Background(), existingTask); err != nil {
			log.Error().
				Err(err).
				Str("task_id", taskID).
				Msg("Failed to process spec approval (orchestrator will retry)")
		}
	}()

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

// BatchTaskProgressResponse contains progress for all tasks in a project
type BatchTaskProgressResponse struct {
	ProjectID string                          `json:"project_id"`
	Tasks     map[string]TaskProgressResponse `json:"tasks"` // keyed by task_id
}

// getBatchTaskProgress godoc
// @Summary Get progress for all tasks in a project
// @Description Get progress information for all spec-driven tasks in a project in a single request. This is more efficient than calling the individual progress endpoint for each task.
// @Tags    spec-driven-tasks
// @Produce json
// @Param   id path string true "Project ID"
// @Param   include_checklist query bool false "Include checklist progress (slower, parses git files)" default(false)
// @Success 200 {object} BatchTaskProgressResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/projects/{id}/tasks-progress [get]
func (s *HelixAPIServer) getBatchTaskProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["id"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Authorize user to access the project
	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionGet); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if checklist progress is requested (it's slower due to git file parsing)
	includeChecklist := r.URL.Query().Get("include_checklist") == "true"

	// Get all non-archived tasks for this project in a single query
	tasks, err := s.Store.ListSpecTasks(ctx, &types.SpecTaskFilters{
		ProjectID:       projectID,
		IncludeArchived: false,
	})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to list tasks for batch progress")
		http.Error(w, "failed to list tasks", http.StatusInternalServerError)
		return
	}

	// Build response with progress for each task
	response := BatchTaskProgressResponse{
		ProjectID: projectID,
		Tasks:     make(map[string]TaskProgressResponse, len(tasks)),
	}

	for _, task := range tasks {
		progress := TaskProgressResponse{
			TaskID:    task.ID,
			Status:    getSpecificationStatus(task.Status),
			CreatedAt: task.CreatedAt,
			UpdatedAt: task.UpdatedAt,
			Specification: PhaseProgress{
				Status:        getSpecificationStatus(task.Status),
				SessionID:     task.PlanningSessionID,
				StartedAt:     &task.CreatedAt,
				CompletedAt:   task.SpecApprovedAt,
				RevisionCount: task.SpecRevisionCount,
			},
			Implementation: PhaseProgress{
				Status:    getImplementationStatus(task.Status),
				SessionID: task.PlanningSessionID,
				StartedAt: task.SpecApprovedAt,
			},
		}

		response.Tasks[task.ID] = progress
	}

	// Fetch checklists in parallel if requested (expensive but parallelizable)
	if includeChecklist && len(tasks) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		// Limit concurrency to avoid overwhelming the filesystem
		semaphore := make(chan struct{}, 10)

		for _, task := range tasks {
			// Skip checklist parsing for finished tasks — the UI doesn't show
			// checklists for done/pull_request/failed tasks, and parsing git
			// files for 169+ completed tasks is expensive and wasteful.
			switch task.Status {
			case types.TaskStatusDone, types.TaskStatusPullRequest,
				types.TaskStatusSpecFailed, types.TaskStatusImplementationFailed:
				continue
			}

			wg.Add(1)
			go func(t *types.SpecTask) {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				checklistProgress := s.getChecklistProgress(ctx, t)
				if checklistProgress != nil {
					mu.Lock()
					if progress, ok := response.Tasks[t.ID]; ok {
						progress.Checklist = checklistProgress
						response.Tasks[t.ID] = progress
					}
					mu.Unlock()
				}
			}(task)
		}

		wg.Wait()
	}

	writeResponseWithETag(w, r, response)
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
	now := time.Now()
	if task.JustDoItMode {
		task.Status = types.TaskStatusQueuedImplementation
	} else {
		// Normal mode: Start spec generation
		task.Status = types.TaskStatusQueuedSpecGeneration
	}
	task.StatusUpdatedAt = &now

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
	ctx := r.Context()
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
	task, err := s.Store.GetSpecTask(ctx, taskID)
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
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Update fields if provided
	if updateReq.Status != "" {
		task.Status = updateReq.Status
		// Update StatusUpdatedAt so task appears at top of new column in Kanban
		now := time.Now()
		task.StatusUpdatedAt = &now

		// When moving back to backlog, clear lifecycle fields so the task
		// starts fresh. Without this, the orchestrator sees old specs and
		// immediately transitions to spec_review without generating new ones.
		if updateReq.Status == types.TaskStatusBacklog {
			task.RequirementsSpec = ""
			task.TechnicalDesign = ""
			task.ImplementationPlan = ""
			task.DesignDocsPushedAt = nil
			task.DesignDocPath = ""
			task.SpecApprovedBy = ""
			task.SpecApprovedAt = nil
			task.SpecApproval = nil
			task.SpecRevisionCount = 0
			task.ImplementationApprovedBy = ""
			task.ImplementationApprovedAt = nil
			task.PlanningSessionID = ""
			task.ExternalAgentID = ""
			task.ZedInstanceID = ""
			task.LastPushCommitHash = ""
			task.LastPushAt = nil
			task.StartedAt = nil
			task.CompletedAt = nil
			task.PlanningStartedAt = nil
			task.MergedToMain = false
			task.MergedAt = nil
			task.MergeCommitHash = ""
			task.RepoPullRequests = nil
		}
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
			session, err := s.Store.GetSession(ctx, task.PlanningSessionID)
			if err == nil && session != nil && session.ParentApp != updateReq.HelixAppID {
				session.ParentApp = updateReq.HelixAppID
				if _, err := s.Store.UpdateSession(ctx, *session); err != nil {
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
	// Update public design docs setting (pointer allows explicit false)
	if updateReq.PublicDesignDocs != nil {
		task.PublicDesignDocs = *updateReq.PublicDesignDocs
	}
	// Update assignee (pointer allows clearing with empty string to unassign)
	if updateReq.AssigneeID != nil {
		newAssigneeID := *updateReq.AssigneeID
		// Only validate if assigning (not when clearing)
		if newAssigneeID != "" {
			// Validate that assignee is an organization member
			_, err := s.Store.GetOrganizationMembership(ctx, &store.GetOrganizationMembershipQuery{
				OrganizationID: task.OrganizationID,
				UserID:         newAssigneeID,
			})
			if err != nil {
				log.Warn().
					Str("task_id", taskID).
					Str("assignee_id", newAssigneeID).
					Str("org_id", task.OrganizationID).
					Err(err).
					Msg("Assignee is not an organization member")
				http.Error(w, "assignee must be an organization member", http.StatusBadRequest)
				return
			}
		}
		task.AssigneeID = newAssigneeID
	}

	// If depends_on is provided, pass IDs to store via task.DependsOn and let UpdateSpecTask sync associations.
	if updateReq.DependsOn != nil {
		task.DependsOn = make([]types.SpecTask, 0, len(updateReq.DependsOn))
		for _, dependsOnID := range updateReq.DependsOn {
			if dependsOnID == "" {
				continue
			}
			task.DependsOn = append(task.DependsOn, types.SpecTask{ID: dependsOnID})
		}
	}

	// Update in store
	err = s.Store.UpdateSpecTask(ctx, task)
	if err != nil {
		if updateReq.DependsOn != nil {
			switch {
			case err.Error() == "failed to update spec task: task cannot depend on itself",
				err.Error() == "failed to update spec task: depends on task not found",
				err.Error() == "failed to update spec task: depends on task must be in same project",
				err.Error() == "failed to update spec task: circular dependency detected":
				http.Error(w, strings.TrimPrefix(err.Error(), "failed to update spec task: "), http.StatusBadRequest)
				return
			}
		}
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to update SpecTask")
		http.Error(w, fmt.Sprintf("failed to update SpecTask: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// deleteSpecTask godoc
// @Summary Delete a spec task
// @Description Delete a spec task
// @Tags spec-driven-tasks
// @Accept json
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 204 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteSpecTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
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

	// It must be archived to be deleted
	if !task.Archived {
		http.Error(w, "task is not archived, please archive it before deleting", http.StatusBadRequest)
		return
	}

	// Delete the task
	err = s.Store.DeleteSpecTask(r.Context(), taskID)
	if err != nil {
		log.Error().Err(err).Str("task_id", taskID).Msg("Failed to delete SpecTask")
		http.Error(w, fmt.Sprintf("failed to delete SpecTask: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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

	// Parse task progress from tasks.md in helix-specs branch.
	// No need to sync from upstream - helix-specs is only written by Helix agents,
	// so our middle repo always has the latest data.
	taskProgress, parseErr := services.ParseTaskProgress(repo.LocalPath, task.ID, task.DesignDocPath)
	if parseErr != nil {
		log.Debug().Err(parseErr).Str("task_id", task.ID).Msg("Could not parse task progress from helix-specs")
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
