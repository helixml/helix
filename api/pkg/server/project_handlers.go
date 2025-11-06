package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/wolf"
	"github.com/rs/zerolog/log"
)

// listProjects godoc
// @Summary List projects
// @Description Get all projects for the current user
// @Tags Projects
// @Accept json
// @Produce json
// @Success 200 {array} types.Project
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects [get]
func (s *HelixAPIServer) listProjects(_ http.ResponseWriter, r *http.Request) ([]*types.Project, *system.HTTPError) {
	user := getRequestUser(r)

	projects, err := s.Store.ListProjects(r.Context(), user.ID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Msg("failed to list projects")
		return nil, system.NewHTTPError500(err.Error())
	}

	return projects, nil
}

// getProject godoc
// @Summary Get project
// @Description Get a project by ID
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} types.Project
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id} [get]
func (s *HelixAPIServer) getProject(_ http.ResponseWriter, r *http.Request) (*types.Project, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project")
		return nil, system.NewHTTPError404("project not found")
	}

	// Check authorization
	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("project_owner_id", project.UserID).
			Msg("user not authorized to access project")
		return nil, system.NewHTTPError404("project not found")
	}

	// Load startup script from internal Git repo (source of truth)
	if project.InternalRepoPath != "" {
		startupScript, err := s.projectInternalRepoService.LoadStartupScript(project.ID, project.InternalRepoPath)
		if err != nil {
			log.Warn().
				Err(err).
				Str("project_id", projectID).
				Str("internal_repo_path", project.InternalRepoPath).
				Msg("failed to load startup script from internal repo, using database value")
			// Continue with database value if git repo read fails
		} else {
			// Git repo is source of truth - use loaded value
			project.StartupScript = startupScript
		}
	}

	return project, nil
}

// createProject godoc
// @Summary Create project
// @Description Create a new project
// @Tags Projects
// @Accept json
// @Produce json
// @Param request body types.ProjectCreateRequest true "Project creation request"
// @Success 200 {object} types.Project
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects [post]
func (s *HelixAPIServer) createProject(_ http.ResponseWriter, r *http.Request) (*types.Project, *system.HTTPError) {
	user := getRequestUser(r)

	var req types.ProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().
			Err(err).
			Msg("failed to decode project create request")
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate required fields
	if req.Name == "" {
		return nil, system.NewHTTPError400("project name is required")
	}

	project := &types.Project{
		ID:             system.GenerateUUID(),
		Name:           req.Name,
		Description:    req.Description,
		UserID:         user.ID,
		GitHubRepoURL:  req.GitHubRepoURL,
		DefaultBranch:  req.DefaultBranch,
		Technologies:   req.Technologies,
		Status:         "active",
		DefaultRepoID:  req.DefaultRepoID,
		StartupScript:  req.StartupScript,
	}

	created, err := s.Store.CreateProject(r.Context(), project)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_name", req.Name).
			Msg("failed to create project")
		return nil, system.NewHTTPError500(err.Error())
	}

	// Initialize internal Git repository for the project
	internalRepoPath, err := s.projectInternalRepoService.InitializeProjectRepo(r.Context(), created)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", created.ID).
			Msg("failed to initialize internal project repository")
		// Continue anyway - project exists in DB, repo can be created later
	} else {
		// Update project with internal repo path
		created.InternalRepoPath = internalRepoPath
		err = s.Store.UpdateProject(r.Context(), created)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", created.ID).
				Msg("failed to update project with internal repo path")
		} else {
			log.Info().
				Str("project_id", created.ID).
				Str("internal_repo_path", internalRepoPath).
				Msg("Internal project repository initialized and linked")
		}

		// Create a GitRepository entry for the internal repo so it can be browsed
		internalRepoID := fmt.Sprintf("%s-internal", created.ID)
		internalRepo := &store.GitRepository{
			ID:             internalRepoID,
			Name:           fmt.Sprintf("%s-internal", data.SlugifyName(created.Name)),
			Description:    "Internal project repository for configuration and metadata",
			OwnerID:        user.ID,
			OrganizationID: created.OrganizationID,
			ProjectID:      created.ID,
			RepoType:       "internal",
			Status:         "ready",
			LocalPath:      internalRepoPath,
			DefaultBranch:  "main",
			MetadataJSON:   "{}",
		}

		err = s.Store.CreateGitRepository(r.Context(), internalRepo)
		if err != nil {
			log.Warn().
				Err(err).
				Str("project_id", created.ID).
				Msg("failed to create git repository entry for internal repo (continuing)")
			// Continue - internal repo works without DB entry, just can't be browsed
		} else {
			log.Info().
				Str("project_id", created.ID).
				Str("internal_repo_id", internalRepoID).
				Msg("Created git repository entry for internal repo")
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", created.ID).
		Str("project_name", created.Name).
		Msg("project created successfully")

	return created, nil
}

