// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package revdial implements a Dialer and Listener which work together
// to turn an accepted connection (for instance, a Hijacked HTTP request) into
// a Dialer which can then create net.Conns connecting back to the original
// dialer, which then gets a net.Listener accepting those conns.
//
// This is basically a very minimal SOCKS5 client & server.
//
// The motivation is that sometimes you want to run a server on a
// machine deep inside a NAT. Rather than connecting to the machine
// directly (which you can't, because of the NAT), you have the
// sequestered machine connect out to a public machine. Both sides
// then use revdial and the public machine can become a client for the
// NATed machine.
package revdial

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/function61/holepunch-server/pkg/wsconnadapter"
	"github.com/gorilla/websocket"

	errutil "github.com/helixml/helix/api/pkg/util/error"
	promutil "github.com/helixml/helix/api/pkg/util/prometheus"
)

// dialerUniqParam is the parameter name of the GET URL form value
// containing the Dialer's random unique ID.
const dialerUniqParam = "revdial.dialer"

// requestIDParam is the parameter name of the per-Dial request ID embedded
// in the pickup URL. The listener echoes the URL verbatim when dialing back
// (and when reporting pickup-failed), so the server can route the resulting
// data connection / error to the specific Dial() that asked for it instead
// of relying on a shared unbuffered channel.
const requestIDParam = "revdial.req"

// dialerPingInterval is used to ensure we are sending constant pings
const dialerPingInterval = 18 * time.Second

// dialResult is the outcome of a single Dial() — either a data connection
// or a pickup-failed error from the listener.
type dialResult struct {
	conn net.Conn
	err  error
}

// The Dialer can create new connections.
type Dialer struct {
	conn       net.Conn // hijacked client conn
	path       string   // e.g. "/revdial"
	uniqID     string
	pickupPath string // path + uniqID: "/revdial?revdial.dialer="+uniqID

	connReady chan string // sends per-Dial request IDs to serve()
	donec     chan struct{}
	closeOnce sync.Once

	// requests maps each in-flight Dial()'s request ID to a buffered (cap 1)
	// channel that receives its dialResult. The data connection arriving via
	// ConnHandler (or an error via pickup-failed) is routed by request ID,
	// so concurrent Dial()s never share a connection or step on each other.
	requestsMu sync.Mutex
	requests   map[string]chan dialResult
}

var (
	dmapMu  sync.Mutex
	dialers = map[string]*Dialer{}
)

// NewDialer returns the side of the connection which will initiate
// new connections. This will typically be the side which did the HTTP
// Hijack. The connection is (typically) the hijacked HTTP client
// connection. The connPath is the HTTP path and optional query (but
// without scheme or host) on the dialer where the ConnHandler is
// mounted.
func NewDialer(c net.Conn, connPath string) *Dialer {
	d := &Dialer{
		path:      connPath,
		uniqID:    newUniqID(),
		conn:      c,
		donec:     make(chan struct{}),
		connReady: make(chan string),
		requests:  make(map[string]chan dialResult),
	}

	join := "?"
	if strings.Contains(connPath, "?") {
		join = "&"
	}
	d.pickupPath = connPath + join + dialerUniqParam + "=" + d.uniqID
	d.register()
	go d.serve()
	return d
}

func newUniqID() string {
	buf := make([]byte, 16)
	rand.Read(buf)
	return fmt.Sprintf("%x", buf)
}

func (d *Dialer) register() {
	dmapMu.Lock()
	defer dmapMu.Unlock()
	defer promutil.DeviceConnectionCount.Add(1)
	dialers[d.uniqID] = d
}

func (d *Dialer) unregister() {
	dmapMu.Lock()
	defer dmapMu.Unlock()
	defer promutil.DeviceConnectionCount.Sub(1)
	delete(dialers, d.uniqID)
}

// Done returns a channel which is closed when d is closed (either by
// this process on purpose, by a local error, or close or error from
// the peer).
func (d *Dialer) Done() <-chan struct{} { return d.donec }

// Close closes the Dialer.
func (d *Dialer) Close() error {
	d.closeOnce.Do(d.close)
	return nil
}

