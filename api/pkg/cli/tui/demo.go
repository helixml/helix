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
	"github.com/helixml/helix/api/pkg/server/wsprotocol"
	"github.com/helixml/helix/api/pkg/types"
	"gorm.io/datatypes"
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
	// Seed conversations with realistic ResponseEntries matching what Zed sends
	ds.interactions["ses_login"] = []*types.Interaction{
		{
			ID:            "int_1",
			SessionID:     "ses_login",
			Created:       time.Now().Add(-45 * time.Minute),
			PromptMessage: "The login page crashes when you enter an email longer than 254 characters. Users are getting a blank white screen.",
			ResponseMessage: "Let me investigate the login crash.\n\n**Tool Call: Read file**\nStatus: Completed\n\nsrc/auth/validate.go (42 lines)\n\n**Tool Call: Run command**\nStatus: Completed\n\n$ grep -n 'regexp' src/auth/validate.go\n15: return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$`).MatchString(email)\n\nI've identified the root cause. The email validation regex has **catastrophic backtracking** on long inputs — it's O(2^n) in the worst case.\n\nHere's my plan:\n1. Replace the regex with a simple string check + length validation\n2. Add a 254-character max length (per RFC 5321)\n3. Add unit tests for edge cases",
			ResponseEntries: entriesToJSON([]wsprotocol.ResponseEntry{
				{Type: "text", Content: "Let me investigate the login crash.", MessageID: "msg_1a"},
				{Type: "tool_call", Content: "src/auth/validate.go (42 lines)", MessageID: "msg_1b",
					ToolName: "Read file", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "$ grep -n 'regexp' src/auth/validate.go\n15: return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$`).MatchString(email)", MessageID: "msg_1c",
					ToolName: "Run command", ToolStatus: "Completed"},
				{Type: "text", Content: "I've identified the root cause. The email validation regex has **catastrophic backtracking** on long inputs — it's O(2^n) in the worst case.\n\nHere's my plan:\n1. Replace the regex with a simple string check + length validation\n2. Add a 254-character max length (per RFC 5321)\n3. Add unit tests for edge cases", MessageID: "msg_1d"},
			}),
			State:      types.InteractionStateComplete,
			DurationMs: 3200,
		},
		{
			ID:            "int_2",
			SessionID:     "ses_login",
			Created:       time.Now().Add(-30 * time.Minute),
			PromptMessage: "Looks good, go ahead and fix it.",
			ResponseMessage: "I'll implement the fix now.\n\n**Tool Call: Edit file**\nStatus: Completed\n\nsrc/auth/validate.go\n- func validateEmail(email string) bool {\n-     return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$`).MatchString(email)\n- }\n+ func validateEmail(email string) bool {\n+     if len(email) > 254 {\n+         return false\n+     }\n+     parts := strings.SplitN(email, \"@\", 2)\n+     return len(parts) == 2 && parts[0] != \"\" && parts[1] != \"\"\n+ }\n\n**Tool Call: Write file**\nStatus: Completed\n\nCreated src/auth/validate_test.go (48 lines)\n\n**Tool Call: Run command**\nStatus: Completed\n\n$ go test ./src/auth/...\nok  \tacme/webapp/auth  0.234s\n\nAll 6 new tests passing. Changes:\n- Replaced regex with simple string check\n- Added 254 char max (RFC 5321)\n- 6 unit tests: empty, valid, too-long, no-@, multiple-@, unicode\n\nPR is ready for review.",
			ResponseEntries: entriesToJSON([]wsprotocol.ResponseEntry{
				{Type: "text", Content: "I'll implement the fix now.", MessageID: "msg_2a"},
				{Type: "tool_call", Content: "src/auth/validate.go\n- func validateEmail(email string) bool {\n-     return regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$`).MatchString(email)\n- }\n+ func validateEmail(email string) bool {\n+     if len(email) > 254 {\n+         return false\n+     }\n+     parts := strings.SplitN(email, \"@\", 2)\n+     return len(parts) == 2 && parts[0] != \"\" && parts[1] != \"\"\n+ }", MessageID: "msg_2b",
					ToolName: "Edit file", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "Created src/auth/validate_test.go (48 lines)", MessageID: "msg_2c",
					ToolName: "Write file", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "$ go test ./src/auth/...\nok  \tacme/webapp/auth  0.234s", MessageID: "msg_2d",
					ToolName: "Run command", ToolStatus: "Completed"},
				{Type: "text", Content: "All 6 new tests passing. Changes:\n- Replaced regex with simple string check\n- Added 254 char max (RFC 5321)\n- 6 unit tests: empty, valid, too-long, no-@, multiple-@, unicode\n\nPR is ready for review.", MessageID: "msg_2e"},
			}),
			State:      types.InteractionStateComplete,
			DurationMs: 8400,
		},
	}

	ds.interactions["ses_oauth"] = []*types.Interaction{
		{
			ID:            "int_3",
			SessionID:     "ses_oauth",
			Created:       time.Now().Add(-2 * time.Hour),
			PromptMessage: "We need OAuth2 support with Google and GitHub providers. Users should be able to sign in with their existing accounts.",
			ResponseMessage: "Let me analyze the current auth system.\n\n**Tool Call: Read file**\nStatus: Completed\n\nsrc/auth/auth.go (156 lines)\n\n**Tool Call: List directory**\nStatus: Completed\n\nsrc/auth/\n  auth.go\n  middleware.go\n  session.go\n  validate.go\n\nAfter reviewing `src/auth/`, here's my technical design:\n\n## OAuth2 Integration Design\n\n### User Stories\n1. As a user, I want to sign in with my Google account\n2. As a user, I want to sign in with my GitHub account\n\n### Architecture\n- New `OAuthProvider` interface with `Google` and `GitHub` implementations\n- OAuth2 callback handler at `/auth/callback/:provider`\n- Session token generation after successful OAuth2 flow\n- Account linking: if email matches existing account, link it\n\n### Acceptance Criteria\n- [ ] Google OAuth2 flow works end-to-end\n- [ ] GitHub OAuth2 flow works end-to-end\n- [ ] Existing password users can link OAuth accounts\n- [ ] Session tokens are stored securely (httpOnly cookies)\n\nShall I proceed with implementation?",
			ResponseEntries: entriesToJSON([]wsprotocol.ResponseEntry{
				{Type: "text", Content: "Let me analyze the current auth system.", MessageID: "msg_3a"},
				{Type: "tool_call", Content: "src/auth/auth.go (156 lines)", MessageID: "msg_3b",
					ToolName: "Read file", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "src/auth/\n  auth.go\n  middleware.go\n  session.go\n  validate.go", MessageID: "msg_3c",
					ToolName: "List directory", ToolStatus: "Completed"},
				{Type: "text", Content: "After reviewing `src/auth/`, here's my technical design:\n\n## OAuth2 Integration Design\n\n### User Stories\n1. As a user, I want to sign in with my Google account\n2. As a user, I want to sign in with my GitHub account\n\n### Architecture\n- New `OAuthProvider` interface with `Google` and `GitHub` implementations\n- OAuth2 callback handler at `/auth/callback/:provider`\n- Session token generation after successful OAuth2 flow\n- Account linking: if email matches existing account, link it\n\n### Acceptance Criteria\n- [ ] Google OAuth2 flow works end-to-end\n- [ ] GitHub OAuth2 flow works end-to-end\n- [ ] Existing password users can link OAuth accounts\n- [ ] Session tokens are stored securely (httpOnly cookies)\n\nShall I proceed with implementation?", MessageID: "msg_3d"},
			}),
			State:      types.InteractionStateComplete,
			DurationMs: 12000,
		},
	}

	ds.interactions["ses_db"] = []*types.Interaction{
		{
			ID:            "int_4",
			SessionID:     "ses_db",
			Created:       time.Now().Add(-20 * time.Minute),
			PromptMessage: "The database queries in the user listing page are really slow. We're seeing 2-3 second load times.",
			ResponseMessage: "Let me investigate the slow queries.\n\n**Tool Call: Run command**\nStatus: Completed\n\n$ grep -rn 'func.*ListUsers' src/api/\nsrc/api/handlers.go:142: func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {\n\n**Tool Call: Read file**\nStatus: Completed\n\nsrc/api/handlers.go (lines 142-168)\n\n**Tool Call: Run command**\nStatus: Completed\n\n$ HELIX_DB_LOG=true go test -run TestListUsers -v ./src/api/\nSELECT * FROM users;\nSELECT * FROM organizations WHERE user_id = 'usr_001';\nSELECT * FROM organizations WHERE user_id = 'usr_002';\n... (498 more queries)\nTotal queries: 501, Duration: 2.3s\n\nFound the problem: **N+1 queries**. The `ListUsers` handler loads users, then for each user loads their organizations in a separate query. With 500 users, that's 501 queries.\n\n```sql\n-- Current (slow): 501 queries\nSELECT * FROM users;                    -- 1 query\nSELECT * FROM orgs WHERE user_id = ?;   -- ×500 queries\n\n-- Fixed (fast): 2 queries  \nSELECT * FROM users;\nSELECT * FROM orgs WHERE user_id IN (?, ?, ...);  -- GORM Preload\n```\n\nI'll fix this with GORM `.Preload(\"Organizations\")`. Estimated improvement: 2.5s → 50ms.",
			ResponseEntries: entriesToJSON([]wsprotocol.ResponseEntry{
				{Type: "text", Content: "Let me investigate the slow queries.", MessageID: "msg_4a"},
				{Type: "tool_call", Content: "$ grep -rn 'func.*ListUsers' src/api/\nsrc/api/handlers.go:142: func (h *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {", MessageID: "msg_4b",
					ToolName: "Run command", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "src/api/handlers.go (lines 142-168)", MessageID: "msg_4c",
					ToolName: "Read file", ToolStatus: "Completed"},
				{Type: "tool_call", Content: "$ HELIX_DB_LOG=true go test -run TestListUsers -v ./src/api/\nSELECT * FROM users;\nSELECT * FROM organizations WHERE user_id = 'usr_001';\nSELECT * FROM organizations WHERE user_id = 'usr_002';\n... (498 more queries)\nTotal queries: 501, Duration: 2.3s", MessageID: "msg_4d",
					ToolName: "Run command", ToolStatus: "Completed"},
				{Type: "text", Content: "Found the problem: **N+1 queries**. The `ListUsers` handler loads users, then for each user loads their organizations in a separate query. With 500 users, that's 501 queries.\n\n```sql\n-- Current (slow): 501 queries\nSELECT * FROM users;                    -- 1 query\nSELECT * FROM orgs WHERE user_id = ?;   -- ×500 queries\n\n-- Fixed (fast): 2 queries  \nSELECT * FROM users;\nSELECT * FROM orgs WHERE user_id IN (?, ?, ...);  -- GORM Preload\n```\n\nI'll fix this with GORM `.Preload(\"Organizations\")`. Estimated improvement: 2.5s → 50ms.", MessageID: "msg_4e"},
			}),
			State:      types.InteractionStateComplete,
			DurationMs: 5600,
		},
	}
}

