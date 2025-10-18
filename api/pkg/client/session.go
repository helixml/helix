package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListSessions(ctx context.Context, f *SessionFilter) (*types.SessionsList, error) {
	var sessions types.SessionsList

	path := "/sessions"

	// Add query parameters if provided
	if f != nil {
		queryParams := []string{}

		if f.OrganizationID != "" {
			queryParams = append(queryParams, "organization_id="+f.OrganizationID)
		}

		if f.Offset > 0 {
			queryParams = append(queryParams, "offset="+strconv.Itoa(f.Offset))
		}

		if f.Limit > 0 {
			queryParams = append(queryParams, "limit="+strconv.Itoa(f.Limit))
		}

		if len(queryParams) > 0 {
			path += "?"
			for i, param := range queryParams {
				if i > 0 {
					path += "&"
				}
				path += param
			}
		}
	}

	err := c.makeRequest(ctx, http.MethodGet, path, nil, &sessions)
	if err != nil {
		return nil, err
	}

	return &sessions, nil
}

// ChatSession sends a chat message to start or continue a session
func (c *HelixClient) ChatSession(ctx context.Context, req *types.SessionChatRequest) (string, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Use the raw HTTP client to handle the response properly
	fullURL := c.url + "/sessions/chat"

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(string(reqBody)))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body
	var buf strings.Builder
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return buf.String(), nil
}
