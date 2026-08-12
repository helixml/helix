package slack

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// SocketApp identifies a configured Socket Mode app and its app-level
// (xapp-) token. A change to AppToken means the operator re-issued the
// token, so the live connection must be torn down and re-established.
type SocketApp struct {
	ID       string
	AppToken string
}

// SocketConnector opens a Socket Mode connection for one app and returns a
// stop function that tears it down. The connection is expected to be
// self-healing (reconnect on transient drops) until either stop() is
// called or ctx is cancelled. ctx bounds the connection's lifetime.
type SocketConnector func(ctx context.Context, app SocketApp) (stop func())

// SocketManager keeps the set of live Socket Mode connections in sync with
// the set of configured socket apps. It exists because Socket Mode has no
// inbound webhook to register: a connection must be actively held open per
// app, and apps are created/edited/deleted at runtime. Reconcile diffs the
// desired set (from list) against the running set and starts/stops/
// restarts connections so installing or editing a socket app takes effect
// without a server restart.
type SocketManager struct {
	list    func(context.Context) ([]SocketApp, error)
	connect SocketConnector
	logger  *slog.Logger

	mu      sync.Mutex
	running map[string]liveSocket
	kick    chan struct{}
}

type liveSocket struct {
	token string
	stop  func()
}

// NewSocketManager builds a manager. list returns the currently-configured
// socket apps (with decrypted app tokens); connect opens one connection.
func NewSocketManager(list func(context.Context) ([]SocketApp, error), connect SocketConnector, logger *slog.Logger) *SocketManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &SocketManager{
		list:    list,
		connect: connect,
		logger:  logger,
		running: map[string]liveSocket{},
		kick:    make(chan struct{}, 1),
	}
}

// Kick requests an immediate reconcile — call it after a socket app is
// created, edited, or deleted so pickup doesn't wait for the next tick.
// Non-blocking and coalescing: it never blocks the caller, and overlapping
// kicks collapse into a single pending reconcile. The reconcile runs on
// the Run loop's context, so a request handler's context never becomes a
// connection's lifetime.
func (m *SocketManager) Kick() {
	select {
	case m.kick <- struct{}{}:
	default:
	}
}

// Reconcile makes the live connections match the configured socket apps:
// it starts connections for newly-configured apps, stops them for removed
// apps, and restarts them when an app's token changes. Idempotent. A list
// error leaves existing connections untouched (a transient store blip must
// not drop healthy sockets).
func (m *SocketManager) Reconcile(ctx context.Context) {
	apps, err := m.list(ctx)
	if err != nil {
		m.logger.Error("slack.socketmode: reconcile list apps", "err", err)
		return
	}

	want := make(map[string]SocketApp, len(apps))
	for _, a := range apps {
		if a.AppToken == "" {
			continue
		}
		want[a.ID] = a
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop connections for apps that are gone or whose token changed.
	for id, live := range m.running {
		if w, ok := want[id]; !ok || w.AppToken != live.token {
			live.stop()
			delete(m.running, id)
			m.logger.Info("slack.socketmode: stopped", "app", id)
		}
	}

	// Start connections for newly-desired apps (including token-change
	// restarts, since the old one was just removed above).
	for id, app := range want {
		if _, ok := m.running[id]; ok {
			continue
		}
		stop := m.connect(ctx, app)
		m.running[id] = liveSocket{token: app.AppToken, stop: stop}
		m.logger.Info("slack.socketmode: started", "app", id)
	}
}

// Run reconciles immediately, then on every interval tick until ctx is
// cancelled, at which point all live connections are stopped. Call in a
// goroutine for the lifetime of the server.
func (m *SocketManager) Run(ctx context.Context, interval time.Duration) {
	m.Reconcile(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-t.C:
			m.Reconcile(ctx)
		case <-m.kick:
			m.Reconcile(ctx)
		}
	}
}

func (m *SocketManager) stopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, live := range m.running {
		live.stop()
		delete(m.running, id)
	}
}
