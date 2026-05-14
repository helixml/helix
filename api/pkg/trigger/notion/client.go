package notion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// notionAPIBase is the root of the Notion v1 REST API.
const notionAPIBase = "https://api.notion.com"

// notionAPIVersion mirrors api/pkg/oauth.notionAPIVersion. Kept as a string
// constant here too so this package stays free of an oauth import (this file
// only builds requests; the bearer token is supplied by the caller).
const notionAPIVersion = "2025-09-03"

// Client is a thin Notion HTTP client. It uses a caller-supplied access token
// (typically pulled out of the user's OAuthConnection) and the pinned
// Notion-Version header. We deliberately don't take a dep on a third-party
// SDK — only four endpoints are needed.
type Client struct {
	accessToken string
	httpClient  *http.Client
}

// NewClient constructs a Notion API client with the supplied bearer token.
func NewClient(accessToken string) *Client {
	return &Client{
		accessToken: accessToken,
		httpClient:  &http.Client{},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		bts, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		rdr = bytes.NewReader(bts)
	}

	req, err := http.NewRequestWithContext(ctx, method, notionAPIBase+path, rdr)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Notion-Version", notionAPIVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notion request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("notion %s %s: status %d body %s", method, path, resp.StatusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Page is the minimal subset of Notion's page object we care about.
type Page struct {
	ID         string                     `json:"id"`
	Properties map[string]json.RawMessage `json:"properties"`
}

// GetPage retrieves a Notion page by ID. Used to fetch the prompt column
// when the webhook payload doesn't already include it.
func (c *Client) GetPage(ctx context.Context, pageID string) (*Page, error) {
	var p Page
	if err := c.do(ctx, http.MethodGet, "/v1/pages/"+pageID, nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// PatchRichTextProperty sets a rich-text property on a page. Used exclusively
// for the Result column write-back — we never PATCH the action column (which
// would risk triggering the user's own Automation in a loop). If the column
// is empty this is a no-op.
func (c *Client) PatchRichTextProperty(ctx context.Context, pageID, propertyName, text string) error {
	if propertyName == "" {
		return nil
	}
	body := map[string]any{
		"properties": map[string]any{
			propertyName: map[string]any{
				"rich_text": []map[string]any{
					{"type": "text", "text": map[string]any{"content": text}},
				},
			},
		},
	}
	return c.do(ctx, http.MethodPatch, "/v1/pages/"+pageID, body, nil)
}

// AppendEmbedBlock appends an embed block (pointing at the supplied URL) to
// the children of a page (i.e. the page body). Returns the created block's ID
// so the caller can record it for later removal.
func (c *Client) AppendEmbedBlock(ctx context.Context, pageID, embedURL string) (string, error) {
	body := map[string]any{
		"children": []map[string]any{
			{
				"object": "block",
				"type":   "embed",
				"embed":  map[string]any{"url": embedURL},
			},
		},
	}
	var resp struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := c.do(ctx, http.MethodPatch, "/v1/blocks/"+pageID+"/children", body, &resp); err != nil {
		return "", err
	}
	if len(resp.Results) == 0 {
		return "", fmt.Errorf("notion: append embed block returned no results")
	}
	return resp.Results[0].ID, nil
}

// DeleteBlock removes a block by ID. Used for best-effort cleanup of the
// embed block when the spectask is cancelled. Caller should log+continue on
// error rather than blocking cancellation.
func (c *Client) DeleteBlock(ctx context.Context, blockID string) error {
	return c.do(ctx, http.MethodDelete, "/v1/blocks/"+blockID, nil, nil)
}
