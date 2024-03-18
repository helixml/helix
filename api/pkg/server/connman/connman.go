package connman

import (
	"context"
	"errors"
	"net"
	"sync"

	revdial "golang.org/x/build/revdial/v2"
)

var (
	ErrNoConnection = errors.New("no connection")
)

// ConnectionManager manages connections to workers
// TODO: Might make sense to port to H2
type ConnectionManager struct {
	deviceDialers map[string]*revdial.Dialer
	lock          sync.RWMutex
}

func New() *ConnectionManager {
	return &ConnectionManager{
		deviceDialers: make(map[string]*revdial.Dialer),
	}
}

func (m *ConnectionManager) Set(key string, conn net.Conn) {
	m.lock.Lock()
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

	return dialer.Dial(ctx)
}
