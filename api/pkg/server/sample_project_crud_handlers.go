package server

import (
	"encoding/json"
	"net/http"

	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listSampleProjectsV2 godoc
// @Summary List sample projects
// @Description Get all available sample projects
// @Tags SampleProjects
// @Accept json
// @Produce json
// @Success 200 {array} types.SampleProject
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sample-projects-v2 [get]
func (s *HelixAPIServer) listSampleProjectsV2(_ http.ResponseWriter, r *http.Request) ([]*types.SampleProject, *system.HTTPError) {
	samples, err := s.Store.ListSampleProjects(r.Context())
	if err != nil {
		log.Error().
			Err(err).
			Msg("failed to list sample projects")
		return nil, system.NewHTTPError500(err.Error())
	}

	return samples, nil
}

// getSampleProjectByID godoc
// @Summary Get sample project
// @Description Get a sample project by ID
// @Tags SampleProjects
// @Accept json
// @Produce json
// @Param id path string true "Sample Project ID"
// @Success 200 {object} types.SampleProject
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sample-projects-v2/{id} [get]
func (s *HelixAPIServer) getSampleProjectByID(_ http.ResponseWriter, r *http.Request) (*types.SampleProject, *system.HTTPError) {
	sampleID := getID(r)

	sample, err := s.Store.GetSampleProject(r.Context(), sampleID)
	if err != nil {
		log.Error().
			Err(err).
			Str("sample_id", sampleID).
			Msg("failed to get sample project")
		return nil, system.NewHTTPError404("sample project not found")
	}

	return sample, nil
}

// instantiateSampleProject godoc
// @Summary Instantiate sample project
// @Description Create a new project from a sample project template
// @Tags SampleProjects
// @Accept json
// @Produce json
// @Param id path string true "Sample Project ID"
// @Param request body types.SampleProjectInstantiateRequest true "Instantiate request"
// @Success 200 {object} types.SampleProjectInstantiateResponse
// @Failure 400 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sample-projects-v2/{id}/instantiate [post]
func (s *HelixAPIServer) instantiateSampleProject(_ http.ResponseWriter, r *http.Request) (*types.SampleProjectInstantiateResponse, *system.HTTPError) {
	user := getRequestUser(r)
	sampleID := getID(r)

	var req types.SampleProjectInstantiateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error().
			Err(err).
			Msg("failed to decode sample project instantiate request")
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Get the sample project
	sample, err := s.Store.GetSampleProject(r.Context(), sampleID)
	if err != nil {
		log.Error().
			Err(err).
			Str("sample_id", sampleID).
			Msg("failed to get sample project for instantiation")
		return nil, system.NewHTTPError404("sample project not found")
	}

	// Create new project from sample
	projectName := req.ProjectName
	if projectName == "" {
		projectName = sample.Name
	}

	project := &types.Project{
		ID:            system.GenerateUUID(),
		Name:          projectName,
		Description:   sample.Description,
		UserID:        user.ID,
		GitHubRepoURL: sample.RepositoryURL,
		Status:        "active",
		StartupScript: sample.StartupScript,
	}

	created, err := s.Store.CreateProject(r.Context(), project)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("sample_id", sampleID).
			Msg("failed to create project from sample")
		return nil, system.NewHTTPError500(err.Error())
	}

	// Clone sample repository into project's internal repo (if sample has a repo URL)
	if sample.RepositoryURL != "" && sample.RepositoryURL != "https://github.com/helixml/sample-todo-app" {
		// Only clone if it's a real URL (not our placeholder URLs)
		internalRepoPath, err := s.projectInternalRepoService.CloneSampleProject(r.Context(), created, sample.RepositoryURL)
		if err != nil {
			log.Warn().
				Err(err).
				Str("project_id", created.ID).
				Str("sample_url", sample.RepositoryURL).
				Msg("failed to clone sample repository, will create empty internal repo instead")

			// Fallback: create empty internal repo with template
			internalRepoPath, err = s.projectInternalRepoService.InitializeProjectRepo(r.Context(), created)
			if err != nil {
				log.Error().
					Err(err).
					Str("project_id", created.ID).
					Msg("failed to initialize internal repository")
			}
		}

		if internalRepoPath != "" {
			// Update project with internal repo path
			created.InternalRepoPath = internalRepoPath
			if err := s.Store.UpdateProject(r.Context(), created); err != nil {
				log.Error().
					Err(err).
					Str("project_id", created.ID).
					Msg("failed to update project with internal repo path")
			}
		}
	} else {
		// No real sample URL, create empty internal repo
		internalRepoPath, err := s.projectInternalRepoService.InitializeProjectRepo(r.Context(), created)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", created.ID).
				Msg("failed to initialize internal repository")
		} else {
			created.InternalRepoPath = internalRepoPath
			if err := s.Store.UpdateProject(r.Context(), created); err != nil {
				log.Error().
					Err(err).
					Str("project_id", created.ID).
					Msg("failed to update project with internal repo path")
			}
		}
	}

	// Parse sample tasks and create spec tasks
	var sampleTasks []types.SampleProjectTask
	if sample.SampleTasks != nil {
		err = json.Unmarshal(sample.SampleTasks, &sampleTasks)
		if err != nil {
			log.Warn().
				Err(err).
				Str("sample_id", sampleID).
				Msg("failed to parse sample tasks")
		}
	}

	// Create spec tasks from sample tasks
	for _, taskTemplate := range sampleTasks {
		specTask := &types.SpecTask{
			ID:          system.GenerateUUID(),
			ProjectID:   created.ID,
			Name:        taskTemplate.Title,
			Description: taskTemplate.Description,
			Type:        taskTemplate.Type,
			Priority:    taskTemplate.Priority,
			Status:      types.TaskStatusBacklog,
			CreatedBy:   user.ID,
		}

		err = s.Store.CreateSpecTask(r.Context(), specTask)
		if err != nil {
			log.Error().
				Err(err).
				Str("project_id", created.ID).
				Str("task_title", taskTemplate.Title).
				Msg("failed to create spec task from sample")
			// Continue creating other tasks even if one fails
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Str("project_id", created.ID).
		Str("sample_id", sampleID).
		Int("tasks_created", len(sampleTasks)).
		Msg("sample project instantiated successfully")

	return &types.SampleProjectInstantiateResponse{
		ProjectID: created.ID,
		Message:   "Sample project instantiated successfully",
	}, nil
}

// Admin-only endpoints for managing sample projects

// createSampleProject godoc
// @Summary Create sample project (Admin)
// @Description Create a new sample project template (admin only)
// @Tags SampleProjects
// @Accept json
// @Produce json
// @Param request body types.SampleProject true "Sample project data"
// @Success 200 {object} types.SampleProject
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/sample-projects [post]
func (s *HelixAPIServer) createSampleProject(_ http.ResponseWriter, r *http.Request) (*types.SampleProject, *system.HTTPError) {
	user := getRequestUser(r)

	// Admin check - user must be admin
	if !user.Admin {
		log.Warn().
			Str("user_id", user.ID).
			Msg("non-admin user attempted to create sample project")
		return nil, system.NewHTTPError401("admin access required")
	}

	var sample types.SampleProject
	if err := json.NewDecoder(r.Body).Decode(&sample); err != nil {
		log.Error().
			Err(err).
			Msg("failed to decode sample project create request")
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Validate required fields
	if sample.Name == "" {
		return nil, system.NewHTTPError400("sample project name is required")
	}
	if sample.RepositoryURL == "" {
		return nil, system.NewHTTPError400("repository URL is required")
	}

	// Generate ID if not provided
	if sample.ID == "" {
		sample.ID = system.GenerateUUID()
	}

	created, err := s.Store.CreateSampleProject(r.Context(), &sample)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("sample_name", sample.Name).
			Msg("failed to create sample project")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("sample_id", created.ID).
		Str("sample_name", created.Name).
		Msg("sample project created successfully")

	return created, nil
}

// deleteSampleProject godoc
// @Summary Delete sample project (Admin)
// @Description Delete a sample project template (admin only)
// @Tags SampleProjects
// @Accept json
// @Produce json
// @Param id path string true "Sample Project ID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security BearerAuth
// @Router /api/v1/admin/sample-projects/{id} [delete]
func (s *HelixAPIServer) deleteSampleProject(_ http.ResponseWriter, r *http.Request) (map[string]string, *system.HTTPError) {
	user := getRequestUser(r)
	sampleID := getID(r)

	// Admin check - user must be admin
	if !user.Admin {
		log.Warn().
			Str("user_id", user.ID).
			Msg("non-admin user attempted to delete sample project")
		return nil, system.NewHTTPError401("admin access required")
	}

	err := s.Store.DeleteSampleProject(r.Context(), sampleID)
	if err != nil {
		log.Error().
			Err(err).
			Str("user_id", user.ID).
			Str("sample_id", sampleID).
			Msg("failed to delete sample project")
		return nil, system.NewHTTPError500(err.Error())
	}

	log.Info().
		Str("user_id", user.ID).
		Str("sample_id", sampleID).
		Msg("sample project deleted successfully")

	return map[string]string{"message": "sample project deleted successfully"}, nil
}
