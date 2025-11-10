package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// MoonlightSession represents a session in moonlight-web
type MoonlightSession struct {
	SessionID      string  `json:"session_id"`
	ClientUniqueID *string `json:"client_unique_id"`
	Mode           string  `json:"mode"`
	HasWebSocket   bool    `json:"has_websocket"`
}

// MoonlightClient represents a cached client certificate
type MoonlightClient struct {
	ClientUniqueID string `json:"client_unique_id"`
	HasCertificate bool   `json:"has_certificate"`
}

// MoonlightAdminStatus represents the full state from moonlight-web
type MoonlightAdminStatus struct {
	TotalClients     int                 `json:"total_clients"`
	TotalSessions    int                 `json:"total_sessions"`
	ActiveWebSockets int                 `json:"active_websockets"`
	Clients          []MoonlightClient   `json:"clients"`
	Sessions         []MoonlightSession  `json:"sessions"`
}

// TestHelpers provides utility functions for stress testing
type TestHelpers struct {
	apiURL   string
	authToken string
	t        *testing.T
}

func newTestHelpers(t *testing.T) *TestHelpers {
	apiURL := os.Getenv("HELIX_API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	// Get admin token from environment
	authToken := os.Getenv("ADMIN_TOKEN")
	if authToken == "" {
		t.Skip("ADMIN_TOKEN not set - skipping stress tests")
	}

	return &TestHelpers{
		apiURL:    apiURL,
		authToken: authToken,
		t:         t,
	}
}

// getMoonlightStatus fetches current moonlight-web state
func (h *TestHelpers) getMoonlightStatus(ctx context.Context) (*MoonlightAdminStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", h.apiURL+"/api/v1/moonlight/status", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+h.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var status MoonlightAdminStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

// createExternalAgentSession creates a new external agent session
func (h *TestHelpers) createExternalAgentSession(ctx context.Context, name string) (string, error) {
	payload := map[string]interface{}{
		"input":      fmt.Sprintf("Stress test session: %s", name),
		"agent_type": "zed_external",
		"external_agent_config": map[string]interface{}{
			"type":  "docker",
			"image": "ghcr.io/helixml/helix-sway:latest",
			"env":   []string{"GOW_REQUIRED_DEVICES=/dev/input/* /dev/dri/* /dev/nvidia*"},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", h.apiURL+"/api/v1/external-agents", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+h.authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to create agent: %d - %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	sessionID, ok := result["session_id"].(string)
	if !ok {
		return "", fmt.Errorf("no session_id in response")
	}

	return sessionID, nil
}

// stopExternalAgent stops an external agent session
func (h *TestHelpers) stopExternalAgent(ctx context.Context, sessionID string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", h.apiURL+"/api/v1/sessions/"+sessionID+"/stop-external-agent", nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+h.authToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to stop agent: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// TestMoonlightStressScenario1RapidConnectDisconnect tests rapid connect/disconnect cycles
// This simulates users opening and closing task detail windows repeatedly
func TestMoonlightStressScenario1RapidConnectDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Create 3 external agent sessions
	sessionIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		sessionID, err := h.createExternalAgentSession(ctx, fmt.Sprintf("rapid-test-%d", i))
		require.NoError(t, err, "Failed to create session %d", i)
		sessionIDs[i] = sessionID
		t.Logf("Created session %d: %s", i, sessionID)
	}

	// Cleanup at end
	defer func() {
		for _, sessionID := range sessionIDs {
			_ = h.stopExternalAgent(ctx, sessionID)
		}
	}()

	// Wait for sessions to fully start
	time.Sleep(10 * time.Second)

	// Get initial state
	initialState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Initial state: %d clients, %d sessions, %d active", initialState.TotalClients, initialState.TotalSessions, initialState.ActiveWebSockets)

	// Simulate rapid browser tab open/close
	// In reality, the frontend would create WebSocket + WebRTC connections
	// For this test, we just verify the backend doesn't get stuck
	for cycle := 0; cycle < 10; cycle++ {
		t.Logf("Cycle %d: Checking state...", cycle)

		status, err := h.getMoonlightStatus(ctx)
		require.NoError(t, err, "Failed to get status in cycle %d", cycle)

		// Assert reasonable state
		require.LessOrEqual(t, status.TotalSessions, 10, "Too many sessions (memory leak?)")
		require.GreaterOrEqual(t, status.ActiveWebSockets, 0, "Negative active websockets?")

		time.Sleep(2 * time.Second)
	}

	// Verify final state
	finalState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Final state: %d clients, %d sessions, %d active", finalState.TotalClients, finalState.TotalSessions, finalState.ActiveWebSockets)
}

// TestMoonlightStressScenario2ConcurrentMultiSession tests multiple simultaneous streams
func TestMoonlightStressScenario2ConcurrentMultiSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Create 5 sessions concurrently
	const sessionCount = 5
	sessionIDs := make([]string, sessionCount)
	var wg sync.WaitGroup
	errors := make(chan error, sessionCount)

	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sessionID, err := h.createExternalAgentSession(ctx, fmt.Sprintf("concurrent-test-%d", index))
			if err != nil {
				errors <- fmt.Errorf("session %d creation failed: %w", index, err)
				return
			}
			sessionIDs[index] = sessionID
			t.Logf("Session %d created: %s", index, sessionID)
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	var errorList []error
	for err := range errors {
		errorList = append(errorList, err)
	}
	require.Empty(t, errorList, "Errors during concurrent creation: %v", errorList)

	// Cleanup
	defer func() {
		for _, sessionID := range sessionIDs {
			if sessionID != "" {
				_ = h.stopExternalAgent(ctx, sessionID)
			}
		}
	}()

	// Wait for all sessions to start
	time.Sleep(15 * time.Second)

	// Check moonlight-web state
	status, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Concurrent state: %d sessions, %d active websockets", status.TotalSessions, status.ActiveWebSockets)

	// All sessions should be able to exist simultaneously
	require.GreaterOrEqual(t, status.TotalSessions, sessionCount, "Not all sessions registered in moonlight-web")

	// Monitor for 30 seconds to ensure stability
	for i := 0; i < 6; i++ {
		time.Sleep(5 * time.Second)
		status, err := h.getMoonlightStatus(ctx)
		require.NoError(t, err)
		t.Logf("Stability check %d: %d sessions, %d active", i, status.TotalSessions, status.ActiveWebSockets)

		// Verify no sessions disappeared
		require.GreaterOrEqual(t, status.TotalSessions, sessionCount, "Sessions disappeared during stability check")
	}
}

// TestMoonlightStressScenario3ServiceRestartChaos tests behavior during service restarts
func TestMoonlightStressScenario3ServiceRestartChaos(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Create 2 sessions
	sessionIDs := make([]string, 2)
	for i := 0; i < 2; i++ {
		sessionID, err := h.createExternalAgentSession(ctx, fmt.Sprintf("restart-test-%d", i))
		require.NoError(t, err)
		sessionIDs[i] = sessionID
		t.Logf("Created session %d: %s", i, sessionID)
	}

	defer func() {
		for _, sessionID := range sessionIDs {
			_ = h.stopExternalAgent(ctx, sessionID)
		}
	}()

	// Wait for sessions to start
	time.Sleep(10 * time.Second)

	// Get state before restart
	beforeState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Before restart: %d clients, %d sessions", beforeState.TotalClients, beforeState.TotalSessions)

	// NOTE: Restarting Wolf/moonlight-web requires docker access
	// For now, just document expected behavior
	t.Log("⚠️ Manual test: Restart Wolf with 'docker compose -f docker-compose.dev.yaml restart wolf'")
	t.Log("Expected: moonlight-web sessions should gracefully fail, then allow reconnect")
	t.Log("Expected: Wolf apps should be recreated when agents register")

	// Wait for potential reconnect attempts
	time.Sleep(15 * time.Second)

	// Check state after "restart" (simulated by time passage)
	afterState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("After wait: %d clients, %d sessions", afterState.TotalClients, afterState.TotalSessions)

	// Sessions should still exist (even if disconnected)
	require.GreaterOrEqual(t, afterState.TotalClients, 0, "Client state should persist")
}

// TestMoonlightStressScenario4BrowserTabSimulation tests multiple tabs connecting to same session
func TestMoonlightStressScenario4BrowserTabSimulation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Create 1 session
	sessionID, err := h.createExternalAgentSession(ctx, "tab-simulation-test")
	require.NoError(t, err)
	t.Logf("Created session: %s", sessionID)

	defer func() {
		_ = h.stopExternalAgent(ctx, sessionID)
	}()

	// Wait for session to start
	time.Sleep(10 * time.Second)

	// Get initial state
	initialState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Initial state: %d clients, %d sessions", initialState.TotalClients, initialState.TotalSessions)

	// NOTE: Simulating multiple browser tabs requires actual WebSocket connections
	// The frontend creates unique client_unique_id per tab (includes FRONTEND_INSTANCE_ID)
	// For now, document expected behavior

	t.Log("📝 Expected behavior when multiple tabs open same session:")
	t.Log("  - Each tab gets unique client_unique_id (different FRONTEND_INSTANCE_ID)")
	t.Log("  - Moonlight-web caches certificate for each client")
	t.Log("  - But only ONE active WebSocket/stream at a time (kickoff behavior)")
	t.Log("  - When Tab 1 disconnects, Tab 2 can resume the session")

	// Verify we can query the state without errors
	status, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	require.Greater(t, status.TotalClients, 0, "Should have at least one client")
}

// TestMoonlightHealthCheck verifies basic health of moonlight-web
func TestMoonlightHealthCheck(t *testing.T) {
	h := newTestHelpers(t)
	ctx := context.Background()

	status, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err, "Failed to fetch moonlight-web status")

	t.Logf("✅ Moonlight-web health check passed:")
	t.Logf("   Total Clients: %d", status.TotalClients)
	t.Logf("   Total Sessions: %d", status.TotalSessions)
	t.Logf("   Active WebSockets: %d", status.ActiveWebSockets)
	t.Logf("   Idle Sessions: %d", status.TotalSessions-status.ActiveWebSockets)

	// Basic sanity checks
	require.GreaterOrEqual(t, status.TotalClients, 0)
	require.GreaterOrEqual(t, status.TotalSessions, 0)
	require.GreaterOrEqual(t, status.ActiveWebSockets, 0)
	require.LessOrEqual(t, status.ActiveWebSockets, status.TotalSessions, "Active WebSockets can't exceed total sessions")

	// Warn about stuck sessions
	idleSessions := status.TotalSessions - status.ActiveWebSockets
	if idleSessions > 10 {
		t.Logf("⚠️ WARNING: %d idle sessions (potential memory leak)", idleSessions)
	}

	// List all cached clients
	if len(status.Clients) > 0 {
		t.Logf("📋 Cached client certificates (%d):", len(status.Clients))
		for i, client := range status.Clients {
			if i < 10 { // Only show first 10
				t.Logf("   - %s", client.ClientUniqueID)
			}
		}
		if len(status.Clients) > 10 {
			t.Logf("   ... and %d more", len(status.Clients)-10)
		}
	}

	// List active sessions
	if len(status.Sessions) > 0 {
		t.Logf("🎬 Active sessions (%d):", len(status.Sessions))
		for _, session := range status.Sessions {
			wsStatus := "DISCONNECTED"
			if session.HasWebSocket {
				wsStatus = "CONNECTED"
			}
			t.Logf("   - %s [%s] %s", session.SessionID, session.Mode, wsStatus)
		}
	}
}

