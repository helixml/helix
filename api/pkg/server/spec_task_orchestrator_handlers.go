package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Create SpecTask from demo repo
// @Description Create a new SpecTask with a demo repository
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param request body CreateSpecTaskFromDemoRequest true "Demo task request"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/from-demo [post]
func (apiServer *HelixAPIServer) createSpecTaskFromDemo(_ http.ResponseWriter, req *http.Request) (*types.SpecTask, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)

	var demoReq CreateSpecTaskFromDemoRequest
	err := json.NewDecoder(req.Body).Decode(&demoReq)
	if err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate demo repo
	validDemos := map[string]bool{
		"nodejs-todo":       true,
		"python-api":        true,
		"react-dashboard":   true,
		"linkedin-outreach": true,
		"helix-blog-posts":  true,
		"empty":             true,
	}

	if !validDemos[demoReq.DemoRepo] {
		return nil, system.NewHTTPError400(fmt.Sprintf("invalid demo repo: %s", demoReq.DemoRepo))
	}

	// Clone demo repo to user's namespace
	repoName := fmt.Sprintf("%s-%d", demoReq.DemoRepo, req.Context().Value("request_time"))
	repo, err := apiServer.gitRepositoryService.CreateSampleRepository(
		ctx,
		&types.CreateSampleRepositoryRequest{
			Name:           repoName,
			Description:    fmt.Sprintf("Demo repository for SpecTask"),
			OwnerID:        user.ID,
			OrganizationID: demoReq.OrganizationID,
			SampleType:     demoReq.DemoRepo,
			KoditIndexing:  true,
			CreatorName:    user.FullName,
			CreatorEmail:   user.Email,
		},
	)
	if err != nil {
		log.Error().Err(err).Str("demo_repo", demoReq.DemoRepo).Msg("Failed to create sample repository")
		return nil, system.NewHTTPError500("failed to create demo repository")
	}

	// Get user's default external agent app for spec tasks
	defaultApp, err := apiServer.getUserDefaultExternalAgentApp(ctx, user.ID)
	if err != nil {
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Msg("Failed to get default external agent app, spec task may fail to start")
	}

	// Create SpecTask
	task := &types.SpecTask{
		ID:             system.GenerateSpecTaskID(),
		ProjectID:      repo.ID,
		Name:           demoReq.Prompt[:min(len(demoReq.Prompt), 100)],
		Description:    demoReq.Prompt,
		Type:           demoReq.Type,
		Priority:       types.SpecTaskPriority(demoReq.Priority),
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: demoReq.Prompt,
		CreatedBy:      user.ID,
	}

	// Set HelixAppID if we found a default app
	if defaultApp != nil {
		task.HelixAppID = defaultApp.ID
	}

	err = apiServer.Store.CreateSpecTask(ctx, task)
	if err != nil {
		return nil, system.NewHTTPError500("failed to create spec task")
	}

	log.Info().
		Str("task_id", task.ID).
		Str("demo_repo", demoReq.DemoRepo).
		Str("repo_path", repo.LocalPath).
		Msg("Created SpecTask with demo repository")

	return task, nil
}

// @Summary Get design docs for SpecTask
// @Description Get the design documents from helix-specs worktree
// @Tags SpecTasks
// @Produce json
// @Param id path string true "SpecTask ID"
// @Success 200 {object} DesignDocsResponse
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/design-docs [get]
func (apiServer *HelixAPIServer) getSpecTaskDesignDocs(_ http.ResponseWriter, req *http.Request) (*DesignDocsResponse, *system.HTTPError) {
	ctx := req.Context()
	vars := mux.Vars(req)
	taskID := vars["id"]

	// Get task
	task, err := apiServer.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		return nil, system.NewHTTPError404("task not found")
	}

	// Get the project's default repository (design docs repo)
	project, err := apiServer.Store.GetProject(ctx, task.ProjectID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get project")
	}

	if project.DefaultRepoID == "" {
		return nil, system.NewHTTPError400("project has no default repository")
	}

	// Get repository
	repo, err := apiServer.gitRepositoryService.GetRepository(ctx, project.DefaultRepoID)
	if err != nil {
		return nil, system.NewHTTPError500("failed to get repository")
	}

	// Read design docs from Git repository helix-specs branch
	// Docs are in task-specific subdirectory: tasks/{date}_{name}_{task_id}/
	designDocs, err := apiServer.readDesignDocsFromGit(repo.LocalPath, task.ID)
	if err != nil {
		log.Error().Err(err).Str("repo_path", repo.LocalPath).Str("task_id", task.ID).Msg("Failed to read design docs from git")
		return nil, system.NewHTTPError500("failed to read design docs")
	}

	response := &DesignDocsResponse{
		TaskID:    task.ID,
		Documents: designDocs,
	}

	return response, nil
}

