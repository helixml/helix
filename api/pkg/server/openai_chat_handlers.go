package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/controller"

	openai "github.com/lukemarsden/go-openai2"
	"github.com/rs/zerolog/log"
)

const (
	BYTE = 1 << (10 * iota)
	KILOBYTE
	MEGABYTE
)

// https://platform.openai.com/docs/api-reference/models
// GET https://app.tryhelix.ai/v1/models
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {

	// TODO: if configured to proxy through to LLM provider, return their models

	// Create a response with a list of available models
	models := []openai.Model{
		{
			ID:          "helix-3.5",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix 3.5",
			Description: "Llama3-8B, fast and good for everyday tasks",
		},
		{
			ID:          "helix-4",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix 4",
			Description: "Llama3 70B, smarter but a bit slower",
		},
		{
			ID:          "helix-mixtral",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix Mixtral",
			Description: "Mistral 8x7B MoE, we rely on this for some use cases",
		},
		{
			ID:          "helix-json",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix JSON",
			Description: "Nous-Hermes 2 Theta, for function calling & JSON output",
		},
		{
			ID:          "helix-small",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix Small",
			Description: "Phi-3 Mini 3.8B, fast and memory efficient",
		},
	}

	response := openai.ModelsList{
		Models: models,
	}

	// Set the content type header
	rw.Header().Set("Content-Type", "application/json")

	// Encode and write the response
	err := json.NewEncoder(rw).Encode(response)
	if err != nil {
		log.Err(err).Msg("error writing response")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://app.tryhelix.ai/v1/chat/completions
func (apiServer *HelixAPIServer) createChatCompletion(rw http.ResponseWriter, r *http.Request) {
	addCorsHeaders(rw)
	if r.Method == "OPTIONS" {
		return
	}

	user := getRequestUser(r)

	if !hasUser(user) {
		http.Error(rw, "unauthorized", http.StatusUnauthorized)
		return
	}

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

	options := &controller.ChatCompletionOptions{
		AppID:       r.URL.Query().Get("app_id"),
		AssistantID: r.URL.Query().Get("assistant_id"),
		RAGSourceID: r.URL.Query().Get("rag_source_id"),
	}

	// Non-streaming request returns the response immediately
	if !chatCompletionRequest.Stream {
		resp, err := apiServer.Controller.ChatCompletion(r.Context(), user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(rw).Encode(resp)
		if err != nil {
			log.Err(err).Msg("error writing response")
		}
		return
	}

	// Streaming request, receive and write the stream in chunks
	stream, err := apiServer.Controller.ChatCompletionStream(r.Context(), user, chatCompletionRequest, options)
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

		writeChunk(rw, bts)
	}

}
