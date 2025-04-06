package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listModels godoc
// @Summary List models
// @Description List models
// @Tags    models

// @Success 200 {array} types.OpenAIModelsList
// @Param provider query string false "Provider"
// @Router /v1/models [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) listModels(rw http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		provider = apiServer.Cfg.Inference.Provider
	}

	user := getRequestUser(r)

	client, err := apiServer.providerManager.GetClient(r.Context(), &manager.GetClientRequest{
		Provider: provider,
		Owner:    user.ID,
	})
	if err != nil {
		log.Err(err).Msg("error getting client")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	models, err := client.ListModels(r.Context())
	if err != nil {
		log.Err(err).Msg("error listing models")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	response := types.OpenAIModelsList{
		Models: models,
	}

	writeResponse(rw, response, http.StatusOK)
}

// Updated function to determine models
func (apiServer *HelixAPIServer) determineModels() ([]types.OpenAIModel, error) {
	// If configured to proxy through to LLM provider, return their models
	if apiServer.Cfg.Inference.Provider != string(types.ProviderHelix) {
		var baseURL string
		var apiKey string
		switch apiServer.Cfg.Inference.Provider {
		case string(types.ProviderOpenAI):
			baseURL = apiServer.Cfg.Providers.OpenAI.BaseURL

			provider, err := apiServer.providerManager.GetClient(context.Background(), &manager.GetClientRequest{
				Provider: string(types.ProviderOpenAI),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get openai client: %w", err)
			}

			apiKey = provider.APIKey()

		case string(types.ProviderTogetherAI):
			baseURL = apiServer.Cfg.Providers.TogetherAI.BaseURL

			provider, err := apiServer.providerManager.GetClient(context.Background(), &manager.GetClientRequest{
				Provider: string(types.ProviderTogetherAI),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to get togetherai client: %w", err)
			}

			apiKey = provider.APIKey()
		default:
			return nil, fmt.Errorf("unsupported inference provider: %s", apiServer.Cfg.Inference.Provider)
		}

		url := baseURL + "/models"
		req, err := http.NewRequest("GET", url, nil)
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
		log.Trace().Str("provider", apiServer.Cfg.Inference.Provider).Msg("Response from provider's models endpoint")
		log.Trace().RawJSON("response_body", body).Msg("Models response")

		var models []types.OpenAIModel
		var rawResponse struct {
			Data []types.OpenAIModel `json:"data"`
		}
		err = json.Unmarshal(body, &rawResponse)
		if err == nil && len(rawResponse.Data) > 0 {
			models = rawResponse.Data
		} else {
			// If unmarshaling into the struct with "data" field fails, try unmarshaling directly into the slice
			// This is how together.ai returns their models
			err = json.Unmarshal(body, &models)
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint (%s): %w, %s", url, err, string(body))
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
			filteredModels := make([]types.OpenAIModel, 0)
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
	return openai.ListModels(context.Background())
}
