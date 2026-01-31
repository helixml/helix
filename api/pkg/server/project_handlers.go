package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/agent/optimus"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listProjects godoc
// @Summary List projects
// @Description Get all projects for the current user
// @Tags Projects
// @Accept json
// @Produce json
// @Param organization_id query string false "Organization ID"
// @Success 200 {array} types.Project
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects [get]
func (s *HelixAPIServer) listProjects(_ http.ResponseWriter, r *http.Request) ([]*types.Project, *system.HTTPError) {
	user := getRequestUser(r)

	orgID := r.URL.Query().Get("organization_id")

	if orgID != "" {
		return s.listOrganizationProjects(r.Context(), user, orgID)
	}

	projects, err := s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
		UserID: user.ID,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Msg("failed to list projects")
		return nil, system.NewHTTPError500(err.Error())
	}

	return projects, nil
}

func (s *HelixAPIServer) listOrganizationProjects(ctx context.Context, user *types.User, orgRef string) ([]*types.Project, *system.HTTPError) {
	org, err := s.lookupOrg(ctx, orgRef)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to lookup org: %s", err))
	}

	_, err = s.authorizeOrgMember(ctx, user, org.ID)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	projects, err := s.Store.ListProjects(ctx, &store.ListProjectsQuery{
		OrganizationID: org.ID,
	})
	if err != nil {
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Load startup script from helix-specs branch in primary repo.
	// Startup script lives at .helix/startup.sh in the helix-specs branch.
	// No need to sync from upstream - helix-specs is only written by Helix,
	// so our middle repo always has the latest data.
	if project.DefaultRepoID != "" {
		primaryRepo, err := s.Store.GetGitRepository(r.Context(), project.DefaultRepoID)
		if err == nil && primaryRepo.LocalPath != "" {
			startupScript, loadErr := s.projectInternalRepoService.LoadStartupScriptFromHelixSpecs(primaryRepo.LocalPath)
			if loadErr != nil {
				log.Warn().
					Err(loadErr).
					Str("project_id", projectID).
					Str("primary_repo_id", project.DefaultRepoID).
					Msg("failed to load startup script from helix-specs branch")
			} else {
				project.StartupScript = startupScript
			}
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

	// Primary repository is REQUIRED - startup script lives in the code repo
	if req.DefaultRepoID == "" {
		return nil, system.NewHTTPError400("primary repository (default_repo_id) is required")
	}

	if req.DefaultHelixAppID == "" {
		return nil, system.NewHTTPError400("default helix app ID is required")
	}

	if req.OrganizationID != "" {
		// Check if user is a member of the organization
		_, err := s.authorizeOrgMember(r.Context(), user, req.OrganizationID)
		if err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

	defaultApp, err := s.Store.GetApp(r.Context(), req.DefaultHelixAppID)
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Deduplicate project name within the workspace (org or personal)
	// Build a set of existing names and add (1), (2), etc. if needed
	var existingProjects []*types.Project
	var listErr error
	if req.OrganizationID != "" {
		existingProjects, listErr = s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
			OrganizationID: req.OrganizationID,
		})
	} else {
		existingProjects, listErr = s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
			UserID: user.ID,
		})
	}
	if listErr != nil {
		log.Warn().Err(listErr).Msg("failed to list projects for name deduplication (continuing)")
	}

	existingNames := make(map[string]bool)
	for _, p := range existingProjects {
		existingNames[p.Name] = true
	}

	// Auto-increment name if it already exists: MyProject -> MyProject (1) -> MyProject (2)
	baseName := req.Name
	uniqueName := baseName
	suffix := 1
	for existingNames[uniqueName] {
		uniqueName = fmt.Sprintf("%s (%d)", baseName, suffix)
		suffix++
	}
	req.Name = uniqueName

	project := &types.Project{
		OrganizationID:    req.OrganizationID,
		ID:                system.GenerateProjectID(),
		Name:              req.Name,
		Description:       req.Description,
		UserID:            user.ID,
		GitHubRepoURL:     req.GitHubRepoURL,
		DefaultBranch:     req.DefaultBranch,
		Technologies:      req.Technologies,
		Status:            "active",
		DefaultRepoID:     req.DefaultRepoID,
		StartupScript:     req.StartupScript,
		DefaultHelixAppID: req.DefaultHelixAppID,
		Guidelines:        req.Guidelines,
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

	// Attach the primary repository to the project
	// This sets the project_id on the repository so it shows up in the project's repo list
	if err := s.Store.AttachRepositoryToProject(r.Context(), created.ID, req.DefaultRepoID); err != nil {
		log.Warn().
			Err(err).
			Str("project_id", created.ID).
			Str("repo_id", req.DefaultRepoID).
			Msg("failed to attach primary repository to project (continuing)")
	}

	// Initialize startup script in the primary code repo
	// Startup script lives at .helix/startup.sh in the primary repository
	primaryRepo, err := s.Store.GetGitRepository(r.Context(), req.DefaultRepoID)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", created.ID).
			Str("primary_repo_id", req.DefaultRepoID).
			Msg("failed to get primary repository")
		// Don't fail project creation - startup script can be added later
	} else if primaryRepo.LocalPath != "" {
		// Use WithExternalRepoWrite with lenient options - don't fail project creation
		// if startup script sync/push fails. The utility still handles rollback on push failure.
		writeErr := s.gitRepositoryService.WithExternalRepoWrite(
			r.Context(),
			primaryRepo,
			services.ExternalRepoWriteOptions{
				Branch:          "helix-specs",
				FailOnSyncError: false, // Don't fail project creation on sync error
				FailOnPushError: false, // Don't fail project creation on push error (but still rollback)
			},
			func() error {
				return s.projectInternalRepoService.InitializeStartupScriptInCodeRepo(
					primaryRepo.LocalPath,
					created.Name,
					req.StartupScript,
					user.FullName,
					user.Email,
				)
			},
		)
		if writeErr != nil {
			log.Warn().
				Err(writeErr).
				Str("project_id", created.ID).
				Str("primary_repo_id", req.DefaultRepoID).
				Msg("failed to initialize startup script in primary code repo (continuing)")
		} else {
			log.Info().
				Str("project_id", created.ID).
				Str("primary_repo_id", req.DefaultRepoID).
				Msg("Startup script initialized in primary code repo")
		}
	}

	// Create project manager agent (Optimus)
	optimusApp := optimus.NewOptimusAgentApp(optimus.OptimusConfig{
		ProjectID:      created.ID,
		ProjectName:    created.Name,
		OrganizationID: req.OrganizationID,
		OwnerID:        user.ID,
		OwnerType:      user.Type,
		DefaultApp:     defaultApp,
	})

	createdOptimus, optimusErr := s.Store.CreateApp(r.Context(), optimusApp)
	if optimusErr != nil {
		log.Warn().
			Err(optimusErr).
			Str("project_id", created.ID).
			Msg("failed to create optimus agent app (continuing)")
	} else {
		created.ProjectManagerHelixAppID = createdOptimus.ID
		if updateErr := s.Store.UpdateProject(r.Context(), created); updateErr != nil {
			log.Warn().
				Err(updateErr).
				Str("project_id", created.ID).
				Str("optimus_app_id", createdOptimus.ID).
				Msg("failed to update project with optimus agent app ID (continuing)")
		} else {
			log.Info().
				Str("project_id", created.ID).
				Str("optimus_app_id", createdOptimus.ID).
				Msg("optimus agent app created and linked to project")
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", created.ID).
		Str("project_name", created.Name).
		Msg("project created successfully")

	// Log audit event for project creation
	if s.auditLogService != nil {
		s.auditLogService.LogProjectCreated(r.Context(), created, user.ID, user.Email)
	}

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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
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
	if req.DefaultHelixAppID != nil {
		project.DefaultHelixAppID = *req.DefaultHelixAppID
	}
	if req.ProjectManagerHelixAppID != nil {
		project.ProjectManagerHelixAppID = *req.ProjectManagerHelixAppID
	}
	if req.PullRequestReviewerHelixAppID != nil {
		project.PullRequestReviewerHelixAppID = *req.PullRequestReviewerHelixAppID
	}
	if req.PullRequestReviewsEnabled != nil {
		project.PullRequestReviewsEnabled = *req.PullRequestReviewsEnabled
	}
	// Track guidelines changes with versioning
	if req.Guidelines != nil && *req.Guidelines != project.Guidelines {
		// Save current version to history before updating
		if project.Guidelines != "" {
			history := &types.GuidelinesHistory{
				ID:         system.GenerateUUID(),
				ProjectID:  projectID,
				Version:    project.GuidelinesVersion,
				Guidelines: project.Guidelines,
				UpdatedBy:  project.GuidelinesUpdatedBy,
				UpdatedAt:  project.GuidelinesUpdatedAt,
			}
			if err := s.Store.CreateGuidelinesHistory(r.Context(), history); err != nil {
				log.Warn().Err(err).Str("project_id", projectID).Msg("failed to save guidelines history")
			}
		}

		// Update guidelines with new version
		project.Guidelines = *req.Guidelines
		project.GuidelinesVersion++
		project.GuidelinesUpdatedAt = time.Now()
		project.GuidelinesUpdatedBy = user.ID
	}
	if req.Metadata != nil {
		project.Metadata = *req.Metadata
	}
	// Skills can be set directly (nil means "don't update")
	if req.Skills != nil {
		project.Skills = req.Skills
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

	// Save startup script to helix-specs branch in the primary repo
	// Note: We save to helix-specs (not main) to avoid conflicts with protected branches on external repos
	if req.StartupScript != nil && project.DefaultRepoID != "" {
		primaryRepo, err := s.Store.GetGitRepository(r.Context(), project.DefaultRepoID)
		if err == nil && primaryRepo.LocalPath != "" {
			var changed bool
			writeErr := s.gitRepositoryService.WithExternalRepoWrite(
				r.Context(),
				primaryRepo,
				services.ExternalRepoWriteOptions{
					Branch:          "helix-specs",
					FailOnSyncError: true,
					FailOnPushError: true,
				},
				func() error {
					var saveErr error
					changed, saveErr = s.projectInternalRepoService.SaveStartupScriptToHelixSpecs(
						primaryRepo.LocalPath,
						*req.StartupScript,
						user.FullName,
						user.Email,
					)
					if saveErr != nil {
						return saveErr
					}
					if !changed {
						log.Debug().
							Str("project_id", projectID).
							Str("primary_repo_id", project.DefaultRepoID).
							Msg("Startup script unchanged")
					}
					return nil
				},
			)
			if writeErr != nil {
				log.Error().
					Err(writeErr).
					Str("project_id", projectID).
					Str("primary_repo_id", project.DefaultRepoID).
					Msg("Failed to save startup script")
				return nil, system.NewHTTPError500(fmt.Sprintf("Failed to save startup script: %s", writeErr.Error()))
			}
			if changed {
				log.Info().
					Str("project_id", projectID).
					Str("primary_repo_id", project.DefaultRepoID).
					Msg("Startup script saved and pushed to upstream")
			}
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Msg("project updated successfully")

	// Log audit events for project updates
	if s.auditLogService != nil {
		// Track which fields were changed for audit log
		var changedFields []string
		if req.Name != nil {
			changedFields = append(changedFields, "name")
		}
		if req.Description != nil {
			changedFields = append(changedFields, "description")
		}
		if req.GitHubRepoURL != nil {
			changedFields = append(changedFields, "github_repo_url")
		}
		if req.DefaultBranch != nil {
			changedFields = append(changedFields, "default_branch")
		}
		if req.Technologies != nil {
			changedFields = append(changedFields, "technologies")
		}
		if req.Status != nil {
			changedFields = append(changedFields, "status")
		}
		if req.DefaultRepoID != nil {
			changedFields = append(changedFields, "default_repo_id")
		}
		if req.AutoStartBacklogTasks != nil {
			changedFields = append(changedFields, "auto_start_backlog_tasks")
		}
		if req.DefaultHelixAppID != nil {
			changedFields = append(changedFields, "default_helix_app_id")
		}
		if req.ProjectManagerHelixAppID != nil {
			changedFields = append(changedFields, "project_manager_helix_app_id")
		}
		if req.PullRequestReviewerHelixAppID != nil {
			changedFields = append(changedFields, "pull_request_reviewer_helix_app_id")
		}
		if req.Metadata != nil {
			changedFields = append(changedFields, "metadata")
		}

		// Log guidelines update separately (it's versioned and more significant)
		if req.Guidelines != nil {
			s.auditLogService.LogProjectGuidelinesUpdated(r.Context(), project, user.ID, user.Email)
		}

		// Log general settings update if any non-guidelines fields changed
		if len(changedFields) > 0 {
			s.auditLogService.LogProjectSettingsUpdated(r.Context(), project, changedFields, user.ID, user.Email)
		}
	}

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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionDelete)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
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

		stopErr := s.externalAgentExecutor.StopDesktop(r.Context(), exploratorySession.ID)
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
			if task.PlanningSessionID != "" {
				log.Info().
					Str("project_id", projectID).
					Str("task_id", task.ID).
					Str("session_id", task.PlanningSessionID).
					Msg("Stopping SpecTask session before project deletion")

				stopErr := s.externalAgentExecutor.StopDesktop(r.Context(), task.PlanningSessionID)
				if stopErr != nil {
					log.Warn().Err(stopErr).Str("session_id", task.PlanningSessionID).Msg("Failed to stop session (continuing with deletion)")
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

	// Log audit event for project deletion
	if s.auditLogService != nil {
		s.auditLogService.LogProjectDeleted(r.Context(), project, user.ID, user.Email)
	}

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
// @Success 200 {array} types.GitRepository
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	repos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Verify the repository exists and belongs to this project
	projectRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
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

	// Check if user has access to the repository
	if err := s.authorizeUserToRepository(r.Context(), user, repo, types.ActionGet); err != nil {
		log.Warn().
			Err(err).
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Verify the repository is attached to this project
	projectRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
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

	err = s.Store.DetachRepositoryFromProject(r.Context(), projectID, repoID)
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

// checkSandboxContainerExists checks if a sandbox container exists
func (s *HelixAPIServer) checkSandboxContainerExists(ctx context.Context, sessionID string, sandboxID string) (bool, error) {
	// Check if the container exists via the executor
	if s.externalAgentExecutor == nil {
		return false, nil
	}

	// Use HasRunningContainer to check if the sandbox is running
	return s.externalAgentExecutor.HasRunningContainer(ctx, sessionID), nil
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
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

	// Check if the external agent (sandbox container) is actually running
	// If not running, update status to "stopped"
	if s.externalAgentExecutor != nil {
		_, err := s.externalAgentExecutor.GetSession(session.ID)
		if err != nil {
			// External agent not running - mark as stopped
			session.Metadata.ExternalAgentStatus = "stopped"
			log.Trace().
				Str("session_id", session.ID).
				Str("project_id", projectID).
				Msg("Exploratory session exists in database but sandbox container is stopped")
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Check if an active exploratory session already exists for this project
	existingSession, err := s.Store.GetProjectExploratorySession(r.Context(), projectID)
	if err == nil && existingSession != nil {
		// Session exists - check if sandbox container is still running
		// If container stopped or never started, restart it with fresh startup script
		containerID := existingSession.Metadata.DevContainerID
		sandboxID := existingSession.SandboxID

		// Determine if container needs restart:
		// - No container ID means container was never started or was cleared
		// - If we have container ID, check if it still exists
		needsRestart := containerID == ""
		if containerID != "" && sandboxID != "" {
			containerExists, checkErr := s.checkSandboxContainerExists(r.Context(), existingSession.ID, sandboxID)
			if checkErr != nil {
				log.Warn().Err(checkErr).Str("container_id", containerID).Msg("Failed to check container status, assuming restart needed")
				needsRestart = true
			} else if !containerExists {
				needsRestart = true
			}
		} else if containerID == "" && sandboxID != "" {
			// Container ID is empty but sandbox exists - need restart
			needsRestart = true
		}

		if needsRestart {
			// Container stopped or never started - restart it
			log.Info().
				Str("session_id", existingSession.ID).
				Str("project_id", projectID).
				Str("container_id", containerID).
				Msg("Exploratory session exists but container stopped - restarting with fresh startup script")

			// Get project repositories for restarting
			projectRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
				ProjectID: projectID,
			})
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

			// Get display settings from project's default agent app
			displayWidth := 1920
			displayHeight := 1080
			displayRefreshRate := 60
			resolution := ""
			zoomLevel := 0
			desktopType := ""
			if project.DefaultHelixAppID != "" {
				app, appErr := s.Store.GetApp(r.Context(), project.DefaultHelixAppID)
				if appErr == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
					w, h := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
					displayWidth = w
					displayHeight = h
					if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
						displayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
					}
					// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
					resolution = app.Config.Helix.ExternalAgentConfig.Resolution
					zoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
					desktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
					log.Debug().
						Str("project_id", projectID).
						Str("app_id", project.DefaultHelixAppID).
						Int("display_width", displayWidth).
						Int("display_height", displayHeight).
						Int("display_refresh_rate", displayRefreshRate).
						Str("resolution", resolution).
						Int("zoom_level", zoomLevel).
						Str("desktop_type", desktopType).
						Msg("Using display settings from project's default agent for exploratory restart")
				}
			}

			// Ensure desktopType has a sensible default (ubuntu) when not set by app config
			// This is critical for video_source_mode: ubuntu uses "pipewire", sway uses "wayland"
			if desktopType == "" {
				desktopType = "ubuntu"
			}

			// Check admin access for privileged mode (UseHostDocker)
			if project.UseHostDocker && !user.Admin {
				return nil, system.NewHTTPError403("UseHostDocker requires admin access")
			}

			// Restart Zed agent with existing session
			zedAgent := &types.DesktopAgent{
				SessionID:           existingSession.ID,
				UserID:              user.ID,
				Input:               fmt.Sprintf("Explore the %s project", project.Name),
				ProjectPath:         "workspace",
				SpecTaskID:          "",
				ProjectID:           projectID,
				PrimaryRepositoryID: primaryRepoID,
				RepositoryIDs:       repositoryIDs,
				DisplayWidth:        displayWidth,
				DisplayHeight:       displayHeight,
				DisplayRefreshRate:  displayRefreshRate,
				Resolution:          resolution,
				ZoomLevel:           zoomLevel,
				DesktopType:         desktopType,
				UseHostDocker:       project.UseHostDocker,
			}

			// Add user's API token for git operations
			if err := s.addUserAPITokenToAgent(r.Context(), zedAgent, user.ID); err != nil {
				log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to add user API token for restart")
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to get user API keys: %v", err))
			}

			agentResp, err := s.externalAgentExecutor.StartDesktop(r.Context(), zedAgent)
			if err != nil {
				log.Error().
					Err(err).
					Str("session_id", existingSession.ID).
					Msg("Failed to restart exploratory session")
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to restart exploratory session: %v", err))
			}

			log.Info().
				Str("session_id", existingSession.ID).
				Str("lobby_id", agentResp.DevContainerID).
				Msg("Exploratory session lobby restarted successfully")

			// Reload session from database to get updated lobby ID/PIN
			// StartDesktop updates session metadata in DB, so we need fresh data
			updatedSession, err := s.Store.GetSession(r.Context(), existingSession.ID)
			if err != nil {
				log.Error().Err(err).Str("session_id", existingSession.ID).Msg("Failed to reload session after restart")
				return existingSession, nil // Return stale session rather than failing
			}

			return updatedSession, nil
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
		OrganizationID: project.OrganizationID, // Inherit org from project
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
	projectRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		ProjectID: projectID,
	})
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

	// Get display settings from project's default agent app
	displayWidth := 1920
	displayHeight := 1080
	displayRefreshRate := 60
	resolution := ""
	zoomLevel := 0
	desktopType := ""
	if project.DefaultHelixAppID != "" {
		app, appErr := s.Store.GetApp(r.Context(), project.DefaultHelixAppID)
		if appErr == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			w, h := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			displayWidth = w
			displayHeight = h
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				displayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			// CRITICAL: Also get resolution preset, zoom level, and desktop type for proper HiDPI scaling
			resolution = app.Config.Helix.ExternalAgentConfig.Resolution
			zoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			desktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
			log.Debug().
				Str("project_id", projectID).
				Str("app_id", project.DefaultHelixAppID).
				Int("display_width", displayWidth).
				Int("display_height", displayHeight).
				Int("display_refresh_rate", displayRefreshRate).
				Str("resolution", resolution).
				Int("zoom_level", zoomLevel).
				Str("desktop_type", desktopType).
				Msg("Using display settings from project's default agent for new exploratory session")
		}
	}

	// Ensure desktopType has a sensible default (ubuntu) when not set by app config
	// This is critical for video_source_mode: ubuntu uses "pipewire", sway uses "wayland"
	if desktopType == "" {
		desktopType = "ubuntu"
	}

	// Check admin access for privileged mode (UseHostDocker)
	if project.UseHostDocker && !user.Admin {
		return nil, system.NewHTTPError403("UseHostDocker requires admin access")
	}

	// Create ZedAgent for exploratory session
	zedAgent := &types.DesktopAgent{
		SessionID:           createdSession.ID,
		UserID:              user.ID,
		Input:               fmt.Sprintf("Explore the %s project", project.Name),
		ProjectPath:         "workspace",
		SpecTaskID:          "",        // No task - exploratory mode
		ProjectID:           projectID, // For loading project repos and startup script
		PrimaryRepositoryID: primaryRepoID,
		RepositoryIDs:       repositoryIDs,
		DisplayWidth:        displayWidth,
		DisplayHeight:       displayHeight,
		DisplayRefreshRate:  displayRefreshRate,
		Resolution:          resolution,
		ZoomLevel:           zoomLevel,
		DesktopType:         desktopType,
		UseHostDocker:       project.UseHostDocker,
	}

	// Add user's API token for git operations (RBAC enforced)
	if err := s.addUserAPITokenToAgent(r.Context(), zedAgent, user.ID); err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to add user API token")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get user API keys: %v", err))
	}

	// Start the desktop agent
	agentResp, err := s.externalAgentExecutor.StartDesktop(r.Context(), zedAgent)
	if err != nil {
		log.Error().
			Err(err).
			Str("session_id", createdSession.ID).
			Str("project_id", projectID).
			Msg("Failed to launch exploratory agent")
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to start exploratory agent: %v", err))
	}

	// Activity tracking now happens in StartDesktop for all desktop agent types

	log.Info().
		Str("session_id", createdSession.ID).
		Str("project_id", projectID).
		Str("dev_container_id", agentResp.DevContainerID).
		Msg("Exploratory session created successfully")

	return createdSession, nil
}

// stopExploratorySession godoc
// @Summary Stop project exploratory session
// @Description Stop the running exploratory session for a project (stops sandbox container, keeps session record)
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
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

	// Stop the desktop agent container
	err = s.externalAgentExecutor.StopDesktop(r.Context(), session.ID)
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

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Get startup script history from primary code repo
	if project.DefaultRepoID == "" {
		return nil, system.NewHTTPError404("project has no primary repository")
	}

	primaryRepo, err := s.Store.GetGitRepository(r.Context(), project.DefaultRepoID)
	if err != nil {
		return nil, system.NewHTTPError404("primary repository not found")
	}

	if primaryRepo.LocalPath == "" {
		return nil, system.NewHTTPError400("primary repository is external - history not available")
	}

	versions, err := s.projectInternalRepoService.GetStartupScriptHistoryFromHelixSpecs(primaryRepo.LocalPath)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Str("primary_repo_id", project.DefaultRepoID).
			Msg("failed to get startup script history from helix-specs branch")
		return nil, system.NewHTTPError500("failed to get startup script history")
	}

	return versions, nil
}