func (d *Dialer) close() {
	d.unregister()
	d.conn.Close()
	close(d.donec)
	// Wake up any in-flight Dial() callers so they don't block until ctx
	// cancels. closing each result chan causes the receiver to read a zero
	// dialResult, which Dial() interprets as "dialer closed".
	d.requestsMu.Lock()
	for id, ch := range d.requests {
		close(ch)
		delete(d.requests, id)
	}
	d.requestsMu.Unlock()
}

// Dial creates a new connection back to the Listener.
func (d *Dialer) Dial(ctx context.Context) (net.Conn, error) {
	requestID := newUniqID()
	resultCh := make(chan dialResult, 1)

	d.requestsMu.Lock()
	d.requests[requestID] = resultCh
	d.requestsMu.Unlock()

	// Always remove the request from the map on exit. If a data connection
	// arrives after we've given up (ctx cancellation, dialer close), the
	// deliverConn path will close it because the request ID is no longer
	// registered — no goroutine leak, no stale conn handed to a future Dial.
	cleanup := func() {
		d.requestsMu.Lock()
		delete(d.requests, requestID)
		d.requestsMu.Unlock()
	}

	// Tell serve we want a connection. serve will emit conn-ready with a
	// pickup path that embeds requestID, so the subsequent dial-back from
	// the listener carries the same ID and ConnHandler can route it back.
	select {
	case d.connReady <- requestID:
	case <-d.donec:
		cleanup()
		return nil, errors.New("revdial.Dialer closed")
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	}

	// Wait for THIS request's result.
	select {
	case res, ok := <-resultCh:
		cleanup()
		if !ok {
			return nil, errors.New("revdial.Dialer closed")
		}
		if res.err != nil {
			return nil, res.err
		}
		return res.conn, nil
	case <-d.donec:
		cleanup()
		return nil, errors.New("revdial.Dialer closed")
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	}
}

// deliverConn routes an incoming data WebSocket to the Dial() that asked
// for it, identified by the request ID parsed from the URL by ConnHandler.
// If no Dial is waiting (caller already gave up), the connection is closed
// to avoid leaking the upgraded WebSocket.
func (d *Dialer) deliverConn(requestID string, c net.Conn) {
	d.requestsMu.Lock()
	ch, ok := d.requests[requestID]
	d.requestsMu.Unlock()
	if !ok {
		c.Close()
		return
	}
	select {
	case ch <- dialResult{conn: c}:
	default:
		// The result chan is buffered with capacity 1; landing in the
		// default branch means a result was already delivered for this
		// request (shouldn't happen — defensive close to avoid leak).
		c.Close()
	}
}

// deliverErr routes a pickup-failed error to the Dial() that asked for it.
func (d *Dialer) deliverErr(requestID string, err error) {
	if requestID == "" {
		return
	}
	d.requestsMu.Lock()
	ch, ok := d.requests[requestID]
	d.requestsMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- dialResult{err: err}:
	default:
	}
}

// extractRequestID pulls the request ID query parameter out of a pickup
// path (which the listener echoes back in pickup-failed messages).
func extractRequestID(path string) string {
	i := strings.Index(path, "?")
	if i < 0 {
		return ""
	}
	q, err := url.ParseQuery(path[i+1:])
	if err != nil {
		return ""
	}
	return q.Get(requestIDParam)
}

