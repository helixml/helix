package tui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/server"
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/store/memorystore"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/datatypes"
)

// tuiTestHarness sets up a test HTTP server backed by real HelixAPIServer
// handlers and an in-memory store. The TUI's APIClient talks to this server.
type tuiTestHarness struct {
	store     *memorystore.MemoryStore
	srv       *server.HelixAPIServer
	httpSrv   *httptest.Server
	apiClient *APIClient
}

func newTUITestHarness(t *testing.T) *tuiTestHarness {
	t.Helper()

	ms := memorystore.New()
	ps := pubsub.NewNoop()
	helixSrv := server.NewTestServer(ms, ps)

	// Seed test data
	seedTestData(t, ms)

	// Create HTTP server with routes the TUI uses
	mux := http.NewServeMux()
	registerTUITestRoutes(mux, ms, helixSrv)

	httpSrv := httptest.NewServer(mux)

	// Create API client
	helixClient, err := client.NewClient(httpSrv.URL, "test-api-key", false)
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPIClient(helixClient)

	return &tuiTestHarness{
		store:     ms,
		srv:       helixSrv,
		httpSrv:   httpSrv,
		apiClient: api,
	}
}

func (h *tuiTestHarness) Close() {
	h.httpSrv.Close()
}

func seedTestData(t *testing.T, ms *memorystore.MemoryStore) {
	t.Helper()
	ctx := context.Background()

	// Create a session with interactions
	session := types.Session{
		ID:      "ses_test1",
		Name:    "Fix login bug",
		Created: time.Now().Add(-1 * time.Hour),
		Updated: time.Now().Add(-5 * time.Minute),
		Owner:   "test-user",
		Mode:    types.SessionModeInference,
		Type:    types.SessionTypeText,
	}
	if _, err := ms.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}

	// Create interactions with ResponseEntries (matching Zed's wire format)
	entries1, _ := json.Marshal([]wsprotocol.ResponseEntry{
		{Type: "text", Content: "I've identified the issue. The email validation regex has catastrophic backtracking.", MessageID: "msg_t1"},
	})
	ix1 := types.Interaction{
		ID:              "int_1",
		SessionID:       "ses_test1",
		Created:         time.Now().Add(-30 * time.Minute),
		PromptMessage:   "The login page crashes on long emails",
		ResponseEntries: datatypes.JSON(entries1),
		State:           types.InteractionStateComplete,
	}
	if _, err := ms.CreateInteraction(ctx, &ix1); err != nil {
		t.Fatal(err)
	}

	entries2, _ := json.Marshal([]wsprotocol.ResponseEntry{
		{Type: "tool_call", Content: "src/auth/validate.go\n- old code\n+ new code", MessageID: "msg_t2a", ToolName: "Edit file", ToolStatus: "Completed"},
		{Type: "text", Content: "Done. Pushed fix to branch fix/login-1.", MessageID: "msg_t2b"},
	})
	ix2 := types.Interaction{
		ID:              "int_2",
		SessionID:       "ses_test1",
		Created:         time.Now().Add(-10 * time.Minute),
		PromptMessage:   "Go ahead and fix it",
		ResponseEntries: datatypes.JSON(entries2),
		State:           types.InteractionStateComplete,
	}
	if _, err := ms.CreateInteraction(ctx, &ix2); err != nil {
		t.Fatal(err)
	}
}