// updateProject godoc
// @Summary Update project
// @Description Update an existing project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body types.ProjectUpdateRequest true "Project update request"
// @Success 200 {object} types.Project
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id} [put]
func (s *HelixAPIServer) updateProject(_ http.ResponseWriter, r *http.Request) (*types.Project, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	var req types.ProjectUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().
			Err(err).
			Msg("failed to decode project update request")
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Get existing project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for update")
		return nil, system.NewHTTPError404("project not found")
	}

	// Check authorization
	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("project_owner_id", project.UserID).
			Msg("user not authorized to update project")
		return nil, system.NewHTTPError404("project not found")
	}

	// Apply updates
	if req.Name != nil {
		project.Name = *req.Name
	}
	if req.Description != nil {
		project.Description = *req.Description
	}
	if req.GitHubRepoURL != nil {
		project.GitHubRepoURL = *req.GitHubRepoURL
	}
	if req.DefaultBranch != nil {
		project.DefaultBranch = *req.DefaultBranch
	}
	if req.Technologies != nil {
		project.Technologies = req.Technologies
	}
	if req.Status != nil {
		project.Status = *req.Status
	}
	if req.DefaultRepoID != nil {
		project.DefaultRepoID = *req.DefaultRepoID
	}
	if req.AutoStartBacklogTasks != nil {
		project.AutoStartBacklogTasks = *req.AutoStartBacklogTasks
	}

	// DON'T update StartupScript in database - Git repo is source of truth
	// It will be saved to git repo below and loaded from there on next fetch

	err = s.Store.UpdateProject(r.Context(), project)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to update project")
		return nil, system.NewHTTPError500(err.Error())
	}

	// Sync changes to internal Git repo
	if project.InternalRepoPath != "" {
		// Update project.json in repo
		if err := s.projectInternalRepoService.UpdateProjectConfig(project); err != nil {
			log.Warn().
				Err(err).
				Str("project_id", projectID).
				Msg("failed to update project config in internal repo (continuing)")
		}

		// If startup script was updated, save to Git repo (source of truth)
		if req.StartupScript != nil {
			if err := s.projectInternalRepoService.SaveStartupScript(projectID, project.InternalRepoPath, *req.StartupScript); err != nil {
				log.Warn().
					Err(err).
					Str("project_id", projectID).
					Msg("failed to save startup script to internal repo (continuing)")
			}
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Msg("project updated successfully")

	return project, nil
}

// deleteProject godoc
// @Summary Delete project
// @Description Delete a project by ID
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id} [delete]
func (s *HelixAPIServer) deleteProject(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Get existing project to check authorization
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for deletion")
		return nil, system.NewHTTPError404("project not found")
	}

	// Check authorization
	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("project_owner_id", project.UserID).
			Msg("user not authorized to delete project")
		return nil, system.NewHTTPError404("project not found")
	}

	// CRITICAL: Stop all active sessions BEFORE deleting the project
	// Otherwise sessions become orphaned and can't be stopped via UI

	// 1. Stop exploratory session if exists
	exploratorySession, err := s.Store.GetProjectExploratorySession(r.Context(), projectID)
	if err == nil && exploratorySession != nil {
		log.Info().
			Str("project_id", projectID).
			Str("session_id", exploratorySession.ID).
			Msg("Stopping exploratory session before project deletion")

		stopErr := s.externalAgentExecutor.StopZedAgent(r.Context(), exploratorySession.ID)
		if stopErr != nil {
			log.Warn().Err(stopErr).Str("session_id", exploratorySession.ID).Msg("Failed to stop exploratory session (continuing with deletion)")
		}
	}

	// 2. Stop all SpecTask planning sessions for this project
	tasks, err := s.Store.ListSpecTasks(r.Context(), &types.SpecTaskFilters{
		ProjectID: projectID,
	})
	if err == nil {
		for _, task := range tasks {
			if task.SpecSessionID != "" {
				log.Info().
					Str("project_id", projectID).
					Str("task_id", task.ID).
					Str("session_id", task.SpecSessionID).
					Msg("Stopping SpecTask planning session before project deletion")

				stopErr := s.externalAgentExecutor.StopZedAgent(r.Context(), task.SpecSessionID)
				if stopErr != nil {
					log.Warn().Err(stopErr).Str("session_id", task.SpecSessionID).Msg("Failed to stop planning session (continuing with deletion)")
				}
			}
		}
	}

	// Now soft delete the project
	err = s.Store.DeleteProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to delete project")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Msg("project archived successfully with all sessions stopped")

	return map[string]string{"message": "project deleted successfully"}, nil
}

