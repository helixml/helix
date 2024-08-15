package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/model"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/types"

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

	// If configured to proxy through to LLM provider, return their models
	// Check if we're configured to proxy through to an LLM provider
	if apiServer.Cfg.Inference.Provider != config.ProviderHelix {
		// Determine the base URL based on the provider
		var baseURL string
		var apiKey string
		switch apiServer.Cfg.Inference.Provider {
		case config.ProviderOpenAI:
			baseURL = apiServer.Cfg.Providers.OpenAI.BaseURL
			apiKey = apiServer.Cfg.Providers.OpenAI.APIKey
		case config.ProviderTogetherAI:
			baseURL = apiServer.Cfg.Providers.TogetherAI.BaseURL
			apiKey = apiServer.Cfg.Providers.TogetherAI.APIKey
		default:
			log.Error().Msgf("Unsupported inference provider: %s", apiServer.Cfg.Inference.Provider)
			http.Error(rw, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Create a new request to the provider's models endpoint
		req, err := http.NewRequest("GET", baseURL+"/models", nil)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create request to provider's models endpoint")
			http.Error(rw, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set the Authorization header
		req.Header.Set("Authorization", "Bearer "+apiKey)

		// Send the request using http.DefaultClient
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send request to provider's models endpoint")
			http.Error(rw, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer resp.Body.Close()

		// Read the response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read response from provider's models endpoint")
			http.Error(rw, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set the content type header
		rw.Header().Set("Content-Type", "application/json")

		// Write the response body directly to the ResponseWriter
		_, err = rw.Write(body)
		if err != nil {
			log.Error().Err(err).Msg("Failed to write response")
		}
		return
	}

	// Create a response with a list of available models
	models := []model.OpenAIModel{
		// helix branded models, Hide: false (the default) so they show up in the UI
		{
			ID:          "helix-3.5",
			Object:      "model",
			OwnedBy:     "helix",
			Name:        "Helix 3.5",
			Description: "Llama3 8B, fast and good for everyday tasks",
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
		// ollama spellings
		// TODO: make these dynamic by having the runners report which models
		// they were configured with, and union them
		{
			ID:      "llama3:instruct",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "mixtral:instruct",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "codellama:70b-instruct-q2_K",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "adrienbrault/nous-hermes2theta-llama3-8b:q8_0",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "phi3:instruct",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "llama3:8b-instruct-fp16",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "llama3:8b-instruct-q6_K",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
		{
			ID:      "llama3:8b-instruct-q8_0",
			Object:  "model",
			OwnedBy: "helix",
			Hide:    true,
		},
	}

	response := model.OpenAIModelsList{
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
func (s *HelixAPIServer) createChatCompletion(rw http.ResponseWriter, r *http.Request) {
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

	modelName, err := types.ProcessModelName(string(s.Cfg.Inference.Provider), chatCompletionRequest.Model, types.SessionModeInference, types.SessionTypeText, false, false)
	if err != nil {
		http.Error(rw, "invalid model name: "+err.Error(), http.StatusBadRequest)
		return
	}

	chatCompletionRequest.Model = modelName.String()

	ctx := oai.SetContextValues(r.Context(), user.ID, "n/a", "n/a")

	options := &controller.ChatCompletionOptions{
		AppID:       r.URL.Query().Get("app_id"),
		AssistantID: r.URL.Query().Get("assistant_id"),
		RAGSourceID: r.URL.Query().Get("rag_source_id"),
	}

	// Non-streaming request returns the response immediately
	if !chatCompletionRequest.Stream {
		resp, err := s.Controller.ChatCompletion(ctx, user, chatCompletionRequest, options)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		rw.Header().Set("Content-Type", "application/json")

		if r.URL.Query().Get("pretty") == "true" {
			// Pretty print the response with indentation
			bts, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				http.Error(rw, err.Error(), http.StatusInternalServerError)
				return
			}

			_, _ = rw.Write(bts)
			return
		}

		err = json.NewEncoder(rw).Encode(resp)
		if err != nil {
			log.Err(err).Msg("error writing response")
		}
		return
	}

	// Streaming request, receive and write the stream in chunks
	stream, err := s.Controller.ChatCompletionStream(ctx, user, chatCompletionRequest, options)
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
