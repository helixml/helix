package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helixml/helix/api/pkg/types"
)

// CreateAppAPIKey creates an app API key bound to appID and returns the key.
// App keys reach only /sessions/chat and /v1/chat/completions, as the app.
// List them with GetAppAPIKeys.
func (c *HelixClient) CreateAppAPIKey(ctx context.Context, appID, name string) (string, error) {
	body, err := json.Marshal(struct {
		Name  string           `json:"name"`
		Type  types.APIKeyType `json:"type"`
		AppID string           `json:"app_id"`
	}{Name: name, Type: types.APIkeytypeApp, AppID: appID})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}
	// The handler returns the new key as a bare JSON string.
	var key string
	if err := c.makeRequest(ctx, http.MethodPost, "/api_keys", bytes.NewReader(body), &key); err != nil {
		return "", err
	}
	return key, nil
}

// DeleteAPIKey deletes one of the caller's API keys.
func (c *HelixClient) DeleteAPIKey(ctx context.Context, key string) error {
	return c.makeRequest(ctx, http.MethodDelete, "/api_keys?key="+url.QueryEscape(key), nil, nil)
}
