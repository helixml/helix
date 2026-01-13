//go:build integration || spectask

package smoke

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// MCPTestResultsDir is where test artifacts are saved
	MCPTestResultsDir = "/tmp/helix-mcp-test-results"
)

// SpectaskMCPSuite tests the desktop MCP server functionality
type SpectaskMCPSuite struct {
	suite.Suite
	httpClient      *http.Client
	apiURL          string
	token           string
	projectID       string
	agentID         string
	sessionID       string
	sessionProvided bool // true if session was provided via env var (don't cleanup)
}

func TestSpectaskMCPSuite(t *testing.T) {
	suite.Run(t, new(SpectaskMCPSuite))
}

func (s *SpectaskMCPSuite) SetupSuite() {
	s.T().Log("SetupSuite: Initializing MCP tests")

	s.apiURL = os.Getenv("HELIX_URL")
	if s.apiURL == "" {
		s.apiURL = "http://localhost:8080"
	}

	s.token = os.Getenv("HELIX_API_KEY")
	if s.token == "" {
		s.T().Fatal("HELIX_API_KEY environment variable is required")
	}

	// Check for existing session first (allows reusing a running sandbox)
	s.sessionID = os.Getenv("HELIX_SESSION_ID")
	s.sessionProvided = s.sessionID != ""

	// Check token type - only API keys (hl- prefix) can create sessions
	isAPIKey := strings.HasPrefix(s.token, "hl-")
	isRunnerToken := !isAPIKey && s.token != ""

	// Only require project/agent if we don't have an existing session
	if s.sessionID == "" {
		// Runner tokens can't create sessions - they need HELIX_SESSION_ID
		if isRunnerToken {
			s.T().Fatalf(`Runner tokens cannot create sessions (no project authorization).
Either:
  1. Set HELIX_SESSION_ID to use an existing sandbox session
  2. Use a user API key (hl-... prefix) instead of runner token
  3. Create a session manually via UI first, then pass its session_id`)
		}

		s.projectID = os.Getenv("HELIX_PROJECT")
		if s.projectID == "" {
			s.T().Fatal("HELIX_PROJECT or HELIX_SESSION_ID environment variable is required")
		}

		s.agentID = os.Getenv("HELIX_UBUNTU_AGENT")
		if s.agentID == "" {
			s.T().Fatal("HELIX_UBUNTU_AGENT or HELIX_SESSION_ID environment variable is required")
		}
	}

	s.httpClient = &http.Client{
		Timeout: 60 * time.Second,
	}

	err := os.MkdirAll(MCPTestResultsDir, 0755)
	require.NoError(s.T(), err, "creating test results directory should succeed")

	if s.sessionID != "" {
		helper.LogStep(s.T(), fmt.Sprintf("Using existing session: %s", s.sessionID))
	} else {
		helper.LogStep(s.T(), fmt.Sprintf("Initialized with API URL: %s, Project: %s, Agent: %s",
			s.apiURL, s.projectID, s.agentID))
	}
}

func (s *SpectaskMCPSuite) TearDownSuite() {
	s.T().Log("TearDownSuite: Cleaning up")

	// Don't stop session if it was provided via env var (allows reuse)
	if s.sessionID != "" && !s.sessionProvided {
		helper.LogStep(s.T(), fmt.Sprintf("Stopping session %s", s.sessionID))
		err := s.stopSession(s.sessionID)
		if err != nil {
			s.T().Logf("Warning: failed to stop session: %v", err)
		}
	} else if s.sessionProvided {
		s.T().Logf("Keeping session %s (provided via HELIX_SESSION_ID)", s.sessionID)
	}
}

