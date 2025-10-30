package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/rs/zerolog/log"
)

// createGitRepository creates a new git repository
// @Summary Create git repository
// @Description Create a new git repository on the server
// @Tags git-repositories
// @Accept json
// @Produce json
// @Param repository body services.GitRepositoryCreateRequest true "Repository creation request"
// @Success 201 {object} services.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createGitRepository(w http.ResponseWriter, r *http.Request) {
	var request services.GitRepositoryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if request.Name == "" {
		http.Error(w, "Repository name is required", http.StatusBadRequest)
		return
	}
	if request.OwnerID == "" {
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
		return
	}
	if request.RepoType == "" {
		request.RepoType = services.GitRepositoryTypeProject
	}

	// Create repository
	repository, err := apiServer.gitRepositoryService.CreateRepository(r.Context(), &request)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create git repository")
		http.Error(w, fmt.Sprintf("Failed to create repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(repository)
}

// getGitRepository retrieves repository information by ID
// @Summary Get git repository
// @Description Get information about a specific git repository
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} services.GitRepository
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getGitRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get git repository")
		http.Error(w, fmt.Sprintf("Repository not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repository)
}

// listGitRepositories lists all repositories, optionally filtered by owner
// @Summary List git repositories
// @Description List all git repositories, optionally filtered by owner and type
// @Tags git-repositories
// @Produce json
// @Param owner_id query string false "Filter by owner ID"
// @Param repo_type query string false "Filter by repository type"
// @Success 200 {array} services.GitRepository
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listGitRepositories(w http.ResponseWriter, r *http.Request) {
	ownerID := r.URL.Query().Get("owner_id")
	repoType := r.URL.Query().Get("repo_type")

	repositories, err := apiServer.gitRepositoryService.ListRepositories(r.Context(), ownerID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list git repositories")
		http.Error(w, fmt.Sprintf("Failed to list repositories: %s", err.Error()), http.StatusInternalServerError)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repositories)
}

// createSpecTaskRepository creates a repository for a SpecTask
// @Summary Create SpecTask repository
// @Description Create a git repository specifically for a SpecTask
// @Tags specs
// @Accept json
// @Produce json
// @Param request body CreateSpecTaskRepositoryRequest true "SpecTask repository creation request"
// @Success 201 {object} services.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/specs/repositories [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createSpecTaskRepository(w http.ResponseWriter, r *http.Request) {
	var request CreateSpecTaskRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if request.SpecTaskID == "" {
		http.Error(w, "SpecTask ID is required", http.StatusBadRequest)
		return
	}

	// Get SpecTask from store
	specTask, err := apiServer.Store.GetSpecTask(r.Context(), request.SpecTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to get SpecTask")
		http.Error(w, fmt.Sprintf("SpecTask not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	repository, err := apiServer.gitRepositoryService.CreateSpecTaskRepository(
		r.Context(),
		specTask,
		request.TemplateFiles,
	)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", request.SpecTaskID).Msg("Failed to create SpecTask repository")
		http.Error(w, fmt.Sprintf("Failed to create SpecTask repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(repository)
}

// createSampleRepository creates a sample/demo repository
// @Summary Create sample repository
// @Description Create a sample/demo git repository from available templates
// @Tags samples
// @Accept json
// @Produce json
// @Param request body CreateSampleRepositoryRequest true "Sample repository creation request"
// @Success 201 {object} services.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/samples/repositories [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createSampleRepository(w http.ResponseWriter, r *http.Request) {
	var request CreateSampleRepositoryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if request.Name == "" {
		http.Error(w, "Repository name is required", http.StatusBadRequest)
		return
	}
	if request.SampleType == "" {
		http.Error(w, "Sample type is required", http.StatusBadRequest)
		return
	}
	if request.OwnerID == "" {
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
		return
	}

	repository, err := apiServer.gitRepositoryService.CreateSampleRepository(
		r.Context(),
		request.Name,
		request.Description,
		request.OwnerID,
		request.SampleType,
		request.KoditIndexing,
	)
	if err != nil {
		log.Error().Err(err).Str("sample_type", request.SampleType).Msg("Failed to create sample repository")
		http.Error(w, fmt.Sprintf("Failed to create sample repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(repository)
}

// getGitRepositoryCloneCommand returns the git clone command for a repository
// @Summary Get clone command
// @Description Get the git clone command for a repository with authentication
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param target_dir query string false "Target directory for clone"
// @Success 200 {object} CloneCommandResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/clone-command [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getGitRepositoryCloneCommand(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	targetDir := r.URL.Query().Get("target_dir")

	// Verify repository exists
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Repository not found for clone command")
		http.Error(w, fmt.Sprintf("Repository not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	cloneCommand := apiServer.gitRepositoryService.GetCloneCommand(repoID, targetDir)

	response := CloneCommandResponse{
		RepositoryID: repoID,
		CloneURL:     repository.CloneURL,
		CloneCommand: cloneCommand,
		TargetDir:    targetDir,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// initializeSampleRepositories creates multiple sample repositories
// @Summary Initialize sample repositories
// @Description Create multiple sample repositories for development/testing
// @Tags samples
// @Accept json
// @Produce json
// @Param request body InitializeSampleRepositoriesRequest true "Initialize samples request"
// @Success 201 {object} InitializeSampleRepositoriesResponse
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/samples/initialize [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) initializeSampleRepositories(w http.ResponseWriter, r *http.Request) {
	var request InitializeSampleRepositoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if request.OwnerID == "" {
		http.Error(w, "Owner ID is required", http.StatusBadRequest)
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
			true, // Enable Kodit indexing by default for sample repos
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

// getSampleTypes returns available sample repository types
// @Summary Get sample repository types
// @Description Get list of available sample repository types and templates
// @Tags specs
// @Produce json
// @Success 200 {object} SampleTypesResponse
// @Router /api/v1/specs/sample-types [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSampleTypes(w http.ResponseWriter, r *http.Request) {
	sampleTypes := []SampleType{
		{
			ID:          "empty",
			Name:        "Empty Repository",
			Description: "An empty project repository ready for any technology stack",
			TechStack:   []string{"any", "blank-slate", "custom"},
		},
		{
			ID:          "nodejs-todo",
			Name:        "Node.js Todo App",
			Description: "A simple todo application built with Node.js and Express",
			TechStack:   []string{"javascript", "nodejs", "express", "mongodb"},
		},
		{
			ID:          "python-api",
			Name:        "Python API Service",
			Description: "A FastAPI microservice with PostgreSQL integration",
			TechStack:   []string{"python", "fastapi", "postgresql", "sqlalchemy"},
		},
		{
			ID:          "react-dashboard",
			Name:        "React Dashboard",
			Description: "A modern admin dashboard built with React and Material-UI",
			TechStack:   []string{"javascript", "react", "typescript", "material-ui"},
		},
		{
			ID:          "linkedin-outreach",
			Name:        "LinkedIn Outreach Campaign",
			Description: "Multi-session campaign to reach out to 100 prospects using LinkedIn skill",
			TechStack:   []string{"business", "linkedin", "crm", "automation"},
		},
		{
			ID:          "helix-blog-posts",
			Name:        "Helix Technical Blog Posts",
			Description: "Write 10 blog posts about Helix system by analyzing the actual codebase",
			TechStack:   []string{"documentation", "git", "markdown", "technical-writing"},
		},
	}

	response := SampleTypesResponse{
		SampleTypes: sampleTypes,
		Count:       len(sampleTypes),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Request/Response types for API documentation

// SampleType represents a sample repository type
type SampleType struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	TechStack   []string `json:"tech_stack"`
}

// SampleTypesResponse represents the response for sample types
type SampleTypesResponse struct {
	SampleTypes []SampleType `json:"sample_types"`
	Count       int          `json:"count"`
}

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
	Name           string `json:"name"`
	Description    string `json:"description"`
	OwnerID        string `json:"owner_id"`
	SampleType     string `json:"sample_type"`
	KoditIndexing  bool   `json:"kodit_indexing"`  // Enable Kodit code intelligence indexing
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
