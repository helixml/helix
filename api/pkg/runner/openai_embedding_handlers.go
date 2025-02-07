package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func (s *HelixRunnerAPIServer) createEmbedding(rw http.ResponseWriter, r *http.Request) {
	if s.runnerOptions == nil {
		http.Error(rw, "runner server not initialized", http.StatusInternalServerError)
		return
	}
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotUUID.String()).Msg("create embedding")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var embeddingRequest openai.EmbeddingRequest
	err = json.Unmarshal(body, &embeddingRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots[slotUUID]
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotUUID.String()), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	if embeddingRequest.Model == "" {
		embeddingRequest.Model = openai.EmbeddingModel(slot.Model)
	}
	if embeddingRequest.Model != openai.EmbeddingModel(slot.Model) {
		http.Error(rw, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
		return
	}

	ownerID := s.runnerOptions.ID
	user := getRequestUser(r)
	if user != nil {
		ownerID = user.ID
		if user.TokenType == types.TokenTypeRunner {
			ownerID = oai.RunnerID
		}
	}

	ctx := oai.SetContextValues(r.Context(), &oai.ContextValues{
		OwnerID:         ownerID,
		SessionID:       "n/a",
		InteractionID:   "n/a",
		OriginalRequest: body,
	})

	// Create the openai client
	openAIClient, err := CreateOpenaiClient(ctx, fmt.Sprintf("%s/v1", slot.Runtime.URL()))
	if err != nil {
		log.Error().Err(err).Msg("error creating openai client")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Trace().Str("model", slot.Model).Msg("creating chat completion")
	resp, err := openAIClient.CreateEmbeddings(ctx, embeddingRequest)
	if err != nil {
		log.Error().Err(err).Msg("error creating chat completion")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	rw.Header().Set("Content-Type", "application/json")

	if r.URL.Query().Get("pretty") == "true" {
		// Pretty print the response with indentation
		bts, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			log.Error().Err(err).Msg("error marshalling response")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		_, _ = rw.Write(bts)
		return
	}

	err = json.NewEncoder(rw).Encode(resp)
	if err != nil {
		log.Error().Err(err).Msg("error writing response")
	}
}