// getProjectRepositories godoc
// @Summary Get project repositories
// @Description Get all repositories attached to a project
// @Tags Projects
// @ID getProjectRepositories
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} store.GitRepository
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/repositories [get]
func (s *HelixAPIServer) getProjectRepositories(_ http.ResponseWriter, r *http.Request) (interface{}, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for repositories list")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to access project repositories")
		return nil, system.NewHTTPError404("project not found")
	}

	repos, err := s.Store.GetProjectRepositories(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project repositories")
		return nil, system.NewHTTPError500(err.Error())
	}

	return repos, nil
}

// setProjectPrimaryRepository godoc
// @Summary Set project primary repository
// @Description Set the primary repository for a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/repositories/{repo_id}/primary [put]
func (s *HelixAPIServer) setProjectPrimaryRepository(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	vars := mux.Vars(r)
	projectID := vars["id"]
	repoID := vars["repo_id"]

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for setting primary repository")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to set project primary repository")
		return nil, system.NewHTTPError404("project not found")
	}

	// Verify the repository exists and belongs to this project
	projectRepos, err := s.Store.GetProjectRepositories(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get project repositories")
		return nil, system.NewHTTPError500("failed to verify repository")
	}

	// Check if the repository is in the project's repository list
	repoFound := false
	for _, repo := range projectRepos {
		if repo.ID == repoID {
			repoFound = true
			break
		}
	}

	if !repoFound {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("repo_id", repoID).
			Msg("repository not found in project")
		return nil, system.NewHTTPError400("repository not attached to this project")
	}

	err = s.Store.SetProjectPrimaryRepository(r.Context(), projectID, repoID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("repo_id", repoID).
			Msg("failed to set project primary repository")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Str("repo_id", repoID).
		Msg("project primary repository set successfully")

	return map[string]string{"message": "primary repository set successfully"}, nil
}

// attachRepositoryToProject godoc
// @Summary Attach repository to project
// @Description Attach an existing repository to a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/repositories/{repo_id}/attach [put]
func (s *HelixAPIServer) attachRepositoryToProject(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	vars := mux.Vars(r)
	projectID := vars["id"]
	repoID := vars["repo_id"]

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for attaching repository")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to attach repository to project")
		return nil, system.NewHTTPError404("project not found")
	}

	// Verify the repository exists and user has access to it
	repo, err := s.Store.GetGitRepository(r.Context(), repoID)
	if err != nil {
		log.Error().
			Err(err).
			Str("repo_id", repoID).
			Msg("failed to get repository for attachment")
		return nil, system.NewHTTPError404("repository not found")
	}

	// Check if user owns the repository
	if repo.OwnerID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("repo_id", repoID).
			Str("repo_owner_id", repo.OwnerID).
			Msg("user not authorized to attach this repository")
		return nil, system.NewHTTPError404("repository not found")
	}

	err = s.Store.AttachRepositoryToProject(r.Context(), projectID, repoID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("repo_id", repoID).
			Msg("failed to attach repository to project")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Str("repo_id", repoID).
		Msg("repository attached to project successfully")

	return map[string]string{"message": "repository attached successfully"}, nil
}

// detachRepositoryFromProject godoc
// @Summary Detach repository from project
// @Description Detach a repository from its project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param repo_id path string true "Repository ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/repositories/{repo_id}/detach [put]
func (s *HelixAPIServer) detachRepositoryFromProject(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	vars := mux.Vars(r)
	projectID := vars["id"]
	repoID := vars["repo_id"]

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for detaching repository")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to detach repository from project")
		return nil, system.NewHTTPError404("project not found")
	}

	// Verify the repository is attached to this project
	projectRepos, err := s.Store.GetProjectRepositories(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get project repositories")
		return nil, system.NewHTTPError500("failed to verify repository")
	}

	repoFound := false
	for _, repo := range projectRepos {
		if repo.ID == repoID {
			repoFound = true
			break
		}
	}

	if !repoFound {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("repo_id", repoID).
			Msg("repository not attached to this project")
		return nil, system.NewHTTPError400("repository not attached to this project")
	}

	err = s.Store.DetachRepositoryFromProject(r.Context(), repoID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("repo_id", repoID).
			Msg("failed to detach repository from project")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Str("repo_id", repoID).
		Msg("repository detached from project successfully")

	return map[string]string{"message": "repository detached successfully"}, nil
}

