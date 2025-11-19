package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// createGitRepository creates a new git repository
// @Summary Create git repository
// @Description Create a new git repository on the server
// @Tags git-repositories
// @Accept json
// @Produce json
// @Param repository body types.GitRepositoryCreateRequest true "Repository creation request"
// @Success 201 {object} types.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories [post]
// @Security BearerAuth
func (s *HelixAPIServer) createGitRepository(w http.ResponseWriter, r *http.Request) {
	var request types.GitRepositoryCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)

	if request.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, request.OrganizationID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
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
		request.RepoType = types.GitRepositoryTypeCode
	}

	// Create repository
	repository, err := s.gitRepositoryService.CreateRepository(r.Context(), &request)
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
// @Success 200 {object} types.GitRepository
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getGitRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)

	repository, err := s.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get git repository")
		http.Error(w, fmt.Sprintf("Repository not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	if repository.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, repository.OrganizationID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
	}

	if repository.OwnerID != user.ID {
		writeErrResponse(w, system.NewHTTPError403("unauthorized"), http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repository)
}

// updateGitRepository updates an existing git repository
// @Summary Update git repository
// @Description Update an existing git repository's metadata
// @Tags git-repositories
// @Accept json
// @Produce json
// @Param id path string true "Repository ID"
// @Param repository body types.GitRepositoryUpdateRequest true "Repository update request"
// @Success 200 {object} types.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id} [put]
// @Security BearerAuth
func (s *HelixAPIServer) updateGitRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)

	var request types.GitRepositoryUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request format: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Get existing one
	existing, err := s.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get existing git repository")
		http.Error(w, fmt.Sprintf("Failed to get existing repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if existing.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, existing.OrganizationID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
	}

	if existing.OwnerID != user.ID {
		writeErrResponse(w, system.NewHTTPError403("unauthorized"), http.StatusForbidden)
		return
	}

	repository, err := s.gitRepositoryService.UpdateRepository(r.Context(), repoID, &request)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to update git repository")
		http.Error(w, fmt.Sprintf("Failed to update repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repository)
}

// deleteGitRepository deletes a git repository
// @Summary Delete git repository
// @Description Delete a git repository and its metadata
// @Tags git-repositories
// @Param id path string true "Repository ID"
// @Success 204 "No Content"
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteGitRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)

	// Get existing one
	existing, err := s.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get existing git repository")
		http.Error(w, fmt.Sprintf("Failed to get existing repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if existing.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, existing.OrganizationID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
	}

	if existing.OwnerID != user.ID {
		writeErrResponse(w, system.NewHTTPError403("unauthorized"), http.StatusForbidden)
		return
	}

	err = s.gitRepositoryService.DeleteRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to delete git repository")
		http.Error(w, fmt.Sprintf("Failed to delete repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// listGitRepositories lists all repositories, optionally filtered by owner
// @Summary List git repositories
// @Description List all git repositories, optionally filtered by owner and type
// @Tags git-repositories
// @Produce json
// @Param owner_id query string false "Filter by owner ID"
// @Param repo_type query string false "Filter by repository type"
// @Param organization_id query string false "Filter by organization ID"
// @Success 200 {array} types.GitRepository
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories [get]
// @Security BearerAuth
func (s *HelixAPIServer) listGitRepositories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ownerID := r.URL.Query().Get("owner_id")
	repoType := r.URL.Query().Get("repo_type")
	orgID := r.URL.Query().Get("organization_id")

	user := getRequestUser(r)

	if orgID != "" {
		_, err := s.authorizeOrgMember(ctx, user, orgID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
	}

	repositories, err := s.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{
		OwnerID:        ownerID,
		OrganizationID: orgID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to list git repositories")
		http.Error(w, fmt.Sprintf("Failed to list repositories: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Filter by repo type if specified
	if repoType != "" {
		filtered := make([]*types.GitRepository, 0)
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

// createSampleRepository creates a sample/demo repository
// @Summary Create sample repository
// @Description Create a sample/demo git repository from available templates
// @Tags samples
// @Accept json
// @Produce json
// @Param request body types.CreateSampleRepositoryRequest true "Sample repository creation request"
// @Success 201 {object} types.GitRepository
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/samples/repositories [post]
// @Security BearerAuth
func (s *HelixAPIServer) createSampleRepository(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)

	var request types.CreateSampleRepositoryRequest
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

	if request.OrganizationID != "" {
		_, err := s.authorizeOrgMember(r.Context(), user, request.OrganizationID)
		if err != nil {
			writeErrResponse(w, err, http.StatusForbidden)
			return
		}
	}

	repository, err := s.gitRepositoryService.CreateSampleRepository(
		r.Context(),
		&request,
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

	var createdRepositories []*types.GitRepository
	var errors []string

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
			&types.CreateSampleRepositoryRequest{
				Name:           sample.Name,
				Description:    sample.Description,
				OwnerID:        request.OwnerID,
				OrganizationID: request.OrganizationID,
				SampleType:     sample.SampleType,
			},
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
		{
			ID:          "jupyter-financial-analysis",
			Name:        "Jupyter Financial Analysis",
			Description: "Financial data analysis using Jupyter notebooks with S&P 500 data and trading signals",
			TechStack:   []string{"python", "jupyter", "pandas", "numpy", "finance", "data-analysis"},
		},
		{
			ID:          "data-platform-api-migration",
			Name:        "Data Platform API Migration Suite",
			Description: "Migrate data pipeline APIs from legacy infrastructure to modern data platform",
			TechStack:   []string{"python", "fastapi", "airflow", "pandas", "sqlalchemy", "pydantic"},
		},
		{
			ID:          "portfolio-management-dotnet",
			Name:        "Portfolio Management System (.NET)",
			Description: "Production-grade portfolio management and trade execution system",
			TechStack:   []string{"csharp", "dotnet", "entity-framework", "messaging", "xunit", "signalr"},
		},
		{
			ID:          "research-analysis-toolkit",
			Name:        "Research Analysis Toolkit (PyForest)",
			Description: "Financial research notebooks for backtesting and portfolio optimization",
			TechStack:   []string{"python", "jupyter", "pandas", "numpy", "pyforest", "backtesting"},
		},
		{
			ID:          "data-validation-toolkit",
			Name:        "Data Validation Toolkit",
			Description: "Compare data structures and validate migrations with quality reports",
			TechStack:   []string{"python", "jupyter", "pandas", "great-expectations", "data-quality"},
		},
		{
			ID:          "angular-analytics-dashboard",
			Name:        "Multi-Tenant Analytics Dashboard",
			Description: "Multi-tenant analytics dashboard with RBAC and real-time updates",
			TechStack:   []string{"angular", "typescript", "rxjs", "ngrx", "primeng", "chartjs"},
		},
		{
			ID:          "angular-version-migration",
			Name:        "Angular Version Migration (15 → 18)",
			Description: "Migrate Angular 15 app to Angular 18 with standalone components",
			TechStack:   []string{"angular", "typescript", "migration", "refactoring"},
		},
		{
			ID:          "cobol-modernization",
			Name:        "Legacy COBOL Modernization",
			Description: "Analyze COBOL code, write specs, and implement in modern language",
			TechStack:   []string{"cobol", "legacy", "python", "modernization", "spec-writing"},
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

// CloneCommandResponse represents the response for clone command request
type CloneCommandResponse struct {
	RepositoryID string `json:"repository_id"`
	CloneURL     string `json:"clone_url"`
	CloneCommand string `json:"clone_command"`
	TargetDir    string `json:"target_dir,omitempty"`
}

// InitializeSampleRepositoriesRequest represents a request to initialize sample repositories
type InitializeSampleRepositoriesRequest struct {
	OwnerID        string   `json:"owner_id"`
	OrganizationID string   `json:"organization_id"`
	SampleTypes    []string `json:"sample_types,omitempty"` // If empty, creates all samples
}

// InitializeSampleRepositoriesResponse represents the response for initializing sample repositories
type InitializeSampleRepositoriesResponse struct {
	CreatedRepositories []*types.GitRepository `json:"created_repositories"`
	CreatedCount        int                    `json:"created_count"`
	Errors              []string               `json:"errors,omitempty"`
	Success             bool                   `json:"success"`
}

// browseGitRepositoryTree browses files and directories at a path
// @Summary Browse repository tree
// @Description Get list of files and directories at a specific path in a repository
// @ID browseGitRepositoryTree
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param path query string false "Path to browse (default: root)"
// @Param branch query string false "Branch to browse (default: HEAD)"
// @Success 200 {object} types.GitRepositoryTreeResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/tree [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) browseGitRepositoryTree(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		path = "."
	}

	branch := r.URL.Query().Get("branch")

	entries, err := apiServer.gitRepositoryService.BrowseTree(r.Context(), repoID, path, branch)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Str("path", path).Msg("Failed to browse repository tree")
		http.Error(w, fmt.Sprintf("Failed to browse repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := &types.GitRepositoryTreeResponse{
		Path:    path,
		Entries: entries,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// listGitRepositoryBranches lists all branches in a repository
// @Summary List repository branches
// @Description Get list of all branches in a repository
// @ID listGitRepositoryBranches
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {array} string
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/branches [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listGitRepositoryBranches(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	branches, err := apiServer.gitRepositoryService.ListBranches(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to list repository branches")
		http.Error(w, fmt.Sprintf("Failed to list branches: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(branches)
}

// getGitRepositoryFile gets the contents of a file
// @Summary Get file contents
// @Description Get the contents of a file at a specific path in a repository
// @ID getGitRepositoryFile
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param path query string true "File path"
// @Param branch query string false "Branch name (defaults to HEAD if not specified)"
// @Success 200 {object} types.GitRepositoryFileResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/file [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getGitRepositoryFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "File path is required", http.StatusBadRequest)
		return
	}

	branch := r.URL.Query().Get("branch") // Optional branch parameter

	content, err := apiServer.gitRepositoryService.GetFileContents(r.Context(), repoID, path, branch)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Str("path", path).Str("branch", branch).Msg("Failed to get file contents")
		http.Error(w, fmt.Sprintf("Failed to get file contents: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := &types.GitRepositoryFileResponse{
		Path:    path,
		Content: content,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