// getProjectGuidelinesHistory returns the history of guidelines changes for a project
// @Summary Get project guidelines history
// @Description Get the version history of guidelines for a project
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {array} types.GuidelinesHistory
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/projects/{id}/guidelines-history [get]
// @Security BearerAuth
func (s *HelixAPIServer) getProjectGuidelinesHistory(_ http.ResponseWriter, r *http.Request) ([]*types.GuidelinesHistory, *system.HTTPError) {
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	vars := mux.Vars(r)
	projectID := vars["id"]
	if projectID == "" {
		return nil, system.NewHTTPError400("missing project ID")
	}

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	history, err := s.Store.ListGuidelinesHistory(r.Context(), "", projectID, "")
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", projectID).
			Msg("failed to get project guidelines history")
		return nil, system.NewHTTPError500("failed to get guidelines history")
	}

	// Populate user display names and emails
	for _, entry := range history {
		if entry.UpdatedBy != "" {
			if u, err := s.Store.GetUser(r.Context(), &store.GetUserQuery{ID: entry.UpdatedBy}); err == nil && u != nil {
				entry.UpdatedByName = u.FullName
				entry.UpdatedByEmail = u.Email
			}
		}
	}

	return history, nil
}

// moveProjectPreview godoc
// @Summary Preview moving a project to an organization
// @Description Check for naming conflicts before moving a project to an organization
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body types.MoveProjectRequest true "Move project request"
// @Success 200 {object} types.MoveProjectPreviewResponse
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/move/preview [post]
func (s *HelixAPIServer) moveProjectPreview(_ http.ResponseWriter, r *http.Request) (*types.MoveProjectPreviewResponse, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	var req types.MoveProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	if req.OrganizationID == "" {
		return nil, system.NewHTTPError400("organization_id is required")
	}

	// Get the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	// Check user owns the project
	if project.UserID != user.ID && !user.Admin {
		return nil, system.NewHTTPError403("you must be the project owner to move it")
	}

	// Check project is not already in an organization
	if project.OrganizationID != "" {
		return nil, system.NewHTTPError400("project is already in an organization")
	}

	// Check user is a member of the target organization
	_, err = s.authorizeOrgMember(r.Context(), user, req.OrganizationID)
	if err != nil {
		return nil, system.NewHTTPError403("you must be a member of the target organization")
	}

	// Check for project name conflicts in target org
	existingProjects, err := s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
		OrganizationID: req.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list org projects: %v", err))
	}

	existingProjectNames := make(map[string]bool)
	for _, p := range existingProjects {
		existingProjectNames[p.Name] = true
	}

	projectPreview := types.MoveProjectPreviewItem{
		CurrentName: project.Name,
		HasConflict: existingProjectNames[project.Name],
	}
	if projectPreview.HasConflict {
		newName := getUniqueProjectName(project.Name, existingProjectNames)
		projectPreview.NewName = &newName
	}

	// Get project repositories
	repoIDs, err := s.Store.GetRepositoriesForProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get project repositories: %v", err))
	}

	// Get existing repo names in target org
	existingRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		OrganizationID: req.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list org repositories: %v", err))
	}

	existingRepoNames := make(map[string]bool)
	for _, repo := range existingRepos {
		existingRepoNames[repo.Name] = true
	}

	// Check each repository for conflicts and shared usage
	var repoPreview []types.MoveRepositoryPreviewItem
	for _, repoID := range repoIDs {
		repo, err := s.Store.GetGitRepository(r.Context(), repoID)
		if err != nil {
			continue // Skip repos that can't be found
		}

		item := types.MoveRepositoryPreviewItem{
			ID:          repo.ID,
			CurrentName: repo.Name,
			HasConflict: existingRepoNames[repo.Name],
		}
		if item.HasConflict {
			newName := services.GetUniqueRepoName(repo.Name, existingRepoNames)
			item.NewName = &newName
		}

		// Check if this repo is shared with other personal workspace projects that will lose access
		// Only personal workspace projects (no org) can share repos with this project
		allProjectIDs, err := s.Store.GetProjectsForRepository(r.Context(), repoID)
		if err == nil && len(allProjectIDs) > 1 {
			for _, otherProjectID := range allProjectIDs {
				if otherProjectID == projectID {
					continue // Skip the project being moved
				}
				otherProject, err := s.Store.GetProject(r.Context(), otherProjectID)
				if err == nil {
					// Only include personal workspace projects (no org set)
					if otherProject.OrganizationID != "" {
						continue
					}
					item.AffectedProjects = append(item.AffectedProjects, types.AffectedProjectInfo{
						ID:   otherProject.ID,
						Name: otherProject.Name,
					})
				}
			}
		}

		repoPreview = append(repoPreview, item)
	}

	return &types.MoveProjectPreviewResponse{
		Project:      projectPreview,
		Repositories: repoPreview,
	}, nil
}