// registerTUITestRoutes adds HTTP handlers for the API endpoints the TUI uses.
// These use the same in-memory store as the HelixAPIServer, so state is shared.
func registerTUITestRoutes(mux *http.ServeMux, ms *memorystore.MemoryStore, helixSrv *server.HelixAPIServer) {
	// Projects (hardcoded for tests)
	mux.HandleFunc("/api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		projects := []*types.Project{
			{
				ID:          "proj_test1",
				Name:        "Test Project",
				Description: "Integration test project",
				Status:      "active",
				Stats: types.ProjectStats{
					TotalTasks:      3,
					InProgressTasks: 1,
				},
			},
		}
		json.NewEncoder(w).Encode(projects)
	})

	// Spec tasks
	mux.HandleFunc("/api/v1/spec-tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// from-prompt is handled by the sub-route below
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		tasks := []*types.SpecTask{
			{
				ID:                "spt_1",
				ProjectID:         "proj_test1",
				Name:              "Fix login bug",
				ShortTitle:        "Fix login",
				Status:            types.TaskStatusImplementation,
				Priority:          types.SpecTaskPriorityHigh,
				BranchName:        "fix/login-1",
				PlanningSessionID: "ses_test1",
				HelixAppID:        "app_1",
				OriginalPrompt:    "Login crashes on long emails",
			},
			{
				ID:             "spt_2",
				ProjectID:      "proj_test1",
				Name:           "Add OAuth2",
				ShortTitle:     "Add auth",
				Status:         types.TaskStatusBacklog,
				Priority:       types.SpecTaskPriorityMedium,
				OriginalPrompt: "Need OAuth2 with Google",
			},
			{
				ID:               "spt_3",
				ProjectID:        "proj_test1",
				Name:             "Refactor DB",
				Status:           types.TaskStatusSpecReview,
				Priority:         types.SpecTaskPriorityMedium,
				RequirementsSpec: "## Requirements\n\n- [ ] P99 < 100ms",
				TechnicalDesign:  "## Design\n\nUse GORM preloading.",
			},
		}
		json.NewEncoder(w).Encode(tasks)
	})

	// Individual spec task
	mux.HandleFunc("/api/v1/spec-tasks/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/spec-tasks/")
		// Handle from-prompt
		if strings.HasPrefix(path, "from-prompt") {
			var req types.CreateTaskRequest
			json.NewDecoder(r.Body).Decode(&req)
			task := &types.SpecTask{
				ID:             "spt_new",
				ProjectID:      req.ProjectID,
				Name:           "New task",
				OriginalPrompt: req.Prompt,
				Status:         types.TaskStatusBacklog,
				Priority:       req.Priority,
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(task)
			return
		}

		// Remove sub-routes
		taskID := strings.SplitN(path, "/", 2)[0]

		// approve-specs
		if strings.Contains(path, "approve-specs") {
			w.WriteHeader(http.StatusOK)
			return
		}

		task := &types.SpecTask{
			ID:                taskID,
			ProjectID:         "proj_test1",
			Name:              "Task " + taskID,
			Status:            types.TaskStatusImplementation,
			Priority:          types.SpecTaskPriorityHigh,
			PlanningSessionID: "ses_test1",
			HelixAppID:        "app_1",
		}
		json.NewEncoder(w).Encode(task)
	})

	// Sessions — interactions (uses real memorystore)
	mux.HandleFunc("/api/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")

		// Chat endpoint
		if path == "chat" {
			w.Write([]byte("I'll look into that right away."))
			return
		}

		// List interactions for a session
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

	// WebSocket sync handler (for real agent tests)
	mux.HandleFunc("/api/v1/external-agents/sync", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.URL.Query().Get("session_id")
		if agentID == "" {
			agentID = r.URL.Query().Get("agent_id")
		}
		if agentID != "" {
			helixSrv.SetExternalAgentUserMapping(agentID, "test-user")
		}
		helixSrv.ExternalAgentSyncHandler()(w, r)
	})
}

// --- Integration tests ---

func TestIntegration_ListProjects(t *testing.T) {
	h := newTUITestHarness(t)
	defer h.Close()

	projects, err := h.apiClient.ListProjects(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 project, got %d", len(projects))
	}
	if projects[0].ID != "proj_test1" {
		t.Errorf("expected proj_test1, got %s", projects[0].ID)
	}
}

