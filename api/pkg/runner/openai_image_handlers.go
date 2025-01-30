package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"
	openai "github.com/sashabaranov/go-openai"
)

func (s *HelixRunnerAPIServer) createImageGeneration(w http.ResponseWriter, r *http.Request) {
	slot_id := mux.Vars(r)["slot_id"]
	slot_uuid, err := uuid.Parse(slot_id)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid slot id: %s", slot_id), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slot_id).Msg("create image generation")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var chatCompletionRequest openai.ImageRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots[slot_uuid]
	if !ok {
		http.Error(w, fmt.Sprintf("slot %s not found", slot_id), http.StatusNotFound)
		return
	}

	addCorsHeaders(w)
	if r.Method == http.MethodOptions {
		return
	}

	if chatCompletionRequest.Model == "" {
		chatCompletionRequest.Model = slot.Model
	}
	if chatCompletionRequest.Model != slot.Model {
		http.Error(w, fmt.Sprintf("model mismatch, expecting %s", slot.Model), http.StatusBadRequest)
		return
	}

	// TODO: Implement image generation
}
