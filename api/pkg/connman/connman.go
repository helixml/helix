package connman

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/revdial"
)

var (
	ErrNoConnection            = errors.New("no connection")
	ErrReconnectTimeout        = errors.New("reconnect timeout")
	ErrMaxPendingDialsExceeded = errors.New("max pending dials exceeded")
)

const (
	// DefaultGracePeriod is how long to wait for reconnection before giving up
	DefaultGracePeriod = 30 * time.Second

	// MaxPendingDials is the maximum number of pending Dial() calls per key
	MaxPendingDials = 100

	// CleanupInterval is how often to check for expired grace periods
	CleanupInterval = 5 * time.Second
)

// dialWaiter represents a pending Dial() call waiting for reconnection
type dialWaiter struct {
	ready chan struct{}
	ctx   context.Context
}

// ConnectionManager manages connections to devices
// It supports graceful reconnection by queueing Dial() requests during
// brief disconnections and completing them when the client reconnects.
type ConnectionManager struct {
	deviceDialers     map[string]*revdial.Dialer
	deviceConnections map[string]net.Conn // Raw connections for simple TCP forwarding
	origins           map[string]string   // Where each key's connection came from, for logs

	// Grace period support for reconnection tolerance
	disconnectedAt map[string]time.Time     // When each key was disconnected
	pendingDials   map[string][]*dialWaiter // Pending Dial() calls per key
	gracePeriod    time.Duration
	// cleanupInterval is CleanupInterval except in tests.
	cleanupInterval time.Duration

	// Callback when grace period expires for a key (e.g., to clean up session state)
	onGracePeriodExpired func(key string)

	lock     sync.RWMutex
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewWithGracePeriod creates a ConnectionManager with a custom grace period
func NewWithGracePeriod(gracePeriod time.Duration) *ConnectionManager {
	return newWithIntervals(gracePeriod, CleanupInterval)
}

func newWithIntervals(gracePeriod, cleanupInterval time.Duration) *ConnectionManager {
	m := &ConnectionManager{
		deviceDialers:     make(map[string]*revdial.Dialer),
		deviceConnections: make(map[string]net.Conn),
		origins:           make(map[string]string),
		disconnectedAt:    make(map[string]time.Time),
		pendingDials:      make(map[string][]*dialWaiter),
		gracePeriod:       gracePeriod,
		cleanupInterval:   cleanupInterval,
		stopCh:            make(chan struct{}),
	}
	go m.cleanupLoop()
	return m
}

// New creates a ConnectionManager with default grace period
func New() *ConnectionManager {
	return NewWithGracePeriod(DefaultGracePeriod)
}

// Stop stops the cleanup goroutine
func (m *ConnectionManager) Stop() {
	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
}

// cleanupLoop periodically removes expired grace period entries
func (m *ConnectionManager) cleanupLoop() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.cleanupExpired()
		}
	}
}

// cleanupExpired removes keys that have exceeded their grace period
func (m *ConnectionManager) cleanupExpired() {
	m.lock.Lock()

	now := time.Now()
	var expiredKeys []string
	for key, disconnectTime := range m.disconnectedAt {
		if now.Sub(disconnectTime) > m.gracePeriod {
			expiredKeys = append(expiredKeys, key)
		}
	}

	// Process expired keys
	for _, key := range expiredKeys {
		// Grace period expired - notify waiting callers and clean up
		log.Printf("[connman] Grace period expired for key=%s, cleaning up", key)

		// Wake up pending waiters with closed channel (they'll get context error or ErrReconnectTimeout)
		if waiters, ok := m.pendingDials[key]; ok {
			for _, w := range waiters {
				close(w.ready)
			}
			delete(m.pendingDials, key)
		}

		delete(m.disconnectedAt, key)
		delete(m.deviceDialers, key)
		delete(m.deviceConnections, key)
		delete(m.origins, key)
	}

	// Get callback reference while holding lock
	callback := m.onGracePeriodExpired
	m.lock.Unlock()

	// Call callback outside of lock to avoid deadlocks
	if callback != nil {
		for _, key := range expiredKeys {
			callback(key)
		}
	}
}

// Displaced describes the connection a Set call replaced.
type Displaced struct {
	// Origin is the origin recorded when the replaced connection was set.
	Origin string
}

