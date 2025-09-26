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

// App represents a Wolf application configuration
type App struct {
	ID                     string    `json:"id"`
	Title                  string    `json:"title"`
	IconPngPath            *string   `json:"icon_png_path"`
	H264GstPipeline        string    `json:"h264_gst_pipeline"`
	HEVCGstPipeline        string    `json:"hevc_gst_pipeline"`
	AV1GstPipeline         string    `json:"av1_gst_pipeline"`
	OpusGstPipeline        string    `json:"opus_gst_pipeline"`
	RenderNode             string    `json:"render_node"`
	StartVirtualCompositor bool      `json:"start_virtual_compositor"`
	StartAudioServer       bool      `json:"start_audio_server"`
	SupportHDR             bool      `json:"support_hdr"`
	Runner                 AppRunner `json:"runner"`
}

// AppRunner defines how the app should be executed
type AppRunner struct {
	Type           string   `json:"type"`
	RunCmd         string   `json:"run_cmd,omitempty"`
	Image          string   `json:"image,omitempty"`            // Docker image
	Name           string   `json:"name,omitempty"`             // Container name
	Env            []string `json:"env,omitempty"`              // Environment variables
	Mounts         []string `json:"mounts,omitempty"`           // Volume mounts
	Devices        []string `json:"devices,omitempty"`          // Device mappings
	Ports          []string `json:"ports"`                      // Port mappings
	BaseCreateJSON string   `json:"base_create_json,omitempty"` // Docker create options
}

// SessionCreateRequest represents the minimal Wolf session creation request
type SessionCreateRequest struct {
	AppID    string `json:"app_id"`
	ClientID string `json:"client_id"`
	ClientIP string `json:"client_ip"`
}

// Session represents a Wolf streaming session (full structure for internal use)
type Session struct {
	AppID             string         `json:"app_id"`
	ClientID          string         `json:"client_id"`
	ClientIP          string         `json:"client_ip"`
	VideoWidth        int            `json:"video_width"`
	VideoHeight       int            `json:"video_height"`
	VideoRefreshRate  int            `json:"video_refresh_rate"`
	AudioChannelCount int            `json:"audio_channel_count"`
	ClientSettings    ClientSettings `json:"client_settings"`
	AESKey            string         `json:"aes_key"`
	AESIV             string         `json:"aes_iv"`
	RTSPFakeIP        string         `json:"rtsp_fake_ip"`
}

// ClientSettings represents client-specific settings
type ClientSettings struct {
	RunUID              int      `json:"run_uid"`
	RunGID              int      `json:"run_gid"`
	MouseAcceleration   float64  `json:"mouse_acceleration"`
	HScrollAcceleration float64  `json:"h_scroll_acceleration"`
	VScrollAcceleration float64  `json:"v_scroll_acceleration"`
	ControllersOverride []string `json:"controllers_override"`
}

// SessionResponse represents the response from creating a session
type SessionResponse struct {
	Success   bool   `json:"success"`
	SessionID string `json:"session_id"`
}

// GenericResponse represents a generic Wolf API response
type GenericResponse struct {
	Success bool `json:"success"`
}

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
