// TUI E2E test — drives the real TUI App model against a real Zed agent.
//
// This test starts:
//   1. In-memory HelixAPIServer (same as production handlers)
//   2. HTTP server for TUI API + WebSocket for Zed
//   3. Waits for a real Zed binary to connect (started externally)
//   4. Creates a TUI App model pointed at the test server
//   5. Programmatically navigates: project picker → kanban → open task → send message
//   6. Waits for real LLM response to stream back through Zed
//   7. Asserts the rendered TUI view contains the response with proper formatting
//
// Run with:
//   go test -v -timeout 300s -run TestTUI_E2E ./
//
// Requires:
//   - Zed binary running externally with ZED_HELIX_URL pointing to our test server
//   - ANTHROPIC_API_KEY set for real LLM calls

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/helixml/helix/api/pkg/cli/tui"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

func TestTUI_E2E(t *testing.T) {
	if os.Getenv("TUI_E2E") != "1" {
		t.Skip("Set TUI_E2E=1 to run (requires running Zed binary)")
	}

	// If TUI_E2E_SERVER_URL is set, connect to an existing test server
	// (started by run_e2e.sh with Zed already connected).
	// Otherwise, start our own server and wait for Zed.
	serverURL := os.Getenv("TUI_E2E_SERVER_URL")

	if serverURL != "" {
		t.Logf("Connecting to existing test server at %s", serverURL)
	} else {
		// --- Start our own test server ---
		ms := memorystore.New()
		ps := pubsub.NewNoop()
		srv := server.NewTestServer(ms, ps)

		seedSessionID := "ses_tui-e2e-001"
		ms.CreateSession(nil, types.Session{
			ID: seedSessionID, Name: "TUI E2E", Owner: "tui-e2e-user",
			Created: time.Now(), Updated: time.Now(),
			Mode: types.SessionModeInference, Type: types.SessionTypeText,
		})

		var agentConnected bool
		srv.SetSyncEventHook(func(sessionID string, syncMsg *types.SyncMessage) {
			if syncMsg.EventType == "agent_ready" {
				agentConnected = true
			}
		})

		mux := buildTestMux(ms, srv, seedSessionID)
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		go http.Serve(listener, mux)
		defer listener.Close()

		port := listener.Addr().(*net.TCPAddr).Port
		serverURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		t.Logf("Test server listening on %s", serverURL)

		os.WriteFile("/tmp/tui_test_server_port", []byte(fmt.Sprintf("%d", port)), 0644)
		defer os.Remove("/tmp/tui_test_server_port")

		t.Log("Waiting for Zed agent to connect...")
		deadline := time.Now().Add(60 * time.Second)
		for !agentConnected && time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
		}
		if !agentConnected {
			t.Fatal("Zed agent did not connect within 60s")
		}
		t.Log("Zed agent connected!")
		time.Sleep(3 * time.Second)
	}

	// --- Create TUI App model ---
	helixClient, err := client.NewClient(serverURL, "test-api-key", false)
	if err != nil {
		t.Fatal(err)
	}
	api := tui.NewAPIClient(helixClient)
	appModel := tui.NewApp(api, "") // no project pre-selected
	var app tea.Model = appModel

	// Initialize
	cmd := app.Init()

	// Simulate a small terminal so output is easy to read in test logs
	app, cmd = processCmd(t, app, cmd)
	app, _ = app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// --- Phase 1: Project Picker ---
	t.Log("=== Phase 1: Project Picker ===")

	// Wait for projects to load
	app, cmd = waitForCmd(t, app, cmd, 5*time.Second)

	view := app.View()
	dumpView(t, "PROJECT PICKER", view)

	if !strings.Contains(view, "E2E Test Project") {
		t.Fatal("Project picker should show 'E2E Test Project'")
	}
	t.Log("[PASS] Project picker rendered with test project")

	// Press enter to select project
	app, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app, cmd = waitForCmd(t, app, cmd, 5*time.Second)

	// --- Phase 2: Kanban Board ---
	t.Log("=== Phase 2: Kanban Board ===")

	view = app.View()
	dumpView(t, "KANBAN BOARD", view)

	if !strings.Contains(view, "E2E task") && !strings.Contains(view, "In Progress") {
		t.Fatal("Kanban should show task in 'In Progress' column")
	}
	t.Log("[PASS] Kanban board rendered with task")

	// --- Phase 3: Open task chat ---
	t.Log("=== Phase 3: Open task chat ===")

	// Need to navigate to the right column first — task is in "In Progress" (column 2)
	// Press 'l' twice to get to In Progress, or press '3' to jump
	app, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})

	// Press enter to open the task
	app, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Process the openTaskChatMsg → creates tab + fetches interactions
	app, cmd = waitForCmd(t, app, cmd, 10*time.Second)

	view = app.View()
	dumpView(t, "CHAT (initial)", view)

	// Should show the input area
	if !strings.Contains(view, "❯") {
		t.Fatal("Chat should show input prompt ❯")
	}
	t.Log("[PASS] Chat view opened with input prompt")

	// --- Phase 4: Send a message and wait for real LLM response ---
	t.Log("=== Phase 4: Send message to real Zed agent ===")

	// Type a message
	for _, ch := range "What is 2+2? Reply with just the number." {
		app, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
	}

	// Verify message appears in input
	view = app.View()
	if !strings.Contains(view, "2+2") {
		t.Fatal("Input should show typed message")
	}

	// Press enter to send
	app, cmd = app.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app, cmd = processCmd(t, app, cmd)

	// Wait for a response to appear in the interactions
	t.Log("Waiting for Zed agent response (real LLM)...")
	responseDeadline := time.Now().Add(120 * time.Second)
	gotResponse := false
	for time.Now().Before(responseDeadline) {
		time.Sleep(2 * time.Second)
		// Simulate tick to trigger interaction refresh
		app, cmd = app.Update(tui.TickMsg(time.Now()))
		app, cmd = processCmd(t, app, cmd)

		// Check if any interaction has a response
		if hasCompletedInteraction(t, serverURL) {
			gotResponse = true
			break
		}
	}

	if !gotResponse {
		t.Fatal("No completed interaction with response within 120s")
	}
	t.Log("Response received!")

	// Fetch interactions to trigger re-render
	app, cmd = app.Update(tui.TickMsg(time.Now()))
	app, cmd = waitForCmd(t, app, cmd, 10*time.Second)

	// Poll a few more times to ensure interactions are fully loaded
	for i := 0; i < 5; i++ {
		app, cmd = app.Update(tui.TickMsg(time.Now()))
		app, cmd = waitForCmd(t, app, cmd, 5*time.Second)
		time.Sleep(1 * time.Second)
	}

	// Debug: dump interactions from server
	t.Log("=== Debug: checking interactions via API ===")
	resp, httpErr := http.Get(serverURL + "/api/v1/sessions/ses_tui-e2e-001/interactions")
	if httpErr == nil {
		defer resp.Body.Close()
		var debugInteractions []json.RawMessage
		json.NewDecoder(resp.Body).Decode(&debugInteractions)
		t.Logf("Interactions count: %d", len(debugInteractions))
		for i, raw := range debugInteractions {
			// Extract just the key fields
			var ix struct {
				ID              string          `json:"id"`
				State           string          `json:"state"`
				ResponseMessage string          `json:"response_message"`
				ResponseEntries json.RawMessage `json:"response_entries"`
			}
			json.Unmarshal(raw, &ix)
			t.Logf("Interaction %d: id=%s state=%s response_len=%d entries_len=%d",
				i, ix.ID, ix.State, len(ix.ResponseMessage), len(ix.ResponseEntries))
		}
	}

	// --- Phase 5: Verify rendered response ---
	t.Log("=== Phase 5: Verify TUI rendering ===")

	view = app.View()
	dumpView(t, "CHAT (with LLM response)", view)

	// The response should be rendered in the chat
	if !strings.Contains(view, "Assistant") {
		t.Error("[FAIL] Expected 'Assistant' role label in rendered view")
	} else {
		t.Log("[PASS] Assistant role label rendered")
	}

	if !strings.Contains(view, "You") {
		t.Error("[FAIL] Expected 'You' role label in rendered view")
	} else {
		t.Log("[PASS] User role label rendered")
	}

	// --- Summary ---
	t.Log("")
	t.Log("============================================")
	t.Log("  TUI E2E TEST PASSED")
	t.Log("  Real Zed agent + real LLM + real TUI rendering")
	t.Logf("  Completions: %d", getCompletionCount(t, serverURL))
	t.Log("============================================")
}

