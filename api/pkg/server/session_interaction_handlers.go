package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// listInteractions godoc
// @Summary List interactions for a session
// @Description List interactions for a session
// @Tags    interactions
// @Produce json
// @Param   id path string true "Session ID"
// @Success 200 {array} types.Interaction
// @Router /api/v1/sessions/{id}/interactions [get]
// @Security BearerAuth
func (s *HelixAPIServer) listInteractions(_ http.ResponseWriter, req *http.Request) ([]*types.Interaction, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	id := mux.Vars(req)["id"]

	session, err := s.Store.GetSession(ctx, id)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", id, err))
	}

	if !canSeeSession(user, session) {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	interactions, _, err := s.Store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID:    id,
		GenerationID: session.GenerationID,
	})
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get interactions for session %s, error: %s", id, err))
	}

	return interactions, nil
}

// getInteraction godoc
// @Summary Get an interaction by ID
// @Description Get an interaction by ID
// @Tags    interactions
// @Produce json
// @Param   id path string true "Session ID"
// @Param   interaction_id path string true "Interaction ID"
// @Success 200 {object} types.Interaction
// @Router /api/v1/sessions/{id}/interactions/{interaction_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getInteraction(_ http.ResponseWriter, req *http.Request) (*types.Interaction, *system.HTTPError) {
	ctx := req.Context()
	user := getRequestUser(req)
	sessionID := mux.Vars(req)["id"]
	interactionID := mux.Vars(req)["interaction_id"]

	// First load the session
	session, err := s.Store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get session %s, error: %s", sessionID, err))
	}

	if !canSeeSession(user, session) {
		return nil, system.NewHTTPError403("you are not allowed to access this session")
	}

	interaction, err := s.Store.GetInteraction(ctx, interactionID)
	if err != nil {
		return nil, system.NewHTTPError500(fmt.Sprintf("failed to get interaction %s, error: %s", interactionID, err))
	}

	if interaction.SessionID != sessionID {
		return nil, system.NewHTTPError403("you are not allowed to access this interaction")
	}

	return interaction, nil
}