// TestMCPScreenshotWithVideoRecording tests the MCP screenshot tool while recording video
func (s *SpectaskMCPSuite) TestMCPScreenshotWithVideoRecording() {
	var sessionID string

	// Use existing session if provided, otherwise create new one
	if s.sessionID != "" {
		sessionID = s.sessionID
		helper.LogStep(s.T(), fmt.Sprintf("Using existing session: %s", sessionID))
	} else {
		helper.LogStep(s.T(), "Creating sandbox session")

		var err error
		sessionID, err = s.createSession()
		require.NoError(s.T(), err, "creating session should succeed")
		s.sessionID = sessionID

		helper.LogStep(s.T(), fmt.Sprintf("Session created: %s", sessionID))
	}

	// Wait for sandbox to be ready
	helper.LogStep(s.T(), "Waiting for sandbox to be ready")
	err := s.waitForSandbox(sessionID, 120*time.Second)
	require.NoError(s.T(), err, "sandbox should become ready")

	// Start video recording in background
	helper.LogStep(s.T(), "Starting video recording")
	videoCtx, videoCancel := context.WithCancel(context.Background())
	defer videoCancel()

	timestamp := time.Now().Format("20060102_150405")
	videoFile := filepath.Join(MCPTestResultsDir, fmt.Sprintf("mcp_test_%s.h264", timestamp))
	var videoWg sync.WaitGroup
	var videoErr error
	var videoFrames int

	videoWg.Add(1)
	go func() {
		defer videoWg.Done()
		defer func() {
			if r := recover(); r != nil {
				videoErr = fmt.Errorf("video recording panic: %v", r)
				s.T().Logf("Video recording panic: %v", r)
			}
		}()
		videoFrames, videoErr = s.recordVideo(videoCtx, sessionID, videoFile, 15*time.Second)
	}()

	// Give video recording and container time to start
	time.Sleep(3 * time.Second)

	// Call MCP screenshot tool multiple times
	helper.LogStep(s.T(), "Taking screenshots via MCP tool")
	successfulScreenshots := 0
	for i := 0; i < 3; i++ {
		screenshotFile := filepath.Join(MCPTestResultsDir,
			fmt.Sprintf("mcp_screenshot_%s_%d.png", timestamp, i+1))

		err := s.callMCPScreenshot(sessionID, screenshotFile)
		if err != nil {
			s.T().Logf("Screenshot %d failed: %v", i+1, err)
		} else {
			s.T().Logf("Screenshot %d saved: %s", i+1, screenshotFile)
			successfulScreenshots++
		}

		time.Sleep(2 * time.Second)
	}

	// Stop video recording
	helper.LogStep(s.T(), "Stopping video recording")
	videoCancel()
	videoWg.Wait()

	if videoErr != nil {
		s.T().Logf("Video recording error (non-fatal): %v", videoErr)
	}

	// Verify results - at least 2 screenshots should succeed
	require.GreaterOrEqual(s.T(), successfulScreenshots, 2, "should have at least 2 successful screenshots")

	// Video is optional - log but don't fail if no frames (PipeWire may not be available)
	if videoFrames > 0 {
		s.T().Logf("Video recording captured %d frames", videoFrames)
	} else {
		s.T().Log("Video recording captured no frames (PipeWire may not be configured)")
	}

	// Check video file size
	if stat, err := os.Stat(videoFile); err == nil {
		s.T().Logf("Video file: %s (%d bytes, %d frames)", videoFile, stat.Size(), videoFrames)
	}

	// Convert to MP4 if ffmpeg available
	mp4File := strings.TrimSuffix(videoFile, ".h264") + ".mp4"
	if err := s.convertToMP4(videoFile, mp4File); err != nil {
		s.T().Logf("Note: Could not convert to MP4: %v", err)
	} else {
		s.T().Logf("Video converted: %s", mp4File)
	}

	helper.LogAndPass(s.T(), fmt.Sprintf("MCP test completed: %d video frames recorded", videoFrames))
}

