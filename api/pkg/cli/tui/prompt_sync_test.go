package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// mockPromptServer tracks synced prompts for testing.
type mockPromptServer struct {
	mu      sync.Mutex
	synced  []types.PromptHistoryEntrySync
	failNext bool
}

func (m *mockPromptServer) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/prompt-history/sync", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()

		if m.failNext {
			m.failNext = false
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}

		var req types.PromptHistorySyncRequest
		json.NewDecoder(r.Body).Decode(&req)

		m.synced = append(m.synced, req.Entries...)

		resp := types.PromptHistorySyncResponse{
			Synced:   len(req.Entries),
			Existing: 0,
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Stub endpoints the API client needs
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return mux
}

func TestPromptSync_BasicSend(t *testing.T) {
	mock := &mockPromptServer{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	req := &types.PromptHistorySyncRequest{
		ProjectID:  "proj_1",
		SpecTaskID: "spt_1",
		Entries: []types.PromptHistoryEntrySync{
			{
				ID:        "prompt_1",
				Content:   "Fix the login bug",
				Status:    "pending",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	resp, err := api.SyncPromptHistory(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Synced != 1 {
		t.Errorf("expected 1 synced, got %d", resp.Synced)
	}

	mock.mu.Lock()
	if len(mock.synced) != 1 {
		t.Errorf("expected 1 synced entry, got %d", len(mock.synced))
	}
	if mock.synced[0].Content != "Fix the login bug" {
		t.Errorf("wrong content: %s", mock.synced[0].Content)
	}
	mock.mu.Unlock()
}

func TestPromptSync_InterruptFlag(t *testing.T) {
	mock := &mockPromptServer{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	// Send with interrupt
	interrupt := true
	req := &types.PromptHistorySyncRequest{
		ProjectID:  "proj_1",
		SpecTaskID: "spt_1",
		Entries: []types.PromptHistoryEntrySync{
			{
				ID:        "prompt_interrupt",
				Content:   "Stop and do this instead",
				Status:    "pending",
				Timestamp: time.Now().UnixMilli(),
				Interrupt: &interrupt,
			},
		},
	}

	_, err := api.SyncPromptHistory(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	mock.mu.Lock()
	if mock.synced[0].Interrupt == nil || !*mock.synced[0].Interrupt {
		t.Error("expected interrupt=true on synced entry")
	}
	mock.mu.Unlock()
}

func TestPromptSync_QueueFlag(t *testing.T) {
	mock := &mockPromptServer{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	// Send without interrupt (queue)
	noInterrupt := false
	req := &types.PromptHistorySyncRequest{
		ProjectID:  "proj_1",
		SpecTaskID: "spt_1",
		Entries: []types.PromptHistoryEntrySync{
			{
				ID:        "prompt_queue",
				Content:   "Do this after you're done",
				Status:    "pending",
				Timestamp: time.Now().UnixMilli(),
				Interrupt: &noInterrupt,
			},
		},
	}

	_, err := api.SyncPromptHistory(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}

	mock.mu.Lock()
	if mock.synced[0].Interrupt == nil || *mock.synced[0].Interrupt {
		t.Error("expected interrupt=false on queued entry")
	}
	mock.mu.Unlock()
}

func TestPromptSync_OfflineQueue(t *testing.T) {
	mock := &mockPromptServer{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	// Simulate offline — first sync fails
	mock.mu.Lock()
	mock.failNext = true
	mock.mu.Unlock()

	req := &types.PromptHistorySyncRequest{
		ProjectID:  "proj_1",
		SpecTaskID: "spt_1",
		Entries: []types.PromptHistoryEntrySync{
			{
				ID:        "prompt_offline",
				Content:   "This was typed offline",
				Status:    "pending",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}

	// First attempt fails
	_, err := api.SyncPromptHistory(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for offline sync")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500 error, got: %v", err)
	}

	// Verify nothing was synced
	mock.mu.Lock()
	if len(mock.synced) != 0 {
		t.Error("expected 0 synced during offline")
	}
	mock.mu.Unlock()

	// Retry succeeds (back online)
	_, err = api.SyncPromptHistory(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success on retry, got: %v", err)
	}

	mock.mu.Lock()
	if len(mock.synced) != 1 {
		t.Errorf("expected 1 synced after retry, got %d", len(mock.synced))
	}
	if mock.synced[0].Content != "This was typed offline" {
		t.Errorf("wrong content after retry: %s", mock.synced[0].Content)
	}
	mock.mu.Unlock()
}

func TestPromptSync_MultiplePrompts(t *testing.T) {
	mock := &mockPromptServer{}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	helixClient, _ := client.NewClient(srv.URL, "test-key", false)
	api := NewAPIClient(helixClient)

	// Queue multiple prompts at once
	interrupt := true
	noInterrupt := false
	req := &types.PromptHistorySyncRequest{
		ProjectID:  "proj_1",
		SpecTaskID: "spt_1",
		Entries: []types.PromptHistoryEntrySync{
			{ID: "p1", Content: "First prompt", Status: "pending", Timestamp: time.Now().UnixMilli(), Interrupt: &noInterrupt},
			{ID: "p2", Content: "Second prompt", Status: "pending", Timestamp: time.Now().UnixMilli() + 1, Interrupt: &noInterrupt},
			{ID: "p3", Content: "Urgent interrupt", Status: "pending", Timestamp: time.Now().UnixMilli() + 2, Interrupt: &interrupt},
		},
	}

	resp, err := api.SyncPromptHistory(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Synced != 3 {
		t.Errorf("expected 3 synced, got %d", resp.Synced)
	}

	mock.mu.Lock()
	if len(mock.synced) != 3 {
		t.Errorf("expected 3 entries, got %d", len(mock.synced))
	}
	// Verify ordering preserved
	if mock.synced[0].Content != "First prompt" {
		t.Error("wrong order: first should be 'First prompt'")
	}
	if mock.synced[2].Content != "Urgent interrupt" {
		t.Error("wrong order: third should be 'Urgent interrupt'")
	}
	// Verify interrupt flags
	if *mock.synced[2].Interrupt != true {
		t.Error("third prompt should have interrupt=true")
	}
	mock.mu.Unlock()
}
