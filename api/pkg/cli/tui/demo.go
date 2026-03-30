package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// demoServer starts an in-process mock Helix API server with realistic
// fake data. Used by `helix tui demo` to explore the TUI interactively
// without a real Helix instance.
type demoServer struct {
	mu           sync.Mutex
	interactions map[string][]*types.Interaction // sessionID -> interactions
	nextIntID    int
}

func newDemoServer() *demoServer {
	ds := &demoServer{
		interactions: make(map[string][]*types.Interaction),
	}
	ds.seedData()
	return ds
}

func (ds *demoServer) seedData() {
	// Seed conversations for each task's session
	ds.interactions["ses_login"] = []*types.Interaction{
		{
			ID:              "int_1",
			SessionID:       "ses_login",
			Created:         time.Now().Add(-45 * time.Minute),
			PromptMessage:   "The login page crashes when you enter an email longer than 254 characters. Users are getting a blank white screen.",
			ResponseMessage: "I've identified the root cause. The email validation regex `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$` has **catastrophic backtracking** on long inputs — it's O(2^n) in the worst case.\n\nHere's my plan:\n1. Replace the regex with a simple `strings.Contains(email, \"@\")` check + length validation\n2. Add a 254-character max length (per RFC 5321)\n3. Add unit tests for edge cases\n\nI'll start implementing now.",
			State:           types.InteractionStateComplete,
			DurationMs:      3200,
		},
		{
			ID:            "int_2",
			SessionID:     "ses_login",
			Created:       time.Now().Add(-30 * time.Minute),
			PromptMessage: "Looks good, go ahead and fix it.",
			ResponseMessage: "Done! I've pushed the fix to `fix/login-1`. Here's what changed:\n\n**src/auth/validate.go**\n```diff\n- func validateEmail(email string) bool {\n-     return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@...`).MatchString(email)\n+ func validateEmail(email string) bool {\n+     if len(email) > 254 {\n+         return false\n+     }\n+     parts := strings.SplitN(email, \"@\", 2)\n+     return len(parts) == 2 && parts[0] != \"\" && parts[1] != \"\"\n```\n\nAlso added 6 unit tests covering: empty, valid, too-long, no-@, multiple-@, and unicode inputs.\n\nAll tests passing. PR is ready for review.",
			State:         types.InteractionStateComplete,
			DurationMs:    8400,
		},
	}

	ds.interactions["ses_oauth"] = []*types.Interaction{
		{
			ID:              "int_3",
			SessionID:       "ses_oauth",
			Created:         time.Now().Add(-2 * time.Hour),
			PromptMessage:   "We need OAuth2 support with Google and GitHub providers. Users should be able to sign in with their existing accounts.",
			ResponseMessage: "I'll design the OAuth2 integration. Let me analyze the current auth system first.\n\nAfter reviewing `src/auth/`, here's my technical design:\n\n## OAuth2 Integration Design\n\n### User Stories\n1. As a user, I want to sign in with my Google account\n2. As a user, I want to sign in with my GitHub account\n3. As an admin, I want to configure OAuth2 providers\n\n### Architecture\n- New `OAuthProvider` interface with `Google` and `GitHub` implementations\n- OAuth2 callback handler at `/auth/callback/:provider`\n- Session token generation after successful OAuth2 flow\n- Account linking: if email matches existing account, link it\n\n### Acceptance Criteria\n- [ ] Google OAuth2 flow works end-to-end\n- [ ] GitHub OAuth2 flow works end-to-end\n- [ ] Existing password users can link OAuth accounts\n- [ ] New OAuth users get accounts created automatically\n- [ ] Session tokens are stored securely (httpOnly cookies)\n\nShall I proceed with implementation?",
			State:           types.InteractionStateComplete,
			DurationMs:      12000,
		},
	}

	ds.interactions["ses_db"] = []*types.Interaction{
		{
			ID:              "int_4",
			SessionID:       "ses_db",
			Created:         time.Now().Add(-20 * time.Minute),
			PromptMessage:   "The database queries in the user listing page are really slow. We're seeing 2-3 second load times.",
			ResponseMessage: "I'm analyzing the query patterns now...\n\nFound the problem: **N+1 queries**. The `ListUsers` handler loads users, then for each user loads their organizations in a separate query. With 500 users, that's 501 queries.\n\n```sql\n-- Current (slow): 501 queries\nSELECT * FROM users;              -- 1 query\nSELECT * FROM orgs WHERE user_id = ?;  -- ×500 queries\n\n-- Fixed (fast): 2 queries\nSELECT * FROM users;\nSELECT * FROM orgs WHERE user_id IN (?, ?, ...);  -- 1 query with preloading\n```\n\nI'll fix this with GORM `.Preload(\"Organizations\")`. Estimated improvement: 2.5s → 50ms.",
			State:           types.InteractionStateComplete,
			DurationMs:      5600,
		},
	}
}

