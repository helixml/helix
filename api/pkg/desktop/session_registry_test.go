package desktop

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestRegistry creates a fresh SessionRegistry (not the global one)
// so tests don't interfere with each other or the background cleanup goroutine.
func newTestRegistry() *SessionRegistry {
	return &SessionRegistry{}
}

// testWSPair creates a paired WebSocket client/server connection for testing.
// Returns the server-side *websocket.Conn (used by SessionRegistry) and a cleanup function.
// The client side drains messages in a background goroutine so writes don't block.
func testWSPair(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{}
	var serverConn *websocket.Conn
	connReady := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		serverConn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade: %v", err)
		}
		close(connReady)
		// Block until test is done — the server handler must stay alive
		// for the WebSocket connection to remain valid.
		select {}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}

	<-connReady

	// Drain client-side messages so server writes don't block.
	go func() {
		for {
			_, _, err := clientConn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	cleanup := func() {
		clientConn.Close()
		serverConn.Close()
		srv.Close()
	}
	return serverConn, cleanup
}

func TestRegisterClient_AssignsUniqueIDs(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	c1 := r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)
	c2 := r.RegisterClient("session-1", "user-2", "Bob", "", "", ws2, mu)

	if c1.ID == c2.ID {
		t.Fatalf("expected different client IDs, both got %d", c1.ID)
	}
}

func TestRegisterClient_AssignsColors(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	c1 := r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)
	c2 := r.RegisterClient("session-1", "user-2", "Bob", "", "", ws2, mu)

	if c1.Color == "" {
		t.Fatal("expected non-empty color for client 1")
	}
	if c2.Color == "" {
		t.Fatal("expected non-empty color for client 2")
	}
	if c1.Color == c2.Color {
		t.Fatalf("expected different colors, both got %s", c1.Color)
	}
}

func TestRegisterClient_EvictsDuplicateClientUniqueID(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	stableUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Register first client with a stable UUID
	c1 := r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws1, mu)

	// Register second client with the SAME UUID (simulating reconnect from same tab)
	c2 := r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws2, mu)

	// Old client should be evicted
	clients := r.GetConnectedUsers("session-1")
	if len(clients) != 1 {
		t.Fatalf("expected 1 client after reconnect, got %d", len(clients))
	}
	if clients[0].ID != c2.ID {
		t.Fatalf("expected surviving client ID %d, got %d", c2.ID, clients[0].ID)
	}

	_ = c1 // Old client was evicted; verified by client count above
}

func TestRegisterClient_DifferentUUIDsCoexist(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}

	// Register two clients with DIFFERENT UUIDs (two browser tabs)
	r.RegisterClient("session-1", "user-1", "Alice", "", "uuid-tab-1", ws1, mu)
	r.RegisterClient("session-1", "user-1", "Alice", "", "uuid-tab-2", ws2, mu)

	clients := r.GetConnectedUsers("session-1")
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients (multi-tab), got %d", len(clients))
	}
}

func TestRegisterClient_EmptyUUIDSkipsEviction(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}

	// Register two clients with empty UUID — should not evict each other
	r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)
	r.RegisterClient("session-1", "user-2", "Bob", "", "", ws2, mu)

	clients := r.GetConnectedUsers("session-1")
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients (empty UUID = no eviction), got %d", len(clients))
	}
}

func TestUnregisterClient_RemovesClient(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()

	mu := &sync.Mutex{}
	c := r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)

	r.UnregisterClient("session-1", c.ID)

	clients := r.GetConnectedUsers("session-1")
	if len(clients) != 0 {
		t.Fatalf("expected 0 clients after unregister, got %d", len(clients))
	}
}

func TestUnregisterClient_AlreadyEvicted_NoPanic(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	stableUUID := "550e8400-e29b-41d4-a716-446655440000"

	// Register and then evict via re-registration with same UUID
	c1 := r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws1, mu)
	r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws2, mu)

	// Now try to unregister the already-evicted client — should be a no-op, not panic
	r.UnregisterClient("session-1", c1.ID)

	clients := r.GetConnectedUsers("session-1")
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
}

func TestUnregisterClient_NonexistentSession(t *testing.T) {
	r := newTestRegistry()

	// Should not panic
	r.UnregisterClient("nonexistent-session", 999)
}

func TestRegisterClient_EvictionClosesOldConnection(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	stableUUID := "reconnect-uuid"

	r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws1, mu)

	// Re-register with same UUID to evict
	r.RegisterClient("session-1", "user-1", "Alice", "", stableUUID, ws2, mu)

	// Writing to the old server-side connection should fail (TCP conn was closed)
	err := ws1.WriteMessage(websocket.TextMessage, []byte("test"))
	if err == nil {
		t.Fatal("expected write to evicted connection to fail, but it succeeded")
	}
}

func TestRegisterClient_CrossSessionIsolation(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	sameUUID := "same-uuid"

	// Same UUID in different sessions should NOT evict
	r.RegisterClient("session-1", "user-1", "Alice", "", sameUUID, ws1, mu)
	r.RegisterClient("session-2", "user-1", "Alice", "", sameUUID, ws2, mu)

	clients1 := r.GetConnectedUsers("session-1")
	clients2 := r.GetConnectedUsers("session-2")

	if len(clients1) != 1 {
		t.Fatalf("expected 1 client in session-1, got %d", len(clients1))
	}
	if len(clients2) != 1 {
		t.Fatalf("expected 1 client in session-2, got %d", len(clients2))
	}
}

func TestUpdateClientActivity(t *testing.T) {
	r := newTestRegistry()

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()

	mu := &sync.Mutex{}
	c := r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)

	before := c.LastSeen
	time.Sleep(10 * time.Millisecond)
	r.UpdateClientActivity("session-1", c.ID)

	if !c.LastSeen.After(before) {
		t.Fatal("expected LastSeen to be updated")
	}
}

func TestUpdateClientActivity_NonexistentSession(t *testing.T) {
	r := newTestRegistry()
	// Should not panic
	r.UpdateClientActivity("nonexistent", 999)
}

func TestGetConnectedUsers_EmptySession(t *testing.T) {
	r := newTestRegistry()
	clients := r.GetConnectedUsers("nonexistent")
	if clients != nil {
		t.Fatalf("expected nil for nonexistent session, got %v", clients)
	}
}

func TestGetLastMover(t *testing.T) {
	r := newTestRegistry()

	// No session yet
	if got := r.GetLastMover("session-1"); got != 0 {
		t.Fatalf("expected 0 for no session, got %d", got)
	}

	ws1, cleanup1 := testWSPair(t)
	defer cleanup1()
	ws2, cleanup2 := testWSPair(t)
	defer cleanup2()

	mu := &sync.Mutex{}
	c1 := r.RegisterClient("session-1", "user-1", "Alice", "", "", ws1, mu)
	c2 := r.RegisterClient("session-1", "user-2", "Bob", "", "", ws2, mu)

	// Broadcast cursor from c1
	r.BroadcastCursorPosition("session-1", c1.ID, 100, 200)
	if got := r.GetLastMover("session-1"); got != c1.ID {
		t.Fatalf("expected last mover %d, got %d", c1.ID, got)
	}

	// Broadcast cursor from c2
	r.BroadcastCursorPosition("session-1", c2.ID, 300, 400)
	if got := r.GetLastMover("session-1"); got != c2.ID {
		t.Fatalf("expected last mover %d, got %d", c2.ID, got)
	}
}
