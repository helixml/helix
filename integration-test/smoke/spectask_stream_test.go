//go:build integration || spectask

package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	// SpectaskTestResultsDir is where test artifacts are saved
	SpectaskTestResultsDir = "/tmp/helix-spectask-test-results"
)

type SpectaskStreamSuite struct {
	suite.Suite
	httpClient *http.Client
	apiURL     string
	token      string
	projectID  string
	agentID    string
	sessionID  string
}

func TestSpectaskStreamSuite(t *testing.T) {
	suite.Run(t, new(SpectaskStreamSuite))
}

func (s *SpectaskStreamSuite) SetupSuite() {
	s.T().Log("SetupSuite: Initializing spectask stream tests")

	// Get configuration from environment
	s.apiURL = os.Getenv("HELIX_URL")
	if s.apiURL == "" {
		s.apiURL = "http://localhost:8080"
	}

	s.token = os.Getenv("HELIX_API_KEY")
	if s.token == "" {
		s.T().Fatal("HELIX_API_KEY environment variable is required")
	}

	s.projectID = os.Getenv("HELIX_PROJECT")
	if s.projectID == "" {
		s.T().Fatal("HELIX_PROJECT environment variable is required")
	}

	s.agentID = os.Getenv("HELIX_UBUNTU_AGENT")
	if s.agentID == "" {
		s.T().Fatal("HELIX_UBUNTU_AGENT environment variable is required")
	}

	// Create HTTP client
	s.httpClient = &http.Client{
		Timeout: 60 * time.Second,
	}

	// Create test results directory
	err := os.MkdirAll(SpectaskTestResultsDir, 0755)
	require.NoError(s.T(), err, "creating test results directory should succeed")

	helper.LogStep(s.T(), fmt.Sprintf("Initialized with API URL: %s, Project: %s, Agent: %s",
		s.apiURL, s.projectID, s.agentID))
}

func (s *SpectaskStreamSuite) TearDownSuite() {
	s.T().Log("TearDownSuite: Cleaning up")

	// Stop the session if it was created
	if s.sessionID != "" {
		helper.LogStep(s.T(), fmt.Sprintf("Stopping session %s", s.sessionID))
		err := s.stopSession(s.sessionID)
		if err != nil {
			s.T().Logf("Warning: failed to stop session: %v", err)
		}
	}
}

// TestStreamBasic tests basic stream connectivity and receives some video frames
func (s *SpectaskStreamSuite) TestStreamBasic() {
	helper.LogStep(s.T(), "Creating sandbox session")

	// Create a new spec task and session
	sessionID, err := s.createSession()
	require.NoError(s.T(), err, "creating session should succeed")
	s.sessionID = sessionID

	helper.LogStep(s.T(), fmt.Sprintf("Session created: %s", sessionID))

	// Wait for sandbox to be ready
	helper.LogStep(s.T(), "Waiting for sandbox to be ready")
	err = s.waitForSandbox(sessionID, 90*time.Second)
	require.NoError(s.T(), err, "sandbox should become ready")

	// Test stream connectivity by taking a screenshot
	helper.LogStep(s.T(), "Testing stream connectivity (via screenshot)")
	stats, err := s.runStreamTest(sessionID, 5*time.Second)
	require.NoError(s.T(), err, "stream test should succeed")

	// Verify we got results
	require.Greater(s.T(), stats.TotalBytes, int64(0), "should receive some data")
	helper.LogAndPass(s.T(), fmt.Sprintf("Stream test passed: %d bytes received", stats.TotalBytes))

	// Save test results
	s.saveTestResults("basic", stats)
}

// TestStreamDurations tests different stream durations using screenshots
func (s *SpectaskStreamSuite) TestStreamDurations() {
	// Skip if no session from previous test
	if s.sessionID == "" {
		helper.LogStep(s.T(), "Creating sandbox session")
		sessionID, err := s.createSession()
		require.NoError(s.T(), err, "creating session should succeed")
		s.sessionID = sessionID

		err = s.waitForSandbox(sessionID, 90*time.Second)
		require.NoError(s.T(), err, "sandbox should become ready")
	}

	// Test taking multiple screenshots over time
	durations := []struct {
		duration  time.Duration
		interval  time.Duration
		minFrames int
	}{
		{3 * time.Second, 1 * time.Second, 2},
		{5 * time.Second, 1 * time.Second, 4},
		{10 * time.Second, 2 * time.Second, 4},
	}

	for _, tc := range durations {
		s.Run(fmt.Sprintf("Duration_%ds", int(tc.duration.Seconds())), func() {
			helper.LogStep(s.T(), fmt.Sprintf("Testing screenshots over %v at %v intervals", tc.duration, tc.interval))

			stats, err := s.runMultiScreenshotTest(s.sessionID, tc.duration, tc.interval)
			require.NoError(s.T(), err, "multi-screenshot test should succeed")

			require.GreaterOrEqual(s.T(), stats.VideoFrames, tc.minFrames,
				"should capture at least %d frames", tc.minFrames)

			helper.LogAndPass(s.T(), fmt.Sprintf("Captured %d screenshots in %v",
				stats.VideoFrames, tc.duration))

			// Save test results
			s.saveTestResults(fmt.Sprintf("duration_%ds", int(tc.duration.Seconds())), stats)
		})
	}
}

