// tui-test-server runs a real HelixAPIServer with in-memory store, serving both
// the WebSocket sync endpoint (for Zed) and REST API endpoints (for the TUI).
//
// This lets us test the full flow:
//   TUI --HTTP--> test-server --WebSocket--> Zed --LLM--> response --WebSocket--> test-server --HTTP--> TUI
//
// The same production code paths run in tests and production.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
)

type testState struct {
	mu          sync.Mutex
	store       *memorystore.MemoryStore
	srv         *server.HelixAPIServer
	agentID     string
	events      []types.SyncMessage
	completions map[string]int // threadID -> completion count
	threadIDs   []string

	// activeSessionID is the child session where the Zed thread lives.
	// After thread_created, this is the session the TUI should query.
	activeSessionID string
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	log.Println("============================================")
	log.Println("  TUI E2E Test Server")
	log.Println("  Real HelixAPIServer + memorystore")
	log.Println("  REST API for TUI + WebSocket for Zed")
	log.Println("============================================")

	// Create in-memory store and pubsub
	ms := memorystore.New()
	ps := pubsub.NewNoop()

	// Seed session for Zed to connect to
	seedSessionID := os.Getenv("HELIX_SESSION_ID")
	if seedSessionID == "" {
		seedSessionID = "ses_tui-e2e-001"
	}
	seedSession := types.Session{
		ID:      seedSessionID,
		Name:    "TUI E2E Test Session",
		Created: time.Now(),
		Updated: time.Now(),
		Owner:   "tui-e2e-user",
		Mode:    types.SessionModeInference,
		Type:    types.SessionTypeText,
	}
	if _, err := ms.CreateSession(context.Background(), seedSession); err != nil {
		log.Fatalf("Failed to create seed session: %v", err)
	}

	// Create test server with production handlers
	srv := server.NewTestServer(ms, ps)

	state := &testState{
		store:       ms,
		srv:         srv,
		completions: make(map[string]int),
	}

	// Register sync event hook for logging
	srv.SetSyncEventHook(func(sessionID string, syncMsg *types.SyncMessage) {
		state.mu.Lock()
		defer state.mu.Unlock()
		state.events = append(state.events, *syncMsg)

		switch syncMsg.EventType {
		case "agent_ready":
			log.Printf("[event] Agent ready on session %s", sessionID)
		case "thread_created":
			tid, _ := syncMsg.Data["acp_thread_id"].(string)
			state.threadIDs = append(state.threadIDs, tid)
			log.Printf("[event] Thread created: %s", truncate(tid, 16))
		case "message_added":
			role, _ := syncMsg.Data["role"].(string)
			log.Printf("[event] Message added: role=%s", role)
		case "message_completed":
			tid, _ := syncMsg.Data["acp_thread_id"].(string)
			state.completions[tid]++
			log.Printf("[event] Message completed on thread %s (total: %d)",
				truncate(tid, 16), state.completions[tid])
		}
	})

	mux := http.NewServeMux()

	// === WebSocket endpoint for Zed ===
	mux.HandleFunc("/api/v1/external-agents/sync", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("session_id")
		if agentID == "" {
			agentID = r.URL.Query().Get("agent_id")
		}
		if agentID != "" {
			srv.SetExternalAgentUserMapping(agentID, "tui-e2e-user")
			state.mu.Lock()
			state.agentID = agentID
			state.mu.Unlock()
			log.Printf("[ws] Agent connecting: %s", agentID)
		}
		srv.ExternalAgentSyncHandler()(w, r)
	})

	// === REST API endpoints for TUI ===

	// Projects
	mux.HandleFunc("/api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		projects := []*types.Project{
			{
				ID:          "proj_e2e",
				Name:        "E2E Test Project",
				Description: "TUI E2E test project with real Zed agent",
				Status:      "active",
				Stats: types.ProjectStats{
					TotalTasks:      1,
					InProgressTasks: 1,
				},
			},
		}
		json.NewEncoder(w).Encode(projects)
	})

	// Spec tasks
	mux.HandleFunc("/api/v1/spec-tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// from-prompt
			var req types.CreateTaskRequest
			json.NewDecoder(r.Body).Decode(&req)
			task := &types.SpecTask{
				ID:                "spt_e2e_1",
				ProjectID:         req.ProjectID,
				Name:              "E2E test task",
				OriginalPrompt:    req.Prompt,
				Status:            types.TaskStatusImplementation,
				Priority:          req.Priority,
				PlanningSessionID: seedSessionID,
				HelixAppID:        "app_e2e",
				CreatedAt:         time.Now(),
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(task)
			return
		}

		tasks := []*types.SpecTask{
			{
				ID:                "spt_e2e_1",
				ProjectID:         "proj_e2e",
				Name:              "E2E test task",
				ShortTitle:        "E2E task",
				Status:            types.TaskStatusImplementation,
				Priority:          types.SpecTaskPriorityHigh,
				BranchName:        "feature/e2e-test",
				PlanningSessionID: seedSessionID,
				HelixAppID:        "app_e2e",
				OriginalPrompt:    "Write a hello world program",
				CreatedAt:         time.Now(),
			},
		}
		json.NewEncoder(w).Encode(tasks)
	})

	// Individual spec task
	mux.HandleFunc("/api/v1/spec-tasks/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/spec-tasks/")
		if strings.Contains(path, "approve-specs") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if strings.HasPrefix(path, "from-prompt") {
			// handled above
			return
		}

		taskID := strings.SplitN(path, "/", 2)[0]
		task := &types.SpecTask{
			ID:                taskID,
			ProjectID:         "proj_e2e",
			Name:              "E2E test task",
			Status:            types.TaskStatusImplementation,
			Priority:          types.SpecTaskPriorityHigh,
			PlanningSessionID: seedSessionID,
			HelixAppID:        "app_e2e",
		}
		json.NewEncoder(w).Encode(task)
	})

	// Sessions — interactions from real memorystore
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")

		// Chat — send message through production code path to Zed
		if path == "chat" || strings.HasSuffix(path, "/chat") {
			var req types.SessionChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			sessionID := req.SessionID
			if sessionID == "" {
				sessionID = seedSessionID
			}

			// Extract message text
			message := ""
			if len(req.Messages) > 0 {
				for _, part := range req.Messages[0].Content.Parts {
					if s, ok := part.(string); ok {
						message = s
						break
					}
				}
			}

			if message == "" {
				http.Error(w, "empty message", http.StatusBadRequest)
				return
			}

			// Find connected agent
			state.mu.Lock()
			agentID := state.agentID
			state.mu.Unlock()

			if agentID == "" {
				http.Error(w, "no agent connected", http.StatusServiceUnavailable)
				return
			}

			// Send through production code path
			requestID := fmt.Sprintf("tui-req-%d", time.Now().UnixNano())

			// Try SendChatMessage first (creates interaction + sends via WS)
			if err := srv.SendChatMessage(sessionID, message, requestID); err != nil {
				log.Printf("[chat] SendChatMessage failed: %v, falling back to QueueCommand", err)
				// Fall back to direct queue
				cmd := types.ExternalAgentCommand{
					Type: "chat_message",
					Data: map[string]interface{}{
						"message":    message,
						"request_id": requestID,
						"agent_name": "zed-agent",
					},
				}
				srv.QueueCommand(agentID, cmd)
			}

			log.Printf("[chat] Sent message to Zed: %s (req=%s)", truncate(message, 40), requestID)

			// Wait for completion (poll)
			deadline := time.Now().Add(120 * time.Second)
			prevCompletions := totalCompletions(state)
			for time.Now().Before(deadline) {
				time.Sleep(500 * time.Millisecond)
				if totalCompletions(state) > prevCompletions {
					break
				}
			}

			// Return the response from the interaction
			interactions := ms.GetAllInteractions()
			var response string
			for _, ix := range interactions {
				if ix.ResponseMessage != "" {
					response = ix.ResponseMessage
				}
			}
			if response == "" {
				response = "(waiting for agent response...)"
			}

			w.Write([]byte(response))
			return
		}

		// List interactions
		if strings.HasSuffix(path, "/interactions") {
			sessionID := strings.TrimSuffix(path, "/interactions")
			interactions, _, err := ms.ListInteractions(context.Background(), &types.ListInteractionsQuery{
				SessionID: sessionID,
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			json.NewEncoder(w).Encode(interactions)
			return
		}

		// Get session
		sessionID := strings.SplitN(path, "/", 2)[0]
		session, err := ms.GetSession(context.Background(), sessionID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(session)
	})

	// Health/status
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		connected := state.agentID != ""
		eventCount := len(state.events)
		threadCount := len(state.threadIDs)
		compCount := 0
		for _, v := range state.completions {
			compCount += v
		}
		state.mu.Unlock()

		status := map[string]interface{}{
			"agent_connected": connected,
			"events":          eventCount,
			"threads":         threadCount,
			"completions":     compCount,
		}
		json.NewEncoder(w).Encode(status)
	})

	// Listen
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("Listen error: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	log.Printf("[server] Listening on http://127.0.0.1:%d", port)

	// Write port file for test scripts
	portFile := "/tmp/tui_test_server_port"
	if err := os.WriteFile(portFile, []byte(fmt.Sprintf("%d", port)), 0644); err != nil {
		log.Fatalf("Failed to write port file: %v", err)
	}

	// Also write a .env file the TUI can source
	envFile := "/tmp/tui_test_env"
	envContent := fmt.Sprintf("HELIX_URL=http://127.0.0.1:%d\nHELIX_API_KEY=test-api-key\n", port)
	os.WriteFile(envFile, []byte(envContent), 0644)

	if err := http.Serve(listener, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func totalCompletions(state *testState) int {
	state.mu.Lock()
	defer state.mu.Unlock()
	total := 0
	for _, v := range state.completions {
		total += v
	}
	return total
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
