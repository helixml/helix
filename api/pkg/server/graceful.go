package server

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// DrainTimeout bounds how long in-flight HTTP requests (including SSE
// streams) may run after SIGTERM before their connections are closed. The
// blue/green deploy stops the old colour with `docker stop -t 60`, so it must
// stay below that.
const DrainTimeout = 45 * time.Second

// drainer shuts the API's HTTP servers down gracefully. On SIGTERM the
// blue/green deploy has already pointed nginx at the new colour; the old one
// must hand its clients over fast:
//
//  1. /api/v1/readyz starts returning 503.
//  2. Hijacked connections (websockets, RevDial tunnels, the runner NATS
//     proxy) are closed at once. http.Server.Shutdown neither waits for nor
//     closes them, and their clients reconnect through nginx to the new
//     colour.
//  3. Shutdown lets in-flight requests finish within DrainTimeout, then the
//     remaining connections are closed.
type drainer struct {
	draining atomic.Bool

	mu      sync.Mutex
	servers []*http.Server
	conns   map[*trackedConn]struct{}
}

func newDrainer() *drainer {
	return &drainer{conns: map[*trackedConn]struct{}{}}
}

// serve serves srv on ln until drain, tracking its connections.
func (d *drainer) serve(srv *http.Server, ln net.Listener, useTLS bool) error {
	d.mu.Lock()
	d.servers = append(d.servers, srv)
	d.mu.Unlock()
	prev := srv.ConnState
	srv.ConnState = func(c net.Conn, state http.ConnState) {
		if state == http.StateHijacked {
			if tc, ok := c.(*tls.Conn); ok {
				c = tc.NetConn()
			}
			if tracked, ok := c.(*trackedConn); ok {
				tracked.hijacked.Store(true)
			}
		}
		if prev != nil {
			prev(c, state)
		}
	}
	ln = &trackingListener{Listener: ln, d: d}
	var err error
	if useTLS {
		err = srv.ServeTLS(ln, "", "")
	} else {
		err = srv.Serve(ln)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// drain runs the shutdown sequence on every server and returns once they
// have all stopped.
func (d *drainer) drain() {
	d.draining.Store(true)
	closed := d.closeHijacked()
	log.Info().Int("hijacked_connections_closed", closed).Dur("timeout", DrainTimeout).
		Msg("draining: closed websockets and tunnels, waiting for in-flight requests")

	ctx, cancel := context.WithTimeout(context.Background(), DrainTimeout)
	defer cancel()
	d.mu.Lock()
	servers := append([]*http.Server(nil), d.servers...)
	d.mu.Unlock()
	var wg sync.WaitGroup
	for _, srv := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			if err := srv.Shutdown(ctx); err != nil {
				log.Warn().Err(err).Str("addr", srv.Addr).Msg("draining: requests still running at the deadline; closing them")
				_ = srv.Close()
			}
		}(srv)
	}
	wg.Wait()
	// A websocket upgraded while the first pass ran.
	d.closeHijacked()
	log.Info().Msg("drained")
}

func (d *drainer) closeHijacked() int {
	d.mu.Lock()
	var hijacked []*trackedConn
	for c := range d.conns {
		if c.hijacked.Load() {
			hijacked = append(hijacked, c)
		}
	}
	d.mu.Unlock()
	for _, c := range hijacked {
		_ = c.Close()
	}
	return len(hijacked)
}

type trackingListener struct {
	net.Listener
	d *drainer
}

func (l *trackingListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	tc := &trackedConn{Conn: c, d: l.d}
	l.d.mu.Lock()
	l.d.conns[tc] = struct{}{}
	l.d.mu.Unlock()
	return tc, nil
}

// trackedConn is a server connection, removed from the drainer's set when
// closed by whoever owns it (the server, or a handler that hijacked it).
type trackedConn struct {
	net.Conn
	d         *drainer
	hijacked  atomic.Bool
	closeOnce sync.Once
}

func (c *trackedConn) Close() error {
	c.closeOnce.Do(func() {
		c.d.mu.Lock()
		delete(c.d.conns, c)
		c.d.mu.Unlock()
	})
	return c.Conn.Close()
}

// readyz reports whether this instance should receive traffic: it is not
// draining and Postgres answers. Load balancers and the blue/green deploy's
// health check poll it.
func (apiServer *HelixAPIServer) readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if apiServer.drainer.draining.Load() {
		http.Error(w, "draining", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := apiServer.Store.Ping(ctx); err != nil {
		log.Warn().Err(err).Msg("readyz: database unreachable")
		http.Error(w, "database unreachable", http.StatusServiceUnavailable)
		return
	}
	_, _ = w.Write([]byte("ok\n"))
}