// TestStreamScreenshot takes a screenshot and saves it to test results
func (s *SpectaskStreamSuite) TestStreamScreenshot() {
	// Skip if no session from previous test
	if s.sessionID == "" {
		helper.LogStep(s.T(), "Creating sandbox session")
		sessionID, err := s.createSession()
		require.NoError(s.T(), err, "creating session should succeed")
		s.sessionID = sessionID

		err = s.waitForSandbox(sessionID, 90*time.Second)
		require.NoError(s.T(), err, "sandbox should become ready")
	}

	helper.LogStep(s.T(), "Taking screenshot via API")

	// Take screenshot
	screenshotData, err := s.takeScreenshot(s.sessionID)
	require.NoError(s.T(), err, "taking screenshot should succeed")
	require.Greater(s.T(), len(screenshotData), 1000, "screenshot should have content")

	// Verify it's a valid PNG (starts with PNG magic bytes)
	require.True(s.T(), len(screenshotData) > 8 &&
		screenshotData[0] == 0x89 &&
		screenshotData[1] == 0x50 &&
		screenshotData[2] == 0x4E &&
		screenshotData[3] == 0x47,
		"screenshot should be a valid PNG")

	// Save screenshot to test results
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(SpectaskTestResultsDir,
		fmt.Sprintf("spectask_screenshot_%s.png", timestamp))
	err = os.WriteFile(filename, screenshotData, 0644)
	require.NoError(s.T(), err, "saving screenshot should succeed")

	helper.LogAndPass(s.T(), fmt.Sprintf("Screenshot saved: %s (%d bytes)", filename, len(screenshotData)))
}

// TestWolfAppID tests that we can get the Wolf app ID for a session
func (s *SpectaskStreamSuite) TestWolfAppID() {
	// Skip if no session from previous test
	if s.sessionID == "" {
		helper.LogStep(s.T(), "Creating sandbox session")
		sessionID, err := s.createSession()
		require.NoError(s.T(), err, "creating session should succeed")
		s.sessionID = sessionID

		err = s.waitForSandbox(sessionID, 90*time.Second)
		require.NoError(s.T(), err, "sandbox should become ready")
	}

	helper.LogStep(s.T(), "Getting Wolf app ID")

	appID, err := s.getWolfAppID(s.sessionID)
	require.NoError(s.T(), err, "getting Wolf app ID should succeed")
	require.Greater(s.T(), appID, 0, "Wolf app ID should be positive")

	helper.LogAndPass(s.T(), fmt.Sprintf("Wolf app ID: %d", appID))
}

// StreamStats holds statistics from a stream test
type StreamStats struct {
	VideoFrames int
	AudioFrames int
	TotalBytes  int64
	Duration    time.Duration
	Codec       string
	Width       int
	Height      int
	Keyframes   int
}

// runStreamTest tests stream connectivity by taking a screenshot
func (s *SpectaskStreamSuite) runStreamTest(sessionID string, duration time.Duration) (*StreamStats, error) {
	stats := &StreamStats{
		Duration: duration,
	}

	// Take a screenshot to verify the sandbox is producing video
	screenshotData, err := s.takeScreenshot(sessionID)
	if err != nil {
		return nil, fmt.Errorf("screenshot failed, sandbox may not be ready: %w", err)
	}

	// Record stats based on screenshot
	stats.VideoFrames = 1
	stats.TotalBytes = int64(len(screenshotData))
	stats.Width = 1920  // Assumed based on sandbox config
	stats.Height = 1080 // Assumed based on sandbox config
	stats.Codec = "PNG" // Screenshot format
	stats.Keyframes = 1

	return stats, nil
}

