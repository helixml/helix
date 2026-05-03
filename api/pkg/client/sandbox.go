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

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/types"
)

// Sandboxes API client.
//
// Auth: every method uses the same Bearer token as the rest of HelixClient.
// All sandbox routes are scoped to an org id; pass it in via the orgID arg.
//
// Note: makeRequest has a 10s timeout, which is fine for control-plane calls
// (list/create/get/delete, exec for non-detached commands). Streaming endpoints
// (terminal websocket, command log SSE) bypass it and use the http client
// directly so they can stay open indefinitely.

// SandboxListFilter narrows ListSandboxes results. ProjectID="" matches all.
type SandboxListFilter struct {
	ProjectID string
}

// ListSandboxRuntimes returns the runtime names this server is configured to
// expose. The UI uses it to populate the runtime dropdown; the CLI uses it
// for tab-completion and validation.
func (c *HelixClient) ListSandboxRuntimes(ctx context.Context) ([]string, error) {
	var resp struct {
		Runtimes []string `json:"runtimes"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, "/sandbox-runtimes", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Runtimes, nil
}

// ListSandboxes returns the sandboxes belonging to an organization.
// Pass nil filter to list all sandboxes in the org.
func (c *HelixClient) ListSandboxes(ctx context.Context, orgID string, filter *SandboxListFilter) (*types.SandboxListResponse, error) {
	path := fmt.Sprintf("/organizations/%s/sandboxes", orgID)
	if filter != nil && filter.ProjectID != "" {
		path += "?project_id=" + url.QueryEscape(filter.ProjectID)
	}
	var resp types.SandboxListResponse
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// CreateSandbox provisions a new sandbox in the organization.
func (c *HelixClient) CreateSandbox(ctx context.Context, orgID string, req *types.CreateSandboxRequest) (*types.Sandbox, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create request: %w", err)
	}
	var sb types.Sandbox
	if err := c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/organizations/%s/sandboxes", orgID), bytes.NewReader(body), &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

// GetSandbox fetches a single sandbox by id.
func (c *HelixClient) GetSandbox(ctx context.Context, orgID, sandboxID string) (*types.Sandbox, error) {
	var sb types.Sandbox
	if err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/sandboxes/%s", orgID, sandboxID), nil, &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

// UpdateSandbox patches name / tags / ttl on an existing sandbox.
func (c *HelixClient) UpdateSandbox(ctx context.Context, orgID, sandboxID string, req *types.UpdateSandboxRequest) (*types.Sandbox, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal update request: %w", err)
	}
	var sb types.Sandbox
	if err := c.makeRequest(ctx, http.MethodPatch, fmt.Sprintf("/organizations/%s/sandboxes/%s", orgID, sandboxID), bytes.NewReader(body), &sb); err != nil {
		return nil, err
	}
	return &sb, nil
}

// DeleteSandbox tears the sandbox down. Best-effort on the hydra side; the
// row is always soft-deleted.
func (c *HelixClient) DeleteSandbox(ctx context.Context, orgID, sandboxID string) error {
	return c.makeRequest(ctx, http.MethodDelete, fmt.Sprintf("/organizations/%s/sandboxes/%s", orgID, sandboxID), nil, nil)
}

// RunSandboxCommand starts a command inside the sandbox. For non-detached
// commands the response includes stdout/stderr once the command has finished.
func (c *HelixClient) RunSandboxCommand(ctx context.Context, orgID, sandboxID string, req *types.RunSandboxCommandRequest) (*types.SandboxCommand, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal command request: %w", err)
	}
	var cmd types.SandboxCommand
	if err := c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/organizations/%s/sandboxes/%s/commands", orgID, sandboxID), bytes.NewReader(body), &cmd); err != nil {
		return nil, err
	}
	return &cmd, nil
}

// ListSandboxCommands returns every command tracked for the sandbox.
func (c *HelixClient) ListSandboxCommands(ctx context.Context, orgID, sandboxID string) ([]*types.SandboxCommand, error) {
	var resp struct {
		Commands []*types.SandboxCommand `json:"commands"`
	}
	if err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/sandboxes/%s/commands", orgID, sandboxID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Commands, nil
}

// GetSandboxCommand fetches a specific command by id.
func (c *HelixClient) GetSandboxCommand(ctx context.Context, orgID, sandboxID, cmdID string) (*types.SandboxCommand, error) {
	var cmd types.SandboxCommand
	if err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/organizations/%s/sandboxes/%s/commands/%s", orgID, sandboxID, cmdID), nil, &cmd); err != nil {
		return nil, err
	}
	return &cmd, nil
}

// KillSandboxCommand sends a signal to a running command. Empty signal
// defaults to TERM on the server side.
func (c *HelixClient) KillSandboxCommand(ctx context.Context, orgID, sandboxID, cmdID, signal string) error {
	path := fmt.Sprintf("/organizations/%s/sandboxes/%s/commands/%s/kill", orgID, sandboxID, cmdID)
	if signal != "" {
		path += "?signal=" + url.QueryEscape(signal)
	}
	return c.makeRequest(ctx, http.MethodPost, path, nil, nil)
}

// StreamSandboxCommandLogs returns the SSE log stream for a command. The
// caller MUST close the returned ReadCloser. stream is "stdout" | "stderr" |
// "" (both); follow=true keeps the stream open until the command exits.
func (c *HelixClient) StreamSandboxCommandLogs(ctx context.Context, orgID, sandboxID, cmdID, stream string, follow bool) (io.ReadCloser, error) {
	q := url.Values{}
	if stream != "" {
		q.Set("stream", stream)
	}
	if follow {
		q.Set("follow", "1")
	}
	path := fmt.Sprintf("/organizations/%s/sandboxes/%s/commands/%s/logs", orgID, sandboxID, cmdID)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("status %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.Body, nil
}

// ReadSandboxFile returns the raw bytes of a file inside the sandbox.
func (c *HelixClient) ReadSandboxFile(ctx context.Context, orgID, sandboxID, path string) ([]byte, error) {
	q := url.Values{"path": []string{path}}
	full := fmt.Sprintf("/organizations/%s/sandboxes/%s/files?%s", orgID, sandboxID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.ReadAll(resp.Body)
}

// WriteSandboxFile writes data to a file in the sandbox. mode=0 leaves the
// permission bits at the server default.
func (c *HelixClient) WriteSandboxFile(ctx context.Context, orgID, sandboxID, path string, data []byte, mode int) error {
	q := url.Values{"path": []string{path}}
	if mode != 0 {
		q.Set("mode", strconv.FormatInt(int64(mode), 8))
	}
	full := fmt.Sprintf("/organizations/%s/sandboxes/%s/files?%s", orgID, sandboxID, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.url+full, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d (%s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// DeleteSandboxFile removes a file or (recursively) directory inside the
// sandbox.
func (c *HelixClient) DeleteSandboxFile(ctx context.Context, orgID, sandboxID, path string, recursive bool) error {
	q := url.Values{"path": []string{path}}
	if recursive {
		q.Set("recursive", "1")
	}
	full := fmt.Sprintf("/organizations/%s/sandboxes/%s/files?%s", orgID, sandboxID, q.Encode())
	return c.makeRequest(ctx, http.MethodDelete, full, nil, nil)
}

// ListSandboxFiles lists a directory inside the sandbox. An empty path
// defaults to /root on the server side.
func (c *HelixClient) ListSandboxFiles(ctx context.Context, orgID, sandboxID, path string) (*types.SandboxFileListResponse, error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	full := fmt.Sprintf("/organizations/%s/sandboxes/%s/files/list", orgID, sandboxID)
	if encoded := q.Encode(); encoded != "" {
		full += "?" + encoded
	}
	var resp types.SandboxFileListResponse
	if err := c.makeRequest(ctx, http.MethodGet, full, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// OpenSandboxTerminal dials the terminal websocket and returns a real
// *websocket.Conn carrying the terminal protocol:
//   - Binary frames: stdin (client → server), stdout/stderr merged (server → client).
//   - Text frames: control JSON, e.g. {"type":"resize","cols":80,"rows":24}.
//
// The caller MUST Close() the returned conn.
func (c *HelixClient) OpenSandboxTerminal(ctx context.Context, orgID, sandboxID, shell string) (*websocket.Conn, error) {
	q := url.Values{}
	if shell != "" {
		q.Set("shell", shell)
	}
	wsURL := strings.Replace(c.url, "http", "ws", 1) +
		fmt.Sprintf("/organizations/%s/sandboxes/%s/terminal", orgID, sandboxID)
	if encoded := q.Encode(); encoded != "" {
		wsURL += "?" + encoded
	}

	header := http.Header{"Authorization": []string{"Bearer " + c.apiKey}}
	dialer := websocket.DefaultDialer
	// Inherit any TLS skip-verify from the http client transport.
	if t, ok := c.httpClient.Transport.(*http.Transport); ok && t != nil && t.TLSClientConfig != nil {
		d := *websocket.DefaultDialer
		d.TLSClientConfig = t.TLSClientConfig
		dialer = &d
	}
	conn, resp, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("ws upgrade failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	return conn, nil
}