// checkWolfLobbyExists checks if a Wolf lobby exists by querying the Wolf API
func (s *HelixAPIServer) checkWolfLobbyExists(ctx context.Context, lobbyID string) (bool, error) {
	// Get Wolf client from executor
	type WolfClientProvider interface {
		GetWolfClient() *wolf.Client
	}
	provider, ok := s.externalAgentExecutor.(WolfClientProvider)
	if !ok {
		return false, fmt.Errorf("executor does not provide Wolf client")
	}
	wolfClient := provider.GetWolfClient()
	if wolfClient == nil {
		return false, fmt.Errorf("Wolf client is nil")
	}

	// List all lobbies and check if ours exists
	lobbies, err := wolfClient.ListLobbies(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to list lobbies: %w", err)
	}

	for _, lobby := range lobbies {
		if lobby.ID == lobbyID {
			return true, nil
		}
	}

	return false, nil
}

// getProjectExploratorySession godoc
// @Summary Get project exploratory session
// @Description Get the active exploratory session for a project (returns null if none exists)
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} types.Session
// @Success 204 "No exploratory session exists"
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/exploratory-session [get]
func (s *HelixAPIServer) getProjectExploratorySession(_ http.ResponseWriter, r *http.Request) (*types.Session, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for exploratory session check")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to view exploratory session")
		return nil, system.NewHTTPError404("project not found")
	}

	// Get exploratory session
	session, err := s.Store.GetProjectExploratorySession(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get exploratory session")
		return nil, system.NewHTTPError500("failed to get exploratory session")
	}

	// If no session found in database, return nil
	if session == nil {
		return nil, nil
	}

	// Check if the external agent (Wolf lobby) is actually running
	// If not running, update status to "stopped"
	if s.externalAgentExecutor != nil {
		_, err := s.externalAgentExecutor.GetSession(session.ID)
		if err != nil {
			// External agent not running - mark as stopped
			session.Metadata.ExternalAgentStatus = "stopped"
			log.Trace().
				Str("session_id", session.ID).
				Str("project_id", projectID).
				Msg("Exploratory session exists in database but Wolf lobby is stopped")
		} else {
			// External agent is running
			session.Metadata.ExternalAgentStatus = "running"
			log.Trace().
				Str("session_id", session.ID).
				Str("project_id", projectID).
				Msg("Exploratory session is running")
		}
	} else {
		// No external agent executor available - assume stopped
		session.Metadata.ExternalAgentStatus = "stopped"
	}

	return session, nil
}