// Set registers a new connection for the given key; the last registration
// wins. origin is a free-form description of where conn came from (e.g. the
// client address), kept for logging.
//
// If the key already had a live connection, that connection is closed and
// described in the returned *Displaced (nil when nothing was replaced).
// Closing it makes a client that is still running notice it lost the key and
// reconnect, instead of holding a socket the API no longer dials.
// If there are pending Dial() calls waiting for reconnection, they will be woken up.
func (m *ConnectionManager) Set(key string, conn net.Conn, origin string) *Displaced {
	var oldDialer *revdial.Dialer
	m.lock.Lock()
	defer func() {
		m.lock.Unlock()
		// Closed outside the lock. The new dialer is already current, so
		// the old dialer's watchDialer does not start a grace period.
		if oldDialer != nil {
			oldDialer.Close()
		}
	}()

	// Check if this is a reconnection (key was in grace period)
	if disconnectTime, wasDisconnected := m.disconnectedAt[key]; wasDisconnected {
		graceDuration := time.Since(disconnectTime)
		log.Printf("[connman] Reconnection for key=%s after %v grace period", key, graceDuration.Round(time.Millisecond))
		delete(m.disconnectedAt, key)
	}

	var displaced *Displaced
	if old, exists := m.deviceDialers[key]; exists {
		displaced = &Displaced{Origin: m.origins[key]}
		oldDialer = old
	}

	// Store both the raw connection (for cleanup) and the dialer (for use)
	m.deviceConnections[key] = conn
	m.origins[key] = origin
	dialer := revdial.NewDialer(conn, "/revdial")
	m.deviceDialers[key] = dialer

	// Start a goroutine to watch for disconnection
	go m.watchDialer(key, dialer)

	// Wake up any pending Dial() calls
	if waiters, ok := m.pendingDials[key]; ok {
		log.Printf("[connman] Waking up %d pending Dial() calls for key=%s", len(waiters), key)
		for _, w := range waiters {
			close(w.ready) // Signal ready
		}
		delete(m.pendingDials, key)
	}
	return displaced
}

// Dial creates a new connection to the device identified by key.
// If the device is temporarily disconnected (within grace period), this will
// block until the device reconnects or the context is cancelled.
func (m *ConnectionManager) Dial(ctx context.Context, key string) (net.Conn, error) {
	m.lock.RLock()
	dialer, ok := m.deviceDialers[key]
	if ok {
		dialerPtr := fmt.Sprintf("%p", dialer)
		m.lock.RUnlock()

		// Use revdial.Dialer to create a new logical connection
		conn, err := dialer.Dial(ctx)
		if err != nil {
			// Log errors to help debug "use of closed network connection" issues
			log.Printf("[connman] Dial failed for key=%s dialer=%s: %v", key, dialerPtr, err)
			return nil, err
		}
		return conn, nil
	}

	// Check if recently disconnected (within grace period)
	disconnectTime, wasConnected := m.disconnectedAt[key]
	if !wasConnected || time.Since(disconnectTime) > m.gracePeriod {
		m.lock.RUnlock()
		return nil, ErrNoConnection
	}

	// Check pending dial limit
	pendingCount := len(m.pendingDials[key])
	if pendingCount >= MaxPendingDials {
		m.lock.RUnlock()
		log.Printf("[connman] Max pending dials exceeded for key=%s (%d pending)", key, pendingCount)
		return nil, ErrMaxPendingDialsExceeded
	}
	m.lock.RUnlock()

	// Queue this request and wait for reconnection
	return m.waitForReconnect(ctx, key)
}

// waitForReconnect queues a Dial() request and waits for the device to reconnect
func (m *ConnectionManager) waitForReconnect(ctx context.Context, key string) (net.Conn, error) {
	waiter := &dialWaiter{
		ready: make(chan struct{}),
		ctx:   ctx,
	}

	// Add to pending list
	m.lock.Lock()
	// Double-check the device hasn't reconnected while we were waiting for the lock
	if dialer, ok := m.deviceDialers[key]; ok {
		m.lock.Unlock()
		return dialer.Dial(ctx)
	}

	// Double-check still in grace period
	disconnectTime, inGracePeriod := m.disconnectedAt[key]
	if !inGracePeriod {
		m.lock.Unlock()
		return nil, ErrNoConnection
	}

	remainingGrace := m.gracePeriod - time.Since(disconnectTime)
	if remainingGrace <= 0 {
		m.lock.Unlock()
		return nil, ErrReconnectTimeout
	}

	m.pendingDials[key] = append(m.pendingDials[key], waiter)
	pendingCount := len(m.pendingDials[key])
	m.lock.Unlock()

	log.Printf("[connman] Dial() waiting for reconnection: key=%s pending=%d remaining_grace=%v",
		key, pendingCount, remainingGrace.Round(time.Millisecond))

	// Wait for reconnection or timeout
	select {
	case <-waiter.ready:
		// Reconnected! Try to dial again
		m.lock.RLock()
		dialer, ok := m.deviceDialers[key]
		m.lock.RUnlock()
		if !ok {
			// Ready was closed but no dialer - grace period expired
			return nil, ErrReconnectTimeout
		}
		log.Printf("[connman] Reconnected, completing Dial() for key=%s", key)
		return dialer.Dial(ctx)

	case <-ctx.Done():
		// Context cancelled - remove from pending list
		m.removePendingWaiter(key, waiter)
		return nil, ctx.Err()
	}
}

