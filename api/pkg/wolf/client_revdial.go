package wolf

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// ConnManagerInterface defines the interface for RevDial connection management
type ConnManagerInterface interface {
	Dial(ctx context.Context, deviceID string) (net.Conn, error)
}

// RevDialClient provides access to Wolf API via RevDial tunnels
// This allows the Helix API to communicate with remote Wolf instances
// that connect outbound to the API (reverse dial pattern)
type RevDialClient struct {
	connman    ConnManagerInterface
	instanceID string
}

// NewRevDialClient creates a new Wolf API client that uses RevDial
func NewRevDialClient(connman ConnManagerInterface, instanceID string) *RevDialClient {
	return &RevDialClient{
		connman:    connman,
		instanceID: instanceID,
	}
}

// connClosingReadCloser wraps a ReadCloser to also close an underlying connection
type connClosingReadCloser struct {
	io.ReadCloser
	conn net.Conn
}

func (c *connClosingReadCloser) Close() error {
	bodyErr := c.ReadCloser.Close()
	connErr := c.conn.Close()
	if bodyErr != nil {
		return bodyErr
	}
	return connErr
}

// makeRevDialRequest makes an HTTP request over a RevDial tunnel
func (c *RevDialClient) makeRevDialRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	// Dial Wolf instance via RevDial
	runnerID := fmt.Sprintf("wolf-%s", c.instanceID)
	conn, err := c.connman.Dial(ctx, runnerID)
	if err != nil {
		return nil, fmt.Errorf("failed to dial Wolf instance %s via RevDial: %w", c.instanceID, err)
	}
	// Note: We do NOT defer conn.Close() here because the response body
	// reads from this connection. The connection is closed when the
	// response body is closed via connClosingReadCloser wrapper.

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, body)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	// Write HTTP request to RevDial connection
	if err := httpReq.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to write request to RevDial connection: %w", err)
	}

	// Read HTTP response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), httpReq)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to read response from RevDial connection: %w", err)
	}

	// Wrap the response body to close the connection when the body is closed
	resp.Body = &connClosingReadCloser{
		ReadCloser: resp.Body,
		conn:       conn,
	}

	return resp, nil
}

// AddApp adds a new application to Wolf
func (c *RevDialClient) AddApp(ctx context.Context, app *App) error {
	body, err := json.Marshal(app)
	if err != nil {
		return fmt.Errorf("failed to marshal app: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/apps/add", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("Wolf API returned success=false")
	}

	return nil
}

// RemoveApp removes an application from Wolf
func (c *RevDialClient) RemoveApp(ctx context.Context, appID string) error {
	body, err := json.Marshal(map[string]string{"id": appID})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/apps/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("Wolf API returned success=false")
	}

	return nil
}

// CreateSession creates a new streaming session
func (c *RevDialClient) CreateSession(ctx context.Context, session *Session) (string, error) {
	body, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/sessions/add", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return "", fmt.Errorf("Wolf API returned success=false")
	}

	return result.SessionID, nil
}

// ListSessions returns all active streaming sessions
func (c *RevDialClient) ListSessions(ctx context.Context) ([]WolfStreamSession, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/sessions", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success  bool                `json:"success"`
		Sessions []WolfStreamSession `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return result.Sessions, nil
}

// StopSession stops a streaming session
func (c *RevDialClient) StopSession(ctx context.Context, sessionID string) error {
	body, err := json.Marshal(map[string]string{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/sessions/stop", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("Wolf API returned success=false")
	}

	return nil
}

// ListApps retrieves all applications from Wolf
func (c *RevDialClient) ListApps(ctx context.Context) ([]App, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/apps", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool  `json:"success"`
		Apps    []App `json:"apps"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return result.Apps, nil
}

// CreateLobby creates a new Wolf lobby (container starts immediately)
func (c *RevDialClient) CreateLobby(ctx context.Context, req *CreateLobbyRequest) (*LobbyCreateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lobby request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/lobbies/create", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result LobbyCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return &result, nil
}

// StopLobby stops a Wolf lobby (tears down container)
func (c *RevDialClient) StopLobby(ctx context.Context, req *StopLobbyRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal stop request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/lobbies/stop", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("Wolf API returned success=false")
	}

	return nil
}

// ListLobbies retrieves all active lobbies from Wolf
func (c *RevDialClient) ListLobbies(ctx context.Context) ([]Lobby, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/lobbies", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result ListLobbiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return result.Lobbies, nil
}

// JoinLobby switches an existing Moonlight session to a specific lobby
func (c *RevDialClient) JoinLobby(ctx context.Context, req *JoinLobbyRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/lobbies/join", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result GenericResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("Wolf API returned success=false")
	}

	return nil
}

// GetSystemMemory retrieves Wolf system memory and resource usage statistics
func (c *RevDialClient) GetSystemMemory(ctx context.Context) (*SystemMemoryResponse, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/system/memory", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool `json:"success"`
		*SystemMemoryResponse
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return result.SystemMemoryResponse, nil
}

// GetSystemHealth retrieves Wolf system health and thread heartbeat status
func (c *RevDialClient) GetSystemHealth(ctx context.Context) (*SystemHealthResponse, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/system/health", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SystemHealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return &result, nil
}

// GetPendingPairRequests returns all pending Moonlight client pair requests via RevDial
func (c *RevDialClient) GetPendingPairRequests() ([]PendingPairRequest, error) {
	ctx := context.Background()
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/pair/pending", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending pair requests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("wolf API error: %s (status %d)", string(body), resp.StatusCode)
	}

	var response struct {
		Success  bool                 `json:"success"`
		Requests []PendingPairRequest `json:"requests"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode pending pair requests: %w", err)
	}

	return response.Requests, nil
}

// PairClient completes the pairing process for a Moonlight client via RevDial
func (c *RevDialClient) PairClient(pairSecret, pin string) error {
	ctx := context.Background()
	pairReq := map[string]string{
		"pair_secret": pairSecret,
		"pin":         pin,
	}

	reqBody, err := json.Marshal(pairReq)
	if err != nil {
		return fmt.Errorf("failed to marshal pair request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/pair/client", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to complete pairing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("pairing failed: %s (status %d)", string(body), resp.StatusCode)
	}

	return nil
}

// Get performs a raw GET request via RevDial (used for SSE streaming, etc.)
func (c *RevDialClient) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.makeRevDialRequest(ctx, "GET", path, nil)
}

// GetKeyboardState retrieves the current keyboard state for all sessions from Wolf via RevDial
func (c *RevDialClient) GetKeyboardState(ctx context.Context) (*KeyboardStateResponse, error) {
	resp, err := c.makeRevDialRequest(ctx, "GET", "/api/v1/keyboard/state", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result KeyboardStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return &result, nil
}

// ResetKeyboardState releases all stuck keys for a given session via RevDial
func (c *RevDialClient) ResetKeyboardState(ctx context.Context, sessionID string) (*KeyboardResetResponse, error) {
	reqBody := KeyboardResetRequest{SessionID: sessionID}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.makeRevDialRequest(ctx, "POST", "/api/v1/keyboard/reset", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result KeyboardResetResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return &result, nil
}
