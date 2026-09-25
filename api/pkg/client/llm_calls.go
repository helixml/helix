package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/helixml/helix/api/pkg/types"
)

// LLMCallFilter narrows ListAppLLMCalls. Page is 1-based; PageSize defaults
// to 10 on the server.
type LLMCallFilter struct {
	SessionID     string
	InteractionID string
	Page          int
	PageSize      int
}

// ListAppLLMCalls returns one page of an app's LLM calls, oldest first. Calls
// carry the full request and response bodies, so pages can be large.
func (c *HelixClient) ListAppLLMCalls(ctx context.Context, appID string, f *LLMCallFilter) (*types.PaginatedLLMCalls, error) {
	path := "/agents/" + url.PathEscape(appID) + "/llm-calls"
	if f != nil {
		q := url.Values{}
		if f.SessionID != "" {
			q.Set("session", f.SessionID)
		}
		if f.InteractionID != "" {
			q.Set("interaction", f.InteractionID)
		}
		if f.Page > 0 {
			q.Set("page", strconv.Itoa(f.Page))
		}
		if f.PageSize > 0 {
			q.Set("pageSize", strconv.Itoa(f.PageSize))
		}
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
	}
	var page types.PaginatedLLMCalls
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}