func entriesToJSON(v interface{}) datatypes.JSON {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return datatypes.JSON(b)
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

	// ServeMux routes /sessions/chat here too since /sessions/ is a prefix match
	if path == "chat" {
		ds.handleChat(w, r)
		return
	}

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

	// Create the interaction in "waiting" state immediately
	ds.mu.Lock()
	ds.nextIntID++
	ixID := fmt.Sprintf("int_demo_%d", ds.nextIntID)
	ix := &types.Interaction{
		ID:            ixID,
		SessionID:     sessionID,
		Created:       time.Now(),
		PromptMessage: message,
		State:         types.InteractionStateWaiting,
	}
	ds.interactions[sessionID] = append(ds.interactions[sessionID], ix)
	ds.mu.Unlock()

	// Stream the response in background — TUI polls interactions to see updates
	go ds.streamResponse(sessionID, ixID, message)

	// Return immediately so the TUI starts showing the spinner
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(""))
}

// streamResponse simulates a Zed agent working: reads files, edits code,
// runs commands — each tool call starts as "Running" then becomes "Completed".
func (ds *demoServer) streamResponse(sessionID, ixID, message string) {
	steps := ds.generateStreamingSteps(message)

	var entries []wsprotocol.ResponseEntry
	var responseText strings.Builder

	for i, step := range steps {
		// Add entry in "Running" state
		entry := wsprotocol.ResponseEntry{
			Type:       step.Type,
			Content:    "",
			MessageID:  fmt.Sprintf("msg_stream_%s_%d", ixID, i),
			ToolName:   step.ToolName,
			ToolStatus: "Running",
		}
		if step.Type == "text" {
			entry.Content = "" // will be streamed char by char
			entry.ToolStatus = ""
		}
		entries = append(entries, entry)

		// Update interaction with running state
		ds.updateInteraction(sessionID, ixID, entries, responseText.String(), types.InteractionStateWaiting)

		// Simulate work time
		time.Sleep(time.Duration(300+rand.Intn(700)) * time.Millisecond)

		// Stream the content character by character (for text) or in chunks (for tool output)
		if step.Type == "text" {
			// Stream text word by word
			words := strings.Fields(step.Content)
			for j, word := range words {
				if j > 0 {
					entries[len(entries)-1].Content += " "
					responseText.WriteString(" ")
				}
				entries[len(entries)-1].Content += word
				responseText.WriteString(word)

				// Update every few words
				if j%3 == 0 {
					ds.updateInteraction(sessionID, ixID, entries, responseText.String(), types.InteractionStateWaiting)
					time.Sleep(50 * time.Millisecond)
				}
			}
			responseText.WriteString("\n\n")
		} else {
			// Tool call: show content after a delay, then mark completed
			time.Sleep(time.Duration(500+rand.Intn(1000)) * time.Millisecond)
			entries[len(entries)-1].Content = step.Content
			entries[len(entries)-1].ToolStatus = "Completed"

			// Add to flat response text in the tool call format
			responseText.WriteString(fmt.Sprintf("**Tool Call: %s**\nStatus: Completed\n\n%s\n\n", step.ToolName, step.Content))
		}

		ds.updateInteraction(sessionID, ixID, entries, responseText.String(), types.InteractionStateWaiting)
		time.Sleep(200 * time.Millisecond)
	}

	// Mark complete
	ds.updateInteraction(sessionID, ixID, entries, responseText.String(), types.InteractionStateComplete)
}

