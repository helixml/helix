package hydra

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client is a client for the Hydra API via Unix socket
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// NewClient creates a new Hydra client that connects via Unix socket
func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	return &Client{
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   60 * time.Second,
		},
		baseURL: "http://hydra",
	}
}

// RevDialClient is a client that communicates with Hydra via RevDial
type RevDialClient struct {
	connman  connmanInterface
	deviceID string
}

// connmanInterface defines the interface for RevDial connection management
type connmanInterface interface {
	Dial(ctx context.Context, deviceID string) (net.Conn, error)
}

// NewRevDialClient creates a new Hydra client that connects via RevDial
func NewRevDialClient(connman connmanInterface, deviceID string) *RevDialClient {
	return &RevDialClient{
		connman:  connman,
		deviceID: deviceID,
	}
}

// DeviceID returns the device ID used for RevDial connections
func (c *RevDialClient) DeviceID() string {
	return c.deviceID
}

// Health checks if Hydra is healthy
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(body))
	}

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// CreateDevContainer creates a dev container via Unix socket
func (c *Client) CreateDevContainer(ctx context.Context, req *CreateDevContainerRequest) (*DevContainerResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/dev-containers", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result DevContainerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteDevContainer stops and removes a dev container via Unix socket
func (c *Client) DeleteDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	url := fmt.Sprintf("%s/api/v1/dev-containers/%s", c.baseURL, sessionID)

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result DevContainerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetDevContainer gets the status of a dev container via Unix socket
func (c *Client) GetDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	url := fmt.Sprintf("%s/api/v1/dev-containers/%s", c.baseURL, sessionID)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result DevContainerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListDevContainers lists all dev containers via Unix socket
func (c *Client) ListDevContainers(ctx context.Context) (*ListDevContainersResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/dev-containers", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result ListDevContainersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetGoldenBuildResult returns the latest golden build result for a project via Unix socket.
// Returns nil result with no error if no result is available.
func (c *Client) GetGoldenBuildResult(ctx context.Context, projectID string) (*GoldenBuildResult, error) {
	url := fmt.Sprintf("%s/api/v1/golden-cache/%s/build-result", c.baseURL, projectID)
	httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No result available
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(body))
	}
	var result GoldenBuildResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// DeleteGoldenCache removes the golden Docker cache for a project via Unix socket
func (c *Client) DeleteGoldenCache(ctx context.Context, projectID string) error {
	url := fmt.Sprintf("%s/api/v1/golden-cache/%s", c.baseURL, projectID)
	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// RevDial client methods - these make HTTP requests over RevDial connections

// CreateDevContainer creates a dev container via RevDial
func (c *RevDialClient) CreateDevContainer(ctx context.Context, req *CreateDevContainerRequest) (*DevContainerResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	respBody, err := c.doRequest(ctx, "POST", "/api/v1/dev-containers", body)
	if err != nil {
		return nil, err
	}

	var result DevContainerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteDevContainer stops and removes a dev container via RevDial
func (c *RevDialClient) DeleteDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s", sessionID)

	respBody, err := c.doRequest(ctx, "DELETE", path, nil)
	if err != nil {
		return nil, err
	}

	var result DevContainerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetDevContainer gets the status of a dev container via RevDial
func (c *RevDialClient) GetDevContainer(ctx context.Context, sessionID string) (*DevContainerResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s", sessionID)

	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result DevContainerResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListDevContainers lists all dev containers via RevDial
func (c *RevDialClient) ListDevContainers(ctx context.Context) (*ListDevContainersResponse, error) {
	respBody, err := c.doRequest(ctx, "GET", "/api/v1/dev-containers", nil)
	if err != nil {
		return nil, err
	}

	var result ListDevContainersResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetSystemStats gets system statistics (GPU info, container counts) via RevDial
func (c *RevDialClient) GetSystemStats(ctx context.Context) (*SystemStatsResponse, error) {
	respBody, err := c.doRequest(ctx, "GET", "/api/v1/system-stats", nil)
	if err != nil {
		return nil, err
	}

	var result SystemStatsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DevContainerClientsResponse is the response from the /clients endpoint
type DevContainerClientsResponse struct {
	SessionID string            `json:"session_id"`
	Clients   []ConnectedClient `json:"clients"`
}

// ConnectedClient represents a connected WebSocket client
type ConnectedClient struct {
	ID        uint32 `json:"id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	Color     string `json:"color"`
	LastX     int32  `json:"last_x"`
	LastY     int32  `json:"last_y"`
	LastSeen  string `json:"last_seen"`
}

// GetDevContainerClients gets connected WebSocket clients for a dev container via RevDial
func (c *RevDialClient) GetDevContainerClients(ctx context.Context, sessionID string) (*DevContainerClientsResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/clients", sessionID)

	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result DevContainerClientsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// VideoStatsResponse contains video streaming statistics from the desktop server
type VideoStatsResponse struct {
	SessionID string        `json:"session_id"`
	Sources   []SourceStats `json:"sources"`
}

// SourceStats contains statistics for a single shared video source
type SourceStats struct {
	NodeID         uint32              `json:"node_id"`
	Running        bool                `json:"running"`
	ClientCount    int                 `json:"client_count"`
	FramesReceived uint64              `json:"frames_received"`
	FramesDropped  uint64              `json:"frames_dropped"`
	GOPBufferSize  int                 `json:"gop_buffer_size"`
	Clients        []ClientBufferStats `json:"clients"`
}

// ClientBufferStats contains buffer statistics for a single streaming client
type ClientBufferStats struct {
	ClientID   uint64 `json:"client_id"`
	BufferUsed int    `json:"buffer_used"`
	BufferSize int    `json:"buffer_size"`
	BufferPct  int    `json:"buffer_pct"`
}

// GetDevContainerVideoStats gets video streaming statistics for a dev container via RevDial
func (c *RevDialClient) GetDevContainerVideoStats(ctx context.Context, sessionID string) (*VideoStatsResponse, error) {
	path := fmt.Sprintf("/api/v1/dev-containers/%s/video/stats", sessionID)

	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var result VideoStatsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetGoldenBuildResult returns the latest golden build result for a project via RevDial.
// Returns nil result with no error if no result is available.
func (c *RevDialClient) GetGoldenBuildResult(ctx context.Context, projectID string) (*GoldenBuildResult, error) {
	path := fmt.Sprintf("/api/v1/golden-cache/%s/build-result", projectID)
	respBody, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		// Check if it's a 404 (no result available)
		if strings.Contains(err.Error(), "status 404") {
			return nil, nil
		}
		return nil, err
	}
	var result GoldenBuildResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// DeleteGoldenCache removes the golden Docker cache for a project via RevDial
func (c *RevDialClient) DeleteGoldenCache(ctx context.Context, projectID string) error {
	path := fmt.Sprintf("/api/v1/golden-cache/%s", projectID)
	_, err := c.doRequest(ctx, "DELETE", path, nil)
	return err
}

// doRequest performs an HTTP request over RevDial
func (c *RevDialClient) doRequest(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	conn, err := c.connman.Dial(ctx, c.deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Hydra via RevDial: %w", err)
	}
	defer conn.Close()

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequest(method, "http://hydra"+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	if err := httpReq.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	bufReader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(bufReader, httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("hydra API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}