func (ds *demoServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/projects", ds.handleProjects)
	mux.HandleFunc("/api/v1/spec-tasks", ds.handleSpecTasks)
	mux.HandleFunc("/api/v1/spec-tasks/", ds.handleSpecTask)
	mux.HandleFunc("/api/v1/sessions/chat", ds.handleChat)
	mux.HandleFunc("/api/v1/sessions/", ds.handleSession)
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{"status": "demo"})
	})

	return mux
}

func (ds *demoServer) handleProjects(w http.ResponseWriter, r *http.Request) {
	projects := []*types.Project{
		{
			ID:          "proj_demo",
			Name:        "acme-webapp",
			Description: "Main web application — React frontend + Go API",
			Status:      "active",
			Stats: types.ProjectStats{
				TotalTasks:      7,
				CompletedTasks:  2,
				InProgressTasks: 2,
				BacklogTasks:    2,
				PlanningTasks:   1,
			},
		},
		{
			ID:          "proj_mobile",
			Name:        "acme-mobile",
			Description: "iOS and Android mobile app",
			Status:      "active",
			Stats: types.ProjectStats{
				TotalTasks:      3,
				InProgressTasks: 1,
			},
		},
	}
	json.NewEncoder(w).Encode(projects)
}

func (ds *demoServer) handleSpecTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// from-prompt
		var req types.CreateTaskRequest
		json.NewDecoder(r.Body).Decode(&req)
		task := &types.SpecTask{
			ID:             fmt.Sprintf("spt_new_%d", time.Now().UnixNano()%10000),
			ProjectID:      req.ProjectID,
			Name:           truncateDemo(req.Prompt, 50),
			OriginalPrompt: req.Prompt,
			Status:         types.TaskStatusBacklog,
			Priority:       req.Priority,
			CreatedAt:      time.Now(),
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(task)
		return
	}

	now := time.Now()
	tasks := []*types.SpecTask{
		{
			ID: "spt_login", ProjectID: "proj_demo",
			Name: "Fix login crash on long emails", ShortTitle: "Fix login crash",
			Status: types.TaskStatusImplementation, Priority: types.SpecTaskPriorityHigh,
			BranchName: "fix/login-1", PlanningSessionID: "ses_login", HelixAppID: "app_1",
			OriginalPrompt: "Login page crashes on long emails",
			AgentWorkState: "working",
			CreatedAt: now.Add(-2 * time.Hour), UpdatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID: "spt_oauth", ProjectID: "proj_demo",
			Name: "Add OAuth2 support (Google + GitHub)", ShortTitle: "Add OAuth2",
			Status: types.TaskStatusSpecReview, Priority: types.SpecTaskPriorityMedium,
			PlanningSessionID: "ses_oauth", HelixAppID: "app_1",
			OriginalPrompt:   "We need OAuth2 with Google and GitHub",
			RequirementsSpec: "## Requirements\n\n### User Stories\n1. As a user, I want to sign in with Google\n2. As a user, I want to sign in with GitHub\n3. As an admin, I want to configure providers\n\n### Acceptance Criteria\n- [ ] Google OAuth2 flow works\n- [ ] GitHub OAuth2 flow works\n- [ ] Account linking works\n- [ ] Session tokens stored securely",
			TechnicalDesign:  "## Technical Design\n\n### Architecture\n- `OAuthProvider` interface\n- `/auth/callback/:provider` handler\n- Session token generation\n\n### Database Changes\n- New `oauth_accounts` table\n- FK to `users` table\n\n### Security\n- PKCE flow for public clients\n- State parameter validation\n- Token encryption at rest",
			CreatedAt: now.Add(-3 * time.Hour),
		},
		{
			ID: "spt_db", ProjectID: "proj_demo",
			Name: "Fix N+1 queries in user listing", ShortTitle: "Fix N+1 queries",
			Status: types.TaskStatusImplementation, Priority: types.SpecTaskPriorityHigh,
			BranchName: "fix/n-plus-one-2", PlanningSessionID: "ses_db", HelixAppID: "app_1",
			OriginalPrompt: "Database queries are slow on user listing page",
			AgentWorkState: "idle",
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID: "spt_search", ProjectID: "proj_demo",
			Name: "Add full-text search to knowledge base", ShortTitle: "Full-text search",
			Status: types.TaskStatusBacklog, Priority: types.SpecTaskPriorityMedium,
			OriginalPrompt: "Users need to search across all knowledge base articles",
			CreatedAt:      now.Add(-4 * time.Hour),
		},
		{
			ID: "spt_dark", ProjectID: "proj_demo",
			Name: "Dark mode support", ShortTitle: "Dark mode",
			Status: types.TaskStatusBacklog, Priority: types.SpecTaskPriorityLow,
			OriginalPrompt: "Add dark mode toggle to settings page",
			CreatedAt:      now.Add(-5 * time.Hour),
		},
		{
			ID: "spt_api_docs", ProjectID: "proj_demo",
			Name: "Regenerate API documentation", ShortTitle: "API docs",
			Status: types.TaskStatusDone, Priority: types.SpecTaskPriorityLow,
			OriginalPrompt: "Update swagger docs for v2 endpoints",
			CompletedAt:    timePtr(now.Add(-24 * time.Hour)),
			CreatedAt:      now.Add(-48 * time.Hour),
		},
		{
			ID: "spt_migrate", ProjectID: "proj_demo",
			Name: "Migrate to PostgreSQL 16", ShortTitle: "PG16 migration",
			Status: types.TaskStatusDone, Priority: types.SpecTaskPriorityMedium,
			OriginalPrompt:  "Upgrade from PostgreSQL 14 to 16",
			CompletedAt:     timePtr(now.Add(-72 * time.Hour)),
			BranchName:      "chore/pg16",
			RepoPullRequests: []types.RepoPR{{PRURL: "https://github.com/acme/webapp/pull/142", PRState: "merged"}},
			CreatedAt:       now.Add(-96 * time.Hour),
		},
	}
	json.NewEncoder(w).Encode(tasks)
}

