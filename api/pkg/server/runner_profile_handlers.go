package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/runner/profile"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// runnerProfileService returns a profile service backed by the API server's
// store. Stateless — fine to construct per-request.
func (apiServer *HelixAPIServer) runnerProfileService() *profile.Service {
	return profile.New(apiServer.Store)
}

// runnerProfileSaveRequest is the body of POST/PUT /api/v1/runner-profiles.
// Mirrors profile.SaveInput verbatim.
type runnerProfileSaveRequest struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	ComposeYAML   string          `json:"compose_yaml"`
	Vendor        types.GPUVendor `json:"vendor,omitempty"`
	Architectures []string        `json:"architectures,omitempty"`
	ModelMatch    string          `json:"model_match,omitempty"`
	MinVRAMBytes  int64           `json:"min_vram_bytes,omitempty"`
}

// listRunnerProfiles godoc
// @Summary List runner profiles
// @Description Return all compose-based runner profiles, ordered by name.
// @Tags    runner_profiles
// @Success 200 {array} types.RunnerProfile
// @Router /api/v1/runner-profiles [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listRunnerProfiles(rw http.ResponseWriter, r *http.Request) {
	profiles, err := apiServer.runnerProfileService().List(r.Context())
	if err != nil {
		log.Err(err).Msg("list runner profiles")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if profiles == nil {
		profiles = []*types.RunnerProfile{}
	}
	writeResponse(rw, profiles, http.StatusOK)
}

// getRunnerProfile godoc
// @Summary Get a runner profile by ID
// @Tags    runner_profiles
// @Param   id path string true "Profile ID"
// @Success 200 {object} types.RunnerProfile
// @Failure 404 {string} string "not found"
// @Router /api/v1/runner-profiles/{id} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	p, err := apiServer.runnerProfileService().Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "profile not found", http.StatusNotFound)
			return
		}
		log.Err(err).Str("id", id).Msg("get runner profile")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(rw, p, http.StatusOK)
}

// createRunnerProfile godoc
// @Summary Create a runner profile
// @Tags    runner_profiles
// @Param   body body runnerProfileSaveRequest true "Profile fields"
// @Success 201 {object} types.RunnerProfile
// @Failure 400 {string} string "invalid request"
// @Router /api/v1/runner-profiles [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	var body runnerProfileSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	p, err := apiServer.runnerProfileService().Create(r.Context(), profile.SaveInput{
		Name:          body.Name,
		Description:   body.Description,
		ComposeYAML:   body.ComposeYAML,
		Vendor:        body.Vendor,
		Architectures: body.Architectures,
		ModelMatch:    body.ModelMatch,
		MinVRAMBytes:  body.MinVRAMBytes,
	})
	if err != nil {
		// Validation errors and parse errors are caller-fixable; surface as
		// 400. Genuine infrastructure failures bubble through as 500.
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(rw, p, http.StatusCreated)
}

// updateRunnerProfile godoc
// @Summary Update a runner profile (full replace)
// @Tags    runner_profiles
// @Param   id   path string                  true "Profile ID"
// @Param   body body runnerProfileSaveRequest true "Profile fields"
// @Success 200  {object} types.RunnerProfile
// @Failure 404  {string} string "not found"
// @Router /api/v1/runner-profiles/{id} [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var body runnerProfileSaveRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	p, err := apiServer.runnerProfileService().Update(r.Context(), profile.SaveInput{
		ID:            id,
		Name:          body.Name,
		Description:   body.Description,
		ComposeYAML:   body.ComposeYAML,
		Vendor:        body.Vendor,
		Architectures: body.Architectures,
		ModelMatch:    body.ModelMatch,
		MinVRAMBytes:  body.MinVRAMBytes,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "profile not found", http.StatusNotFound)
			return
		}
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	writeResponse(rw, p, http.StatusOK)
}

// deleteRunnerProfile godoc
// @Summary Delete a runner profile
// @Tags    runner_profiles
// @Param   id path string true "Profile ID"
// @Success 204 {string} string "no content"
// @Failure 404 {string} string "not found"
// @Router /api/v1/runner-profiles/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) deleteRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	err := apiServer.runnerProfileService().Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "profile not found", http.StatusNotFound)
			return
		}
		log.Err(err).Str("id", id).Msg("delete runner profile")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rw.WriteHeader(http.StatusNoContent)
}
