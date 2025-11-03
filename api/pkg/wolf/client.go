package wolf

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Client provides access to Wolf API via Unix socket
type Client struct {
	socketPath string
	httpClient *http.Client
}

// NewClient creates a new Wolf API client
func NewClient(socketPath string) *Client {
	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
			Timeout: 30 * time.Second,
		},
	}
}

// Use minimal types that exactly match the working XFCE configuration
type App = MinimalWolfApp
type AppRunner = MinimalWolfRunner

// Deprecated: Use WolfStreamSession from wolf_client_generated.go instead
// Kept for backward compatibility - will be removed in future version
type Session = WolfStreamSession

// Deprecated: Use WolfClientSettings from wolf_client_generated.go instead
// Kept for backward compatibility - will be removed in future version
type ClientSettings = WolfClientSettings

// SessionCreateRequest represents the minimal Wolf session creation request
// This is a simplified request structure for backwards compatibility
type SessionCreateRequest struct {
	AppID    string `json:"app_id"`
	ClientID string `json:"client_id"`
	ClientIP string `json:"client_ip"`
}

// Deprecated: Use WolfSessionResponse from wolf_client_generated.go instead
// Kept for backward compatibility - will be removed in future version
type SessionResponse = WolfSessionResponse

// Deprecated: Use WolfGenericResponse from wolf_client_generated.go instead
// Kept for backward compatibility - will be removed in future version
type GenericResponse = WolfGenericResponse

// AddApp adds a new application to Wolf
func (c *Client) AddApp(ctx context.Context, app *App) error {
	body, err := json.Marshal(app)
	if err != nil {
		return fmt.Errorf("failed to marshal app: %w", err)
	}

	// Debug: log the JSON being sent to Wolf
	fmt.Printf("DEBUG: Sending app to Wolf API: %s\n", string(body))

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/apps/add", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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

// Wolf pairing structures
type PairRequest struct {
	ClientName string `json:"client_name"`
	PIN        string `json:"pin"`
	UUID       string `json:"uuid"`
}

type PendingPairRequest struct {
	ClientIP   string `json:"client_ip"`
	PairSecret string `json:"pair_secret"`
}

// makeRequest is a helper method for making HTTP requests to Wolf API
func (c *Client) makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, "http://localhost"+path, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

// Get makes a GET request to the Wolf API and returns the response
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.makeRequest("GET", path, nil)
}