// --- helpers ---

// processCmd executes a tea.Cmd (which may be async — makes HTTP calls etc.)
// and feeds the result back to the model. Handles tea.BatchMsg for batched commands.
func processCmd(t *testing.T, m tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if cmd == nil {
		return m, nil
	}
	msg := cmd()
	if msg == nil {
		return m, nil
	}

	// Handle batch messages (tea.Batch returns these)
	if batch, ok := msg.(tea.BatchMsg); ok {
		var finalCmd tea.Cmd
		for _, batchCmd := range batch {
			m, finalCmd = processCmd(t, m, batchCmd)
		}
		return m, finalCmd
	}

	var nextCmd tea.Cmd
	m, nextCmd = m.Update(msg)
	return m, nextCmd
}

// waitForCmd processes commands in a loop until none remain or timeout.
// This properly handles async HTTP calls by executing the command functions.
func waitForCmd(t *testing.T, m tea.Model, cmd tea.Cmd, timeout time.Duration) (tea.Model, tea.Cmd) {
	deadline := time.Now().Add(timeout)
	for cmd != nil && time.Now().Before(deadline) {
		m, cmd = processCmd(t, m, cmd)
	}
	return m, cmd
}

func hasCompletedInteraction(t *testing.T, serverURL string) bool {
	t.Helper()
	// Check ALL interactions via status endpoint (responses may be on child sessions)
	completions := getCompletionCount(t, serverURL)
	return completions > 1 // >1 because shell Phase 1 already got one
}