// getUniqueProjectName returns a unique project name by appending (1), (2), etc. if needed.
func getUniqueProjectName(baseName string, existingNames map[string]bool) string {
	name := baseName
	suffix := 1
	for existingNames[name] {
		name = fmt.Sprintf("%s (%d)", baseName, suffix)
		suffix++
	}
	return name
}

// moveProject godoc
// @Summary Move a project to an organization
// @Description Move a project from personal workspace to an organization
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body types.MoveProjectRequest true "Move project request"
// @Success 200 {object} types.Project
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/move [post]
func (s *HelixAPIServer) moveProject(_ http.ResponseWriter, r *http.Request) (*types.Project, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	var req types.MoveProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	if req.OrganizationID == "" {
		return nil, system.NewHTTPError400("organization_id is required")
	}

	// Get the project
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	// Check user owns the project
	if project.UserID != user.ID && !user.Admin {
		return nil, system.NewHTTPError403("you must be the project owner to move it")
	}

	// Check project is not already in an organization
	if project.OrganizationID != "" {
		return nil, system.NewHTTPError400("project is already in an organization")
	}

	// Check user is a member of the target organization
	_, err = s.authorizeOrgMember(r.Context(), user, req.OrganizationID)
	if err != nil {
		return nil, system.NewHTTPError403("you must be a member of the target organization")
	}

	// Check for project name conflicts in target org
	existingProjects, err := s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{
		OrganizationID: req.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list org projects: %v", err))
	}

	existingProjectNames := make(map[string]bool)
	for _, p := range existingProjects {
		existingProjectNames[p.Name] = true
	}

	// Rename project if there's a conflict
	if existingProjectNames[project.Name] {
		project.Name = getUniqueProjectName(project.Name, existingProjectNames)
	}

	// Get project repositories
	repoIDs, err := s.Store.GetRepositoriesForProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get project repositories: %v", err))
	}

	// Get existing repo names in target org
	existingRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		OrganizationID: req.OrganizationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list org repositories: %v", err))
	}

	existingRepoNames := make(map[string]bool)
	for _, repo := range existingRepos {
		existingRepoNames[repo.Name] = true
	}

	// Update project organization ID
	project.OrganizationID = req.OrganizationID
	project.UpdatedAt = time.Now()

	if err := s.Store.UpdateProject(r.Context(), project); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update project: %v", err))
	}

	// Update each repository's organization ID
	for _, repoID := range repoIDs {
		repo, err := s.Store.GetGitRepository(r.Context(), repoID)
		if err != nil {
			log.Warn().Err(err).Str("repo_id", repoID).Msg("failed to get repository for move")
			continue
		}

		// Rename repo if there's a conflict
		if existingRepoNames[repo.Name] {
			repo.Name = services.GetUniqueRepoName(repo.Name, existingRepoNames)
		}

		repo.OrganizationID = req.OrganizationID
		repo.UpdatedAt = time.Now()

		if err := s.Store.UpdateGitRepository(r.Context(), repo); err != nil {
			log.Warn().Err(err).Str("repo_id", repoID).Msg("failed to update repository organization")
		}
	}

	// Update project_repositories junction table
	projectRepos, err := s.Store.ListProjectRepositories(r.Context(), &types.ListProjectRepositoriesQuery{
		ProjectID: projectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("failed to list project repositories for move")
	} else {
		for _, pr := range projectRepos {
			pr.OrganizationID = req.OrganizationID
			pr.UpdatedAt = time.Now()
			if err := s.Store.UpdateProjectRepository(r.Context(), pr); err != nil {
				log.Warn().Err(err).
					Str("project_id", pr.ProjectID).
					Str("repo_id", pr.RepositoryID).
					Msg("failed to update project repository organization")
			}
		}
	}

	// Update sessions to belong to the new organization
	sessions, _, err := s.Store.ListSessions(r.Context(), store.ListSessionsQuery{
		ProjectID: projectID,
	})
	if err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("failed to list sessions for move")
	} else {
		for _, session := range sessions {
			session.OrganizationID = req.OrganizationID
			session.Updated = time.Now()
			if _, err := s.Store.UpdateSession(r.Context(), *session); err != nil {
				log.Warn().Err(err).
					Str("session_id", session.ID).
					Str("project_id", projectID).
					Msg("failed to update session organization")
			}
		}
		log.Info().
			Int("session_count", len(sessions)).
			Str("project_id", projectID).
			Msg("updated sessions for project move")
	}

	// Log audit event
	log.Info().
		Str("user_id", user.ID).
		Str("project_id", projectID).
		Str("organization_id", req.OrganizationID).
		Str("project_name", project.Name).
		Msg("project moved to organization")

	return project, nil
}