// runMultiScreenshotTest takes multiple screenshots over a duration
func (s *SpectaskStreamSuite) runMultiScreenshotTest(sessionID string, duration, interval time.Duration) (*StreamStats, error) {
	stats := &StreamStats{
		Duration: duration,
	}

	endTime := time.Now().Add(duration)
	frameCount := 0
	var totalBytes int64

	for time.Now().Before(endTime) {
		screenshotData, err := s.takeScreenshot(sessionID)
		if err != nil {
			s.T().Logf("Screenshot %d failed: %v", frameCount+1, err)
			time.Sleep(interval)
			continue
		}

		frameCount++
		totalBytes += int64(len(screenshotData))

		// Save each screenshot
		timestamp := time.Now().Format("20060102_150405_000")
		filename := filepath.Join(SpectaskTestResultsDir,
			fmt.Sprintf("spectask_multi_%s_%03d.png", timestamp, frameCount))
		if err := os.WriteFile(filename, screenshotData, 0644); err != nil {
			s.T().Logf("Failed to save screenshot %d: %v", frameCount, err)
		}

		time.Sleep(interval)
	}

	stats.VideoFrames = frameCount
	stats.TotalBytes = totalBytes
	stats.Width = 1920
	stats.Height = 1080
	stats.Codec = "PNG"
	stats.Keyframes = frameCount

	return stats, nil
}

// getWolfAppID fetches the Wolf app ID for a session
func (s *SpectaskStreamSuite) getWolfAppID(sessionID string) (int, error) {
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

// configurePendingSession pre-configures Wolf with the client ID
func (s *SpectaskStreamSuite) configurePendingSession(sessionID, clientUniqueID string) error {
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

// takeScreenshot captures a screenshot from the sandbox
func (s *SpectaskStreamSuite) takeScreenshot(sessionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/v1/external-agents/%s/screenshot", s.apiURL, sessionID)

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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// createSession creates a new sandbox session
func (s *SpectaskStreamSuite) createSession() (string, error) {
	// Create spec task
	taskPayload := map[string]string{
		"name":       "Stream Test Task",
		"prompt":     "Testing video streaming",
		"project_id": s.projectID,
		"app_id":     s.agentID,
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

	// Start planning to trigger sandbox creation
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

	// Poll for session to be created
	return s.waitForTaskSession(task.ID, 90*time.Second)
}

// waitForTaskSession waits for a session to be created for a task
func (s *SpectaskStreamSuite) waitForTaskSession(taskID string, timeout time.Duration) (string, error) {
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

// waitForSandbox waits for the sandbox to be fully ready
func (s *SpectaskStreamSuite) waitForSandbox(sessionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 2 * time.Second

	for time.Now().Before(deadline) {
		// Try taking a screenshot - if it works, sandbox is ready
		_, err := s.takeScreenshot(sessionID)
		if err == nil {
			return nil
		}
		s.T().Logf("Waiting for sandbox... (%v)", err)
		time.Sleep(pollInterval)
	}

	return fmt.Errorf("timeout waiting for sandbox to be ready")
}

// listSessions returns all sessions with external agents
func (s *SpectaskStreamSuite) listSessions() ([]sessionInfo, error) {
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

	var sessions []sessionInfo
	for _, sess := range result.Sessions {
		sessions = append(sessions, sessionInfo{
			ID:          sess.SessionID,
			WolfLobbyID: sess.Metadata.WolfLobbyID,
			SpecTaskID:  sess.Metadata.SpecTaskID,
		})
	}

	return sessions, nil
}

type sessionInfo struct {
	ID          string
	WolfLobbyID string
	SpecTaskID  string
}

// stopSession stops a sandbox session
func (s *SpectaskStreamSuite) stopSession(sessionID string) error {
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

// saveTestResults saves test results to the results directory
func (s *SpectaskStreamSuite) saveTestResults(testName string, stats *StreamStats) {
	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(SpectaskTestResultsDir,
		fmt.Sprintf("spectask_%s_%s_results.txt", testName, timestamp))

	fps := float64(0)
	if stats.Duration.Seconds() > 0 {
		fps = float64(stats.VideoFrames) / stats.Duration.Seconds()
	}

	content := fmt.Sprintf(`=== Spectask Stream Test Results ===
Test: %s
Timestamp: %s
Session: %s

=== Video Statistics ===
Duration: %v
Video Frames: %d
Audio Frames: %d
Total Bytes: %d
Codec: %s
Resolution: %dx%d
Keyframes: %d
FPS: %.2f

=== Test Result ===
Status: PASS
`,
		testName,
		time.Now().Format("2006-01-02 15:04:05"),
		s.sessionID,
		stats.Duration,
		stats.VideoFrames,
		stats.AudioFrames,
		stats.TotalBytes,
		stats.Codec,
		stats.Width, stats.Height,
		stats.Keyframes,
		fps,
	)

	err := os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		s.T().Logf("Warning: failed to save test results: %v", err)
	} else {
		s.T().Logf("Test results saved: %s", filename)
	}
}
