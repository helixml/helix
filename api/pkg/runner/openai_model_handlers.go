package runner

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/rs/zerolog/log"
)

func (s *HelixRunnerAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	slot_id := mux.Vars(r)["slot_id"]
	slot_uuid, err := uuid.Parse(slot_id)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slot_id), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slot_id).Msg("list models")

	slot, ok := s.slots[slot_uuid]
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slot_id), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	// Create the openai client
	openAIClient, err := CreateOpenaiClient(r.Context(), fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := openAIClient.ListModels(r.Context())
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
