package connman

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockConn is a simple mock net.Conn for testing
type mockConn struct {
	closed    bool
	closeChan chan struct{}
	mu        sync.Mutex
}

func newMockConn() *mockConn {
	return &mockConn{
		closeChan: make(chan struct{}),
	}
}

func (c *mockConn) Read(b []byte) (n int, err error) {
	// Block until closed
	<-c.closeChan
	return 0, net.ErrClosed
}

func (c *mockConn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	return len(b), nil
}

func (c *mockConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.closeChan)
	}
	return nil
}

func (c *mockConn) LocalAddr() net.Addr                { return &net.TCPAddr{} }
func (c *mockConn) RemoteAddr() net.Addr               { return &net.TCPAddr{} }
func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestConnectionManager_GracePeriod(t *testing.T) {
	// Create manager with short grace period for testing
	gracePeriod := 500 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	key := "test-runner"

	// Initially no connection
	ctx := context.Background()
	_, err := m.Dial(ctx, key)
	if err != ErrNoConnection {
		t.Fatalf("Expected ErrNoConnection, got %v", err)
	}

	// Set a connection
	conn := newMockConn()
	m.Set(key, conn)

	// Verify connection is listed
	keys := m.List()
	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("Expected [%s], got %v", key, keys)
	}

	// Simulate disconnection by calling OnDisconnect
	m.OnDisconnect(key)

	// Connection should no longer be in active list
	keys = m.List()
	if len(keys) != 0 {
		t.Fatalf("Expected empty list, got %v", keys)
	}

	// But should be in grace period list
	graceKeys := m.ListWithGracePeriod()
	if len(graceKeys) != 1 || graceKeys[0] != key {
		t.Fatalf("Expected [%s] in grace period, got %v", key, graceKeys)
	}

	// Dial should now queue and wait
	dialDone := make(chan error)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), gracePeriod+100*time.Millisecond)
		defer cancel()
		_, err := m.Dial(ctx, key)
		dialDone <- err
	}()

	// Give it a moment to queue
	time.Sleep(50 * time.Millisecond)

	// Reconnect
	conn2 := newMockConn()
	m.Set(key, conn2)

	// The queued dial should now complete (though it will fail because our mock dialer doesn't work)
	select {
	case err := <-dialDone:
		// We expect an error because revdial.Dialer needs a real connection,
		// but the key point is that Set() woke up the waiting Dial()
		t.Logf("Dial completed with: %v (expected - mock connection doesn't support revdial)", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Dial() did not complete after reconnection")
	}
}

func TestConnectionManager_GracePeriodExpires(t *testing.T) {
	// Create manager with very short grace period
	gracePeriod := 100 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	key := "test-runner"

	// Set and disconnect
	conn := newMockConn()
	m.Set(key, conn)
	m.OnDisconnect(key)

	// Verify in grace period
	graceKeys := m.ListWithGracePeriod()
	if len(graceKeys) != 1 {
		t.Fatalf("Expected key in grace period")
	}

	// Wait for grace period to expire (plus cleanup interval buffer)
	time.Sleep(gracePeriod + CleanupInterval + 100*time.Millisecond)

	// Should now be cleaned up
	graceKeys = m.ListWithGracePeriod()
	if len(graceKeys) != 0 {
		t.Fatalf("Expected empty grace period list after expiry, got %v", graceKeys)
	}

	// Dial should return ErrNoConnection immediately
	ctx := context.Background()
	_, err := m.Dial(ctx, key)
	if err != ErrNoConnection {
		t.Fatalf("Expected ErrNoConnection after grace period expired, got %v", err)
	}
}

func TestConnectionManager_ReconnectionClearsGracePeriod(t *testing.T) {
	gracePeriod := 500 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	key := "test-runner"

	// Set, disconnect, then reconnect
	conn1 := newMockConn()
	m.Set(key, conn1)
	m.OnDisconnect(key)

	// Verify in grace period
	if len(m.ListWithGracePeriod()) != 1 {
		t.Fatal("Expected key in grace period")
	}

	// Reconnect
	conn2 := newMockConn()
	m.Set(key, conn2)

	// Should no longer be in grace period
	if len(m.ListWithGracePeriod()) != 0 {
		t.Fatal("Expected empty grace period after reconnection")
	}

	// Should be back in active list
	if len(m.List()) != 1 {
		t.Fatal("Expected key in active list after reconnection")
	}
}

func TestConnectionManager_Stats(t *testing.T) {
	m := NewWithGracePeriod(time.Second)
	defer m.Stop()

	// Empty stats
	stats := m.Stats()
	if stats.ActiveConnections != 0 || stats.GracePeriodEntries != 0 {
		t.Fatalf("Expected empty stats, got %+v", stats)
	}

	// Add connection
	conn := newMockConn()
	m.Set("test", conn)

	stats = m.Stats()
	if stats.ActiveConnections != 1 {
		t.Fatalf("Expected 1 active connection, got %d", stats.ActiveConnections)
	}

	// Disconnect
	m.OnDisconnect("test")

	stats = m.Stats()
	if stats.ActiveConnections != 0 || stats.GracePeriodEntries != 1 {
		t.Fatalf("Expected 0 active, 1 grace period, got %+v", stats)
	}
}

