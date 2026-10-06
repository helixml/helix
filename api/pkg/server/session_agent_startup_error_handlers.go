package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const maxAgentStartupErrorLength = 4096

type AgentStartupErrorRequest struct {
	Error string `json:"error"`
}

type AgentStartupErrorResponse struct {
	Status        string `json:"status"`
	InteractionID string `json:"interaction_id,omitempty"`
	Transitioned  bool   `json:"transitioned"`
}

// reportAgentStartupError godoc
// @Summary Report a fatal in-container agent configuration error
// @Description Called by the settings-sync daemon when the session's Zed configuration is rejected. Atomically fails the latest waiting interaction so the task does not remain on an infinite spinner.
// @Tags Sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param request body AgentStartupErrorRequest true "Fatal startup error"
// @Success 200 {object} AgentStartupErrorResponse
// @Router /api/v1/sessions/{id}/agent-startup-error [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) reportAgentStartupError(_ http.ResponseWriter, req *http.Request) (*AgentStartupErrorResponse, *system.HTTPError) {
	sessionID := mux.Vars(req)["id"]
	if sessionID == "" {
		return nil, system.NewHTTPError400("missing session id")
	}
	user := getRequestUser(req)
	if user == nil {
		return nil, system.NewHTTPError401("unauthenticated")
	}

	var body AgentStartupErrorRequest
	if err := json.NewDecoder(io.LimitReader(req.Body, maxAgentStartupErrorLength+1024)).Decode(&body); err != nil {
		return nil, system.NewHTTPError400(fmt.Sprintf("invalid request body: %v", err))
	}
	body.Error = strings.TrimSpace(body.Error)
	if body.Error == "" {
		return nil, system.NewHTTPError400("error is required")
	}
	if len(body.Error) > maxAgentStartupErrorLength {
		return nil, system.NewHTTPError400("error is too long")
	}

	ctx := req.Context()
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError404(fmt.Sprintf("session %s not found", sessionID))
	}
	if err := apiServer.authorizeUserToSession(ctx, user, session, types.ActionUpdate); err != nil {
		return nil, system.NewHTTPError403(err.Error())
	}

	interactions, _, err := apiServer.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    sessionID,
		GenerationID: session.GenerationID,
		PerPage:      1,
		Order:        "created DESC",
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to load latest interaction: %v", err))
	}
	response := &AgentStartupErrorResponse{Status: "ok"}
	if len(interactions) == 0 || interactions[0].State != types.InteractionStateWaiting {
		return response, nil
	}

	interaction := interactions[0]
	errorMessage := "Agent startup failed: " + body.Error
	transitioned, err := apiServer.Store.MarkInteractionErrorIfWaiting(
		ctx, interaction.ID, interaction.GenerationID, errorMessage,
	)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to mark interaction as errored: %v", err))
	}
	response.InteractionID = interaction.ID
	response.Transitioned = transitioned
	if !transitioned {
		return response, nil
	}

	now := time.Now()
	interaction.State = types.InteractionStateError
	interaction.Error = errorMessage
	interaction.Updated = now
	interaction.Completed = now
	if interaction.PromptID != "" {
		if err := apiServer.Store.MarkPromptAsCrashed(ctx, interaction.PromptID, errorMessage); err != nil {
			log.Warn().Err(err).
				Str("session_id", sessionID).
				Str("interaction_id", interaction.ID).
				Msg("agent startup error: failed to mark queued prompt as crashed")
		}
	}
	if apiServer.pubsub != nil {
		if err := apiServer.publishInteractionUpdateToFrontend(sessionID, session.Owner, interaction); err != nil {
			log.Warn().Err(err).
				Str("session_id", sessionID).
				Str("interaction_id", interaction.ID).
				Msg("agent startup error: failed to publish interaction update")
		}
	}

	return response, nil
}
