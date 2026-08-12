package hydra

// Sandboxes API client methods on RevDialClient. These wrap the
// /api/v1/dev-containers/{id}/{exec,files,terminal} endpoints exposed by
// hydra's HTTP server.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// SandboxCommandResponse is the JSON shape returned by hydra exec endpoints.
type SandboxCommandResponse struct {
	ID         string                 `json:"id"`
	SandboxID  string                 `json:"sandbox_id"`
	Cmd        string                 `json:"cmd"`
	Args       []string               `json:"args,omitempty"`
	Cwd        string                 `json:"cwd,omitempty"`
	Sudo       bool                   `json:"sudo,omitempty"`
	Detached   bool                   `json:"detached,omitempty"`
	Status     string                 `json:"status"`
	ExitCode   *int                   `json:"exit_code,omitempty"`
	StartedAt  string                 `json:"started_at"`
	FinishedAt *string                `json:"finished_at,omitempty"`
	Stdout     string                 `json:"stdout,omitempty"`
	Stderr     string                 `json:"stderr,omitempty"`
	Extra      map[string]interface{} `json:"-"`
}

// ListSandboxCommandsResponse is the JSON shape from GET .../exec.
type ListSandboxCommandsResponse struct {
	Commands []SandboxCommandResponse `json:"commands"`
}

// SandboxFileEntry mirrors hydra.ListDirectoryEntry.
type SandboxFileEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// ListSandboxFilesResponse is the JSON shape from GET .../files/list.
type ListSandboxFilesResponse struct {
	Path    string             `json:"path"`
	Entries []SandboxFileEntry `json:"entries"`
}

