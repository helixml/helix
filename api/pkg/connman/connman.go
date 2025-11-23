package connman

import (
	"context"
	"errors"
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
	// Use proper revdial.Dialer for multiplexing multiple logical connections
	m.deviceDialers[key] = revdial.NewDialer(conn, "/revdial")
	m.lock.Unlock()
}

func (m *ConnectionManager) Dial(ctx context.Context, key string) (net.Conn, error) {
	m.lock.RLock()
	dialer, ok := m.deviceDialers[key]
	if !ok {
		m.lock.RUnlock()
		return nil, ErrNoConnection
	}
	m.lock.RUnlock()

	// Use revdial.Dialer to create a new logical connection
	return dialer.Dial(ctx)
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