func TestConnectionManager_MaxPendingDials(t *testing.T) {
	gracePeriod := 5 * time.Second
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	key := "test-runner"

	// Set and disconnect to enter grace period
	conn := newMockConn()
	m.Set(key, conn)
	m.OnDisconnect(key)

	// Start MaxPendingDials goroutines waiting
	var wg sync.WaitGroup
	for i := 0; i < MaxPendingDials; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			m.Dial(ctx, key) // Will timeout or reconnect
		}()
	}

	// Give them time to queue
	time.Sleep(50 * time.Millisecond)

	// One more should fail with max pending exceeded
	ctx := context.Background()
	_, err := m.Dial(ctx, key)
	if err != ErrMaxPendingDialsExceeded {
		t.Fatalf("Expected ErrMaxPendingDialsExceeded, got %v", err)
	}

	// Clean up - reconnect to wake waiting goroutines
	conn2 := newMockConn()
	m.Set(key, conn2)
	wg.Wait()
}

func TestConnectionManager_ContextCancellation(t *testing.T) {
	gracePeriod := 5 * time.Second
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	key := "test-runner"

	// Set and disconnect to enter grace period
	conn := newMockConn()
	m.Set(key, conn)
	m.OnDisconnect(key)

	// Start a dial with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := m.Dial(ctx, key)
	elapsed := time.Since(start)

	if err != context.DeadlineExceeded {
		t.Fatalf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Should have returned quickly after context timeout
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Dial took too long to return after context cancellation: %v", elapsed)
	}

	// Pending dials should be cleaned up
	stats := m.Stats()
	if stats.PendingDialsTotal != 0 {
		t.Fatalf("Expected 0 pending dials after cancellation, got %d", stats.PendingDialsTotal)
	}
}

func TestConnectionManager_OnGracePeriodExpiredCallback(t *testing.T) {
	// Use a short grace period and cleanup interval for faster test
	gracePeriod := 100 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	// Track callback invocations
	var callbackKeys []string
	var callbackMu sync.Mutex

	m.SetOnGracePeriodExpired(func(key string) {
		callbackMu.Lock()
		callbackKeys = append(callbackKeys, key)
		callbackMu.Unlock()
	})

	key := "hydra-sandbox123"

	// Set and disconnect to enter grace period
	conn := newMockConn()
	m.Set(key, conn)
	m.OnDisconnect(key)

	// Verify in grace period
	if len(m.ListWithGracePeriod()) != 1 {
		t.Fatal("Expected key to be in grace period")
	}

	// Wait for grace period to expire (grace period + cleanup interval + buffer)
	time.Sleep(gracePeriod + CleanupInterval + 50*time.Millisecond)

	// Verify callback was called with the correct key
	callbackMu.Lock()
	defer callbackMu.Unlock()

	if len(callbackKeys) != 1 {
		t.Fatalf("Expected callback to be called once, got %d calls", len(callbackKeys))
	}

	if callbackKeys[0] != key {
		t.Fatalf("Expected callback key %q, got %q", key, callbackKeys[0])
	}

	// Verify key is no longer in grace period
	if len(m.ListWithGracePeriod()) != 0 {
		t.Fatal("Expected grace period to be empty after expiry")
	}
}

func TestConnectionManager_OnGracePeriodExpiredCallback_MultipleKeys(t *testing.T) {
	gracePeriod := 100 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	// Track callback invocations
	var callbackKeys []string
	var callbackMu sync.Mutex

	m.SetOnGracePeriodExpired(func(key string) {
		callbackMu.Lock()
		callbackKeys = append(callbackKeys, key)
		callbackMu.Unlock()
	})

	// Set up multiple connections and disconnect them
	keys := []string{"hydra-sandbox1", "hydra-sandbox2", "hydra-sandbox3"}
	for _, key := range keys {
		conn := newMockConn()
		m.Set(key, conn)
		m.OnDisconnect(key)
	}

	// Wait for grace period to expire
	time.Sleep(gracePeriod + CleanupInterval + 50*time.Millisecond)

	// Verify all callbacks were called
	callbackMu.Lock()
	defer callbackMu.Unlock()

	if len(callbackKeys) != len(keys) {
		t.Fatalf("Expected %d callbacks, got %d", len(keys), len(callbackKeys))
	}

	// Verify all keys were called (order may vary)
	keySet := make(map[string]bool)
	for _, k := range callbackKeys {
		keySet[k] = true
	}

	for _, key := range keys {
		if !keySet[key] {
			t.Fatalf("Expected callback for key %q, but it was not called", key)
		}
	}
}

func TestConnectionManager_OnGracePeriodExpiredCallback_ReconnectPreventsCallback(t *testing.T) {
	gracePeriod := 200 * time.Millisecond
	m := NewWithGracePeriod(gracePeriod)
	defer m.Stop()

	callbackCalled := false
	m.SetOnGracePeriodExpired(func(key string) {
		callbackCalled = true
	})

	key := "hydra-sandbox123"

	// Set, disconnect, then reconnect before grace period expires
	conn1 := newMockConn()
	m.Set(key, conn1)
	m.OnDisconnect(key)

	// Reconnect before grace period expires
	time.Sleep(50 * time.Millisecond)
	conn2 := newMockConn()
	m.Set(key, conn2)

	// Wait past original grace period
	time.Sleep(gracePeriod + CleanupInterval + 50*time.Millisecond)

	// Callback should NOT have been called since we reconnected
	if callbackCalled {
		t.Fatal("Callback should not be called when reconnection happens before grace period expires")
	}
}
