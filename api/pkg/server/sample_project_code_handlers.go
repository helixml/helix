package server

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// getSampleProjectCode godoc
// @Summary Get sample project starter code
// @Description Get the starter code and file structure for a sample project
// @Tags    sample-projects
// @Produce json
// @Param   projectId path string true "Project ID"
// @Success 200 {object} services.SampleProjectCode
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/sample-projects/{projectId}/code [get]
func (s *HelixAPIServer) getSampleProjectCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["projectId"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	projectCode, err := s.sampleProjectCodeService.GetProjectCode(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to get sample project code")
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectCode)
}

// getSampleProjectCodeArchive godoc
// @Summary Get sample project code as archive
// @Description Get all files for a sample project as a flat map (for container initialization)
// @Tags    sample-projects
// @Produce json
// @Param   projectId path string true "Project ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/sample-projects/{projectId}/archive [get]
func (s *HelixAPIServer) getSampleProjectCodeArchive(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["projectId"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	archive, err := s.sampleProjectCodeService.GetProjectCodeArchive(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to get sample project code archive")
		http.Error(w, "project not found", http.StatusNotFound)
		return
	}

	log.Info().
		Str("project_id", projectID).
		Int("file_count", len(archive)).
		Msg("Serving sample project code archive")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(archive)
}

// listSampleProjects godoc
// @Summary List available sample projects
// @Description Get a list of all available sample projects with their metadata
// @Tags    sample-projects
// @Produce json
// @Success 200 {array} services.SampleProjectCode
// @Failure 500 {object} types.APIError
// @Router  /api/v1/sample-projects [get]
func (s *HelixAPIServer) listSampleProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projects := s.sampleProjectCodeService.ListAvailableProjects(ctx)

	log.Info().
		Int("project_count", len(projects)).
		Msg("Listing available sample projects")

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projects)
}