// removePendingWaiter removes a specific waiter from the pending list
func (m *ConnectionManager) removePendingWaiter(key string, waiter *dialWaiter) {
	m.lock.Lock()
	defer m.lock.Unlock()

	waiters := m.pendingDials[key]
	for i, w := range waiters {
		if w == waiter {
			m.pendingDials[key] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}
	if len(m.pendingDials[key]) == 0 {
		delete(m.pendingDials, key)
	}
}

// watchDialer monitors a dialer's Done() channel and calls OnDisconnect when it closes
func (m *ConnectionManager) watchDialer(key string, dialer *revdial.Dialer) {
	select {
	case <-dialer.Done():
		// Dialer closed - check if this is still the current dialer for this key
		m.lock.RLock()
		currentDialer := m.deviceDialers[key]
		m.lock.RUnlock()

		// Only call OnDisconnect if this is still the current dialer
		// (avoids race condition where new connection replaced the old one)
		if currentDialer == dialer {
			log.Printf("[connman] Dialer closed for key=%s, starting grace period", key)
			m.OnDisconnect(key)
		}
	case <-m.stopCh:
		// ConnectionManager is shutting down
		return
	}
}

// OnDisconnect is called when a connection is lost.
// Instead of immediately removing the key, it starts a grace period during which
// new Dial() calls will be queued and completed if the device reconnects.
func (m *ConnectionManager) OnDisconnect(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Only start grace period if there's actually a connection to disconnect
	if _, ok := m.deviceDialers[key]; !ok {
		return
	}

	log.Printf("[connman] Connection lost for key=%s, starting %v grace period", key, m.gracePeriod)

	// Record disconnection time
	m.disconnectedAt[key] = time.Now()

	// Remove the dialer but keep tracking via disconnectedAt
	delete(m.deviceDialers, key)
	delete(m.deviceConnections, key)
	delete(m.origins, key)
}

// Remove immediately removes a connection from the manager.
// Use OnDisconnect() for graceful handling of temporary disconnections.
func (m *ConnectionManager) Remove(key string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Wake up any pending waiters
	if waiters, ok := m.pendingDials[key]; ok {
		for _, w := range waiters {
			close(w.ready)
		}
		delete(m.pendingDials, key)
	}

	delete(m.disconnectedAt, key)
	delete(m.deviceDialers, key)
	delete(m.deviceConnections, key)
	delete(m.origins, key)
}

// List returns all active connection keys
func (m *ConnectionManager) List() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	keys := make([]string, 0, len(m.deviceDialers))
	for key := range m.deviceDialers {
		keys = append(keys, key)
	}
	return keys
}

// ListWithGracePeriod returns keys that are in the grace period (disconnected but may reconnect)
func (m *ConnectionManager) ListWithGracePeriod() []string {
	m.lock.RLock()
	defer m.lock.RUnlock()

	keys := make([]string, 0, len(m.disconnectedAt))
	for key := range m.disconnectedAt {
		keys = append(keys, key)
	}
	return keys
}

// Stats returns statistics about the connection manager state
func (m *ConnectionManager) Stats() ConnectionManagerStats {
	m.lock.RLock()
	defer m.lock.RUnlock()

	totalPending := 0
	for _, waiters := range m.pendingDials {
		totalPending += len(waiters)
	}

	return ConnectionManagerStats{
		ActiveConnections:  len(m.deviceDialers),
		GracePeriodEntries: len(m.disconnectedAt),
		PendingDialsTotal:  totalPending,
		PendingDialsPerKey: len(m.pendingDials),
	}
}

// ConnectionManagerStats contains statistics about the connection manager
type ConnectionManagerStats struct {
	ActiveConnections  int
	GracePeriodEntries int
	PendingDialsTotal  int
	PendingDialsPerKey int
}

// SetOnGracePeriodExpired sets a callback that will be invoked when a key's
// grace period expires. This can be used to clean up external state (e.g.,
// session metadata) when a sandbox definitively disconnects.
func (m *ConnectionManager) SetOnGracePeriodExpired(fn func(key string)) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.onGracePeriodExpired = fn
}
