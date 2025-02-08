package runner

import (
	"encoding/json"
	"errors"
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

func (s *HelixRunnerAPIServer) createChatCompletion(rw http.ResponseWriter, r *http.Request) {
	slotID := mux.Vars(r)["slot_id"]
	slotUUID, err := uuid.Parse(slotID)
	if err != nil {
		http.Error(rw, fmt.Sprintf("invalid slot id: %s", slotID), http.StatusBadRequest)
		return
	}
	log.Trace().Str("slot_id", slotUUID.String()).Msg("create chat completion")

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var chatCompletionRequest openai.ChatCompletionRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	slot, ok := s.slots.Load(slotUUID)
	if !ok {
		http.Error(rw, fmt.Sprintf("slot %s not found", slotUUID.String()), http.StatusNotFound)
		return
	}

	addCorsHeaders(rw)
	if r.Method == http.MethodOptions {
		return
	}

	// If this is a lora model we have some slightly different validation logic
	// TODO(Phil): This is a bit gross. Is it simpler to just have a separate path for finetuned models?
	_, sessionID, _, err := parseHelixLoraModelName(slot.Model)
	if err == nil {
		// Then it's a lora model
		chatCompletionRequest.Model = buildLocalLoraDir(sessionID)
	} else {
		// A normal ollama-like model
		if chatCompletionRequest.Model == "" {
			chatCompletionRequest.Model = slot.Model
		}

		if chatCompletionRequest.Model != slot.Model {
			http.Error(rw, fmt.Sprintf("model mismatch, expecting %s, got %s", slot.Model, chatCompletionRequest.Model), http.StatusBadRequest)
			return
		}
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

	// Non-streaming request returns the response immediately
	if !chatCompletionRequest.Stream {
		log.Trace().Str("model", slot.Model).Msg("creating chat completion")
		resp, err := openAIClient.CreateChatCompletion(ctx, chatCompletionRequest)
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
		return
	}

	// Streaming request, receive and write the stream in chunks
	stream, err := openAIClient.CreateChatCompletionStream(ctx, chatCompletionRequest)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("Connection", "keep-alive")

	// Write the stream into the response
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write the response to the client
		bts, err := json.Marshal(response)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := writeChunk(rw, bts); err != nil {
			log.Error().Msgf("failed to write completion chunk: %v", err)
		}
	}
}

// ---TODO(phil): The following is all copied from the main server ---

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

func addCorsHeaders(w http.ResponseWriter) {
	// Set headers to allow requests from any origin
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

type contextKey string

const userKey contextKey = "user"

func getRequestUser(req *http.Request) *types.User {
	ctxValue := req.Context().Value(userKey)
	if ctxValue == nil {
		return nil
	}
	user := ctxValue.(types.User)
	return &user
}

func writeChunk(w io.Writer, chunk []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", string(chunk))
	if err != nil {
		return fmt.Errorf("error writing chunk '%s': %w", string(chunk), err)
	}

	// Flush the ResponseWriter buffer to send the chunk immediately
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	} else {
		log.Warn().Msg("ResponseWriter does not support Flusher interface")
	}

	return nil
}
