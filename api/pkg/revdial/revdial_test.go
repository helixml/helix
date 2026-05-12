package revdial

import (
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

// pipeConn wraps a net.Conn into a controlled close mechanism for tests.
type pipeConn struct {
	net.Conn
	mu     sync.Mutex
	closed bool
}

func (p *pipeConn) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	return p.Conn.Close()
}

func newDialerPair(t *testing.T) (*Dialer, net.Conn) {
	t.Helper()
	a, b := net.Pipe()
	d := NewDialer(a, "/revdial")
	t.Cleanup(func() {
		d.Close()
		a.Close()
		b.Close()
	})
	return d, b
}

// TestPickupPathContainsRequestID confirms that conn-ready emitted by the
// dialer for each Dial() carries a unique request ID in the pickup URL.
// Two concurrent Dial()s must get two distinct request IDs so the listener
// can echo each back unambiguously.
func TestPickupPathContainsRequestID(t *testing.T) {
	t.Parallel()

	d, peer := newDialerPair(t)

	// Drain whatever the dialer writes (keep-alive, conn-ready) so its
	// write deadline doesn't fire and its serve loop keeps running.
	var (
		mu      sync.Mutex
		written []string
	)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := peer.Read(buf)
			if n > 0 {
				mu.Lock()
				written = append(written, string(buf[:n]))
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	// Kick off two concurrent Dial requests; we don't care that they will
	// time out (no listener actually picks up) — we only want to inspect
	// the conn-ready paths the dialer emitted.
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			_, _ = d.Dial(ctx)
		}()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	all := strings.Join(written, "")
	count := strings.Count(all, requestIDParam+"=")
	if count < 2 {
		t.Fatalf("expected at least 2 conn-ready paths to embed %s=, got %d in: %q", requestIDParam, count, all)
	}

	// Verify the IDs differ — each Dial() must have a fresh request ID.
	ids := map[string]bool{}
	for _, line := range strings.Split(all, "\n") {
		i := strings.Index(line, requestIDParam+"=")
		if i < 0 {
			continue
		}
		rest := line[i+len(requestIDParam)+1:]
		if j := strings.IndexAny(rest, `"&\\`); j >= 0 {
			rest = rest[:j]
		}
		ids[rest] = true
	}
	if len(ids) < 2 {
		t.Fatalf("expected distinct request IDs across concurrent Dial()s, got %v in: %q", ids, all)
	}
}

func TestExtractRequestID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		want string
	}{
		{"/revdial?revdial.dialer=abc&revdial.req=xyz", "xyz"},
		{"/revdial?revdial.req=xyz&revdial.dialer=abc", "xyz"},
		{"/revdial?revdial.dialer=abc", ""},
		{"/revdial", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := extractRequestID(tc.path); got != tc.want {
			t.Errorf("extractRequestID(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestDeliverConnNoWaiterClosesConn verifies that data connections arriving
// for a request ID with no waiter (caller already gave up) are closed
// rather than leaked.
func TestDeliverConnNoWaiterClosesConn(t *testing.T) {
	t.Parallel()
	d, _ := newDialerPair(t)

	a, b := net.Pipe()
	defer b.Close()
	wrapped := &pipeConn{Conn: a}
	d.deliverConn("nonexistent-request-id", wrapped)

	wrapped.mu.Lock()
	closed := wrapped.closed
	wrapped.mu.Unlock()
	if !closed {
		t.Fatal("expected deliverConn to close conn when no waiter is registered")
	}
}
