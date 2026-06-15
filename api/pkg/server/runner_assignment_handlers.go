package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/inferencerouter"
	"github.com/helixml/helix/api/pkg/runner/profile"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// refreshInferenceRouterFromHeartbeat is called after a sandbox heartbeat
// is processed. It pushes the latest inference-relevant state into the
// in-memory inferencerouter so request routing reflects the new
// hardware / profile / health view immediately.
//
// If the heartbeat carries no GPU information (a pure-agent sandbox),
// the router still records the runner so it appears in admin views, but
// it won't be picked for any inference request because RouteableModels()
// requires a non-nil ActiveProfile + Status="running".
func (apiServer *HelixAPIServer) refreshInferenceRouterFromHeartbeat(ctx context.Context, sandboxID string, hb *types.SandboxHeartbeatRequest) {
	if apiServer.inferenceRouter == nil {
		return
	}
	// Look up the assignment to find the active profile (if any).
	var activeProfile *types.RunnerProfile
	if asn, err := apiServer.Store.GetRunnerAssignment(ctx, sandboxID); err == nil {
		if p, perr := apiServer.runnerProfileService().Get(ctx, asn.ProfileID); perr == nil {
			activeProfile = p
		}
	}
	// No network address is stored: inference is dispatched to the sandbox over
	// its outbound RevDial tunnel (keyed by sandbox id), so the API never needs
	// to resolve or route to a sandbox host. This works for sandboxes on any
	// network, including behind NAT. See dispatchToSandbox in helix_openai_server.go.
	apiServer.inferenceRouter.SetRunnerState(&inferencerouter.RunnerState{
		ID:            sandboxID,
		Status:        hb.ProfileStatus,
		ActiveProfile: activeProfile,
		GPUs:          hb.GPUs,
		LastSeen:      time.Now(),
	})
}

// runnerInfoFromAPI returns the GPU inventory for a given runner ID, sourced
// from the inference router's in-memory state (populated by sandbox status
// heartbeats). Returns empty slice if the runner isn't connected — the
// compatibility check then fails on the count constraint, which is the
// right behaviour (don't guess at hardware).
func (apiServer *HelixAPIServer) runnerInfoFromAPI(runnerID string) []profile.RunnerGPUInfo {
	if apiServer.inferenceRouter == nil {
		return nil
	}
	state := apiServer.inferenceRouter.GetRunner(runnerID)
	if state == nil {
		return nil
	}
	out := make([]profile.RunnerGPUInfo, 0, len(state.GPUs))
	for _, g := range state.GPUs {
		out = append(out, profile.RunnerGPUInfo{
			Index:        g.Index,
			Vendor:       g.Vendor,
			Architecture: g.Architecture,
			ModelName:    g.ModelName,
			TotalVRAM:    g.TotalMemory,
		})
	}
	return out
}

type runnerProfileAssignRequest struct {
	ProfileID string `json:"profile_id"`
}

// listCompatibleRunnerProfiles godoc
// @Summary List runner profiles compatible with the given runner
// @Description Returns the subset of profiles whose GPU compatibility specification
// @Description is satisfied by the runner's reported hardware inventory.
// @Tags    runner_profiles
// @Param   runner_id path string true "Runner ID"
// @Success 200 {array} types.RunnerProfile
// @Router /api/v1/runners/{runner_id}/compatible-profiles [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listCompatibleRunnerProfiles(rw http.ResponseWriter, r *http.Request) {
	runnerID := mux.Vars(r)["runner_id"]
	all, err := apiServer.runnerProfileService().List(r.Context())
	if err != nil {
		log.Err(err).Str("runner_id", runnerID).Msg("list profiles for compatibility")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	gpus := apiServer.runnerInfoFromAPI(runnerID)
	out := profile.FilterCompatible(all, gpus)
	if out == nil {
		out = []*types.RunnerProfile{}
	}
	writeResponse(rw, out, http.StatusOK)
}

// assignRunnerProfile godoc
// @Summary Assign a profile to a runner
// @Description Validates GPU compatibility, persists the assignment, and notifies
// @Description the runner over NATS to apply the profile.
// @Tags    runner_profiles
// @Param   runner_id path string                       true "Runner ID"
// @Param   body      body runnerProfileAssignRequest    true "Profile ID"
// @Success 200 {object} types.RunnerAssignment
// @Failure 422 {string} string "incompatible: <constraint> — <detail>"
// @Router /api/v1/runners/{runner_id}/assign-profile [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) assignRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	runnerID := mux.Vars(r)["runner_id"]
	var body runnerProfileAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(rw, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.ProfileID == "" {
		http.Error(rw, "profile_id is required", http.StatusBadRequest)
		return
	}

	p, err := apiServer.runnerProfileService().Get(r.Context(), body.ProfileID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "profile not found", http.StatusNotFound)
			return
		}
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Compatibility check (defence in depth — the dropdown also filters).
	gpus := apiServer.runnerInfoFromAPI(runnerID)
	if err := profile.Compatibility(p.GPURequirement, gpus); err != nil {
		// Use 422 (Unprocessable Entity) for compatibility failures so they
		// distinguish from a malformed request (400) and from a missing
		// profile (404).
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	// Persist the assignment.
	a := &types.RunnerAssignment{
		RunnerID:   runnerID,
		ProfileID:  body.ProfileID,
		AssignedBy: getRequestUserID(r),
	}
	saved, err := apiServer.Store.SetRunnerAssignment(r.Context(), a)
	if err != nil {
		log.Err(err).Msg("persist runner assignment")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO(sandbox-absorbs-runner): once compose-manager is in the
	// sandbox, send a NATS message on runner.{id}.cmd telling it to
	// apply this profile. For now the API just persists; the runner
	// fetches its own assignment on connect.

	writeResponse(rw, saved, http.StatusOK)
}

// clearRunnerProfile godoc
// @Summary Clear a runner's profile assignment
// @Description Deletes the runner-to-profile assignment and tells the runner
// @Description to tear down any active compose stack. Idempotent.
// @Tags    runner_profiles
// @Param   runner_id path string true "Runner ID"
// @Success 204 {string} string "no content"
// @Router /api/v1/runners/{runner_id}/clear-profile [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) clearRunnerProfile(rw http.ResponseWriter, r *http.Request) {
	runnerID := mux.Vars(r)["runner_id"]
	if err := apiServer.Store.DeleteRunnerAssignment(r.Context(), runnerID); err != nil {
		log.Err(err).Str("runner_id", runnerID).Msg("clear runner assignment")
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// TODO(sandbox-absorbs-runner): NATS cmd to tear down the compose stack.
	rw.WriteHeader(http.StatusNoContent)
}

// getRunnerAssignment godoc
// @Summary Get a runner's current profile assignment
// @Tags    runner_profiles
// @Param   runner_id path string true "Runner ID"
// @Success 200 {object} types.RunnerAssignment
// @Failure 404 {string} string "no assignment"
// @Router /api/v1/runners/{runner_id}/assignment [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRunnerAssignment(rw http.ResponseWriter, r *http.Request) {
	runnerID := mux.Vars(r)["runner_id"]
	a, err := apiServer.Store.GetRunnerAssignment(r.Context(), runnerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.Error(rw, "no assignment for runner", http.StatusNotFound)
			return
		}
		http.Error(rw, "internal error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeResponse(rw, a, http.StatusOK)
}

// getRequestUserID extracts the authenticated user ID from the request
// context, returning empty string if absent. Used for the assigned_by
// audit trail.
func getRequestUserID(r *http.Request) string {
	if u := getRequestUser(r); u != nil {
		return u.ID
	}
	return ""
}
