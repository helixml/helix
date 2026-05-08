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

func (c *HelixClient) ListSessions(ctx context.Context, f *SessionFilter) (*types.PaginatedSessionsList, error) {
	var sessions types.PaginatedSessionsList

	path := "/sessions"

	if f != nil {
		queryParams := []string{}

		if f.OrganizationID != "" {
			queryParams = append(queryParams, "org_id="+f.OrganizationID)
		}

		if f.ProjectID != "" {
			queryParams = append(queryParams, "project_id="+f.ProjectID)
		}

		if f.Page > 0 {
			queryParams = append(queryParams, "page="+strconv.Itoa(f.Page))
		}

		if f.PageSize > 0 {
			queryParams = append(queryParams, "page_size="+strconv.Itoa(f.PageSize))
		}

		if len(queryParams) > 0 {
			path += "?" + strings.Join(queryParams, "&")
		}
	}

	err := c.makeRequest(ctx, http.MethodGet, path, nil, &sessions)
	if err != nil {
		return nil, err
	}

	return &sessions, nil
}

func (c *HelixClient) GetSession(ctx context.Context, sessionID string) (*types.Session, error) {
	var session types.Session
	err := c.makeRequest(ctx, http.MethodGet, "/sessions/"+sessionID, nil, &session)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (c *HelixClient) StopExternalAgent(ctx context.Context, sessionID string) error {
	return c.makeRequest(ctx, http.MethodDelete, "/sessions/"+sessionID+"/stop-external-agent", nil, nil)
}

// ChatSession sends a chat message to start or continue a session.
// Returns the raw response body. For external agent sessions, the response
// is the created Session JSON. For streaming sessions, it's the SSE stream.
func (c *HelixClient) ChatSession(ctx context.Context, req *types.SessionChatRequest) (string, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

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

	var buf strings.Builder
	_, err = io.Copy(&buf, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	return buf.String(), nil
}