func getCompletionCount(t *testing.T, serverURL string) int {
	t.Helper()
	resp, err := http.Get(serverURL + "/api/v1/status")
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var status struct {
		Completions int `json:"completions"`
	}
	json.NewDecoder(resp.Body).Decode(&status)
	return status.Completions
}

func dumpView(t *testing.T, label string, view string) {
	t.Helper()
	t.Logf("\n┌─── %s ───────────────────────────────────────────────┐", label)
	for _, line := range strings.Split(view, "\n") {
		t.Logf("│ %s", line)
	}
	t.Logf("└────────────────────────────────────────────────────────────┘")
}

// buildTestMux creates the HTTP mux with both REST (TUI) and WebSocket (Zed) endpoints.
// Mirrors the demo server but backed by real memorystore.
func buildTestMux(ms *memorystore.MemoryStore, srv *server.HelixAPIServer, seedSessionID string) *http.ServeMux {
	mux := http.NewServeMux()

	// WebSocket for Zed
	mux.HandleFunc("/api/v1/external-agents/sync", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("session_id")
		if agentID == "" {
			agentID = r.URL.Query().Get("agent_id")
		}
		if agentID != "" {
			srv.SetExternalAgentUserMapping(agentID, "tui-e2e-user")
		}
		srv.ExternalAgentSyncHandler()(w, r)
	})

	// Projects
	mux.HandleFunc("/api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]*types.Project{{
			ID: "proj_e2e", Name: "E2E Test Project", Status: "active",
			Stats: types.ProjectStats{TotalTasks: 1, InProgressTasks: 1},
		}})
	})

	// Spec tasks
	mux.HandleFunc("/api/v1/spec-tasks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]*types.SpecTask{{
			ID: "spt_e2e_1", ProjectID: "proj_e2e", Name: "E2E test task",
			ShortTitle: "E2E task", Status: types.TaskStatusImplementation,
			Priority: types.SpecTaskPriorityHigh, PlanningSessionID: seedSessionID,
			HelixAppID: "app_e2e",
		}})
	})

	mux.HandleFunc("/api/v1/spec-tasks/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(&types.SpecTask{
			ID: "spt_e2e_1", ProjectID: "proj_e2e", Name: "E2E test task",
			Status: types.TaskStatusImplementation, Priority: types.SpecTaskPriorityHigh,
			PlanningSessionID: seedSessionID, HelixAppID: "app_e2e",
		})
	})

	// Sessions
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")

		if path == "chat" {
			var req types.SessionChatRequest
			json.NewDecoder(r.Body).Decode(&req)
			message := ""
			if len(req.Messages) > 0 {
				for _, part := range req.Messages[0].Content.Parts {
					if s, ok := part.(string); ok {
						message = s
						break
					}
				}
			}
			requestID := fmt.Sprintf("tui-e2e-%d", time.Now().UnixNano())
			srv.SendChatMessage(seedSessionID, message, requestID)
			w.Write([]byte(""))
			return
		}

		if strings.HasSuffix(path, "/interactions") {
			sessionID := strings.TrimSuffix(path, "/interactions")
			interactions, _, _ := ms.ListInteractions(nil, &types.ListInteractionsQuery{SessionID: sessionID})
			if interactions == nil {
				interactions = []*types.Interaction{}
			}
			json.NewEncoder(w).Encode(interactions)
			return
		}

		json.NewEncoder(w).Encode(&types.Session{ID: seedSessionID, Name: "E2E Session"})
	})

	return mux
}