func TestIntegration_ListSpecTasks(t *testing.T) {
	h := newTUITestHarness(t)
	defer h.Close()

	tasks, err := h.apiClient.ListSpecTasks(context.Background(), "proj_test1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}

	// Verify kanban column sorting
	kanban := NewKanbanModel(h.apiClient, "proj_test1")
	kanban.Update(tasksLoadedMsg{tasks: tasks})

	// Check column counts
	backlogCount := len(kanban.columns[ColBacklog])
	planningCount := len(kanban.columns[ColPlanning])
	progressCount := len(kanban.columns[ColInProgress])

	if backlogCount != 1 {
		t.Errorf("expected 1 backlog task, got %d", backlogCount)
	}
	if planningCount != 1 {
		t.Errorf("expected 1 planning task (spec_review), got %d", planningCount)
	}
	if progressCount != 1 {
		t.Errorf("expected 1 in-progress task, got %d", progressCount)
	}
}

func TestIntegration_ListInteractions(t *testing.T) {
	h := newTUITestHarness(t)
	defer h.Close()

	interactions, err := h.apiClient.ListInteractions(context.Background(), "ses_test1")
	if err != nil {
		t.Fatal(err)
	}
	if len(interactions) != 2 {
		t.Errorf("expected 2 interactions, got %d", len(interactions))
	}
	if interactions[0].PromptMessage != "The login page crashes on long emails" {
		t.Errorf("unexpected prompt: %s", interactions[0].PromptMessage)
	}
}

func TestIntegration_ChatRender(t *testing.T) {
	h := newTUITestHarness(t)
	defer h.Close()

	// Create a chat model for the task
	task := &types.SpecTask{
		ID:                "spt_1",
		Name:              "Fix login bug",
		ShortTitle:        "Fix login",
		Status:            types.TaskStatusImplementation,
		Priority:          types.SpecTaskPriorityHigh,
		BranchName:        "fix/login-1",
		PlanningSessionID: "ses_test1",
		HelixAppID:        "app_1",
	}

	chat := NewChatModel(h.apiClient, task)
	chat.SetSize(80, 24)

	// Fetch interactions
	interactions, err := h.apiClient.ListInteractions(context.Background(), "ses_test1")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the loaded message
	chat.Update(interactionsLoadedMsg{
		sessionID:    "ses_test1",
		interactions: interactions,
	})

	// Render and verify
	view := chat.View()
	if !strings.Contains(view, "You") {
		t.Error("expected 'You' role label in rendered chat")
	}
	if !strings.Contains(view, "Assistant") {
		t.Error("expected 'Assistant' role label in rendered chat")
	}
	if !strings.Contains(view, "login") {
		t.Error("expected 'login' in rendered chat content")
	}
}

func TestIntegration_KanbanRender(t *testing.T) {
	h := newTUITestHarness(t)
	defer h.Close()

	tasks, err := h.apiClient.ListSpecTasks(context.Background(), "proj_test1")
	if err != nil {
		t.Fatal(err)
	}

	kanban := NewKanbanModel(h.apiClient, "proj_test1")
	kanban.SetSize(120, 30)
	kanban.Update(tasksLoadedMsg{tasks: tasks})

	view := kanban.View()
	if !strings.Contains(view, "Backlog") {
		t.Error("expected 'Backlog' column header")
	}
	if !strings.Contains(view, "In Progress") {
		t.Error("expected 'In Progress' column header")
	}
	if !strings.Contains(view, "Fix login") {
		t.Error("expected 'Fix login' task name")
	}
}

func TestIntegration_ArchiveTask(t *testing.T) {
	// Set up a mock server that tracks archive calls
	var archiveCalledWith string
	var archiveBody string

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/spec-tasks/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/spec-tasks/")
		if strings.HasSuffix(path, "/archive") && r.Method == http.MethodPatch {
			taskID := strings.TrimSuffix(path, "/archive")
			archiveCalledWith = taskID
			body, _ := io.ReadAll(r.Body)
			archiveBody = string(body)
			w.WriteHeader(http.StatusOK)
			return
		}
		json.NewEncoder(w).Encode(&types.SpecTask{ID: "spt_1"})
	})
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	err := api.ArchiveTask(context.Background(), "spt_test123")
	if err != nil {
		t.Fatalf("ArchiveTask failed: %v", err)
	}

	if archiveCalledWith != "spt_test123" {
		t.Errorf("expected archive called with spt_test123, got %q", archiveCalledWith)
	}

	if !strings.Contains(archiveBody, `"archived":true`) {
		t.Errorf("expected body to contain archived:true, got %q", archiveBody)
	}
}