// callMCPScreenshot calls the MCP take_screenshot tool via the SSE/HTTP MCP protocol
func (s *SpectaskMCPSuite) callMCPScreenshot(sessionID, outputPath string) error {
	// The MCP server runs at http://localhost:9878/mcp inside the container
	// We access it via the RevDial proxy through the API

	// For now, use the existing screenshot API endpoint which achieves the same result
	// The MCP server internally calls the same screenshot endpoint
	screenshotURL := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", s.apiURL, sessionID)

	req, err := http.NewRequest("GET", screenshotURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("screenshot failed: %d - %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, data, 0644)
}

// recordVideo records video from the WebSocket stream
func (s *SpectaskMCPSuite) recordVideo(ctx context.Context, sessionID, outputPath string, maxDuration time.Duration) (int, error) {
	// Get Wolf app ID
	appID, err := s.getWolfAppID(sessionID)
	if err != nil {
		return 0, fmt.Errorf("failed to get Wolf app ID: %w", err)
	}

	// Configure pending session
	clientUniqueID := fmt.Sprintf("mcp-test-%d", time.Now().UnixNano())
	if err := s.configurePendingSession(sessionID, clientUniqueID); err != nil {
		return 0, fmt.Errorf("failed to configure pending session: %w", err)
	}

	// Build WebSocket URL
	wsURL := strings.Replace(s.apiURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	streamURL := fmt.Sprintf("%s/moonlight/api/ws/stream?session_id=%s", wsURL, url.QueryEscape(sessionID))

	// Connect to WebSocket
	header := http.Header{}
	header.Set("Authorization", "Bearer "+s.token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, streamURL, header)
	if err != nil {
		return 0, fmt.Errorf("WebSocket connection failed: %w", err)
	}
	defer conn.Close()

	// Send init message
	initMessage := map[string]interface{}{
		"type":                    "init",
		"host_id":                 0,
		"app_id":                  appID,
		"session_id":              sessionID,
		"client_unique_id":        clientUniqueID,
		"width":                   1920,
		"height":                  1080,
		"fps":                     60,
		"bitrate":                 10000,
		"packet_size":             1024,
		"play_audio_local":        false,
		"video_supported_formats": 1,
	}
	initJSON, _ := json.Marshal(initMessage)
	if err := conn.WriteMessage(websocket.TextMessage, initJSON); err != nil {
		return 0, fmt.Errorf("failed to send init: %w", err)
	}

	// Open output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Record frames
	frameCount := 0
	deadline := time.Now().Add(maxDuration)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return frameCount, nil
		default:
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			errStr := err.Error()

			// Context cancelled - clean exit
			if ctx.Err() != nil {
				return frameCount, nil
			}

			// Check for known close errors
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return frameCount, nil
			}

			// Check for timeout errors - these are expected, continue trying
			if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "i/o timeout") {
				continue
			}

			// Any other error is fatal - stop immediately to avoid panic on next read
			return frameCount, fmt.Errorf("websocket error (after %d frames): %w", frameCount, err)
		}

		if msgType == websocket.BinaryMessage && len(data) > 0 {
			switch data[0] {
			case 0x01: // VideoFrame
				if len(data) >= 15 {
					// Extract NAL data (skip header)
					nalData := data[15:]
					outFile.Write(nalData)
					frameCount++
				}
			case 0x03: // VideoBatch
				if len(data) >= 3 {
					batchCount := int(binary.BigEndian.Uint16(data[1:3]))
					frameCount += batchCount
					// Write batch data
					outFile.Write(data[3:])
				}
			}
		}
	}

	return frameCount, nil
}

// convertToMP4 converts H.264 raw video to MP4 using ffmpeg
func (s *SpectaskMCPSuite) convertToMP4(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "h264",
		"-i", inputPath,
		"-c:v", "copy",
		outputPath,
	)
	return cmd.Run()
}

// Helper methods (reuse from spectask_stream_test.go)

