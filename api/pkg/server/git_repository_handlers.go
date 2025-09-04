package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/rs/zerolog/log"
)

// createGitRepository creates a new git repository
func (apiServer *HelixAPIServer) createGitRepository(w http.ResponseWriter, r *http.Request) {
	var request services.GitRepositoryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	// Validate required fields
	if request.Name == "" {
		system.Error(w, http.StatusBadRequest, "Repository name is required")
		return
	}
	if request.OwnerID == "" {
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}
	if request.RepoType == "" {
		request.RepoType = services.GitRepositoryTypeProject
	}

	// Create repository
	repository, err := apiServer.gitRepositoryService.CreateRepository(r.Context(), &request)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create git repository")
		system.Error(w, http.StatusInternalServerError, "Failed to create repository: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusCreated, repository)
}

// getGitRepository retrieves repository information by ID
func (apiServer *HelixAPIServer) getGitRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		system.Error(w, http.StatusBadRequest, "Repository ID is required")
		return
	}

	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get git repository")
		system.Error(w, http.StatusNotFound, "Repository not found: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusOK, repository)
}

// listGitRepositories lists all repositories, optionally filtered by owner
func (apiServer *HelixAPIServer) listGitRepositories(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	repoType := r.URL.Query().Get("repo_type")

	repositories, err := apiServer.gitRepositoryService.ListRepositories(r.Context(), ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list git repositories")
		system.Error(w, http.StatusInternalServerError, "Failed to list repositories: %s", err.Error())
		return
	}

	// Filter by repo type if specified
	if repoType != "" {
		filtered := make([]*services.GitRepository, 0)
		for _, repo := range repositories {
			if string(repo.RepoType) == repoType {
				filtered = append(filtered, repo)
			}
		}
		repositories = filtered
	}

	system.JsonResponse(w, http.StatusOK, repositories)
}

// createSpecTaskRepository creates a repository for a SpecTask
func (apiServer *HelixAPIServer) createSpecTaskRepository(w http.ResponseWriter, r *http.Request) {
	var request CreateSpecTaskRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	if request.SpecTaskID == "" {
		system.Error(w, http.StatusBadRequest, "SpecTask ID is required")
		return
	}

	// Get SpecTask from store
	specTask, err := apiServer.Store.GetSpecTask(r.Context(), request.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to get SpecTask")
		system.Error(w, http.StatusNotFound, "SpecTask not found: %s", err.Error())
		return
	}

	repository, err := apiServer.gitRepositoryService.CreateSpecTaskRepository(
		r.Context(),
		specTask,
		request.TemplateFiles,
	)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to create SpecTask repository")
		system.Error(w, http.StatusInternalServerError, "Failed to create SpecTask repository: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusCreated, repository)
}

// createSampleRepository creates a sample/demo repository
func (apiServer *HelixAPIServer) createSampleRepository(w http.ResponseWriter, r *http.Request) {
	var request CreateSampleRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	if request.Name == "" {
		system.Error(w, http.StatusBadRequest, "Repository name is required")
		return
	}
	if request.SampleType == "" {
		system.Error(w, http.StatusBadRequest, "Sample type is required")
		return
	}
	if request.OwnerID == "" {
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}

	repository, err := apiServer.gitRepositoryService.CreateSampleRepository(
		r.Context(),
		request.Name,
		request.Description,
		request.OwnerID,
		request.SampleType,
	)
	if err != nil {
		log.Error().Err(err).Str("sample_type", request.SampleType).Msg("Failed to create sample repository")
		system.Error(w, http.StatusInternalServerError, "Failed to create sample repository: %s", err.Error())
		return
	}

	system.JsonResponse(w, http.StatusCreated, repository)
}

// getGitRepositoryCloneCommand returns the git clone command for a repository
func (apiServer *HelixAPIServer) getGitRepositoryCloneCommand(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		system.Error(w, http.StatusBadRequest, "Repository ID is required")
		return
	}

	targetDir := r.URL.Query().Get("target_dir")

	// Verify repository exists
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for clone command")
		system.Error(w, http.StatusNotFound, "Repository not found: %s", err.Error())
		return
	}

	cloneCommand := apiServer.gitRepositoryService.GetCloneCommand(repoID, targetDir)

	response := CloneCommandResponse{
		RepositoryID: repoID,
		CloneURL:     repository.CloneURL,
		CloneCommand: cloneCommand,
		TargetDir:    targetDir,
	}

	system.JsonResponse(w, http.StatusOK, response)
}