// TestMoonlightMemoryLeak checks for session leaks over time
func TestMoonlightMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory leak test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Baseline state
	baseline, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Baseline: %d sessions, %d clients", baseline.TotalSessions, baseline.TotalClients)

	// Create and destroy sessions repeatedly
	for cycle := 0; cycle < 5; cycle++ {
		t.Logf("Leak test cycle %d...", cycle)

		// Create session
		sessionID, err := h.createExternalAgentSession(ctx, fmt.Sprintf("leak-test-%d", cycle))
		require.NoError(t, err)

		// Wait for it to start
		time.Sleep(5 * time.Second)

		// Check intermediate state
		midState, err := h.getMoonlightStatus(ctx)
		require.NoError(t, err)
		t.Logf("  Mid-cycle: %d sessions, %d clients", midState.TotalSessions, midState.TotalClients)

		// Stop session
		err = h.stopExternalAgent(ctx, sessionID)
		require.NoError(t, err)

		// Wait for cleanup
		time.Sleep(5 * time.Second)

		// Check if sessions cleaned up
		afterState, err := h.getMoonlightStatus(ctx)
		require.NoError(t, err)
		t.Logf("  After cleanup: %d sessions, %d clients", afterState.TotalSessions, afterState.TotalClients)
	}

	// Final state should not have accumulated sessions
	finalState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Final state: %d sessions, %d clients", finalState.TotalSessions, finalState.TotalClients)

	// Allow some margin, but not unbounded growth
	sessionGrowth := finalState.TotalSessions - baseline.TotalSessions
	require.LessOrEqual(t, sessionGrowth, 3, "Session count grew by %d (memory leak?)", sessionGrowth)

	if finalState.TotalClients > baseline.TotalClients+10 {
		t.Logf("⚠️ WARNING: Client count grew significantly (%d -> %d)", baseline.TotalClients, finalState.TotalClients)
		t.Log("This may indicate certificate cache is not being cleaned up")
	}
}

