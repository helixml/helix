package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/model"
	"github.com/rs/zerolog/log"
)

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
		log.Trace().Str("provider", string(apiServer.Cfg.Inference.Provider)).Msg("Response from provider's models endpoint")
		log.Trace().RawJSON("response_body", body).Msg("Models response")

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
				return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint: %w, %s", err, string(body))
			}
		}

		// Sort models: meta-llama/* models first, then the rest, both in alphabetical order
		sort.Slice(models, func(i, j int) bool {
			if models[i].ID == "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo" {
				return true
			}
			if models[j].ID == "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo" {
				return false
			}

			iIsMetaLlama31 := strings.HasPrefix(models[i].ID, "meta-llama/Meta-Llama-3.1")
			jIsMetaLlama31 := strings.HasPrefix(models[j].ID, "meta-llama/Meta-Llama-3.1")

			if iIsMetaLlama31 && !jIsMetaLlama31 {
				return true
			}
			if !iIsMetaLlama31 && jIsMetaLlama31 {
				return false
			}

			iIsMetaLlama := strings.HasPrefix(models[i].ID, "meta-llama/")
			jIsMetaLlama := strings.HasPrefix(models[j].ID, "meta-llama/")

			if iIsMetaLlama && !jIsMetaLlama {
				return true
			}
			if !iIsMetaLlama && jIsMetaLlama {
				return false
			}

			return models[i].ID < models[j].ID
		})

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