// readDesignDocsFromGit reads design documents from the helix-specs branch
func (apiServer *HelixAPIServer) readDesignDocsFromGit(repoPath string, taskID string) ([]DesignDocument, error) {
	// First, list all files in helix-specs branch to find task directory
	// Format: design/tasks/{date}_{name}_{task_id}/
	cmd := exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// helix-specs branch might not exist yet
		log.Debug().Err(err).Str("repo_path", repoPath).Msg("No helix-specs branch found")
		return []DesignDocument{}, nil
	}

	// Find task directory by matching task ID in any file path
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var taskDir string
	for _, file := range files {
		if strings.Contains(file, taskID) {
			// Extract directory path (e.g., design/tasks/2025-11-11_..._taskid/)
			parts := strings.Split(file, "/")
			if len(parts) >= 3 {
				taskDir = strings.Join(parts[:len(parts)-1], "/")
				break
			}
		}
	}

	if taskDir == "" {
		log.Debug().Str("task_id", taskID).Msg("No design docs directory found for task in helix-specs")
		return []DesignDocument{}, nil
	}

	log.Info().Str("task_dir", taskDir).Str("task_id", taskID).Msg("Found design docs directory")

	// Read all .md files from the task directory
	var docs []DesignDocument
	for _, file := range files {
		if !strings.HasPrefix(file, taskDir+"/") || !strings.HasSuffix(file, ".md") {
			continue
		}

		// Read file content from helix-specs branch
		contentCmd := exec.Command("git", "show", fmt.Sprintf("helix-specs:%s", file))
		contentCmd.Dir = repoPath
		content, err := contentCmd.CombinedOutput()
		if err != nil {
			log.Warn().Err(err).Str("file", file).Msg("Failed to read design doc file")
			continue
		}

		// Extract just the filename (e.g., requirements.md)
		filename := strings.TrimPrefix(file, taskDir+"/")

		docs = append(docs, DesignDocument{
			Filename: filename,
			Content:  string(content),
			Path:     file,
		})
	}

	log.Info().Int("doc_count", len(docs)).Str("task_id", taskID).Msg("Read design documents from helix-specs")
	return docs, nil
}

// Response types

type CreateSpecTaskFromDemoRequest struct {
	Prompt         string `json:"prompt" validate:"required"`
	DemoRepo       string `json:"demo_repo" validate:"required"`
	Type           string `json:"type"`
	Priority       string `json:"priority"`
	OrganizationID string `json:"organization_id"`
}

type DesignDocsResponse struct {
	TaskID    string           `json:"task_id"`
	Documents []DesignDocument `json:"documents"`
}

type DesignDocument struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
	Path     string `json:"path"`
}

// Helper functions

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetOrchestrator returns the orchestrator instance
// func (apiServer *HelixAPIServer) GetOrchestrator() *services.SpecTaskOrchestrator {
// 	return apiServer.specTaskOrchestrator
// }

// GetGitService returns the git repository service
// func (apiServer *HelixAPIServer) GetGitService() *services.GitRepositoryService {
// 	return apiServer.gitRepositoryService
// }

