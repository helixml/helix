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

// https://platform.openai.com/docs/api-reference/chat/create
// POST https://app.tryhelix.ai//v1/chat/completions
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

	if chatCompletionRequest.Stream {
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

		return
	}

	// Non-streaming request
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
}