func TestIntegration_SpinnerBritishVerbs(t *testing.T) {
	spinner := NewSpinner("ctrl+b")
	view := spinner.View()

	// Should contain one of the British verbs
	found := false
	for _, verb := range britishVerbs {
		if strings.Contains(view, verb) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("spinner should contain a British verb, got: %s", view)
	}

	// Should contain the flower/asterisk
	if !strings.Contains(view, "✽") && !strings.Contains(view, "✦") && !strings.Contains(view, "✧") {
		t.Error("spinner should contain a spinner frame character")
	}

	// Should contain a tip
	if !strings.Contains(view, "Tip:") {
		t.Error("spinner should contain a tip")
	}
}

func TestIntegration_DiffRendering(t *testing.T) {
	diff := RenderInlineDiff("src/auth.go", "bad code", "good code", 60)

	if !strings.Contains(diff, "src/auth.go") {
		t.Error("diff should contain filename")
	}
	if !strings.Contains(diff, "- bad code") {
		t.Error("diff should contain removed line")
	}
	if !strings.Contains(diff, "+ good code") {
		t.Error("diff should contain added line")
	}
}

func TestIntegration_ConnectionManager(t *testing.T) {
	cm := NewConnectionManager()

	if !cm.IsConnected() {
		t.Error("should start connected")
	}

	bar := cm.RenderBar(80)
	if bar != "" {
		t.Error("should not show bar when connected")
	}

	// Simulate failures
	cm.RecordFailure(nil)
	if !cm.IsConnected() {
		t.Error("should still be connected after 1 failure")
	}

	cm.RecordFailure(nil)
	if cm.IsConnected() {
		t.Error("should be disconnected after 2 failures")
	}

	bar = cm.RenderBar(80)
	if !strings.Contains(bar, "Last contact") {
		t.Error("bar should show 'Last contact' when disconnected")
	}

	// Recovery
	cm.RecordSuccess()
	if !cm.IsConnected() {
		t.Error("should be connected after success")
	}
}

func TestIntegration_TabBar(t *testing.T) {
	tb := NewTabBar()
	tb.SetWidth(120)

	if tb.TabCount() != 1 {
		t.Errorf("expected 1 tab (kanban), got %d", tb.TabCount())
	}

	// Add a task tab
	task := &types.SpecTask{
		ID:   "spt_1",
		Name: "Fix login",
	}
	tb.AddTab(task)

	if tb.TabCount() != 2 {
		t.Errorf("expected 2 tabs, got %d", tb.TabCount())
	}
	if tb.ActiveIndex() != 1 {
		t.Error("new tab should be active")
	}

	// Render
	view := tb.View()
	if !strings.Contains(view, "kanban") {
		t.Error("tab bar should contain 'kanban'")
	}
	if !strings.Contains(view, "Fix login") {
		t.Error("tab bar should contain task name")
	}

	// Navigate
	tb.PrevTab()
	if tb.ActiveIndex() != 0 {
		t.Error("should be on kanban tab")
	}

	tb.NextTab()
	if tb.ActiveIndex() != 1 {
		t.Error("should be on task tab")
	}

	// Find by task
	found := tb.FindTabByTask("spt_1")
	if found == nil {
		t.Error("should find tab by task ID")
	}

	// Close
	tb.CloseTab()
	if tb.TabCount() != 1 {
		t.Error("should have 1 tab after close")
	}
}
