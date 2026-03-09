package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// DefaultAnthropicBaseURL is used when no ANTHROPIC_BASE_URL is configured.
const DefaultAnthropicBaseURL = "https://api.anthropic.com"

type ListAnthropicModelsResponse struct {
	Data    []AnthropicModel `json:"data"`
	FirstID string           `json:"first_id"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}

type AnthropicModel struct {
	CreatedAt   time.Time `json:"created_at"`
	DisplayName string    `json:"display_name"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
}

// listAnthropicModels fetches models from an Anthropic-compatible endpoint.
// It uses c.baseURL to support both direct Anthropic API and proxy configurations.
func (c *RetryableClient) listAnthropicModels(ctx context.Context) ([]types.OpenAIModel, error) {
	// Use c.baseURL if set, otherwise default to Anthropic's API
	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = DefaultAnthropicBaseURL + "/v1"
	}

	// Ensure baseURL ends with /v1 for the models endpoint
	baseURL = strings.TrimSuffix(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = baseURL + "/v1"
	}

	url := baseURL + "/models"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request to provider's models endpoint: %w", err)
	}

	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to provider's models endpoint: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get models from '%s' provider: %s - %s", url, resp.Status, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from provider's models endpoint: %w", err)
	}

	var response ListAnthropicModelsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response from provider's models endpoint: %w", err)
	}

	var openaiModels []types.OpenAIModel
	for _, model := range response.Data {
		openaiModels = append(openaiModels, types.OpenAIModel{
			ID:          model.ID,
			Description: model.DisplayName,
			Type:        "chat",
			// Don't hardcode context length - let model info provider fill it in
			// via getProviderModels which enriches models with ModelInfo
		})
	}
	return openaiModels, nil
}
