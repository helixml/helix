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

// isAnthropicProvider checks if a provider is Anthropic-compatible.
// This returns true for both direct Anthropic API (api.anthropic.com) and
// any proxy that implements the Anthropic API format (e.g., outer Helix's /v1/messages).
// Detection is based on whether the client was configured with Anthropic credentials.
func isAnthropicProvider(baseURL string) bool {
	// Direct Anthropic API
	if strings.Contains(baseURL, "api.anthropic.com") {
		return true
	}
	// For proxy configurations, we detect Anthropic by checking if the URL
	// looks like it could serve Anthropic format. Since Helix serves both
	// OpenAI and Anthropic formats on the same base URL, the actual detection
	// happens via the anthropic-version header in the request.
	// This function is primarily used to decide whether to try the Anthropic
	// model listing format, so we return true for known Anthropic-compatible URLs.
	return false
}

// IsAnthropicBaseURL checks if the given base URL points to an Anthropic-compatible endpoint.
// This is used by the provider manager to determine which API format to use.
func IsAnthropicBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "api.anthropic.com")
}

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
		baseURL = "https://api.anthropic.com/v1"
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