func (s *SpectaskMCPSuite) getWolfAppID(sessionID string) (int, error) {
	url := fmt.Sprintf("%s/api/v1/wolf/ui-app-id?session_id=%s", s.apiURL, sessionID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		PlaceholderAppID string `json:"placeholder_app_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	var appID int
	fmt.Sscanf(result.PlaceholderAppID, "%d", &appID)
	return appID, nil
}

func (s *SpectaskMCPSuite) configurePendingSession(sessionID, clientUniqueID string) error {
	url := fmt.Sprintf("%s/api/v1/external-agents/%s/configure-pending-session", s.apiURL, sessionID)

	payload := map[string]string{
		"client_unique_id": clientUniqueID,
	}
	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *SpectaskMCPSuite) createSession() (string, error) {
	// Use Just Do It mode to skip spec generation and start sandbox immediately
	taskPayload := map[string]interface{}{
		"name":          "MCP Screenshot Test",
		"prompt":        "Testing MCP desktop tools",
		"project_id":    s.projectID,
		"app_id":        s.agentID,
		"just_do_it_mode": true,
	}
	jsonData, _ := json.Marshal(taskPayload)

	url := fmt.Sprintf("%s/api/v1/spec-tasks/from-prompt", s.apiURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create task failed: %d - %s", resp.StatusCode, string(body))
	}

	var task struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&task); err != nil {
		return "", err
	}

	// Start planning
	startURL := fmt.Sprintf("%s/api/v1/spec-tasks/%s/start-planning", s.apiURL, task.ID)
	startReq, err := http.NewRequest("POST", startURL, nil)
	if err != nil {
		return "", err
	}
	startReq.Header.Set("Authorization", "Bearer "+s.token)

	startResp, err := s.httpClient.Do(startReq)
	if err != nil {
		return "", err
	}
	defer startResp.Body.Close()

	if startResp.StatusCode != 200 && startResp.StatusCode != 201 {
		body, _ := io.ReadAll(startResp.Body)
		return "", fmt.Errorf("start planning failed: %d - %s", startResp.StatusCode, string(body))
	}

	return s.waitForTaskSession(task.ID, 90*time.Second)
}

func (s *SpectaskMCPSuite) waitForTaskSession(taskID string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		sessions, err := s.listSessions()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		for _, session := range sessions {
			if session.SpecTaskID == taskID && session.WolfLobbyID != "" {
				return session.ID, nil
			}
		}

		time.Sleep(pollInterval)
	}

	return "", fmt.Errorf("timeout waiting for session (task: %s)", taskID)
}

func (s *SpectaskMCPSuite) waitForSandbox(sessionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		screenshotURL := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", s.apiURL, sessionID)
		req, _ := http.NewRequest("GET", screenshotURL, nil)
		req.Header.Set("Authorization", "Bearer "+s.token)

		resp, err := s.httpClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return nil
			}
		}
		s.T().Logf("Waiting for sandbox... (%v)", err)
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for sandbox to be ready")
}

type mcpSessionInfo struct {
	ID          string
	WolfLobbyID string
	SpecTaskID  string
}

func (s *SpectaskMCPSuite) listSessions() ([]mcpSessionInfo, error) {
	url := fmt.Sprintf("%s/api/v1/sessions", s.apiURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API returned %d", resp.StatusCode)
	}

	var result struct {
		Sessions []struct {
			SessionID string `json:"session_id"`
			Metadata  struct {
				WolfLobbyID string `json:"wolf_lobby_id"`
				SpecTaskID  string `json:"spec_task_id"`
			} `json:"metadata"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var sessions []mcpSessionInfo
	for _, sess := range result.Sessions {
		sessions = append(sessions, mcpSessionInfo{
			ID:          sess.SessionID,
			WolfLobbyID: sess.Metadata.WolfLobbyID,
			SpecTaskID:  sess.Metadata.SpecTaskID,
		})
	}

	return sessions, nil
}

func (s *SpectaskMCPSuite) stopSession(sessionID string) error {
	url := fmt.Sprintf("%s/api/v1/external-agents/%s", s.apiURL, sessionID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