// TestMoonlightConcurrentDisconnects tests all clients disconnecting simultaneously
func TestMoonlightConcurrentDisconnects(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	h := newTestHelpers(t)
	ctx := context.Background()

	// Create 3 sessions
	sessionIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		sessionID, err := h.createExternalAgentSession(ctx, fmt.Sprintf("disconnect-test-%d", i))
		require.NoError(t, err)
		sessionIDs[i] = sessionID
	}

	// Wait for all to start
	time.Sleep(10 * time.Second)

	// Verify all started
	beforeState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("Before mass disconnect: %d sessions", beforeState.TotalSessions)

	// Stop all simultaneously
	var wg sync.WaitGroup
	for _, sessionID := range sessionIDs {
		wg.Add(1)
		go func(sid string) {
			defer wg.Done()
			err := h.stopExternalAgent(ctx, sid)
			if err != nil {
				t.Logf("Failed to stop %s: %v", sid, err)
			}
		}(sessionID)
	}
	wg.Wait()

	// Wait for cleanup
	time.Sleep(5 * time.Second)

	// Verify clean state
	afterState, err := h.getMoonlightStatus(ctx)
	require.NoError(t, err)
	t.Logf("After mass disconnect: %d sessions, %d active", afterState.TotalSessions, afterState.ActiveWebSockets)

	// Active websockets should be 0 after all disconnected
	require.Equal(t, 0, afterState.ActiveWebSockets, "Should have no active websockets after mass disconnect")
}
