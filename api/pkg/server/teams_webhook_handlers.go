package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
)

// teamsWebhookHandler handles incoming webhook requests from Microsoft Teams Bot Framework
// This endpoint receives activities from the Bot Framework Connector Service
//
// @Summary Handle Teams webhook
// @Description Process incoming activities from Microsoft Teams Bot Framework
// @Tags Teams
// @Accept json
// @Produce json
// @Param appID path string true "Helix App ID"
// @Success 200 {string} string "OK"
// @Failure 404 {string} string "Bot not found"
// @Failure 503 {string} string "Bot not ready"
// @Router /api/v1/teams/webhook/{appID} [post]
func (s *HelixAPIServer) teamsWebhookHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appID := vars["appID"]

	log.Debug().
		Str("app_id", appID).
		Str("method", r.Method).
		Str("content_type", r.Header.Get("Content-Type")).
		Msg("received Teams webhook request")

	// Get the bot from the trigger manager
	bot := s.trigger.GetTeamsBot(appID)
	if bot == nil {
		log.Error().Str("app_id", appID).Msg("Teams bot not found for app")
		http.Error(w, "Teams bot not found for this app", http.StatusNotFound)
		return
	}

	// Delegate to the bot's activity handler
	bot.HandleActivity(w, r)
}
