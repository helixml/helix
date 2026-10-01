package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	openai "github.com/sashabaranov/go-openai"

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

// ChatSessionCompletion sends one blocking (non-streaming) chat turn and
// returns the OpenAI-style completion; its ID is the session id, which is new
// when req.SessionID was empty. The call lasts as long as the turn: give ctx a
// deadline that covers it.
func (c *HelixClient) ChatSessionCompletion(ctx context.Context, req *types.SessionChatRequest) (*openai.ChatCompletionResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("ChatSessionCompletion needs a non-streaming request")
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	var resp openai.ChatCompletionResponse
	if err := c.makeRequest(ctx, http.MethodPost, "/sessions/chat", bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// InteractionFilter pages a session's interactions. Page is 0-based: page 1
// is the SECOND page. Order is "asc" (default, oldest first) or "desc".
type InteractionFilter struct {
	Page    int
	PerPage int
	Order   string
}

// ListInteractions returns one page of a session's interactions (turns).
func (c *HelixClient) ListInteractions(ctx context.Context, sessionID string, f *InteractionFilter) (*types.PaginatedInteractions, error) {
	path := "/sessions/" + url.PathEscape(sessionID) + "/interactions"
	if f != nil {
		q := url.Values{"page": {strconv.Itoa(f.Page)}}
		if f.PerPage > 0 {
			q.Set("per_page", strconv.Itoa(f.PerPage))
		}
		if f.Order != "" {
			q.Set("order", f.Order)
		}
		path += "?" + q.Encode()
	}
	var page types.PaginatedInteractions
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}