// RunSandboxCommand starts an exec inside the sandbox container.
func (c *RevDialClient) RunSandboxCommand(ctx context.Context, sessionID string, req *ExecRequest) (*SandboxCommandResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/api/v1/dev-containers/%s/exec", sessionID)
	respBody, err := c.doRequest(ctx, "POST", path, body)
	if err != nil {
		return nil, err
	}
	var out SandboxCommandResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListSandboxCommands returns every command tracked for this sandbox.
func (c *RevDialClient) ListSandboxCommands(ctx context.Context, sessionID string) (*ListSandboxCommandsResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/exec", sessionID)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out ListSandboxCommandsResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSandboxCommand returns a single command record.
func (c *RevDialClient) GetSandboxCommand(ctx context.Context, sessionID, cmdID string) (*SandboxCommandResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/exec/%s", sessionID, cmdID)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var out SandboxCommandResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// KillSandboxCommand sends a signal to a running command.
func (c *RevDialClient) KillSandboxCommand(ctx context.Context, sessionID, cmdID, signal string) error {
	q := url.Values{}
	if signal != "" {
		q.Set("signal", signal)
	}
	path := fmt.Sprintf("/api/v1/dev-containers/%s/exec/%s/kill", sessionID, cmdID)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	_, err := c.doRequest(ctx, "POST", path, nil)
	return err
}

// StreamSandboxCommandLogs opens a streaming SSE connection for a command's
// logs. The returned reader emits the raw text/event-stream bytes; callers
// can parse them or pipe straight to an HTTP response.
func (c *RevDialClient) StreamSandboxCommandLogs(ctx context.Context, sessionID, cmdID, stream string, follow bool) (io.ReadCloser, error) {
	q := url.Values{}
	if stream != "" {
		q.Set("stream", stream)
	}
	if follow {
		q.Set("follow", "1")
	}
	path := fmt.Sprintf("/api/v1/dev-containers/%s/exec/%s/logs", sessionID, cmdID)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return c.openStream(ctx, "GET", path)
}

// ReadSandboxFile downloads a file from the sandbox.
func (c *RevDialClient) ReadSandboxFile(ctx context.Context, sessionID, path string) ([]byte, error) {
	q := url.Values{}
	q.Set("path", path)
	endpoint := fmt.Sprintf("/api/v1/dev-containers/%s/files?%s", sessionID, q.Encode())
	return c.doRequest(ctx, "GET", endpoint, nil)
}

// WriteSandboxFile uploads bytes to a file inside the sandbox.
func (c *RevDialClient) WriteSandboxFile(ctx context.Context, sessionID, path string, data []byte, mode int) error {
	q := url.Values{}
	q.Set("path", path)
	if mode > 0 {
		q.Set("mode", strconv.FormatInt(int64(mode), 8))
	}
	endpoint := fmt.Sprintf("/api/v1/dev-containers/%s/files?%s", sessionID, q.Encode())
	_, err := c.doRequest(ctx, "PUT", endpoint, data)
	return err
}

// DeleteSandboxFile removes a file or directory inside the sandbox.
func (c *RevDialClient) DeleteSandboxFile(ctx context.Context, sessionID, path string, recursive bool) error {
	q := url.Values{}
	q.Set("path", path)
	if recursive {
		q.Set("recursive", "1")
	}
	endpoint := fmt.Sprintf("/api/v1/dev-containers/%s/files?%s", sessionID, q.Encode())
	_, err := c.doRequest(ctx, "DELETE", endpoint, nil)
	return err
}

// ListSandboxFiles enumerates a directory inside the sandbox.
func (c *RevDialClient) ListSandboxFiles(ctx context.Context, sessionID, path string) (*ListSandboxFilesResponse, error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	endpoint := fmt.Sprintf("/api/v1/dev-containers/%s/files/list", sessionID)
	if encoded := q.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}
	body, err := c.doRequest(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	var out ListSandboxFilesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ForgetSandboxOps drops every cached command record for a sandbox in hydra.
func (c *RevDialClient) ForgetSandboxOps(ctx context.Context, sessionID string) error {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/forget", sessionID)
	_, err := c.doRequest(ctx, "POST", path, nil)
	return err
}

// OpenSandboxTerminal opens a websocket connection to the sandbox terminal
// over revdial and returns a real *websocket.Conn. The caller is responsible
// for closing it.
//
// Implementation: configure a websocket.Dialer with a NetDialContext that
// returns the revdial-tunneled net.Conn, then let gorilla perform the standard
// upgrade handshake on top of it.
func (c *RevDialClient) OpenSandboxTerminal(ctx context.Context, sessionID, shell string) (*websocket.Conn, error) {
	q := url.Values{}
	if shell != "" {
		q.Set("shell", shell)
	}
	path := fmt.Sprintf("/api/v1/dev-containers/%s/terminal", sessionID)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	dialer := &websocket.Dialer{
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return c.connman.Dial(ctx, c.deviceID)
		},
		HandshakeTimeout: 15 * time.Second,
	}
	wsConn, resp, err := dialer.DialContext(ctx, "ws://hydra"+path, http.Header{})
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("ws upgrade failed (status %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		return nil, fmt.Errorf("ws upgrade: %w", err)
	}
	return wsConn, nil
}

// openStream issues an HTTP request and returns the response body as a stream
// (no buffering). Used for SSE log streaming.
func (c *RevDialClient) openStream(ctx context.Context, method, path string) (io.ReadCloser, error) {
	conn, err := c.connman.Dial(ctx, c.deviceID)
	if err != nil {
		return nil, fmt.Errorf("revdial dial: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://hydra"+path, nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write request: %w", err)
	}
	bufReader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(bufReader, req)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		conn.Close()
		return nil, fmt.Errorf("stream error (status %d): %s", resp.StatusCode, string(body))
	}
	return &streamCloser{ReadCloser: resp.Body, conn: conn}, nil
}

// streamCloser closes both the body and the underlying conn.
type streamCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (s *streamCloser) Close() error {
	err := s.ReadCloser.Close()
	if cerr := s.conn.Close(); cerr != nil && err == nil {
		err = cerr
	}
	return err
}

// ProbeDevContainerPort makes a HEAD request to the container's
// in-network port via the hydra proxy. Returns nil for any HTTP
// response (including 4xx/5xx), and a non-nil error only when the
// transport itself fails (e.g. connection refused, hydra unreachable).
// Used by the web service readiness check to detect "app has bound to
// the port" without caring what the app does with the request.
func (c *RevDialClient) ProbeDevContainerPort(ctx context.Context, sandboxID string, port int) error {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/proxy/%d/", sandboxID, port)
	_, err := c.doRequest(ctx, http.MethodHead, path, nil)
	return err
}
