package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// @Summary Get live agent fleet progress
// @Description Get real-time progress of all agents working on SpecTasks
// @Tags SpecTasks
// @Produce json
// @Success 200 {object} LiveAgentFleetProgressResponse
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/agents/fleet/live-progress [get]
func (apiServer *HelixAPIServer) getAgentFleetLiveProgress(_ http.ResponseWriter, req *http.Request) (*LiveAgentFleetProgressResponse, *system.HTTPError) {
	ctx := req.Context()

	// Get orchestrator (would be initialized in server startup)
	orchestrator := apiServer.GetOrchestrator()
	if orchestrator == nil {
		return nil, system.NewHTTPError500("orchestrator not initialized")
	}

	// Get live progress
	progress := orchestrator.GetLiveProgress()

	// Convert to response format
	agents := []AgentProgressItem{}
	for _, p := range progress {
		agents = append(agents, AgentProgressItem{
			AgentID:     p.AgentID,
			TaskID:      p.TaskID,
			TaskName:    p.TaskName,
			CurrentTask: convertTaskItem(p.CurrentTask),
			TasksBefore: convertTaskItems(p.TasksBefore),
			TasksAfter:  convertTaskItems(p.TasksAfter),
			LastUpdate:  p.LastUpdate.Format("2006-01-02T15:04:05Z"),
			Phase:       string(p.Phase),
		})
	}

	response := &LiveAgentFleetProgressResponse{
		Agents:    agents,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	return response, nil
}

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

	// Get git repository service
	gitService := apiServer.GetGitService()
	if gitService == nil {
		return nil, system.NewHTTPError500("git service not initialized")
	}

	// Clone demo repo to user's namespace
	repoName := fmt.Sprintf("%s-%d", demoReq.DemoRepo, req.Context().Value("request_time"))
	repo, err := gitService.CreateSampleRepository(
		ctx,
		repoName,
		fmt.Sprintf("Demo repository for SpecTask"),
		user.ID,
		demoReq.DemoRepo,
	)
	if err != nil {
		log.Error().Err(err).Str("demo_repo", demoReq.DemoRepo).Msg("Failed to create sample repository")
		return nil, system.NewHTTPError500("failed to create demo repository")
	}

	// Create SpecTask
	task := &types.SpecTask{
		ProjectID:      repo.ID,
		Name:           demoReq.Prompt[:min(len(demoReq.Prompt), 100)],
		Description:    demoReq.Prompt,
		Type:           demoReq.Type,
		Priority:       demoReq.Priority,
		Status:         types.TaskStatusBacklog,
		OriginalPrompt: demoReq.Prompt,
		CreatedBy:      user.ID,
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
// @Description Get the design documents from helix-design-docs worktree
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

	// Get orchestrator
	orchestrator := apiServer.GetOrchestrator()
	if orchestrator == nil {
		return nil, system.NewHTTPError500("orchestrator not initialized")
	}

	// Get task
	task, err := apiServer.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		return nil, system.NewHTTPError404("task not found")
	}

	// Get orchestrated task to find design docs path
	// Note: This assumes the task has been through implementation setup
	// In production, we'd store the design docs path in the database

	response := &DesignDocsResponse{
		TaskID:           task.ID,
		ProgressMarkdown: "", // Would read from worktree
		DesignMarkdown:   "", // Would read from worktree
		CurrentTaskIndex: -1,
	}

	return response, nil
}

// Response types

type LiveAgentFleetProgressResponse struct {
	Agents    []AgentProgressItem `json:"agents"`
	Timestamp string              `json:"timestamp"`
}

type AgentProgressItem struct {
	AgentID     string         `json:"agent_id"`
	TaskID      string         `json:"task_id"`
	TaskName    string         `json:"task_name"`
	CurrentTask *TaskItemDTO   `json:"current_task"`
	TasksBefore []TaskItemDTO  `json:"tasks_before"`
	TasksAfter  []TaskItemDTO  `json:"tasks_after"`
	LastUpdate  string         `json:"last_update"`
	Phase       string         `json:"phase"`
}

type TaskItemDTO struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

type CreateSpecTaskFromDemoRequest struct {
	Prompt   string `json:"prompt" validate:"required"`
	DemoRepo string `json:"demo_repo" validate:"required"`
	Type     string `json:"type"`
	Priority string `json:"priority"`
}

type DesignDocsResponse struct {
	TaskID           string `json:"task_id"`
	ProgressMarkdown string `json:"progress_markdown"`
	DesignMarkdown   string `json:"design_markdown"`
	CurrentTaskIndex int    `json:"current_task_index"`
}

// Helper functions

func convertTaskItem(item *services.TaskItem) *TaskItemDTO {
	if item == nil {
		return nil
	}
	return &TaskItemDTO{
		Index:       item.Index,
		Description: item.Description,
		Status:      string(item.Status),
	}
}

func convertTaskItems(items []services.TaskItem) []TaskItemDTO {
	dtos := []TaskItemDTO{}
	for _, item := range items {
		dtos = append(dtos, TaskItemDTO{
			Index:       item.Index,
			Description: item.Description,
			Status:      string(item.Status),
		})
	}
	return dtos
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GetOrchestrator returns the orchestrator instance
func (apiServer *HelixAPIServer) GetOrchestrator() *services.SpecTaskOrchestrator {
	return apiServer.specTaskOrchestrator
}

// GetGitService returns the git repository service
func (apiServer *HelixAPIServer) GetGitService() *services.GitRepositoryService {
	return apiServer.gitRepositoryService
}