// serve blocks and runs the control message loop, keeping the peer
// alive and notifying the peer when new connections are available.
func (d *Dialer) serve() error {
	defer d.Close()
	go func() {
		defer d.Close()
		br := bufio.NewReader(d.conn)
		for {
			line, err := br.ReadSlice('\n')
			if err != nil {
				return
			}
			var msg controlMsg
			if err := json.Unmarshal(line, &msg); err != nil {
				log.Printf("revdial.Dialer read invalid JSON: %q: %v", line, err)
				return
			}
			switch msg.Command {
			case "pickup-failed":
				// The listener echoes the pickup path back, which carries
				// the request ID we embedded when sending conn-ready. Route
				// the error to that specific Dial() instead of broadcasting
				// it to whichever caller happens to be waiting.
				requestID := extractRequestID(msg.ConnPath)
				d.deliverErr(requestID, fmt.Errorf("revdial listener failed to pick up connection: %v", msg.Err))
			}
		}
	}()
	for {
		if err := d.sendMessage(controlMsg{Command: "keep-alive"}); err != nil {
			return err
		}

		t := time.NewTimer(dialerPingInterval)
		select {
		case <-t.C:
			continue
		case requestID := <-d.connReady:
			t.Stop()
			// Embed the per-Dial request ID in the pickup URL so the
			// listener's dial-back identifies which Dial() to satisfy.
			pickupPath := d.pickupPath + "&" + requestIDParam + "=" + requestID
			if err := d.sendMessage(controlMsg{
				Command:  "conn-ready",
				ConnPath: pickupPath,
			}); err != nil {
				d.deliverErr(requestID, err)
				return err
			}
		case <-d.donec:
			t.Stop()
			return errors.New("revdial.Dialer closed")
		}
	}
}

func (d *Dialer) sendMessage(m controlMsg) error {
	j, _ := json.Marshal(m)
	d.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	j = append(j, '\n')
	_, err := d.conn.Write(j)
	d.conn.SetWriteDeadline(time.Time{})
	return err
}

// NewListener returns a new Listener, accepting connections which
// arrive from the provided server connection, which should be after
// any necessary authentication (usually after an HTTP exchange).
//
// The provided dialServer func is responsible for connecting back to
// the server and doing TLS setup.
func NewListener(serverConn net.Conn, dialServer func(context.Context, string) (*websocket.Conn, *http.Response, error)) *Listener {
	ln := &Listener{
		sc:    serverConn,
		dial:  dialServer,
		connc: make(chan net.Conn, 8), // arbitrary
		donec: make(chan struct{}),
	}
	go ln.run()
	return ln
}

var _ net.Listener = (*Listener)(nil)

// Listener is a net.Listener, returning new connections which arrive
// from a corresponding Dialer.
type Listener struct {
	sc     net.Conn
	connc  chan net.Conn
	donec  chan struct{}
	dial   func(context.Context, string) (*websocket.Conn, *http.Response, error)
	writec chan<- []byte

	mu     sync.Mutex // guards below and writing to rw
	closed bool
}

type controlMsg struct {
	Command  string `json:"command,omitempty"`  // "keep-alive", "conn-ready", "pickup-failed"
	ConnPath string `json:"connPath,omitempty"` // conn pick-up URL path for "conn-url", "pickup-failed"
	Err      string `json:"err,omitempty"`
}

// controlReadTimeout is the maximum time to wait for a control message.
// The server sends keep-alive messages periodically, so if we don't receive
// anything within this timeout, the connection is likely dead.
// This prevents blocking forever when the server dies without sending a close frame.
const controlReadTimeout = 60 * time.Second

// run reads control messages from the public server forever until the connection dies, which
// then closes the listener.
func (ln *Listener) run() {
	defer ln.Close()

	// Write loop
	writec := make(chan []byte, 8)
	ln.writec = writec
	go func() {
		for {
			select {
			case <-ln.donec:
				return
			case msg := <-writec:
				if _, err := ln.sc.Write(msg); err != nil {
					log.Printf("revdial.Listener: error writing message to server: %v", err)
					ln.Close()
					return
				}
			}
		}
	}()

	// Read loop with timeout to detect dead connections
	br := bufio.NewReader(ln.sc)
	for {
		// Set read deadline to detect dead connections
		// If the server is alive, it will send keep-alive messages or conn-ready commands
		if err := ln.sc.SetReadDeadline(time.Now().Add(controlReadTimeout)); err != nil {
			log.Printf("revdial.Listener: failed to set read deadline: %v", err)
			return
		}

		line, err := br.ReadSlice('\n')
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				log.Printf("revdial.Listener: read timeout after %v, closing connection", controlReadTimeout)
			}
			return
		}
		var msg controlMsg
		if err := json.Unmarshal(line, &msg); err != nil {
			log.Printf("revdial.Listener read invalid JSON: %q: %v", line, err)
			return
		}
		switch msg.Command {
		case "keep-alive":
			// Occasional no-op message from server to keep
			// us alive through NAT timeouts.
		case "conn-ready":
			go ln.grabConn(msg.ConnPath)
		default:
			// Ignore unknown messages
		}
	}
}

