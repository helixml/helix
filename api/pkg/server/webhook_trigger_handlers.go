package server

import (
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/store"

	"github.com/rs/zerolog/log"
)

// webhookTriggerHandler - webhook trigger handler for Azure DevOps, GitHub, etc.
// Body can be dynamic and configuration will be looked up through the trigger configuration ID.
func (s *HelixAPIServer) webhookTriggerHandler(w http.ResponseWriter, r *http.Request) {
	id := getID(r)

	triggerConfig, err := s.Store.GetTriggerConfiguration(r.Context(), &store.GetTriggerConfigurationQuery{
		ID: id,
	})
	if err != nil {
		writeErrResponse(w, err, http.StatusNotFound)
		return
	}

	defer r.Body.Close()

	bts, err := io.ReadAll(r.Body)
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("trigger_config_id", triggerConfig.ID).
		Str("trigger_config_app_id", triggerConfig.AppID).
		Msgf("Received webhook trigger for trigger configuration %s", id)

	err = s.trigger.ProcessWebhook(r.Context(), triggerConfig, bts)
	if err != nil {
		writeErrResponse(w, err, http.StatusInternalServerError)
		return
	}

	writeResponse(w, nil, http.StatusOK)
}
