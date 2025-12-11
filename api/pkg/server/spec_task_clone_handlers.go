package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// cloneSpecTask clones a task to multiple projects
// @Summary Clone a spec task to multiple projects
// @Description Clone a spec task (with its prompt, spec, and plan) to other projects
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param taskId path string true "Source task ID"
// @Param request body types.CloneTaskRequest true "Clone request"
// @Success 200 {object} types.CloneTaskResponse
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/clone [post]
// @Security ApiKeyAuth
func (s *HelixAPIServer) cloneSpecTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	// Parse request
	var req types.CloneTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Get source task
	sourceTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Source task not found: %v", err), http.StatusNotFound)
		return
	}

	// Create clone group to track this batch
	cloneGroup := &types.CloneGroup{
		SourceTaskID:        sourceTask.ID,
		SourceProjectID:     sourceTask.ProjectID,
		SourceTaskName:      sourceTask.Name,
		SourcePrompt:        sourceTask.OriginalPrompt,
		SourceRequirements:  sourceTask.RequirementsSpec,
		SourceTechnicalSpec: sourceTask.TechnicalDesign,
		TotalTargets:        len(req.TargetProjectIDs) + len(req.CreateProjects),
		CreatedBy:           user.ID,
	}

	cloneGroup, err = s.Store.CreateCloneGroup(ctx, cloneGroup)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create clone group")
		http.Error(w, fmt.Sprintf("Failed to create clone group: %v", err), http.StatusInternalServerError)
		return
	}

	response := &types.CloneTaskResponse{
		CloneGroupID: cloneGroup.ID,
		ClonedTasks:  []types.CloneTaskResult{},
		Errors:       []types.CloneTaskError{},
	}

	// Clone to existing projects
	for _, projectID := range req.TargetProjectIDs {
		result, err := s.cloneTaskToProject(ctx, sourceTask, projectID, cloneGroup.ID, user.ID, req.AutoStart)
		if err != nil {
			response.Errors = append(response.Errors, types.CloneTaskError{
				ProjectID: projectID,
				Error:     err.Error(),
			})
			response.TotalFailed++
		} else {
			response.ClonedTasks = append(response.ClonedTasks, *result)
			response.TotalCloned++
		}
	}

	// Create projects for repos and clone
	for _, createSpec := range req.CreateProjects {
		// Quick-create project for this repo
		project, err := s.quickCreateProjectForRepo(ctx, createSpec.RepoID, createSpec.Name, user.ID)
		if err != nil {
			response.Errors = append(response.Errors, types.CloneTaskError{
				RepoID: createSpec.RepoID,
				Error:  fmt.Sprintf("Failed to create project: %v", err),
			})
			response.TotalFailed++
			continue
		}

		// Clone to the new project
		result, err := s.cloneTaskToProject(ctx, sourceTask, project.ID, cloneGroup.ID, user.ID, req.AutoStart)
		if err != nil {
			response.Errors = append(response.Errors, types.CloneTaskError{
				ProjectID: project.ID,
				RepoID:    createSpec.RepoID,
				Error:     err.Error(),
			})
			response.TotalFailed++
		} else {
			response.ClonedTasks = append(response.ClonedTasks, *result)
			response.TotalCloned++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// cloneTaskToProject creates a copy of a task in the target project
func (s *HelixAPIServer) cloneTaskToProject(ctx context.Context, source *types.SpecTask, projectID, cloneGroupID, userID string, autoStart bool) (*types.CloneTaskResult, error) {
	// Create new task with copied data
	newTask := &types.SpecTask{
		ID:                 system.GenerateSpecTaskID(),
		ProjectID:          projectID,
		Name:               source.Name,
		Description:        source.Description,
		Type:               source.Type,
		Priority:           source.Priority,
		Status:             "backlog",
		OriginalPrompt:     source.OriginalPrompt,
		RequirementsSpec:   source.RequirementsSpec,
		TechnicalDesign:    source.TechnicalDesign,
		ImplementationPlan: source.ImplementationPlan,
		JustDoItMode:       source.JustDoItMode,
		UseHostDocker:      source.UseHostDocker,
		ClonedFromID:       source.ID,
		ClonedFromProjectID: source.ProjectID,
		CloneGroupID:       cloneGroupID,
		CreatedBy:          userID,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.Store.CreateSpecTask(ctx, newTask); err != nil {
		return nil, fmt.Errorf("failed to create cloned task: %w", err)
	}

	result := &types.CloneTaskResult{
		TaskID:    newTask.ID,
		ProjectID: projectID,
		Status:    "created",
	}

	// Auto-start if requested
	if autoStart {
		go func() {
			// Use background context since the HTTP request will complete before this finishes
			bgCtx := context.Background()

			// Start spec generation using the spec-driven task service
			if newTask.JustDoItMode {
				s.specDrivenTaskService.StartJustDoItMode(bgCtx, newTask)
			} else {
				s.specDrivenTaskService.StartSpecGeneration(bgCtx, newTask)
			}

			log.Info().Str("task_id", newTask.ID).Msg("Auto-started cloned task")
		}()
		result.Status = "started"
	}

	return result, nil
}

// quickCreateProjectForRepo creates a minimal project for a repository
func (s *HelixAPIServer) quickCreateProjectForRepo(ctx context.Context, repoID, name, userID string) (*types.Project, error) {
	// Get the repo
	repo, err := s.Store.GetGitRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	// Use repo name if no name provided
	if name == "" {
		name = repo.Name
	}

	// Create project
	project := &types.Project{
		ID:             system.GenerateProjectID(),
		Name:           name,
		Description:    fmt.Sprintf("Project for %s", repo.Name),
		UserID:         userID,
		OrganizationID: repo.OrganizationID,
		DefaultRepoID:  repoID,
		Status:         "active",
	}

	created, err := s.Store.CreateProject(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	project = created

	// Attach the repo to the project
	if err := s.Store.AttachRepositoryToProject(ctx, project.ID, repoID); err != nil {
		log.Warn().Err(err).Msg("Failed to attach repository to project")
	}

	return project, nil
}

// listCloneGroups lists all clone groups where a task was the source
// @Summary List clone groups for a task
// @Description Get all clone groups where this task was the source
// @Tags SpecTasks
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {array} types.CloneGroup
// @Router /api/v1/spec-tasks/{taskId}/clone-groups [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) listCloneGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	groups, err := s.Store.ListCloneGroupsForTask(ctx, taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list clone groups: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}

// getCloneGroupProgress gets progress of all tasks in a clone group
// @Summary Get progress of all tasks in a clone group
// @Description Get status breakdown and progress of all cloned tasks
// @Tags CloneGroups
// @Produce json
// @Param groupId path string true "Clone group ID"
// @Success 200 {object} types.CloneGroupProgress
// @Failure 404 {object} types.APIError
// @Router /api/v1/clone-groups/{groupId}/progress [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) getCloneGroupProgress(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	groupID := vars["groupId"]

	progress, err := s.Store.GetCloneGroupProgress(ctx, groupID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get clone group progress: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(progress)
}

// listReposWithoutProjects lists repositories without associated projects
// @Summary List repositories without projects
// @Description Get all repositories that don't have an associated project
// @Tags Repositories
// @Produce json
// @Param organization_id query string false "Filter by organization ID"
// @Success 200 {array} types.GitRepository
// @Router /api/v1/repositories/without-projects [get]
// @Security ApiKeyAuth
func (s *HelixAPIServer) listReposWithoutProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgID := r.URL.Query().Get("organization_id")

	repos, err := s.Store.ListReposWithoutProjects(ctx, orgID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list repositories: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repos)
}

// QuickCreateProjectRequest for creating a project from a repo
type QuickCreateProjectRequest struct {
	RepoID string `json:"repo_id"`
	Name   string `json:"name,omitempty"`
}

// quickCreateProject creates a project for a repository
// @Summary Quick-create a project for a repository
// @Description Create a minimal project for a repository that doesn't have one
// @Tags Projects
// @Accept json
// @Produce json
// @Param request body QuickCreateProjectRequest true "Quick create request"
// @Success 200 {object} types.Project
// @Failure 400 {object} types.APIError
// @Router /api/v1/projects/quick-create [post]
// @Security ApiKeyAuth
func (s *HelixAPIServer) quickCreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)

	var req QuickCreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	if req.RepoID == "" {
		http.Error(w, "repo_id is required", http.StatusBadRequest)
		return
	}

	project, err := s.quickCreateProjectForRepo(ctx, req.RepoID, req.Name, user.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}
