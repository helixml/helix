package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

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

// Updated function to determine models
func (apiServer *HelixAPIServer) determineModels() ([]model.OpenAIModel, error) {
	// If configured to proxy through to LLM provider, return their models
	if apiServer.Cfg.Inference.Provider != config.ProviderHelix {
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
			return nil, fmt.Errorf("unsupported inference provider: %s", apiServer.Cfg.Inference.Provider)
		}

		req, err := http.NewRequest("GET", baseURL+"/models", nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request to provider's models endpoint: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to send request to provider's models endpoint: %w", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response from provider's models endpoint: %w", err)
		}

		// Log the response body for debugging purposes
		log.Debug().Str("provider", string(apiServer.Cfg.Inference.Provider)).Msg("Response from provider's models endpoint")
		log.Debug().RawJSON("response_body", body).Msg("Models response")

		var models []model.OpenAIModel
		var rawResponse struct {
			Data []model.OpenAIModel `json:"data"`
		}
		err = json.Unmarshal(body, &rawResponse)
		if err == nil && len(rawResponse.Data) > 0 {
			models = rawResponse.Data
		} else {
			// If unmarshaling into the struct with "data" field fails, try unmarshaling directly into the slice
			// This is how together.ai returns their models
			err = json.Unmarshal(body, &models)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint: %w", err)
			}
		}

		// Hack to workaround that OpenAI returns models like dall-e-3, which we
		// can't send chat completion requests to. Use a rough heuristic to
		// filter out models we can't use.

		// Check if any model starts with "gpt-"
		hasGPTModel := false
		for _, m := range models {
			if strings.HasPrefix(m.ID, "gpt-") {
				hasGPTModel = true
				break
			}
		}

		// If there's a GPT model, filter out non-GPT models
		if hasGPTModel {
			filteredModels := make([]model.OpenAIModel, 0)
			for _, m := range models {
				if strings.HasPrefix(m.ID, "gpt-") {
					filteredModels = append(filteredModels, m)
				}
			}
			models = filteredModels
		}

		return models, nil
	}

	// Return the list of Helix models
	return []model.OpenAIModel{
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
			ID:      "llama3:70b",
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
	}, nil
}

// Updated listModels function
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	models, err := apiServer.determineModels()
	if err != nil {
		log.Error().Err(err).Msg("Failed to determine models")
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return
	}

	response := model.OpenAIModelsList{
		Models: models,
	}

	rw.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(rw).Encode(response)
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
		log.Error().Msg("unauthorized")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10*MEGABYTE))
	if err != nil {
		log.Error().Err(err).Msg("error reading body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	var chatCompletionRequest openai.ChatCompletionRequest
	err = json.Unmarshal(body, &chatCompletionRequest)
	if err != nil {
		log.Error().Err(err).Msg("error unmarshalling body")
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}

	modelName, err := types.ProcessModelName(string(s.Cfg.Inference.Provider), chatCompletionRequest.Model, types.SessionModeInference, types.SessionTypeText, false, false)
	if err != nil {
		log.Error().Err(err).Msg("error processing model name")
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
