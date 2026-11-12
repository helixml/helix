package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// runnerIDPattern is the shape of a runner (sandbox host) id: the value a
// host runs with as SANDBOX_INSTANCE_ID.
var runnerIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// registerRunnerTokenRoutes wires the admin API for per-runner tokens.
func (apiServer *HelixAPIServer) registerRunnerTokenRoutes(adminRouter *mux.Router) {
	adminRouter.HandleFunc("/admin/runner-tokens", apiServer.createRunnerToken).Methods(http.MethodPost)
	adminRouter.HandleFunc("/admin/runner-tokens", apiServer.listRunnerTokens).Methods(http.MethodGet)
	adminRouter.HandleFunc("/admin/runner-tokens/{id}", apiServer.revokeRunnerToken).Methods(http.MethodDelete)
}

// createRunnerToken godoc
// @Summary Mint a per-runner token
// @Description Mint a token bound to one runner id. The token is returned once and only its hash is stored.
// @Tags    runner_tokens
// @Param   body body types.CreateRunnerTokenRequest true "Runner id and an optional name"
// @Success 201 {object} types.CreateRunnerTokenResponse
// @Failure 400 {string} string "invalid request"
// @Router /api/v1/admin/runner-tokens [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) createRunnerToken(rw http.ResponseWriter, r *http.Request) {
	var req types.CreateRunnerTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "invalid request body", http.StatusBadRequest)
		return
	}
	req.RunnerID = strings.TrimSpace(req.RunnerID)
	if !runnerIDPattern.MatchString(req.RunnerID) {
		http.Error(rw, "runner_id must be 1-128 characters of letters, digits, '.', '_' or '-'", http.StatusBadRequest)
		return
	}
	// Runner tokens never register desktop tunnels (those are bound to a
	// session's key), so a token for a desktop- id could do nothing.
	if strings.HasPrefix(req.RunnerID, "desktop-") {
		http.Error(rw, "runner_id must be a sandbox host id, not a desktop id", http.StatusBadRequest)
		return
	}
	// The runner id is the bare host id (SANDBOX_INSTANCE_ID), the key the
	// sandbox registry and runner assignments use; the token covers the
	// host's hydra-/sandbox- tunnels on its own. A prefixed id is refused
	// rather than rewritten, so what is stored is exactly what was asked for.
	if bare := types.CanonicalRunnerID(req.RunnerID); bare != req.RunnerID {
		http.Error(rw, fmt.Sprintf("runner_id must be the bare host id (%q), not a tunnel id", bare), http.StatusBadRequest)
		return
	}

	secret, err := system.GenerateRunnerToken()
	if err != nil {
		http.Error(rw, "failed to generate token", http.StatusInternalServerError)
		return
	}
	created, err := apiServer.Store.CreateRunnerToken(r.Context(), &types.RunnerToken{
		ID:          system.GenerateRunnerTokenID(),
		RunnerID:    req.RunnerID,
		Name:        strings.TrimSpace(req.Name),
		TokenHash:   types.HashRunnerToken(secret),
		TokenPrefix: types.RunnerTokenVisiblePrefix(secret),
		CreatedBy:   getRequestUserID(r),
	})
	if err != nil {
		log.Error().Err(err).Str("runner_id", req.RunnerID).Msg("failed to create runner token")
		http.Error(rw, "failed to create runner token", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("runner_token_id", created.ID).
		Str("runner_id", created.RunnerID).
		Str("created_by", created.CreatedBy).
		Msg("Runner token created")

	writeResponse(rw, &types.CreateRunnerTokenResponse{RunnerToken: *created, Token: secret}, http.StatusCreated)
}

// listRunnerTokens godoc
// @Summary List per-runner tokens
// @Description List per-runner tokens (never their secrets), newest first, revoked ones included.
// @Tags    runner_tokens
// @Param   runner_id query string false "Only this runner's tokens"
// @Success 200 {array} types.RunnerToken
// @Router /api/v1/admin/runner-tokens [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listRunnerTokens(rw http.ResponseWriter, r *http.Request) {
	tokens, err := apiServer.Store.ListRunnerTokens(r.Context(), r.URL.Query().Get("runner_id"))
	if err != nil {
		log.Error().Err(err).Msg("failed to list runner tokens")
		http.Error(rw, "failed to list runner tokens", http.StatusInternalServerError)
		return
	}
	if tokens == nil {
		tokens = []*types.RunnerToken{}
	}
	writeResponse(rw, tokens, http.StatusOK)
}

// revokeRunnerToken godoc
// @Summary Revoke a per-runner token
// @Description Revoke a per-runner token. Requests with it are refused (401) from then on; tunnels it already opened stay up until the host reconnects.
// @Tags    runner_tokens
// @Param   id path string true "Runner token id"
// @Success 200 {object} types.RunnerToken
// @Failure 404 {string} string "not found"
// @Router /api/v1/admin/runner-tokens/{id} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) revokeRunnerToken(rw http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	revoked, err := apiServer.Store.RevokeRunnerToken(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(rw, "runner token not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Error().Err(err).Str("runner_token_id", id).Msg("failed to revoke runner token")
		http.Error(rw, "failed to revoke runner token", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("runner_token_id", revoked.ID).
		Str("runner_id", revoked.RunnerID).
		Str("revoked_by", getRequestUserID(r)).
		Msg("Runner token revoked")

	writeResponse(rw, revoked, http.StatusOK)
}