type streamStep struct {
	Type     string // "text" or "tool_call"
	Content  string
	ToolName string
}

func (ds *demoServer) generateStreamingSteps(message string) []streamStep {
	lower := strings.ToLower(message)

	if strings.Contains(lower, "test") {
		return []streamStep{
			{Type: "text", Content: "I'll run the test suite for you."},
			{Type: "tool_call", ToolName: "Run command",
				Content: "$ go test ./...\nok  \tacme/webapp/auth     0.234s\nok  \tacme/webapp/api      1.102s\nok  \tacme/webapp/store    0.567s\n\nPASS"},
			{Type: "text", Content: "All 42 tests passing. The new email validation tests cover 6 edge cases including unicode and max-length inputs."},
		}
	}

	if strings.Contains(lower, "fix") || strings.Contains(lower, "bug") {
		return []streamStep{
			{Type: "text", Content: "Let me investigate and fix that."},
			{Type: "tool_call", ToolName: "Search files",
				Content: "Found 3 matches in 2 files"},
			{Type: "tool_call", ToolName: "Read file",
				Content: "src/api/handlers.go (lines 42-68)"},
			{Type: "tool_call", ToolName: "Edit file",
				Content: "src/api/handlers.go\n- if err != nil {\n-     log.Println(err)\n-     return\n- }\n+ if err != nil {\n+     http.Error(w, fmt.Sprintf(\"request failed: %v\", err), http.StatusBadRequest)\n+     return\n+ }"},
			{Type: "tool_call", ToolName: "Run command",
				Content: "$ go test ./src/api/...\nok  \tacme/webapp/api  0.891s"},
			{Type: "text", Content: "Fixed. The error was being silently swallowed instead of returned to the client. Now returns a proper 400 with the error message. Tests passing."},
		}
	}

	if strings.Contains(lower, "status") || strings.Contains(lower, "progress") {
		return []streamStep{
			{Type: "tool_call", ToolName: "Run command",
				Content: "$ git log --oneline -5\na1b2c3d Fix email validation regex\ne4f5g6h Add max-length check\ni7j8k9l Add unit tests for edge cases"},
			{Type: "tool_call", ToolName: "Run command",
				Content: "$ git diff --stat main\n src/auth/validate.go      | 12 +++---\n src/auth/validate_test.go | 48 +++++++++++++++++++++\n 2 files changed, 54 insertions(+), 6 deletions(-)"},
			{Type: "text", Content: "Branch `fix/login-1` is 3 commits ahead of main. All tests passing, coverage at 87% (+2%). Ready for review."},
		}
	}

	// Default: generic investigation pattern
	return []streamStep{
		{Type: "text", Content: "Let me look into that for you."},
		{Type: "tool_call", ToolName: "Search files",
			Content: "Searching codebase for relevant files..."},
		{Type: "tool_call", ToolName: "Read file",
			Content: "src/main.go (128 lines)"},
		{Type: "text", Content: "I've analyzed the codebase. Here's what I found and my recommended approach:\n\n1. The current implementation has a straightforward fix\n2. I'll make the change and add tests\n3. Should be ready for review shortly"},
	}
}

func (ds *demoServer) updateInteraction(sessionID, ixID string, entries []wsprotocol.ResponseEntry, responseText string, state types.InteractionState) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	interactions := ds.interactions[sessionID]
	for _, ix := range interactions {
		if ix.ID == ixID {
			ix.ResponseMessage = responseText
			ix.State = state
			ix.Updated = time.Now()
			if entriesJSON, err := json.Marshal(entries); err == nil {
				ix.ResponseEntries = datatypes.JSON(entriesJSON)
			}
			break
		}
	}
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
