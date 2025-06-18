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

func isAnthropicProvider(baseURL string) bool {
	return strings.Contains(baseURL, "https://api.anthropic.com")
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

func (c *RetryableClient) listAnthropicModels(ctx context.Context) ([]types.OpenAIModel, error) {
	url := "https://api.anthropic.com/v1/models"

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
		return nil, fmt.Errorf("failed to get models from '%s' provider: %s", url, resp.Status)
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
			// All models have 200k length
			// Ref: https://docs.anthropic.com/en/docs/about-claude/models/overview#model-comparison-table
			ContextLength: 200000,
		})
	}
	return openaiModels, nil
}
