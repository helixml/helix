package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/agent/optimus"
	"github.com/helixml/helix/api/pkg/hydra"
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
		UserID:       user.ID,
		IncludeStats: true,
	})
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Msg("failed to list projects")
		return nil, system.NewHTTPError500(err.Error())
	}

	s.populateActiveAgentSessions(projects)

	return projects, nil
}

// populateActiveAgentSessions gets active agent sessions from memory for each project and adds it to the project stats
func (s *HelixAPIServer) populateActiveAgentSessions(projects []*types.Project) {
	desktopSessions := s.externalAgentExecutor.ListSessions()
	for _, project := range projects {
		for _, session := range desktopSessions {
			if session.ProjectID == project.ID {
				project.Stats.ActiveAgentSessions++
			}
		}
	}
}

func (s *HelixAPIServer) listOrganizationProjects(ctx context.Context, user *types.User, orgRef string) ([]*types.Project, *system.HTTPError) {
	org, err := s.lookupOrg(ctx, orgRef)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to lookup org: %s", err))
	}

	orgMembership, err := s.authorizeOrgMember(ctx, user, org.ID)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	projects, err := s.Store.ListProjects(ctx, &store.ListProjectsQuery{
		OrganizationID: org.ID,
		IncludeStats:   true,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	// Org owners see all projects
	if orgMembership.Role == types.OrganizationRoleOwner {
		return projects, nil
	}

	// Non-owners only see projects they have access to
	var authorizedProjects []*types.Project
	for _, project := range projects {
		if err := s.authorizeUserToProject(ctx, user, project, types.ActionGet); err != nil {
			continue
		}
		authorizedProjects = append(authorizedProjects, project)
	}

	return authorizedProjects, nil
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

	// Prune stale sandbox entries from DockerCacheStatus (lazy cleanup on read).
	// Remove entries for sandboxes that no longer exist in the database.
	if project.Metadata.DockerCacheStatus != nil && len(project.Metadata.DockerCacheStatus.Sandboxes) > 0 {
		sandboxes, sbErr := s.Store.ListSandboxes(r.Context())
		if sbErr == nil {
			knownIDs := make(map[string]bool, len(sandboxes))
			for _, sb := range sandboxes {
				knownIDs[sb.ID] = true
			}
			pruned := false
			for sbID := range project.Metadata.DockerCacheStatus.Sandboxes {
				if !knownIDs[sbID] {
					delete(project.Metadata.DockerCacheStatus.Sandboxes, sbID)
					pruned = true
				}
			}
			if pruned {
				if updateErr := s.Store.UpdateProject(r.Context(), project); updateErr != nil {
					log.Warn().Err(updateErr).Str("project_id", projectID).Msg("Failed to prune stale sandbox cache entries")
				}
			}
		}
	}

	// Recover stale "building" golden build states (lazy recovery on read).
	// After an API restart, the monitoring goroutine is dead but DB still says
	// "building". If the build isn't tracked in memory AND the container isn't
	// running, reset the status so the UI doesn't get stuck.
	//
	// Skip during the first 90s after startup — RecoverStaleBuilds needs up
	// to 60s to wait for sandbox reconnect and re-attach monitoring goroutines.
	// Without this grace period, a page load during startup races with recovery
	// and resets status to "none" before the sandbox can reconnect.
	if project.Metadata.DockerCacheStatus != nil && time.Since(s.startTime) > 90*time.Second {
		staleRecovered := false
		for sbID, sbState := range project.Metadata.DockerCacheStatus.Sandboxes {
			if sbState.Status != "building" || sbState.BuildSessionID == "" {
				continue
			}
			if s.goldenBuildService.IsTracking(project.ID, sbID) {
				continue
			}
			if s.externalAgentExecutor.HasRunningContainer(r.Context(), sbState.BuildSessionID) {
				continue
			}
			log.Info().
				Str("project_id", projectID).
				Str("sandbox_id", sbID).
				Str("session_id", sbState.BuildSessionID).
				Msg("Recovering stale golden build: monitoring goroutine dead and container not running")
			sbState.Status = "none"
			sbState.BuildSessionID = ""
			sbState.Error = ""
			staleRecovered = true
		}
		if staleRecovered {
			if updateErr := s.Store.UpdateProject(r.Context(), project); updateErr != nil {
				log.Warn().Err(updateErr).Str("project_id", projectID).Msg("Failed to reset stale golden build status")
			}
		}
	}

	// Load startup script from helix-specs branch in primary repo.
	// Sync from upstream first — helix-specs can be modified outside Helix
	// (e.g., direct git pushes), so we need the latest version.
	if project.DefaultRepoID != "" {
		primaryRepo, err := s.Store.GetGitRepository(r.Context(), project.DefaultRepoID)
		if err == nil && primaryRepo.LocalPath != "" {
			syncErr := s.gitRepositoryService.WithExternalRepoRead(r.Context(), primaryRepo, func() error {
				startupScript, loadErr := s.projectInternalRepoService.LoadStartupScriptFromHelixSpecs(primaryRepo.LocalPath)
				if loadErr != nil {
					return loadErr
				}
				project.StartupScript = startupScript
				return nil
			})
			if syncErr != nil {
				log.Warn().
					Err(syncErr).
					Str("project_id", projectID).
					Str("primary_repo_id", project.DefaultRepoID).
					Msg("failed to load startup script from helix-specs branch")
			}
		}
	}

	// If no startup script was found in the git repo, synthesize one from the
	// declarative startup fields set via `helix apply -f project.yaml`.
	// When the user edits the script in the UI, it gets saved to the git repo
	// and will take precedence on subsequent loads.
	if project.StartupScript == "" && (project.StartupInstall != "" || project.StartupStart != "") {
		project.StartupScript = synthesizeStartupScript(project.StartupInstall, project.StartupStart)
		project.StartupScriptFromYAML = true // Synthesized from YAML fields
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

	systemSettings, err := s.Store.GetSystemSettings(r.Context())
	if err != nil {
		log.Warn().
			Err(err).
			Msg("failed to get system settings (continuing)")
		return nil, system.NewHTTPError500(err.Error())
	}

	// Create project manager agent (Optimus)
	optimusApp := optimus.NewOptimusAgentApp(optimus.OptimusConfig{
		ProjectID:      created.ID,
		ProjectName:    created.Name,
		OrganizationID: req.OrganizationID,
		OwnerID:        user.ID,
		OwnerType:      user.Type,
		DefaultApp:     defaultApp,
		SystemSettings: systemSettings,
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
	if req.KoditEnabled != nil {
		project.KoditEnabled = *req.KoditEnabled
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
		// Merge metadata fields selectively to avoid overwriting fields
		// managed by backend services (e.g., DockerCacheStatus).
		if req.Metadata.BoardSettings != nil {
			project.Metadata.BoardSettings = req.Metadata.BoardSettings
		}
		// AutoWarmDockerCache is a bool — always apply from the request
		// since it's user-controlled.
		project.Metadata.AutoWarmDockerCache = req.Metadata.AutoWarmDockerCache
		// DockerCacheStatus is managed exclusively by GoldenBuildService — never overwrite from API request.
	}
	// Skills can be set directly (nil means "don't update")
	if req.Skills != nil {
		project.Skills = req.Skills
	}

	// DON'T update StartupScript in database - Git repo is source of truth
	// It will be saved to git repo below and loaded from there on next fetch

	// If user is editing the startup script via UI, clear the YAML-controlled flag
	// since they're now manually managing the script
	if req.StartupScript != nil {
		project.StartupScriptFromYAML = false
	}

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

	// Validate org scoping: project and repo must be in the same org scope
	if project.OrganizationID != repo.OrganizationID {
		log.Warn().
			Str("user_id", user.ID).
			Str("project_id", projectID).
			Str("project_org", project.OrganizationID).
			Str("repo_id", repoID).
			Str("repo_org", repo.OrganizationID).
			Msg("cannot attach repository from different org scope")
		return nil, system.NewHTTPError400("repository must be in the same organization as the project")
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

	// Update default_repo_id if it is empty or stale (points to a repo no longer attached).
	attachedRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{ProjectID: projectID})
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("failed to list repositories after attach")
		return nil, system.NewHTTPError500(err.Error())
	}
	defaultIsValid := false
	for _, ar := range attachedRepos {
		if ar.ID == project.DefaultRepoID {
			defaultIsValid = true
			break
		}
	}
	if !defaultIsValid {
		if err := s.Store.SetProjectPrimaryRepository(r.Context(), projectID, repoID); err != nil {
			log.Error().Err(err).Str("project_id", projectID).Str("repo_id", repoID).Msg("failed to set default repo after attach")
			return nil, system.NewHTTPError500(err.Error())
		}
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

	// If the detached repo was the default, update default_repo_id to another attached repo or clear it.
	if project.DefaultRepoID == repoID {
		remainingRepos, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{ProjectID: projectID})
		if err != nil {
			log.Error().Err(err).Str("project_id", projectID).Msg("failed to list repositories after detach")
			return nil, system.NewHTTPError500(err.Error())
		}
		newDefault := ""
		if len(remainingRepos) > 0 {
			newDefault = remainingRepos[0].ID
		}
		if err := s.Store.SetProjectPrimaryRepository(r.Context(), projectID, newDefault); err != nil {
			log.Error().Err(err).Str("project_id", projectID).Msg("failed to update default repo after detach")
			return nil, system.NewHTTPError500(err.Error())
		}
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
			resolution := "1080p"
			zoomLevel := 200
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

			// Restart Zed agent with existing session
			zedAgent := &types.DesktopAgent{
				OrganizationID:      project.OrganizationID,
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

	// Create ZedAgent for team desktop
	zedAgent := &types.DesktopAgent{
		OrganizationID:      project.OrganizationID,
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

	// Sync from upstream first — helix-specs can be modified outside Helix
	var versions []services.StartupScriptVersion
	syncErr := s.gitRepositoryService.WithExternalRepoRead(r.Context(), primaryRepo, func() error {
		var err error
		versions, err = s.projectInternalRepoService.GetStartupScriptHistoryFromHelixSpecs(primaryRepo.LocalPath)
		return err
	})
	if syncErr != nil {
		log.Error().
			Err(syncErr).
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

	// Add warning about agents not being moved
	warnings := []string{
		"Agents configured on this project will not be moved. You'll need to create new agents in the target organization and update the project settings.",
	}

	return &types.MoveProjectPreviewResponse{
		Project:      projectPreview,
		Repositories: repoPreview,
		Warnings:     warnings,
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

// getProjectUsage godoc
// @Summary Get project token usage
// @Description Get token usage metrics for a project (combined across all tasks)
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param aggregation_level query string false "Aggregation level (5min, hourly, daily)" default(hourly)
// @Param from query string false "Start time (RFC3339 format)"
// @Param to query string false "End time (RFC3339 format)"
// @Success 200 {array} types.AggregatedUsageMetric
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/usage [get]
func (s *HelixAPIServer) getProjectUsage(_ http.ResponseWriter, r *http.Request) ([]*types.AggregatedUsageMetric, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	if user == nil {
		return nil, system.NewHTTPError401("user not found")
	}

	// Get the project and check authorization
	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Parse aggregation level
	aggregationLevel := store.AggregationLevelHourly
	switch r.URL.Query().Get("aggregation_level") {
	case "daily":
		aggregationLevel = store.AggregationLevelDaily
	case "5min":
		aggregationLevel = store.AggregationLevel5Min
	}

	// Parse time range
	var from time.Time
	var to time.Time

	if r.URL.Query().Get("from") != "" {
		from, err = time.Parse(time.RFC3339, r.URL.Query().Get("from"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse from date: %s", err))
		}
	} else {
		// Default to last 7 days for project-level usage
		from = time.Now().Add(-7 * 24 * time.Hour)
	}

	if r.URL.Query().Get("to") != "" {
		to, err = time.Parse(time.RFC3339, r.URL.Query().Get("to"))
		if err != nil {
			return nil, system.NewHTTPError400(fmt.Sprintf("failed to parse to date: %s", err))
		}
	} else {
		to = time.Now()
	}

	// Get aggregated usage metrics for the project (combined across all tasks)
	metrics, err := s.Store.GetAggregatedUsageMetrics(r.Context(), &store.GetAggregatedUsageMetricsQuery{
		AggregationLevel: aggregationLevel,
		UserID:           user.ID,
		ProjectID:        projectID,
		// No SpecTaskID - get combined usage for the whole project
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, system.NewHTTPError500(err.Error())
	}

	return metrics, nil
}

// triggerGoldenBuild godoc
// @Summary Trigger golden Docker cache build
// @Description Manually trigger a golden Docker cache build for a project
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 409 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/docker-cache/build [post]
func (s *HelixAPIServer) triggerGoldenBuild(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if s.goldenBuildService == nil {
		return nil, system.NewHTTPError500("golden build service not available")
	}

	err = s.goldenBuildService.TriggerManualGoldenBuild(r.Context(), project)
	if err != nil {
		return nil, &system.HTTPError{StatusCode: 409, Message: err.Error()}
	}

	return map[string]string{"message": "golden build triggered"}, nil
}

// cancelGoldenBuild godoc
// @Summary Cancel running golden Docker cache builds
// @Description Stop all running golden builds for a project
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 409 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/docker-cache/cancel [post]
func (s *HelixAPIServer) cancelGoldenBuild(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	if s.goldenBuildService == nil {
		return nil, system.NewHTTPError500("golden build service not available")
	}

	err = s.goldenBuildService.CancelGoldenBuilds(r.Context(), project)
	if err != nil {
		return nil, &system.HTTPError{StatusCode: 409, Message: err.Error()}
	}

	return map[string]string{"message": "golden builds cancelled"}, nil
}

// deleteDockerCache godoc
// @Summary Clear golden Docker cache
// @Description Remove the golden Docker cache for a project from all sandboxes
// @Tags Projects
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/docker-cache [delete]
func (s *HelixAPIServer) deleteDockerCache(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionUpdate)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	// Send delete to all online sandboxes
	sandboxes, err := s.Store.ListSandboxes(r.Context())
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list sandboxes: %v", err))
	}

	var errors []string
	deleted := 0
	for _, sb := range sandboxes {
		if sb.Status != "online" {
			continue
		}
		hydraClient := hydra.NewRevDialClient(s.connman, fmt.Sprintf("hydra-%s", sb.ID))
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		err := hydraClient.DeleteGoldenCache(ctx, projectID)
		cancel()
		if err != nil {
			errors = append(errors, fmt.Sprintf("sandbox %s: %v", sb.ID, err))
		} else {
			deleted++
		}
	}

	if len(errors) > 0 && deleted == 0 {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to clear cache: %s", errors[0]))
	}

	// Reset project docker cache status — empty sandboxes map
	project.Metadata.DockerCacheStatus = &types.DockerCacheState{
		Sandboxes: make(map[string]*types.SandboxCacheState),
	}
	if err := s.Store.UpdateProject(r.Context(), project); err != nil {
		log.Warn().Err(err).Str("project_id", projectID).Msg("Failed to reset docker cache status")
	}

	return map[string]string{"message": fmt.Sprintf("cache cleared on %d sandbox(es)", deleted)}, nil
}

// getDockerCacheZFSTree returns the ZFS snapshot/clone tree for a project's docker cache.
// Proxies to Hydra on the first online sandbox.
func (s *HelixAPIServer) getDockerCacheZFSTree(_ http.ResponseWriter, r *http.Request) (interface{}, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	project, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	err = s.authorizeUserToProject(r.Context(), user, project, types.ActionGet)
	if err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	sandboxes, err := s.Store.ListSandboxes(r.Context())
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to list sandboxes: %v", err))
	}

	for _, sb := range sandboxes {
		if sb.Status != "online" {
			continue
		}
		hydraClient := hydra.NewRevDialClient(s.connman, fmt.Sprintf("hydra-%s", sb.ID))
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		tree, err := hydraClient.GetZFSTree(ctx, projectID)
		cancel()
		if err != nil {
			continue // try next sandbox
		}
		return tree, nil
	}

	// No sandbox available — return empty tree
	return &hydra.ZFSTree{Available: false}, nil
}

// PinnedProjectsResponse is the response body for pin/unpin endpoints
type PinnedProjectsResponse struct {
	PinnedProjectIDs []string `json:"pinned_project_ids"`
}

// pinProject godoc
// @Summary Pin a project
// @Description Pin a project for the current user so it appears at the top of the projects board
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} PinnedProjectsResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/pin [post]
func (s *HelixAPIServer) pinProject(_ http.ResponseWriter, r *http.Request) (*PinnedProjectsResponse, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	// Verify project exists
	_, err := s.Store.GetProject(r.Context(), projectID)
	if err != nil {
		return nil, system.NewHTTPError404("project not found")
	}

	userMeta, err := s.Store.EnsureUserMeta(r.Context(), types.UserMeta{ID: user.ID})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to load user meta: %v", err))
	}

	// Add project ID if not already pinned
	for _, id := range userMeta.Config.PinnedProjectIDs {
		if id == projectID {
			return &PinnedProjectsResponse{PinnedProjectIDs: userMeta.Config.PinnedProjectIDs}, nil
		}
	}
	userMeta.Config.PinnedProjectIDs = append(userMeta.Config.PinnedProjectIDs, projectID)

	if _, err := s.Store.UpdateUserMeta(r.Context(), *userMeta); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update user meta: %v", err))
	}

	return &PinnedProjectsResponse{PinnedProjectIDs: userMeta.Config.PinnedProjectIDs}, nil
}

// unpinProject godoc
// @Summary Unpin a project
// @Description Remove a project from the current user's pinned projects
// @Tags Projects
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Success 200 {object} PinnedProjectsResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/{id}/pin [delete]
func (s *HelixAPIServer) unpinProject(_ http.ResponseWriter, r *http.Request) (*PinnedProjectsResponse, *system.HTTPError) {
	user := getRequestUser(r)
	projectID := getID(r)

	userMeta, err := s.Store.EnsureUserMeta(r.Context(), types.UserMeta{ID: user.ID})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to load user meta: %v", err))
	}

	// Remove project ID from pinned list
	updated := userMeta.Config.PinnedProjectIDs[:0]
	for _, id := range userMeta.Config.PinnedProjectIDs {
		if id != projectID {
			updated = append(updated, id)
		}
	}
	userMeta.Config.PinnedProjectIDs = updated

	if _, err := s.Store.UpdateUserMeta(r.Context(), *userMeta); err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to update user meta: %v", err))
	}

	return &PinnedProjectsResponse{PinnedProjectIDs: userMeta.Config.PinnedProjectIDs}, nil
}

// applyProject godoc
// @Summary Apply a project YAML
// @Description Idempotent upsert of a project from a declarative YAML spec
// @Tags Projects
// @Accept json
// @Produce json
// @Param request body types.ProjectApplyRequest true "Project apply request"
// @Success 200 {object} types.ProjectApplyResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/projects/apply [put]
func (s *HelixAPIServer) applyProject(_ http.ResponseWriter, r *http.Request) (*types.ProjectApplyResponse, *system.HTTPError) {
	user := getRequestUser(r)

	var req types.ProjectApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	if req.Name == "" {
		return nil, system.NewHTTPError400("name is required")
	}

	// Validate repositories
	if err := req.Spec.ValidateRepositories(); err != nil {
		return nil, system.NewHTTPError400(err.Error())
	}

	// Resolve org: if provided, verify membership
	orgID := req.OrganizationID
	if orgID != "" {
		if _, err := s.authorizeOrgMember(r.Context(), user, orgID); err != nil {
			return nil, system.NewHTTPError403(err.Error())
		}
	}

	// Idempotency: look up existing project by name + org/user
	var existingProjects []*types.Project
	var listErr error
	if orgID != "" {
		existingProjects, listErr = s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{OrganizationID: orgID})
	} else {
		existingProjects, listErr = s.Store.ListProjects(r.Context(), &store.ListProjectsQuery{UserID: user.ID})
	}
	if listErr != nil {
		return nil, system.NewHTTPError500(listErr.Error())
	}

	var project *types.Project
	for _, p := range existingProjects {
		if p.Name == req.Name {
			project = p
			break
		}
	}

	wasCreated := project == nil
	if wasCreated {
		project = &types.Project{
			ID:             system.GenerateProjectID(),
			Name:           req.Name,
			UserID:         user.ID,
			OrganizationID: orgID,
			Status:         "active",
		}
	}

	// Apply spec fields
	if req.Spec.Description != "" {
		project.Description = req.Spec.Description
	}
	if len(req.Spec.Technologies) > 0 {
		project.Technologies = req.Spec.Technologies
	}
	if req.Spec.Guidelines != "" {
		project.Guidelines = req.Spec.Guidelines
	}
	if req.Spec.Startup != nil {
		// Prefer unified Script field, fall back to Install/Start for backward compatibility
		if req.Spec.Startup.Script != "" {
			project.StartupInstall = "" // Clear legacy fields
			project.StartupStart = ""
			// The script will be written to helix-specs branch below
		} else {
			project.StartupInstall = req.Spec.Startup.Install
			project.StartupStart = req.Spec.Startup.Start
		}
		project.StartupScriptFromYAML = true
	}
	if req.Spec.AutoStartBacklogTasks {
		project.AutoStartBacklogTasks = true
	}
	if req.Spec.Kanban != nil && req.Spec.Kanban.WIPLimits != nil {
		if project.Metadata.BoardSettings == nil {
			project.Metadata.BoardSettings = &types.BoardSettings{}
		}
		project.Metadata.BoardSettings.WIPLimits = types.WIPLimits{
			Planning:       req.Spec.Kanban.WIPLimits.Planning,
			Implementation: req.Spec.Kanban.WIPLimits.Implementation,
			Review:         req.Spec.Kanban.WIPLimits.Review,
		}
	}

	if wasCreated {
		if _, err := s.Store.CreateProject(r.Context(), project); err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to create project: %v", err))
		}
	} else {
		if err := s.Store.UpdateProject(r.Context(), project); err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to update project: %v", err))
		}
	}

	// Attach repositories
	var primaryRepo *types.GitRepository
	resolvedRepos := req.Spec.ResolvedRepositories()
	for _, repoSpec := range resolvedRepos {
		// Find-or-create git repository by external URL
		repo, err := s.Store.GetGitRepositoryByExternalURL(r.Context(), orgID, repoSpec.URL)
		if err != nil {
			if err != store.ErrNotFound {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to look up repository %s: %v", repoSpec.URL, err))
			}
			// Create it
			branch := repoSpec.DefaultBranch
			if branch == "" {
				branch = "main"
			}
			// Derive a short human-readable name from the URL (e.g. "robot-hq" from
			// "https://github.com/binocarlos/robot-hq"). This name is used as the
			// workspace directory name inside the sandbox (/home/retro/work/<name>),
			// so it must never be the full URL string.
			repoName := strings.TrimSuffix(path.Base(repoSpec.URL), ".git")
			if repoName == "" || repoName == "." {
				repoName = repoSpec.URL
			}
			repo = &types.GitRepository{
				ID:             system.GenerateUUID(),
				Name:           repoName,
				OrganizationID: orgID,
				OwnerID:        user.ID,
				RepoType:       types.GitRepositoryTypeCode,
				IsExternal:     true,
				ExternalURL:    repoSpec.URL,
				CloneURL:       repoSpec.URL,
				DefaultBranch:  branch,
				Status:         types.GitRepositoryStatusActive,
			}
			if err := s.Store.CreateGitRepository(r.Context(), repo); err != nil {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to create repository %s: %v", repoSpec.URL, err))
			}
		}
		if err := s.Store.AttachRepositoryToProject(r.Context(), project.ID, repo.ID); err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to attach repository %s: %v", repoSpec.URL, err))
		}
		if repoSpec.Primary {
			if err := s.Store.SetProjectPrimaryRepository(r.Context(), project.ID, repo.ID); err != nil {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to set primary repository: %v", err))
			}
			primaryRepo = repo
		}
	}

	// Write startup script to helix-specs branch in the primary repo.
	// For newly-created external repos (LocalPath == ""), trigger an async clone that
	// calls SaveStartupScriptToHelixSpecs once the clone completes.
	// For repos already cloned (LocalPath != ""), write the script synchronously.
	if primaryRepo != nil && req.Spec.Startup != nil {
		var startupScript string
		if req.Spec.Startup.Script != "" {
			startupScript = req.Spec.Startup.Script
		} else if req.Spec.Startup.Install != "" || req.Spec.Startup.Start != "" {
			startupScript = synthesizeStartupScript(req.Spec.Startup.Install, req.Spec.Startup.Start)
		}
		if startupScript != "" {
			userName := user.FullName
			userEmail := user.Email
			repoSvc := s.projectInternalRepoService

			if primaryRepo.LocalPath == "" {
				// Repo not yet cloned — trigger async clone; write startup script after clone succeeds.
				log.Info().Str("repo_id", primaryRepo.ID).Msg("Triggering async clone to initialize startup script in helix-specs")
				s.gitRepositoryService.CloneRepositoryAsync(primaryRepo, func(localPath string) {
					if _, err := repoSvc.SaveStartupScriptToHelixSpecs(localPath, startupScript, userName, userEmail); err != nil {
						log.Warn().Err(err).Str("repo_id", primaryRepo.ID).Msg("failed to write startup script to helix-specs after clone")
					} else {
						log.Info().Str("repo_id", primaryRepo.ID).Msg("Startup script written to helix-specs after async clone")
					}
				})
			} else {
				// Repo already cloned — write startup script synchronously.
				if _, err := repoSvc.SaveStartupScriptToHelixSpecs(primaryRepo.LocalPath, startupScript, userName, userEmail); err != nil {
					log.Warn().Err(err).Str("repo_id", primaryRepo.ID).Msg("failed to write startup script to helix-specs")
				}
			}
		}
	}

	// Seed Kanban tasks (idempotent by title — only creates tasks not already present)
	if len(req.Spec.Tasks) > 0 {
		existingTasks, err := s.Store.ListSpecTasks(r.Context(), &types.SpecTaskFilters{ProjectID: project.ID})
		if err != nil {
			return nil, system.NewHTTPError500(fmt.Sprintf("failed to list tasks: %v", err))
		}
		existingTitles := make(map[string]bool, len(existingTasks))
		for _, t := range existingTasks {
			existingTitles[t.Name] = true
		}
		for _, taskSpec := range req.Spec.Tasks {
			if taskSpec.Title == "" || existingTitles[taskSpec.Title] {
				continue
			}
			task := &types.SpecTask{
				ID:             system.GenerateUUID(),
				ProjectID:      project.ID,
				UserID:         user.ID,
				CreatedBy:      user.ID,
				OrganizationID: orgID,
				Name:           taskSpec.Title,
				Description:    taskSpec.Description,
				Status:         types.TaskStatusBacklog,
				Priority:       types.SpecTaskPriorityMedium,
				Type:           "task",
			}
			if err := s.Store.CreateSpecTask(r.Context(), task); err != nil {
				log.Warn().Err(err).Str("title", taskSpec.Title).Msg("failed to seed task")
			}
		}
	}

	// Create or update the project's agent app from the agent spec
	var agentAppID string
	if req.Spec.Agent != nil {
		agentSpec := req.Spec.Agent
		// Map runtime string → AgentType + CodeAgentRuntime.
		// When runtime is set, the agent runs inside a Zed desktop container (zed_external).
		// When omitted, a plain chat agent (helix_basic) is created.
		agentType, codeRuntime := projectAgentRuntimeToTypes(agentSpec.Runtime)

		var credType types.CodeAgentCredentialType
		if agentSpec.Credentials == "subscription" {
			credType = types.CodeAgentCredentialTypeSubscription
		}

		assistant := types.AssistantConfig{
			Name:                    agentSpec.Name,
			Model:                   agentSpec.Model,
			Provider:                agentSpec.Provider,
			AgentType:               agentType,
			CodeAgentRuntime:        codeRuntime,
			CodeAgentCredentialType: credType,
		}
		if agentSpec.Tools != nil {
			assistant.WebSearch = types.AssistantWebSearch{Enabled: agentSpec.Tools.WebSearch}
			assistant.Browser = types.AssistantBrowser{Enabled: agentSpec.Tools.Browser}
			assistant.Calculator = types.AssistantCalculator{Enabled: agentSpec.Tools.Calculator}
		}

		appHelixConfig := types.AppHelixConfig{
			Name:             agentSpec.Name,
			Assistants:       []types.AssistantConfig{assistant},
			DefaultAgentType: agentType,
		}
		if agentType == types.AgentTypeZedExternal && agentSpec.Display != nil {
			appHelixConfig.ExternalAgentEnabled = true
			appHelixConfig.ExternalAgentConfig = &types.ExternalAgentConfig{
				Resolution:         agentSpec.Display.Resolution,
				DesktopType:        agentSpec.Display.DesktopType,
				DisplayRefreshRate: agentSpec.Display.FPS,
			}
		} else if agentType == types.AgentTypeZedExternal {
			appHelixConfig.ExternalAgentEnabled = true
		}

		var agentApp *types.App
		if project.DefaultHelixAppID != "" {
			agentApp, _ = s.Store.GetApp(r.Context(), project.DefaultHelixAppID)
		}

		if agentApp != nil {
			agentApp.Config.Helix = appHelixConfig
			if _, err := s.Store.UpdateApp(r.Context(), agentApp); err != nil {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to update agent app: %v", err))
			}
			agentAppID = agentApp.ID
		} else {
			agentApp = &types.App{
				ID:             system.GenerateUUID(),
				Owner:          user.ID,
				OwnerType:      types.OwnerTypeUser,
				OrganizationID: orgID,
				Config: types.AppConfig{
					Helix: appHelixConfig,
				},
			}
			if _, err := s.Store.CreateApp(r.Context(), agentApp); err != nil {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to create agent app: %v", err))
			}
			agentAppID = agentApp.ID
			project.DefaultHelixAppID = agentApp.ID
			if err := s.Store.UpdateProject(r.Context(), project); err != nil {
				return nil, system.NewHTTPError500(fmt.Sprintf("failed to link agent app to project: %v", err))
			}
		}
	}

	return &types.ProjectApplyResponse{
		ProjectID:  project.ID,
		Created:    wasCreated,
		AgentAppID: agentAppID,
	}, nil
}

