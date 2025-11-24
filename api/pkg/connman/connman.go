package connman

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/helixml/helix/api/pkg/revdial"
)

var (
	ErrNoConnection = errors.New("no connection")
)

// ConnectionManager manages connections to devices
// It's pretty simple now but it'll be responsible for more
// once we support running multiple controllers
type ConnectionManager struct {
	deviceDialers     map[string]*revdial.Dialer
	deviceConnections map[string]net.Conn // Raw connections for simple TCP forwarding
	lock              sync.RWMutex
}

func New() *ConnectionManager {
	return &ConnectionManager{
		deviceDialers:     make(map[string]*revdial.Dialer),
		deviceConnections: make(map[string]net.Conn),
	}
}

func (m *ConnectionManager) Set(key string, conn net.Conn) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// If there's an old connection for this key, just replace it
	// Don't try to close the old connection - it might already be closed from
	// API restart, and closing it again could cause issues
	// The old dialer will become unreachable and get garbage collected
	if _, exists := m.deviceDialers[key]; exists {
		// Log that we're replacing an existing connection (for debugging)
		// This is expected during API restarts when clients reconnect
	}

	// Store both the raw connection (for cleanup) and the dialer (for use)
	m.deviceConnections[key] = conn
	m.deviceDialers[key] = revdial.NewDialer(conn, "/revdial")
}

func (m *ConnectionManager) Dial(ctx context.Context, key string) (net.Conn, error) {
	m.lock.RLock()
	dialer, ok := m.deviceDialers[key]
	dialerPtr := fmt.Sprintf("%p", dialer) // Get pointer address for debugging
	if !ok {
		m.lock.RUnlock()
		return nil, ErrNoConnection
	}
	m.lock.RUnlock()

	// Use revdial.Dialer to create a new logical connection
	conn, err := dialer.Dial(ctx)
	if err != nil {
		// Log errors to help debug "use of closed network connection" issues
		log.Printf("[connman] Dial failed for key=%s dialer=%s: %v", key, dialerPtr, err)
		return nil, err
	}
	log.Printf("[connman] Dial successful for key=%s dialer=%s", key, dialerPtr)
	return conn, nil
}

// Remove removes a connection from the manager (for cleanup after disconnection)
func (m *ConnectionManager) Remove(key string) {
	m.lock.Lock()
	delete(m.deviceDialers, key)
	delete(m.deviceConnections, key)
	m.lock.Unlock()
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
