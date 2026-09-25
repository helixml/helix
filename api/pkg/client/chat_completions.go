package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

// ChatCompletion calls the OpenAI-compatible /v1/chat/completions endpoint
// (outside /api/v1). A non-empty appID scopes the call to that app: its model,
// system prompt and organization (for billing) apply.
func (c *HelixClient) ChatCompletion(ctx context.Context, appID string, req *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	fullURL := strings.TrimSuffix(c.url, "/api/v1") + "/v1/chat/completions"
	if appID != "" {
		fullURL += "?app_id=" + url.QueryEscape(appID)
	}
	var resp openai.ChatCompletionResponse
	if err := c.makeRequestURL(ctx, http.MethodPost, fullURL, bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