func (ln *Listener) sendMessage(m controlMsg) {
	j, _ := json.Marshal(m)
	j = append(j, '\n')
	ln.writec <- j
}

func (ln *Listener) grabConn(path string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	wsConn, resp, err := ln.dial(ctx, path)
	if err != nil {
		ln.sendMessage(controlMsg{Command: "pickup-failed", ConnPath: path, Err: err.Error()})
		return
	}

	failPickup := func(err error) {
		wsConn.Close()
		log.Printf("revdial.Listener: failed to pick up connection to %s: %v", path, err)
		ln.sendMessage(controlMsg{Command: "pickup-failed", ConnPath: path, Err: err.Error()})
	}

	if resp.StatusCode != 101 {
		failPickup(fmt.Errorf("non-101 response %v", resp.Status))
		return
	}

	select {
	case ln.connc <- wsconnadapter.New(wsConn):
	case <-ln.donec:
		// Listener closed while we were picking up — drop the connection
		// instead of leaking it. (connc is never closed, so the send case
		// above can never panic.)
		wsConn.Close()
	}
}

// Closed reports whether the listener has been closed.
func (ln *Listener) Closed() bool {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	return ln.closed
}

// Accept blocks and returns a new connection, or an error. connc is never
// closed (see Close); closure is signalled by donec, so we select on both and
// still drain any connection already buffered before closure.
func (ln *Listener) Accept() (net.Conn, error) {
	select {
	case c := <-ln.connc:
		return c, nil
	case <-ln.donec:
		select {
		case c := <-ln.connc:
			return c, nil
		default:
			return nil, ErrListenerClosed
		}
	}
}

// ErrListenerClosed is returned by Accept after Close has been called.
var ErrListenerClosed = errors.New("revdial: Listener closed")

// Close closes the Listener, making future Accept calls return an
// error.
func (ln *Listener) Close() error {
	ln.mu.Lock()
	defer ln.mu.Unlock()
	if ln.closed {
		return nil
	}
	go ln.sc.Close()
	ln.closed = true
	// Signal closure via donec ONLY. We must never close connc: it has multiple
	// concurrent senders (grabConn goroutines), and a send on a closed channel
	// panics — even inside a select — which previously crashed hydra and took a
	// whole runner offline. grabConn and Accept both select on donec instead.
	close(ln.donec)
	return nil
}

// Addr returns a dummy address. This exists only to conform to the
// net.Listener interface.
func (ln *Listener) Addr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "revdial" }
func (fakeAddr) String() string  { return "revdialconn" }

// ConnHandler returns the HTTP handler that needs to be mounted somewhere
// that the Listeners can dial out and get to. A dialer to connect to it
// is given to NewListener and the path to reach it is given to NewDialer
// to use in messages to the listener.
func ConnHandler(upgrader websocket.Upgrader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dialerUniq := r.FormValue(dialerUniqParam)
		requestID := r.FormValue(requestIDParam)

		dmapMu.Lock()
		d, ok := dialers[dialerUniq]
		dmapMu.Unlock()
		if !ok {
			errutil.WriteCloudError(w, errutil.NewCloudError(http.StatusInternalServerError, errutil.CloudErrorCodeInternalServerError, "unknown dialer"))
			return
		}

		wsConn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errutil.WriteCloudError(w, errutil.NewCloudError(http.StatusInternalServerError, errutil.CloudErrorCodeInternalServerError, "unknown dialer"))
			return
		}

		// deliverConn returns immediately: it either hands the WebSocket
		// to the waiting Dial() (buffered chan) or closes it. No more
		// 30s matchConn block holding an HTTP handler goroutine.
		d.deliverConn(requestID, wsconnadapter.New(wsConn))
	})
}