// startExploratorySession godoc
// @Summary Start project exploratory session
// @Description Start or return existing exploratory session for a project
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} types.Session
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/exploratory-session [post]
func (s *HelixAPIServer) startExploratorySession(_ http.ResponseWriter, r *http.Request) (*types.Session, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for exploratory session")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to start exploratory session")
		return nil, system.NewHTTPError404("project not found")
	}

	// Check if an active exploratory session already exists for this project
	existingSession, err := s.Store.GetProjectExploratorySession(r.Context(), projectID)
	if err == nil && existingSession != nil {
		// Session exists - check if lobby is still running
		// If lobby stopped, restart it with fresh startup script
		lobbyID := existingSession.Metadata.WolfLobbyID
		if lobbyID != "" {
			// Check if lobby exists in Wolf
			lobbyExists, checkErr := s.checkWolfLobbyExists(r.Context(), lobbyID)
			if checkErr != nil {
				log.Warn().Err(checkErr).Str("lobby_id", lobbyID).Msg("Failed to check lobby status")
			}

			if !lobbyExists {
				// Lobby stopped - restart it
				log.Info().
					Str("session_id", existingSession.ID).
					Str("project_id", projectID).
					Str("lobby_id", lobbyID).
					Msg("Exploratory session exists but lobby stopped - restarting with fresh startup script")

				// Get project repositories for restarting
				projectRepos, err := s.Store.GetProjectRepositories(r.Context(), projectID)
				if err != nil {
					log.Warn().Err(err).Str("project_id", projectID).Msg("Failed to get project repositories for restart")
					projectRepos = nil
				}

				// Build repository IDs
				repositoryIDs := []string{}
				for _, repo := range projectRepos {
					if repo.ID != "" {
						repositoryIDs = append(repositoryIDs, repo.ID)
					}
				}

				// Determine primary repository
				primaryRepoID := project.DefaultRepoID
				if primaryRepoID == "" && len(projectRepos) > 0 {
					primaryRepoID = projectRepos[0].ID
				}

				// Get user's API key for git HTTP authentication
				userAPIKeys, keyErr := s.Store.ListAPIKeys(r.Context(), &store.ListAPIKeysQuery{
					Owner:     user.ID,
					OwnerType: types.OwnerTypeUser,
				})
				if keyErr != nil {
					log.Error().Err(keyErr).Str("user_id", user.ID).Msg("Failed to get user API keys for restart")
					return nil, system.NewHTTPError500("failed to get user API keys for git operations")
				}

				if len(userAPIKeys) == 0 {
					log.Error().Str("user_id", user.ID).Msg("User has no API keys - cannot restart exploratory session")
					return nil, system.NewHTTPError500("user has no API keys - create one in Account Settings")
				}

				userAPIToken := userAPIKeys[0].Key

				// Restart Zed agent with existing session
				zedAgent := &types.ZedAgent{
					SessionID:           existingSession.ID,
					UserID:              user.ID,
					Input:               fmt.Sprintf("Explore the %s project", project.Name),
					ProjectPath:         "workspace",
					SpecTaskID:          "",
					ProjectID:           projectID,
					PrimaryRepositoryID: primaryRepoID,
					RepositoryIDs:       repositoryIDs,
					Env: []string{
						fmt.Sprintf("USER_API_TOKEN=%s", userAPIToken),
					},
				}

				agentResp, err := s.externalAgentExecutor.StartZedAgent(r.Context(), zedAgent)
				if err != nil {
					log.Error().
						Err(err).
						Str("session_id", existingSession.ID).
						Msg("Failed to restart exploratory session")
					return nil, system.NewHTTPError500(fmt.Sprintf("failed to restart exploratory session: %v", err))
				}

				log.Info().
					Str("session_id", existingSession.ID).
					Str("lobby_id", agentResp.WolfLobbyID).
					Msg("Exploratory session lobby restarted successfully")

				// Reload session from database to get updated lobby ID/PIN
				// StartZedAgent updates session metadata in DB, so we need fresh data
				updatedSession, err := s.Store.GetSession(r.Context(), existingSession.ID)
				if err != nil {
					log.Error().Err(err).Str("session_id", existingSession.ID).Msg("Failed to reload session after restart")
					return existingSession, nil // Return stale session rather than failing
				}

				return updatedSession, nil
			}
		}

		// Session exists and lobby is running - return as-is
		log.Info().
			Str("session_id", existingSession.ID).
			Str("project_id", projectID).
			Msg("returning existing exploratory session for project")
		return existingSession, nil
	}

	// Create a new session for the exploratory agent
	sessionMetadata := types.SessionMetadata{
		Stream:      true,
		AgentType:   "zed_external",
		ProjectID:   projectID,
		SessionRole: "exploratory",
	}

	session := &types.Session{
		ID:             system.GenerateSessionID(),
		Name:           fmt.Sprintf("Explore: %s", project.Name),
		Created:        time.Now(),
		Updated:        time.Now(),
		ParentSession:  "",
		Mode:           types.SessionModeInference,
		Type:           types.SessionTypeText,
		Provider:       "anthropic",
		ModelName:      "external_agent",
		LoraDir:        "",
		Owner:          user.ID,
		OwnerType:      types.OwnerTypeUser,
		Metadata:       sessionMetadata,
	}

	createdSession, err := s.Store.CreateSession(r.Context(), *session)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to create exploratory session")
		return nil, system.NewHTTPError500("failed to create session")
	}

	// Get all project repositories
	projectRepos, err := s.Store.GetProjectRepositories(r.Context(), projectID)
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Failed to get project repositories for exploratory session")
		projectRepos = nil
	}

	// Build list of repository IDs
	repositoryIDs := []string{}
	for _, repo := range projectRepos {
		if repo.ID != "" {
			repositoryIDs = append(repositoryIDs, repo.ID)
		}
	}

	// Determine primary repository
	primaryRepoID := project.DefaultRepoID
	if primaryRepoID == "" && len(projectRepos) > 0 {
		primaryRepoID = projectRepos[0].ID
	}

	// Get user's API key for git HTTP authentication
	userAPIKeys, err := s.Store.ListAPIKeys(r.Context(), &store.ListAPIKeysQuery{
		Owner:     user.ID,
		OwnerType: types.OwnerTypeUser,
	})
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to get user API keys")
		return nil, system.NewHTTPError500("failed to get user API keys for git operations")
	}

	if len(userAPIKeys) == 0 {
		log.Error().Str("user_id", user.ID).Msg("User has no API keys - cannot clone repositories")
		return nil, system.NewHTTPError500("user has no API keys - create one in Account Settings to use git features")
	}

	userAPIToken := userAPIKeys[0].Key

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Msg("Using user's API key for git operations (RBAC enforced)")

	// Create ZedAgent for exploratory session
	zedAgent := &types.ZedAgent{
		SessionID:           createdSession.ID,
		UserID:              user.ID,
		Input:               fmt.Sprintf("Explore the %s project", project.Name),
		ProjectPath:         "workspace",
		SpecTaskID:          "", // No task - exploratory mode
		ProjectID:           projectID, // For loading project repos and startup script
		PrimaryRepositoryID: primaryRepoID,
		RepositoryIDs:       repositoryIDs,
		Env: []string{
			// Pass user's API key for git operations (NOT server's RunnerToken)
			// This ensures RBAC is enforced - agent can only access repos the user can access
			fmt.Sprintf("USER_API_TOKEN=%s", userAPIToken),
		},
	}

	// Start the Zed agent via Wolf executor
	agentResp, err := s.externalAgentExecutor.StartZedAgent(r.Context(), zedAgent)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", createdSession.ID).
			Str("project_id", projectID).
			Msg("Failed to launch exploratory agent")
		return nil, system.NewHTTPError500("failed to start exploratory agent")
	}

	// Activity tracking now happens in StartZedAgent (wolf_executor.go)
	// for all external agent types (exploratory, spectask, regular agents)

	log.Info().
		Str("session_id", createdSession.ID).
		Str("project_id", projectID).
		Str("wolf_lobby_id", agentResp.WolfLobbyID).
		Msg("Exploratory session created successfully")

	return createdSession, nil
}

