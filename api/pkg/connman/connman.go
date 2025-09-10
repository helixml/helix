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
