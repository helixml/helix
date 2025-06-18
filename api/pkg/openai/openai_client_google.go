package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

func isGoogleProvider(baseURL string) bool {
	return strings.Contains(baseURL, "generativelanguage.googleapis.com")
}

type ListGoogleModelsResponse struct {
	Models        []GoogleModel `json:"models"`
	NextPageToken string        `json:"nextPageToken"`
}

type GoogleModel struct {
	Name                       string   `json:"name"`
	Version                    string   `json:"version"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
	Temperature                float64  `json:"temperature"`
	TopP                       float64  `json:"topP"`
	TopK                       int      `json:"topK"`
	MaxTemperature             float64  `json:"maxTemperature"`
}

// Google models are served on https://generativelanguage.googleapis.com/v1beta/models
func (c *RetryableClient) listGoogleModels(ctx context.Context) ([]types.OpenAIModel, error) {
	url := "https://generativelanguage.googleapis.com/v1beta/models?key=" + c.apiKey

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to provider's models endpoint: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to provider's models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get models from '%s' provider: %s", url, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from provider's models endpoint: %w", err)
	}

	var response ListGoogleModelsResponse
	err = json.Unmarshal(body, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint: %w", err)
	}
	// TODO: add paginations

	var openaiModels []types.OpenAIModel
	for _, model := range response.Models {
		if !isGenerativeGoogleModel(&model) {
			continue
		}

		openaiModels = append(openaiModels, types.OpenAIModel{
			ID:            model.Name,
			Description:   model.Description,
			Type:          "chat",
			ContextLength: model.InputTokenLimit,
		})
	}

	return openaiModels, nil
}

func isGenerativeGoogleModel(model *GoogleModel) bool {
	return slices.Contains(model.SupportedGenerationMethods, "generateContent")
}