// initializeSampleRepositories creates default sample repositories
func (apiServer *HelixAPIServer) initializeSampleRepositories(w http.ResponseWriter, r *http.Request) {
	var request InitializeSampleRepositoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		system.Error(w, http.StatusBadRequest, "Invalid request format: %s", err.Error())
		return
	}

	if request.OwnerID == "" {
		system.Error(w, http.StatusBadRequest, "Owner ID is required")
		return
	}

	sampleTypes := []struct {
		Name        string
		Description string
		SampleType  string
	}{
		{
			Name:        "Node.js Todo App",
			Description: "A simple todo application built with Node.js and Express",
			SampleType:  "nodejs-todo",
		},
		{
			Name:        "Python API Service",
			Description: "A FastAPI microservice with PostgreSQL integration",
			SampleType:  "python-api",
		},
		{
			Name:        "React Dashboard",
			Description: "A modern admin dashboard built with React and Material-UI",
			SampleType:  "react-dashboard",
		},
	}

	createdRepositories := make([]*services.GitRepository, 0)
	errors := make([]string, 0)

	for _, sample := range sampleTypes {
		// Skip if this sample type is disabled
		if len(request.SampleTypes) > 0 {
			found := false
			for _, enabledType := range request.SampleTypes {
				if enabledType == sample.SampleType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		repository, err := apiServer.gitRepositoryService.CreateSampleRepository(
			r.Context(),
			sample.Name,
			sample.Description,
			request.OwnerID,
			sample.SampleType,
		)
		if err != nil {
			log.Error().Err(err).Str("sample_type", sample.SampleType).Msg("Failed to create sample repository")
			errors = append(errors, fmt.Sprintf("Failed to create %s: %s", sample.Name, err.Error()))
		} else {
			createdRepositories = append(createdRepositories, repository)
		}
	}

	response := InitializeSampleRepositoriesResponse{
		CreatedRepositories: createdRepositories,
		CreatedCount:        len(createdRepositories),
		Errors:              errors,
		Success:             len(errors) == 0,
	}

	statusCode := http.StatusOK
	if len(errors) > 0 && len(createdRepositories) == 0 {
		statusCode = http.StatusInternalServerError
	}

	system.JsonResponse(w, statusCode, response)
}

// Request/Response types for API documentation

// CreateSpecTaskRepositoryRequest represents a request to create a SpecTask repository
type CreateSpecTaskRepositoryRequest struct {
	SpecTaskID    string            `json:"spec_task_id"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	OwnerID       string            `json:"owner_id"`
	ProjectID     string            `json:"project_id,omitempty"`
	TemplateFiles map[string]string `json:"template_files,omitempty"`
}

// CreateSampleRepositoryRequest represents a request to create a sample repository
type CreateSampleRepositoryRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	OwnerID     string `json:"owner_id"`
	SampleType  string `json:"sample_type"`
}

// CloneCommandResponse represents the response for clone command request
type CloneCommandResponse struct {
	RepositoryID string `json:"repository_id"`
	CloneURL     string `json:"clone_url"`
	CloneCommand string `json:"clone_command"`
	TargetDir    string `json:"target_dir,omitempty"`
}

// InitializeSampleRepositoriesRequest represents a request to initialize sample repositories
type InitializeSampleRepositoriesRequest struct {
	OwnerID     string   `json:"owner_id"`
	SampleTypes []string `json:"sample_types,omitempty"` // If empty, creates all samples
}

// InitializeSampleRepositoriesResponse represents the response for initializing sample repositories
type InitializeSampleRepositoriesResponse struct {
	CreatedRepositories []*services.GitRepository `json:"created_repositories"`
	CreatedCount        int                       `json:"created_count"`
	Errors              []string                  `json:"errors,omitempty"`
	Success             bool                      `json:"success"`
}