func (ds *demoServer) handleSpecTask(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/spec-tasks/")

	if strings.Contains(path, "approve-specs") {
		w.WriteHeader(http.StatusOK)
		return
	}
	if strings.HasPrefix(path, "from-prompt") {
		ds.handleSpecTasks(w, r) // delegate to POST handler
		return
	}

	taskID := strings.SplitN(path, "/", 2)[0]
	task := &types.SpecTask{
		ID: taskID, ProjectID: "proj_demo",
		Name: "Task " + taskID, Status: types.TaskStatusImplementation,
		Priority: types.SpecTaskPriorityHigh, PlanningSessionID: "ses_login",
		HelixAppID: "app_1",
	}
	json.NewEncoder(w).Encode(task)
}

func (ds *demoServer) handleSession(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")

	if strings.HasSuffix(path, "/interactions") {
		sessionID := strings.TrimSuffix(path, "/interactions")
		ds.mu.Lock()
		interactions := ds.interactions[sessionID]
		ds.mu.Unlock()

		if interactions == nil {
			interactions = []*types.Interaction{}
		}
		json.NewEncoder(w).Encode(interactions)
		return
	}

	sessionID := strings.SplitN(path, "/", 2)[0]
	session := &types.Session{
		ID:      sessionID,
		Name:    "Session " + sessionID,
		Created: time.Now().Add(-1 * time.Hour),
		Updated: time.Now(),
	}
	json.NewEncoder(w).Encode(session)
}