// synthesizeStartupScript builds a shell script from declarative startup fields
// (startup_install / startup_start) set via `helix apply -f project.yaml`.
// It is shown in the UI when no .helix/startup.sh exists in the git repo yet.
func synthesizeStartupScript(install, start string) string {
	var sb strings.Builder
	sb.WriteString("#!/bin/bash\nset -e\n")
	if install != "" {
		sb.WriteString("\n# Install dependencies\n")
		sb.WriteString(install)
		sb.WriteString("\n")
	}
	if start != "" {
		sb.WriteString("\n# Start services\n")
		sb.WriteString(start)
		sb.WriteString("\n")
	}
	return sb.String()
}

// projectAgentRuntimeToTypes maps the human-friendly runtime string from project.yaml
// to the internal AgentType + CodeAgentRuntime pair.
// When runtime is empty or unrecognised, defaults to claude_code (recommended: handles
// context compaction automatically, unlike Zed's built-in agent).
func projectAgentRuntimeToTypes(runtime string) (types.AgentType, types.CodeAgentRuntime) {
	switch runtime {
	case "zed", "zed_agent":
		return types.AgentTypeZedExternal, types.CodeAgentRuntimeZedAgent
	case "qwen_code":
		return types.AgentTypeZedExternal, types.CodeAgentRuntimeQwenCode
	case "gemini_cli":
		return types.AgentTypeZedExternal, types.CodeAgentRuntimeGeminiCLI
	case "codex_cli":
		return types.AgentTypeZedExternal, types.CodeAgentRuntimeCodexCLI
	default:
		// "claude_code" or empty/unrecognised → Claude Code CLI (default)
		return types.AgentTypeZedExternal, types.CodeAgentRuntimeClaudeCode
	}
}