// stopExploratorySession godoc
// @Summary Stop project exploratory session
// @Description Stop the running exploratory session for a project (stops Wolf container, keeps session record)
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/exploratory-session [delete]
func (s *HelixAPIServer) stopExploratorySession(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Check if user has access to the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("failed to get project for stopping exploratory session")
		return nil, system.NewHTTPError404("project not found")
	}

	if project.UserID != user.ID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Msg("user not authorized to stop exploratory session")
		return nil, system.NewHTTPError404("project not found")
	}

	// Get the active exploratory session
	session, err := s.Store.GetProjectExploratorySession(r.Context(), projectID)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get exploratory session to stop")
		return nil, system.NewHTTPError500("failed to get exploratory session")
	}

	if session == nil {
		return nil, system.NewHTTPError404("no exploratory session found")
	}

	// Stop the Zed agent (Wolf container)
	err = s.externalAgentExecutor.StopZedAgent(r.Context(), session.ID)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", session.ID).
			Str("project_id", projectID).
			Msg("Failed to stop exploratory session")
		return nil, system.NewHTTPError500("failed to stop exploratory session")
	}

	log.Info().
		Str("session_id", session.ID).
		Str("project_id", projectID).
		Msg("Exploratory session stopped successfully")

	return map[string]string{
		"message":    "exploratory session stopped",
		"session_id": session.ID,
	}, nil
}

// getProjectStartupScriptHistory godoc
// @Summary Get startup script version history
// @Description Get git commit history for project startup script
// @Tags Projects
// @Success 200 {array} services.StartupScriptVersion
// @Param id path string true "Project ID"
// @Router /api/v1/projects/{id}/startup-script/history [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProjectStartupScriptHistory(_ http.ResponseWriter, r *http.Request) ([]services.StartupScriptVersion, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	// Get project ID from URL
	vars := mux.Vars(r)
	projectID := vars["id"]
	if projectID == "" {
		return nil, system.NewHTTPError400("missing project ID")
	}

	// Get project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	// Check authorization
	if project.UserID != user.ID {
		return nil, system.NewHTTPError403("forbidden")
	}

	// Get startup script history from internal repo
	if project.InternalRepoPath == "" {
		return nil, system.NewHTTPError404("project has no internal repository")
	}

	versions, err := s.projectInternalRepoService.GetStartupScriptHistory(project.InternalRepoPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get startup script history")
		return nil, system.NewHTTPError500("failed to get startup script history")
	}

	return versions, nil
}