// GetPendingPairRequests returns all pending Moonlight client pair requests
func (c *Client) GetPendingPairRequests() ([]PendingPairRequest, error) {
	resp, err := c.makeRequest("GET", "/api/v1/pair/pending", nil)
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

// PairClient completes the pairing process for a Moonlight client
func (c *Client) PairClient(pairSecret, pin string) error {
	pairReq := map[string]string{
		"pair_secret": pairSecret,
		"pin":         pin,
	}

	reqBody, err := json.Marshal(pairReq)
	if err != nil {
		return fmt.Errorf("failed to marshal pair request: %w", err)
	}

	resp, err := c.makeRequest("POST", "/api/v1/pair/client", bytes.NewReader(reqBody))
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

// RemoveApp removes an application from Wolf
func (c *Client) RemoveApp(ctx context.Context, appID string) error {
	body, err := json.Marshal(map[string]string{"id": appID})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/apps/delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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
func (c *Client) CreateSession(ctx context.Context, session *Session) (string, error) {
	// Wolf expects all session fields for this version
	body, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("failed to marshal session: %w", err)
	}


	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/sessions/add", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
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
func (c *Client) ListSessions(ctx context.Context) ([]WolfStreamSession, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/v1/sessions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
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
func (c *Client) StopSession(ctx context.Context, sessionID string) error {
	body, err := json.Marshal(map[string]string{"session_id": sessionID})
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/sessions/stop", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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

// WolfPairedClient represents a paired Moonlight client from Wolf API
type WolfPairedClient struct {
	ClientID       string `json:"client_id"`
	AppStateFolder string `json:"app_state_folder"`
}

// GetPairedClients retrieves all paired Moonlight clients from Wolf
func (c *Client) GetPairedClients(ctx context.Context) ([]WolfPairedClient, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/v1/clients", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Wolf API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Success bool               `json:"success"`
		Clients []WolfPairedClient `json:"clients"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("Wolf API returned success=false")
	}

	return result.Clients, nil
}

// ListApps retrieves all applications from Wolf
func (c *Client) ListApps(ctx context.Context) ([]App, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/v1/apps", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
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

// Lobby types and methods for Wolf UI auto-start functionality

// LobbyVideoSettings configures video streaming for a lobby
type LobbyVideoSettings struct {
	Width                     int    `json:"width"`
	Height                    int    `json:"height"`
	RefreshRate               int    `json:"refresh_rate"`
	WaylandRenderNode         string `json:"wayland_render_node"`
	RunnerRenderNode          string `json:"runner_render_node"`
	VideoProducerBufferCaps   string `json:"video_producer_buffer_caps"`
}

// LobbyAudioSettings configures audio for a lobby
type LobbyAudioSettings struct {
	ChannelCount int `json:"channel_count"`
}

// CreateLobbyRequest represents the request to create a new lobby
type CreateLobbyRequest struct {
	ProfileID              string              `json:"profile_id"`
	Name                   string              `json:"name"`
	MultiUser              bool                `json:"multi_user"`
	StopWhenEveryoneLeaves bool                `json:"stop_when_everyone_leaves"`
	PIN                    []int16             `json:"pin,omitempty"`
	VideoSettings          *LobbyVideoSettings `json:"video_settings"`
	AudioSettings          *LobbyAudioSettings `json:"audio_settings"`
	RunnerStateFolder      string              `json:"runner_state_folder"`
	Runner                 interface{}         `json:"runner"` // MinimalWolfRunner or similar
}

// LobbyCreateResponse represents the response from creating a lobby
type LobbyCreateResponse struct {
	Success bool   `json:"success"`
	LobbyID string `json:"lobby_id"`
}

// StopLobbyRequest represents the request to stop a lobby
type StopLobbyRequest struct {
	LobbyID string `json:"lobby_id"`
	PIN     []int16 `json:"pin,omitempty"`
}

// Lobby represents a Wolf lobby
type Lobby struct {
	ID                     string      `json:"id"`
	Name                   string      `json:"name"`
	StartedByProfileID     string      `json:"started_by_profile_id"`
	MultiUser              bool        `json:"multi_user"`
	StopWhenEveryoneLeaves bool        `json:"stop_when_everyone_leaves"`
	PIN                    []int16     `json:"pin,omitempty"`
	Runner                 interface{} `json:"runner,omitempty"` // Runner configuration (used for extracting session ID from env vars)
}

// ListLobbiesResponse represents the response from listing lobbies
type ListLobbiesResponse struct {
	Success bool    `json:"success"`
	Lobbies []Lobby `json:"lobbies"`
}

// CreateLobby creates a new Wolf lobby (container starts immediately)
func (c *Client) CreateLobby(ctx context.Context, req *CreateLobbyRequest) (*LobbyCreateResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lobby request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/lobbies/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
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
func (c *Client) StopLobby(ctx context.Context, req *StopLobbyRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal stop request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/lobbies/stop", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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
func (c *Client) ListLobbies(ctx context.Context) ([]Lobby, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/v1/lobbies", nil)
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

// JoinLobbyRequest represents the request to join a lobby with an existing session
type JoinLobbyRequest struct {
	LobbyID            string  `json:"lobby_id"`
	MoonlightSessionID string  `json:"moonlight_session_id"` // Wolf expects string, not int64
	PIN                []int16 `json:"pin,omitempty"`
}

// JoinLobby switches an existing Moonlight session to a specific lobby
// This is how Wolf UI switches clients between lobbies
func (c *Client) JoinLobby(ctx context.Context, req *JoinLobbyRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal join request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "http://localhost/api/v1/lobbies/join", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
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