// @Summary Stop SpecTask external agent
// @Description Manually stop the external agent for a SpecTask (frees GPU)
// @Tags SpecTasks
// @Param id path string true "SpecTask ID"
// @Success 200 {string} string "OK"
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/external-agent/stop [post]
func (apiServer *HelixAPIServer) stopSpecTaskExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	specTaskID := vars["id"]

	// Get SpecTask
	task, err := apiServer.Store.GetSpecTask(req.Context(), specTaskID)
	if err != nil {
		http.Error(res, "SpecTask not found", http.StatusNotFound)
		return
	}

	// Check user owns this task
	if task.CreatedBy != user.ID {
		http.Error(res, "not authorized to stop this agent", http.StatusUnauthorized)
		return
	}

	// Get external agent
	externalAgent, err := apiServer.Store.GetSpecTaskExternalAgent(req.Context(), specTaskID)
	if err != nil {
		http.Error(res, "External agent not found", http.StatusNotFound)
		return
	}

	if externalAgent.Status != "running" {
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(types.SpecTaskStopResponse{
			Message:         "External agent is already stopped",
			Status:          externalAgent.Status,
			ExternalAgentID: externalAgent.ID,
			WorkspaceDir:    externalAgent.WorkspaceDir,
		})
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("external_agent_id", externalAgent.ID).
		Str("wolf_app_id", externalAgent.WolfAppID).
		Msg("Manually stopping SpecTask external agent")

	// Stop Wolf app
	err = apiServer.externalAgentExecutor.StopDesktop(req.Context(), externalAgent.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stop external agent")
		http.Error(res, fmt.Sprintf("Failed to stop external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Update external agent status
	externalAgent.Status = "stopped"
	externalAgent.LastActivity = time.Now()
	err = apiServer.Store.UpdateSpecTaskExternalAgent(req.Context(), externalAgent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update external agent status")
	}

	// Update all sessions to reflect agent is stopped
	for _, sessionID := range externalAgent.HelixSessionIDs {
		session, err := apiServer.Store.GetSession(req.Context(), sessionID)
		if err == nil {
			session.Metadata.ExternalAgentStatus = "stopped_manual"
			apiServer.Store.UpdateSession(req.Context(), *session)
		}
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(types.SpecTaskStopResponse{
		Message:         "External agent stopped successfully",
		ExternalAgentID: externalAgent.ID,
		WorkspaceDir:    externalAgent.WorkspaceDir,
		Note:            "Workspace preserved in filestore - use start endpoint to resume",
	})
}

// @Summary Start SpecTask external agent
// @Description Start or resume the external agent for a SpecTask (allocates GPU)
// @Tags SpecTasks
// @Param id path string true "SpecTask ID"
// @Success 200 {string} string "OK"
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/external-agent/start [post]
func (apiServer *HelixAPIServer) startSpecTaskExternalAgent(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	specTaskID := vars["id"]

	// Get SpecTask
	task, err := apiServer.Store.GetSpecTask(req.Context(), specTaskID)
	if err != nil {
		http.Error(res, "SpecTask not found", http.StatusNotFound)
		return
	}

	// Check user owns this task
	if task.CreatedBy != user.ID {
		http.Error(res, "not authorized to start this agent", http.StatusUnauthorized)
		return
	}

	// Get external agent
	externalAgent, err := apiServer.Store.GetSpecTaskExternalAgent(req.Context(), specTaskID)
	if err != nil {
		http.Error(res, "External agent not found - create SpecTask first", http.StatusNotFound)
		return
	}

	if externalAgent.Status == "running" {
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(types.SpecTaskStartResponse{
			Message:         "External agent is already running",
			ExternalAgentID: externalAgent.ID,
			WolfAppID:       externalAgent.WolfAppID,
			WorkspaceDir:    externalAgent.WorkspaceDir,
		})
		return
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Str("external_agent_id", externalAgent.ID).
		Str("workspace_dir", externalAgent.WorkspaceDir).
		Msg("Starting SpecTask external agent (resurrection)")

	// Get project repositories (needed for Zed startup arguments)
	project, err := apiServer.Store.GetProject(req.Context(), task.ProjectID)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get project for resurrection, continuing without repo info")
	}

	// Ensure task.HelixAppID is set - inherit from project default for old tasks
	taskUpdated := false
	if task.HelixAppID == "" && project != nil && project.DefaultHelixAppID != "" {
		task.HelixAppID = project.DefaultHelixAppID
		taskUpdated = true
		log.Info().
			Str("task_id", task.ID).
			Str("helix_app_id", project.DefaultHelixAppID).
			Msg("[Resurrect] Inherited HelixAppID from project default")
	}
	if taskUpdated {
		if err := apiServer.Store.UpdateSpecTask(req.Context(), task); err != nil {
			log.Warn().Err(err).Str("task_id", task.ID).Msg("Failed to persist inherited HelixAppID (continuing)")
		}
		// Also update the planning session's ParentApp if it was empty
		if task.PlanningSessionID != "" {
			session, err := apiServer.Store.GetSession(req.Context(), task.PlanningSessionID)
			if err == nil && session != nil && session.ParentApp == "" {
				session.ParentApp = task.HelixAppID
				if _, err := apiServer.Store.UpdateSession(req.Context(), *session); err != nil {
					log.Warn().Err(err).Str("session_id", session.ID).Msg("Failed to update session ParentApp (continuing)")
				} else {
					log.Info().Str("session_id", session.ID).Str("parent_app", task.HelixAppID).Msg("[Resurrect] Updated session ParentApp")
				}
			}
		}
	}

	var repositoryIDs []string
	var primaryRepoID string
	if project != nil {
		repos, err := apiServer.Store.ListGitRepositories(req.Context(), &types.ListGitRepositoriesRequest{
			ProjectID: task.ProjectID,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get project repositories for resurrection")
		} else {
			for _, repo := range repos {
				repositoryIDs = append(repositoryIDs, repo.ID)
			}
			primaryRepoID = project.DefaultRepoID
			if primaryRepoID == "" && len(repositoryIDs) > 0 {
				primaryRepoID = repositoryIDs[0]
			}
		}
	}

	// Get display settings from app's ExternalAgentConfig (or use defaults)
	displayWidth := 1920
	displayHeight := 1080
	displayRefreshRate := 60
	resolution := ""
	zoomLevel := 0
	desktopType := ""
	if task.HelixAppID != "" {
		app, err := apiServer.Store.GetApp(req.Context(), task.HelixAppID)
		if err == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			width, height := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			displayWidth = width
			displayHeight = height
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				displayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
			resolution = app.Config.Helix.ExternalAgentConfig.Resolution
			zoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			desktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
		}
	}

	// Ensure desktopType has a sensible default (ubuntu) when not set by app config
	// This is critical for video_source_mode: ubuntu uses "pipewire", sway uses "wayland"
	if desktopType == "" {
		desktopType = "ubuntu"
	}

	// Resurrect agent with SAME workspace
	agentReq := &types.ZedAgent{
		SessionID:           externalAgent.ID,
		HelixSessionID:      task.PlanningSessionID, // CRITICAL: Use planning session for settings-sync-daemon to fetch correct CodeAgentConfig
		UserID:              task.CreatedBy,
		WorkDir:             externalAgent.WorkspaceDir, // SAME workspace - all state preserved!
		ProjectPath:         "backend",
		RepositoryIDs:       repositoryIDs,      // Needed for Zed startup arguments
		PrimaryRepositoryID: primaryRepoID,      // Needed for design docs path
		SpecTaskID:          task.ID,            // CRITICAL: Must pass SpecTaskID for correct workspace path computation
		UseHostDocker:       task.UseHostDocker, // Use host Docker socket if requested
		DisplayWidth:        displayWidth,
		DisplayHeight:       displayHeight,
		DisplayRefreshRate:  displayRefreshRate,
		Resolution:          resolution,
		ZoomLevel:           zoomLevel,
		DesktopType:         desktopType,
	}

	// Add user's API token for git operations
	if err := apiServer.addUserAPITokenToAgent(req.Context(), agentReq, task.CreatedBy); err != nil {
		log.Error().Err(err).Str("user_id", task.CreatedBy).Msg("Failed to add user API token for SpecTask resurrection")
		http.Error(res, fmt.Sprintf("Failed to get user API keys: %v", err), http.StatusInternalServerError)
		return
	}

	agentResp, err := apiServer.externalAgentExecutor.StartDesktop(req.Context(), agentReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to start external agent")
		http.Error(res, fmt.Sprintf("Failed to start external agent: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Update external agent status
	externalAgent.WolfAppID = agentResp.WolfAppID
	externalAgent.Status = "running"
	externalAgent.LastActivity = time.Now()
	err = apiServer.Store.UpdateSpecTaskExternalAgent(req.Context(), externalAgent)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update external agent status")
	}

	// Update activity tracking
	err = apiServer.Store.UpsertExternalAgentActivity(req.Context(), &types.ExternalAgentActivity{
		ExternalAgentID: externalAgent.ID,
		SpecTaskID:      task.ID,
		LastInteraction: time.Now(),
		AgentType:       "spectask",
		WolfAppID:       externalAgent.WolfAppID,
		WorkspaceDir:    externalAgent.WorkspaceDir,
		UserID:          task.CreatedBy,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update activity tracking")
	}

	// Update all sessions to reflect agent is running
	for _, sessionID := range externalAgent.HelixSessionIDs {
		session, err := apiServer.Store.GetSession(req.Context(), sessionID)
		if err == nil {
			session.Metadata.ExternalAgentStatus = "running"
			apiServer.Store.UpdateSession(req.Context(), *session)
		}
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(types.SpecTaskStartResponse{
		Message:         "External agent started successfully",
		ExternalAgentID: externalAgent.ID,
		WolfAppID:       agentResp.WolfAppID,
		WorkspaceDir:    externalAgent.WorkspaceDir,
		ScreenshotURL:   agentResp.ScreenshotURL,
		StreamURL:       agentResp.StreamURL,
		Note:            "Agent resumed with all previous state (threads, git repos, Zed state)",
	})
}

// @Summary Get SpecTask external agent status
// @Description Get the current status and info for a SpecTask's external agent
// @Tags SpecTasks
// @Param id path string true "SpecTask ID"
// @Produce json
// @Success 200 {object} SpecTaskExternalAgentStatusResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/spec-tasks/{id}/external-agent/status [get]
func (apiServer *HelixAPIServer) getSpecTaskExternalAgentStatus(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	specTaskID := vars["id"]

	// Get SpecTask
	task, err := apiServer.Store.GetSpecTask(req.Context(), specTaskID)
	if err != nil {
		http.Error(res, "SpecTask not found", http.StatusNotFound)
		return
	}

	// Check user owns this task
	if task.CreatedBy != user.ID {
		http.Error(res, "not authorized to view this agent", http.StatusUnauthorized)
		return
	}

	// Get external agent
	externalAgent, err := apiServer.Store.GetSpecTaskExternalAgent(req.Context(), specTaskID)
	if err != nil {
		// No external agent yet (task hasn't started)
		res.Header().Set("Content-Type", "application/json")
		json.NewEncoder(res).Encode(types.SpecTaskStatusResponse{
			Exists:  false,
			Message: "External agent not created yet - task must enter planning phase",
		})
		return
	}

	// Get activity info for idle time
	var idleMinutes int
	activity, err := apiServer.Store.GetExternalAgentActivity(req.Context(), externalAgent.ID)
	if err == nil {
		idleMinutes = int(time.Since(activity.LastInteraction).Minutes())
	}

	var lastActivityPtr *time.Time
	if !externalAgent.LastActivity.IsZero() {
		lastActivityPtr = &externalAgent.LastActivity
	}

	response := types.SpecTaskStatusResponse{
		Exists:          true,
		ExternalAgentID: externalAgent.ID,
		Status:          externalAgent.Status,
		WolfAppID:       externalAgent.WolfAppID,
		WorkspaceDir:    externalAgent.WorkspaceDir,
		HelixSessionIDs: externalAgent.HelixSessionIDs,
		ZedThreadIDs:    externalAgent.ZedThreadIDs,
		SessionCount:    len(externalAgent.HelixSessionIDs),
		Created:         externalAgent.Created,
		LastActivity:    lastActivityPtr,
		IdleMinutes:     idleMinutes,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// SpecTaskExternalAgentStatusResponse represents external agent status
type SpecTaskExternalAgentStatusResponse struct {
	Exists           bool     `json:"exists"`
	ExternalAgentID  string   `json:"external_agent_id,omitempty"`
	Status           string   `json:"status,omitempty"`
	WolfAppID        string   `json:"wolf_app_id,omitempty"`
	WorkspaceDir     string   `json:"workspace_dir,omitempty"`
	HelixSessionIDs  []string `json:"helix_session_ids,omitempty"`
	SessionCount     int      `json:"session_count,omitempty"`
	IdleMinutes      int      `json:"idle_minutes,omitempty"`
	WillTerminateIn  int      `json:"will_terminate_in,omitempty"`
	WarningThreshold bool     `json:"warning_threshold,omitempty"`
}