func (ds *demoServer) handleChat(w http.ResponseWriter, r *http.Request) {
	var req types.SessionChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	message := ""
	if len(req.Messages) > 0 {
		for _, part := range req.Messages[0].Content.Parts {
			if s, ok := part.(string); ok {
				message = s
				break
			}
		}
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = "ses_login"
	}

	// Generate a fake response
	response := ds.generateResponse(message)

	// Add to interactions
	ds.mu.Lock()
	ds.nextIntID++
	userIx := &types.Interaction{
		ID:            fmt.Sprintf("int_demo_%d", ds.nextIntID),
		SessionID:     sessionID,
		Created:       time.Now(),
		PromptMessage: message,
		ResponseMessage: response,
		State:         types.InteractionStateComplete,
		DurationMs:    2000 + rand.Intn(8000),
	}
	ds.interactions[sessionID] = append(ds.interactions[sessionID], userIx)
	ds.mu.Unlock()

	// Simulate thinking time
	time.Sleep(time.Duration(500+rand.Intn(1500)) * time.Millisecond)

	w.Write([]byte(response))
}

func (ds *demoServer) generateResponse(message string) string {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "hello") || strings.Contains(lower, "hi") {
		return "Hello! I'm the Zed agent working on this task. How can I help?"
	}
	if strings.Contains(lower, "status") {
		return "Here's the current status:\n\n- **Branch**: `fix/login-1` (3 commits ahead of main)\n- **Tests**: All 42 passing\n- **Coverage**: 87% (+2% from this PR)\n- **PR**: Ready for review\n\nAnything else you'd like to know?"
	}
	if strings.Contains(lower, "test") {
		return "Running tests...\n\n```\n$ go test ./...\nok  \tgithub.com/acme/webapp/auth     0.234s\nok  \tgithub.com/acme/webapp/api      1.102s\nok  \tgithub.com/acme/webapp/store    0.567s\n\nPASS\nAll 42 tests passing.\n```\n\nAll green! The new email validation tests cover 6 edge cases."
	}
	if strings.Contains(lower, "diff") || strings.Contains(lower, "change") {
		return "Here are the changes so far:\n\n**src/auth/validate.go** — replaced regex with simple check\n**src/auth/validate_test.go** — added 6 new test cases\n**src/api/handlers.go** — added input length validation\n\nTotal: 3 files changed, +45 lines, -12 lines"
	}
	if strings.Contains(lower, "approve") || strings.Contains(lower, "lgtm") {
		return "Great! I'll merge the PR and close this task.\n\n✅ PR #87 merged to main\n✅ Task marked as done\n✅ Branch `fix/login-1` cleaned up"
	}

	responses := []string{
		"I'll look into that. Let me analyze the codebase and get back to you with a plan.",
		"Good question. Let me check the relevant files and run some tests to verify.",
		"I've made the changes you requested. Let me run the test suite to make sure everything still passes.",
		"Understood. I'll implement that approach. Here's what I'll do:\n\n1. Update the data model\n2. Modify the API handlers\n3. Add migration\n4. Write tests\n\nStarting now...",
	}
	return responses[rand.Intn(len(responses))]
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func truncateDemo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}

// startDemoServer starts the mock server and returns a connected APIClient.
func startDemoServer() (*APIClient, func()) {
	ds := newDemoServer()

	server := &http.Server{Handler: ds.handler()}
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}

	go server.Serve(listener)

	addr := listener.Addr().String()
	url := "http://" + addr

	helixClient, err := client.NewClient(url, "demo-key", false)
	if err != nil {
		panic(err)
	}
	api := NewAPIClient(helixClient)

	cleanup := func() {
		server.Close()
	}

	return api, cleanup
}
